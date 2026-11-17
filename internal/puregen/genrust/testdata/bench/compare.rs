// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

use gentl::JsonMode;
use gentl::basictl::Result;
use gentl::types::{
    BenchBatch::BenchBatch, BenchBytes::BenchBytes, BenchInts::BenchInts, BenchRow::BenchRow,
};
use std::{
    fs,
    hint::black_box,
    path::Path,
    time::{Duration, Instant},
};

type Writer<T> = fn(&T, &mut Vec<u8>) -> Result<()>;
type Reader<T> = fn(&mut T, &[u8]);

fn measure(mut f: impl FnMut()) -> f64 {
    let mut batch = |count| {
        let start = Instant::now();
        for _ in 0..count {
            f();
        }
        start.elapsed()
    };
    let mut count = 1;
    loop {
        let elapsed = batch(count);
        if elapsed >= Duration::from_millis(12) {
            count = ((count as f64 * 0.060 / elapsed.as_secs_f64()) as usize).max(1);
            break;
        }
        count *= 2;
    }
    let mut samples = [0.0; 7];
    for value in &mut samples {
        *value = batch(count).as_nanos() as f64 / count as f64;
    }
    samples.sort_unstable_by(f64::total_cmp);
    samples[samples.len() / 2]
}

fn run<T: Default>(source: &T, dir: &Path, mode: &str, codecs: [(&str, Writer<T>, Reader<T>); 3]) {
    let mut dest = T::default();
    let tl1 = codecs[0].1;
    let mut expected = Vec::new();
    tl1(source, &mut expected).unwrap();
    for (name, write, read) in codecs {
        let mut buf = Vec::new();
        write(source, &mut buf).unwrap();
        if mode == "prepare" {
            fs::write(dir.join(format!("rust.{name}")), buf).unwrap();
            continue;
        }
        let wire = fs::read(dir.join(format!("go.{name}"))).unwrap();
        read(&mut dest, &wire);
        let mut actual = Vec::new();
        tl1(&dest, &mut actual).unwrap();
        assert_eq!(
            actual,
            expected,
            "{} {name}: Go input changed TL1 value",
            dir.display()
        );
        if mode == "verify" {
            continue;
        }
        let size = buf.len();
        let enc = measure(|| {
            buf.clear();
            write(black_box(source), black_box(&mut buf)).unwrap();
            black_box(buf.as_slice());
        });
        let dec = measure(|| {
            read(black_box(&mut dest), black_box(&wire));
            black_box(&dest);
        });
        println!("{name},{size},{enc:.3},{dec:.3}");
    }
}

macro_rules! codecs {
    ($ty:ty) => {
        [
            ("TL1", <$ty>::write_tl1, |dest, mut input| {
                dest.read_tl1(&mut input).unwrap();
                assert!(input.is_empty());
            }),
            ("TL2", <$ty>::write_tl2, |dest, mut input| {
                dest.read_tl2(&mut input).unwrap();
                assert!(input.is_empty());
            }),
            (
                "JSON",
                |value, buf| value.write_json(buf, JsonMode::Tl1),
                |dest, input| {
                    dest.read_json(input, JsonMode::Tl1).unwrap();
                },
            ),
        ]
    };
}

fn main() {
    let args: Vec<_> = std::env::args().collect();
    let dir = Path::new(&args[2]);
    let input = fs::read(dir.join("input.json")).unwrap();
    macro_rules! bench {
        ($ty:ty) => {{
            let mut value = <$ty>::default();
            value.read_json(&input, JsonMode::Tl1).unwrap();
            run(&value, dir, &args[3], codecs!($ty));
        }};
    }
    match args[1].as_str() {
        "bytes" => bench!(BenchBytes),
        "ints" => bench!(BenchInts),
        "row" => bench!(BenchRow),
        "batch" => bench!(BenchBatch),
        other => panic!("unknown type {other}"),
    }
}
