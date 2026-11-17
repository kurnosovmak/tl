// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

use gentl::basictl::Error;
use gentl::types::{
    CasesBytesTestVector::CasesBytesTestVector, CasesMyCycle1::CasesMyCycle1,
    CasesTestAllDicts::CasesTestAllDicts,
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
fn go_round_trip() {
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
