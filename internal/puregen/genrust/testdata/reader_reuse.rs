// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

use gentl::{
    JsonMode,
    basictl::Result,
    types::{BenchBatch::BenchBatch, BenchRow::BenchRow},
};

const GROUP: &[u8] = br#"{"epoch":1,"events":[{"type":"bench.group","value":{"name":"ab","rows":[{"id":1,"detail":"alpha","sequence":0,"tags":["ab","cd"]},{"samples":[1,2]}]}}]}"#;
const OTHER: &[u8] = br#"{"epoch":2,"events":[{"type":"bench.tick","value":{"value":99}},{"type":"bench.update","value":{"row":{"label":"","samples":[]}}}]}"#;

fn batch(input: &[u8]) -> BenchBatch {
    let mut value = BenchBatch::default();
    value.read_json(input, JsonMode::Tl1).unwrap();
    value
}
type Write = fn(&BenchBatch, &mut Vec<u8>) -> Result<()>;
type Read = fn(&mut BenchBatch, &mut &[u8]) -> Result<()>;
type WithScratch = fn(&mut BenchBatch, &mut &[u8], &mut BenchBatch) -> Result<()>;
fn wire(value: &BenchBatch, write: Write) -> Vec<u8> {
    let mut data = Vec::new();
    write(value, &mut data).unwrap();
    data
}

#[test]
fn changing_layouts_and_transactional_errors() {
    let formats: [(Read, WithScratch, Write); 3] = [
        (
            BenchBatch::read_tl1,
            BenchBatch::read_tl1_with_scratch,
            BenchBatch::write_tl1,
        ),
        (
            BenchBatch::read_tl1_boxed,
            BenchBatch::read_tl1_boxed_with_scratch,
            BenchBatch::write_tl1_boxed,
        ),
        (
            BenchBatch::read_tl2,
            BenchBatch::read_tl2_with_scratch,
            BenchBatch::write_tl2,
        ),
    ];
    for (read, buffered, write) in formats {
        let first = wire(&batch(GROUP), write);
        let second = wire(&batch(OTHER), write);
        let empty = wire(&BenchBatch::default(), write);
        for use_scratch in [false, true] {
            let mut dest = batch(OTHER);
            let mut scratch = batch(GROUP);
            let mut decode = |dest: &mut BenchBatch, cursor: &mut &[u8]| {
                if use_scratch {
                    buffered(dest, cursor, &mut scratch)
                } else {
                    read(dest, cursor)
                }
            };
            for data in [&first, &second, &empty, &second, &first] {
                let mut framed = data.clone();
                framed.extend_from_slice(&[0xa5, 0x5a]);
                let mut cursor = framed.as_slice();
                decode(&mut dest, &mut cursor).unwrap();
                assert_eq!(cursor, [0xa5, 0x5a]);
                assert_eq!(wire(&dest, write), *data);
            }
            // Probe must reject invalid input without touching the receiver.
            // Compare every mutation against decoding into a fresh receiver.
            for i in 0..first.len() {
                let mut mutated = first.clone();
                mutated[i] ^= 0x80;
                for invalid in [&second[..i.min(second.len() - 1)], mutated.as_slice()] {
                    let mut fresh = BenchBatch::default();
                    let mut query = invalid;
                    let expected = read(&mut fresh, &mut query);
                    let mut cursor = invalid;
                    let result = decode(&mut dest, &mut cursor);
                    assert_eq!(result, expected);
                    if result.is_err() {
                        assert_eq!(cursor, invalid);
                        assert_eq!(wire(&dest, write), first);
                    } else {
                        assert_eq!(cursor, query);
                        assert_eq!(wire(&dest, write), wire(&fresh, write));
                    }
                    decode(&mut dest, &mut first.as_slice()).unwrap();
                }
            }
        }
    }
}

#[test]
fn direct_json_matches_general_reader() {
    let mut dest = BenchRow::default();
    for input in [
        br#"{"label":"a","fields_mask":133,"id":1}"#.as_slice(),
        br#"{"samples":[],"label":"","detail":{"base64":"/wA="},"sequence":0}"#,
        br#"{"tags":["abc","a\nb"],"score":0.25,"extra_mask":1073741824}"#,
        br#"{"id":"-9","detail":"\u263a"}"#,
        br#"{"id":2,"id":3}"#,
        br#"{"tags":null}"#,
        br#"{"samples":[false]}"#,
        br#"[]"#,
        br#"{"detail":{"base64":"YQ==","base64":"Yg=="}}"#,
        br#"{"unknown":{}}"#,
    ] {
        let mut general = BenchRow::default();
        let mut before = Vec::new();
        dest.write_tl1(&mut before).unwrap();
        let result = dest.read_json(input, JsonMode::Tl1);
        assert_eq!(result, general.read_json(input, JsonMode::Tl2));
        let mut actual = Vec::new();
        dest.write_tl1(&mut actual).unwrap();
        if result.is_ok() {
            let mut expected = Vec::new();
            general.write_tl1(&mut expected).unwrap();
            assert_eq!(actual, expected);
        } else {
            assert_eq!(actual, before);
        }
    }
}

#[test]
fn in_place_recovers_after_truncated_messages() {
    let formats: [(Read, Write); 3] = [
        (BenchBatch::read_tl1_in_place, BenchBatch::write_tl1),
        (
            BenchBatch::read_tl1_boxed_in_place,
            BenchBatch::write_tl1_boxed,
        ),
        (BenchBatch::read_tl2_in_place, BenchBatch::write_tl2),
    ];
    for (read, write) in formats {
        let full = wire(&batch(GROUP), write);
        let other = wire(&batch(OTHER), write);
        let mut dest = BenchBatch::default();
        for end in 0..full.len() {
            let mut cursor = &full[..end];
            assert!(read(&mut dest, &mut cursor).is_err());
            assert_eq!(cursor, &full[..end]);
            for good in [&other, &full] {
                let mut framed = good.clone();
                framed.push(0xab);
                let mut cursor = framed.as_slice();
                read(&mut dest, &mut cursor).unwrap();
                assert_eq!(cursor, [0xab]);
                assert_eq!(wire(&dest, write), *good);
            }
        }
    }
}

#[test]
fn matching_layout_reuses_nested_buffers() {
    use gentl::types::BenchEvent::BenchEvent;
    fn buffers(value: &BenchBatch) -> [*const (); 4] {
        let BenchEvent::Group(group) = &value.Events[0] else {
            panic!("expected group")
        };
        [
            value.Events.as_ptr().cast(),
            group.Rows.as_ptr().cast(),
            group.Rows[0].Detail.as_ref().unwrap().as_ptr().cast(),
            group.Rows[1].Samples.as_ref().unwrap().as_ptr().cast(),
        ]
    }
    let formats: [(Read, WithScratch, Write); 2] = [
        (
            BenchBatch::read_tl1,
            BenchBatch::read_tl1_with_scratch,
            BenchBatch::write_tl1,
        ),
        (
            BenchBatch::read_tl2,
            BenchBatch::read_tl2_with_scratch,
            BenchBatch::write_tl2,
        ),
    ];
    for (read, buffered, write) in formats {
        let mut dest = batch(GROUP);
        let mut scratch = batch(GROUP);
        let first = buffers(&dest);
        let second = buffers(&scratch);
        let mut changed = batch(GROUP);
        let BenchEvent::Group(group) = &mut changed.Events[0] else {
            unreachable!()
        };
        group.Rows[0]
            .Detail
            .as_mut()
            .unwrap()
            .copy_from_slice(b"omega");
        group.Rows[1]
            .Samples
            .as_mut()
            .unwrap()
            .copy_from_slice(&[-42, 99]);
        let encoded = wire(&changed, write);
        read(&mut dest, &mut encoded.as_slice()).unwrap();
        assert_eq!(buffers(&dest), first);
        assert_eq!(wire(&dest, write), encoded);
        buffered(&mut dest, &mut encoded.as_slice(), &mut scratch).unwrap();
        assert_eq!(buffers(&dest), second);
        assert_eq!(buffers(&scratch), first);
        assert_eq!(wire(&dest, write), encoded);
    }
}
