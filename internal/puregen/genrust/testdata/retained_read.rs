// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

use gentl::{
    JsonMode, RetainedVec,
    basictl::Result,
    types::{BenchBatch::BenchBatch, BenchEvent::BenchEvent},
};

type Read = fn(&mut BenchBatch, &mut &[u8]) -> Result<()>;
type Write = fn(&BenchBatch, &mut Vec<u8>) -> Result<()>;

fn batch(count: usize, rows: usize, epoch: i32) -> BenchBatch {
    let mut value = BenchBatch::default();
    value.Epoch = epoch;
    for i in 0..count {
        let mut group = gentl::types::BenchGroup::BenchGroup::default();
        for j in 0..rows {
            let mut row = gentl::types::BenchRow::BenchRow::default();
            row.FieldsMask = 1 << 7;
            row.Id = i as i32;
            row.Samples = Some(vec![j as i32, epoch]);
            group.Rows.push(row);
        }
        value.Events.push(BenchEvent::Group(group));
    }
    value
}

fn wire(value: &BenchBatch, write: Write) -> Vec<u8> {
    let mut out = Vec::new();
    write(value, &mut out).unwrap();
    out
}

fn read_frame(value: &mut BenchBatch, read: Read, bytes: &[u8]) {
    let mut framed = bytes.to_vec();
    framed.extend_from_slice(&[0xa5, 0x5a]);
    let mut cursor = framed.as_slice();
    read(value, &mut cursor).unwrap();
    assert_eq!(cursor, [0xa5, 0x5a]);
}

fn buffers(value: &BenchBatch, i: usize, j: usize) -> [*const (); 3] {
    let BenchEvent::Group(group) = &value.Events[i] else {
        panic!("expected group")
    };
    [
        value.Events.as_ptr().cast(),
        group.Rows.as_ptr().cast(),
        group.Rows[j].Samples.as_ref().unwrap().as_ptr().cast(),
    ]
}

#[test]
fn nested_buffers_survive_shrinking_and_regrowing() {
    let formats: [(Read, Read, Write); 3] = [
        (
            BenchBatch::read_tl1,
            BenchBatch::read_tl1_in_place,
            BenchBatch::write_tl1,
        ),
        (
            BenchBatch::read_tl1_boxed,
            BenchBatch::read_tl1_boxed_in_place,
            BenchBatch::write_tl1_boxed,
        ),
        (
            BenchBatch::read_tl2,
            BenchBatch::read_tl2_in_place,
            BenchBatch::write_tl2,
        ),
    ];
    for (atomic, in_place, write) in formats {
        let large = wire(&batch(192, 4, 1), write);
        let mut changed = batch(192, 4, 42);
        changed.Events[120] = BenchEvent::Tick(gentl::types::BenchTick::BenchTick { Value: 123 });
        let updated = wire(&changed, write);
        for read in [atomic, in_place] {
            let mut dest = BenchBatch::default();
            // Populate both banks for a transactional read.
            read_frame(&mut dest, read, &large);
            read_frame(&mut dest, read, &large);
            let tail = buffers(&dest, 191, 3);
            let prefix = buffers(&dest, 0, 3);
            for (count, rows) in [(3, 1), (0, 0), (24, 0)] {
                let small = batch(count, rows, -7);
                let bytes = wire(&small, write);
                read_frame(&mut dest, read, &bytes);
                assert_eq!(dest.Events.len(), count);
                assert_eq!(wire(&dest, write), bytes);
                let mut json = Vec::new();
                dest.write_json(&mut json, JsonMode::Tl1).unwrap();
                let mut expected = Vec::new();
                small.write_json(&mut expected, JsonMode::Tl1).unwrap();
                assert_eq!(json, expected);
            }
            read_frame(&mut dest, read, &updated);
            assert_eq!(buffers(&dest, 191, 3), tail);
            assert_eq!(buffers(&dest, 0, 3), prefix);
            assert_eq!(wire(&dest, write), updated);
        }
        let mut dest = BenchBatch::default();
        read_frame(&mut dest, atomic, &large);
        for end in [0, 1, updated.len() / 2, updated.len() - 1] {
            let input = &updated[..end];
            let mut cursor = input;
            assert!(atomic(&mut dest, &mut cursor).is_err());
            assert_eq!(cursor, input);
            assert_eq!(wire(&dest, write), large);
        }
        read_frame(&mut dest, atomic, &updated);
        assert_eq!(wire(&dest, write), updated);
    }
}

#[test]
fn discard_and_conversion_expose_only_active_elements() {
    use std::rc::Rc;
    let item = Rc::new(7);
    let mut values: RetainedVec<_> = vec![item.clone(), item.clone()].into();
    values.clear();
    values.push(item.clone());
    assert_eq!(values.iter().count(), 1);
    assert_eq!(Rc::strong_count(&item), 3);
    values.discard_spare();
    assert_eq!(Rc::strong_count(&item), 2);
    values.clear();
    assert_eq!(values.into_iter().count(), 0);
    assert_eq!(Rc::strong_count(&item), 1);
}
