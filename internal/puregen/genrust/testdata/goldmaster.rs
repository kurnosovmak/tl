// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

use gentl::JsonMode;
use serde_json::Value;

fn take_record<'a>(buf: &mut &'a [u8]) -> &'a [u8] {
    let (size, data) = buf.split_at(4);
    let size = u32::from_le_bytes(size.try_into().unwrap()) as usize;
    let (record, rest) = data.split_at(size);
    *buf = rest;
    record
}

macro_rules! goldmaster {
    ($test:ident, $ty:ty, $name:literal, $fixture:literal, $read_tl1:ident, $write_tl1:ident) => {
        #[test]
        fn $test() {
            let mut records = include_bytes!($fixture).as_slice();
            let mut value = <$ty>::default();
            let mut output = Vec::new();
            let mut sample = 0;
            while !records.is_empty() {
                sample += 1;
                let expected_tl1 = take_record(&mut records);
                let expected_tl2 = take_record(&mut records);
                let expected_transcode = take_record(&mut records);
                let expected_json = take_record(&mut records);

                let mut input = expected_tl1;
                value
                    .$read_tl1(&mut input)
                    .unwrap_or_else(|err| panic!("{} sample {sample} TL1 decode: {err}", $name));
                assert!(
                    input.is_empty(),
                    "{} sample {sample} TL1 trailing bytes",
                    $name
                );
                output.clear();
                value.$write_tl1(&mut output).unwrap();
                assert_eq!(output, expected_tl1, "{} sample {sample} TL1", $name);

                output.clear();
                value.write_tl2(&mut output).unwrap();
                assert_eq!(
                    output, expected_transcode,
                    "{} sample {sample} TL1 to TL2",
                    $name
                );

                let mut input = expected_tl2;
                value
                    .read_tl2(&mut input)
                    .unwrap_or_else(|err| panic!("{} sample {sample} TL2 decode: {err}", $name));
                assert!(
                    input.is_empty(),
                    "{} sample {sample} TL2 trailing bytes",
                    $name
                );
                output.clear();
                value.write_tl2(&mut output).unwrap();
                assert_eq!(output, expected_tl2, "{} sample {sample} TL2", $name);

                value
                    .read_json(expected_json, JsonMode::Tl1)
                    .unwrap_or_else(|err| panic!("{} sample {sample} JSON decode: {err}", $name));
                output.clear();
                value.write_json(&mut output, JsonMode::Tl1).unwrap();
                let actual: Value = serde_json::from_slice(&output).unwrap();
                let expected: Value = serde_json::from_slice(expected_json).unwrap();
                assert_eq!(actual, expected, "{} sample {sample} JSON", $name);
            }
        }
    };
}
