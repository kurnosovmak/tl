// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

use gentl::JsonMode;
use gentl::basictl::Error;
use gentl::types::{SampleBlob::SampleBlob, SampleRecord::SampleRecord, SampleState::SampleState};

#[test]
fn field_blocks_and_packed_bits() {
    let expected = include_bytes!("record.tl2");
    let mut record = SampleRecord::default();
    let mut input = include_bytes!("record_with_old_field.tl2").as_slice();
    record.read_tl2(&mut input).unwrap();
    assert!(input.is_empty());
    assert_eq!(record.F0, Some(0));
    assert!(record.Flag);
    assert_eq!(record.F6, -17);
    assert_eq!(record.F7, 42);
    assert_eq!(
        record.Flags,
        [true, false, false, false, false, false, false, true, true]
    );
    let mut output = Vec::new();
    record.write_tl2(&mut output).unwrap();
    assert_eq!(output, expected.as_slice());

    let truncated = &expected[..expected.len() - 1];
    let mut input = truncated;
    assert_eq!(record.read_tl2(&mut input), Err(Error::UnexpectedEof));
    assert_eq!(input, truncated);
    assert_eq!(record.F0, Some(0));
}

#[test]
fn length_boundaries() {
    for (len, prefix) in [
        (253, &[253][..]),
        (254, &[254, 0, 0]),
        (65_789, &[254, 255, 255]),
        (65_790, &[255, 254, 0, 1, 0, 0, 0, 0, 0]),
    ] {
        let value = SampleBlob(vec![0xf0; len]);
        let mut encoded = Vec::new();
        value.write_tl2(&mut encoded).unwrap();
        assert_eq!(&encoded[..prefix.len()], prefix);
        assert_eq!(&encoded[prefix.len()..], value.0);

        let mut wire = encoded.as_slice();
        let mut decoded = SampleBlob::default();
        decoded.read_tl2(&mut wire).unwrap();
        assert!(wire.is_empty());
        assert_eq!(decoded.0, value.0);
    }
}

#[test]
fn native_union_variants() {
    let expected = [6, 3, 1, 9, 0, 0, 0];
    let mut state = SampleState::default();
    state.read_tl2(&mut expected.as_slice()).unwrap();
    let SampleState::Running(value) = &state else {
        panic!("unexpected enum variant");
    };
    assert_eq!(value.Count, 9);
    let mut output = Vec::new();
    state.write_tl2(&mut output).unwrap();
    assert_eq!(output, expected);

    state
        .read_json(br#"{"type":"idle"}"#, JsonMode::Tl2)
        .unwrap();
    assert!(matches!(state, SampleState::Idle));
    assert!(matches!(
        state.read_json(br#"{"type":"idle","type":"running"}"#, JsonMode::Tl2),
        Err(Error::InvalidJson(_))
    ));
    assert!(matches!(state, SampleState::Idle));
}
