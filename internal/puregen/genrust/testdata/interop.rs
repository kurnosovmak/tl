// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

use gentl::JsonMode;
use gentl::basictl::Error;
use gentl::types::{
    CasesBytesTestVector::CasesBytesTestVector, CasesMyCycle1::CasesMyCycle1,
    CasesTL2TestArrayFixedBool::CasesTL2TestArrayFixedBool, CasesTestAllDicts::CasesTestAllDicts,
    CasesTestAllPossibleFieldConfigsContainer::CasesTestAllPossibleFieldConfigsContainer,
    CasesTestArray::CasesTestArray, CasesTestEnumContainer::CasesTestEnumContainer,
    CasesTestInplaceStructArgs2::CasesTestInplaceStructArgs2, CasesTestMaybe::CasesTestMaybe,
    CasesTestTuple::CasesTestTuple, CasesTestUnionContainer::CasesTestUnionContainer,
    CasesTestVector::CasesTestVector,
};

macro_rules! round_trip {
    ($type:ty, $file:literal) => {{
        let expected = include_bytes!($file).as_slice();
        let mut input = expected;
        let mut output = Vec::new();
        let mut value = <$type>::default();
        while !input.is_empty() {
            let len = input.len();
            value.read_tl1_boxed(&mut input).unwrap();
            assert!(input.len() < len);
            value.write_tl1_boxed(&mut output).unwrap();
        }
        assert_eq!(output, expected);
    }};
}

#[test]
fn go_tl1_round_trip() {
    round_trip!(CasesMyCycle1, "cycle.bin");
    round_trip!(CasesTestArray, "array.bin");
    round_trip!(CasesTestVector, "vector.bin");
    round_trip!(CasesTestTuple, "tuple.bin");
    round_trip!(CasesTestAllDicts, "dictionaries.bin");
    round_trip!(CasesTestMaybe, "maybe.bin");
    round_trip!(CasesTestUnionContainer, "union.bin");
    round_trip!(CasesTestEnumContainer, "enum.bin");
    round_trip!(CasesTestAllPossibleFieldConfigsContainer, "masks.bin");
    round_trip!(CasesTestInplaceStructArgs2, "nat_args.bin");
    round_trip!(CasesBytesTestVector, "strings.bin");
    round_trip!(CasesTL2TestArrayFixedBool, "bools.bin");
}

macro_rules! round_trip_tl2 {
    ($type:ty, $file:literal) => {{
        let expected = include_bytes!($file).as_slice();
        let mut input = expected;
        let mut output = Vec::new();
        let mut value = <$type>::default();
        while !input.is_empty() {
            let len = input.len();
            value.read_tl2(&mut input).unwrap();
            assert!(input.len() < len);
            value.write_tl2(&mut output).unwrap();
        }
        assert_eq!(output, expected);
    }};
}

#[test]
fn go_tl2_round_trip() {
    round_trip_tl2!(CasesMyCycle1, "cycle.tl2");
    round_trip_tl2!(CasesTestArray, "array.tl2");
    round_trip_tl2!(CasesTestVector, "vector.tl2");
    round_trip_tl2!(CasesTestTuple, "tuple.tl2");
    round_trip_tl2!(CasesTestAllDicts, "dictionaries.tl2");
    round_trip_tl2!(CasesTestMaybe, "maybe.tl2");
    round_trip_tl2!(CasesTestUnionContainer, "union.tl2");
    round_trip_tl2!(CasesTestEnumContainer, "enum.tl2");
    round_trip_tl2!(CasesTestAllPossibleFieldConfigsContainer, "masks.tl2");
    round_trip_tl2!(CasesTestInplaceStructArgs2, "nat_args.tl2");
    round_trip_tl2!(CasesTL2TestArrayFixedBool, "bools.tl2");
}

macro_rules! round_trip_json {
    ($type:ty, $file:literal) => {{
        let mut value = <$type>::default();
        for line in include_str!($file).lines() {
            value
                .read_json(line.as_bytes(), JsonMode::Tl1)
                .unwrap_or_else(|err| panic!("{}: {err} in {line}", $file));
            let mut output = Vec::new();
            value.write_json(&mut output, JsonMode::Tl1).unwrap();
            let expected: serde_json::Value = serde_json::from_str(line).unwrap();
            let actual: serde_json::Value = serde_json::from_slice(&output).unwrap();
            assert_eq!(actual, expected, "{}", $file);
        }
    }};
}

#[test]
fn go_json_round_trip() {
    round_trip_json!(CasesMyCycle1, "cycle.jsonl");
    round_trip_json!(CasesTestArray, "array.jsonl");
    round_trip_json!(CasesTestVector, "vector.jsonl");
    round_trip_json!(CasesTestTuple, "tuple.jsonl");
    round_trip_json!(CasesTestAllDicts, "dictionaries.jsonl");
    round_trip_json!(CasesTestMaybe, "maybe.jsonl");
    round_trip_json!(CasesTestUnionContainer, "union.jsonl");
    round_trip_json!(CasesTestEnumContainer, "enum.jsonl");
    round_trip_json!(CasesTestAllPossibleFieldConfigsContainer, "masks.jsonl");
    round_trip_json!(CasesTestInplaceStructArgs2, "nat_args.jsonl");
    round_trip_json!(CasesTL2TestArrayFixedBool, "bools.jsonl");
    round_trip_json!(CasesBytesTestVector, "strings.jsonl");
}

#[test]
fn errors() {
    let data = include_bytes!("strings.bin");
    let mut value = CasesBytesTestVector {
        Arr: vec![b"old".to_vec()],
    };
    let truncated = &data[..data.len() - 1];
    let mut input = truncated;
    assert_eq!(value.read_tl1_boxed(&mut input), Err(Error::UnexpectedEof));
    assert_eq!(input, truncated);
    assert_eq!(value.Arr, vec![b"old".to_vec()]);
    assert!(matches!(
        value.read_tl1_boxed(&mut [0; 4].as_slice()),
        Err(Error::WrongTag { .. }),
    ));

    let mut bad_padding = data.to_vec();
    *bad_padding.last_mut().unwrap() = 1;
    assert_eq!(
        value.read_tl1_boxed(&mut bad_padding.as_slice()),
        Err(Error::InvalidPadding),
    );

    let bad_array = CasesTestArray { N: 2, Arr: vec![1] };
    let mut output = vec![0xff];
    assert_eq!(
        bad_array.write_tl1_boxed(&mut output),
        Err(Error::InvalidLength),
    );
    assert_eq!(output, vec![0xff]);
}

#[test]
fn tl1_huge_string() {
    let value = CasesBytesTestVector {
        Arr: vec![vec![0xff; 1 << 24]],
    };
    let mut wire = Vec::new();
    value.write_tl1(&mut wire).unwrap();
    assert_eq!(&wire[..12], &[1, 0, 0, 0, 255, 0, 0, 0, 1, 0, 0, 0]);
    let mut input = wire.as_slice();
    let mut decoded = CasesBytesTestVector::default();
    decoded.read_tl1(&mut input).unwrap();
    assert!(input.is_empty());
    assert_eq!(decoded.Arr, value.Arr);
}
