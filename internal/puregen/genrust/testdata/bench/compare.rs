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
type ScratchReader<T> = fn(&mut T, &[u8], &mut T);

type ReuseReaders<T> = (ScratchReader<T>, Reader<T>);

fn format_ns(ns: Option<f64>) -> String {
    ns.map(|ns| format!("{ns:.3}")).unwrap_or_default()
}

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

fn run<T: Default>(
    source: &T,
    dir: &Path,
    mode: &str,
    codecs: [(&str, Writer<T>, Reader<T>); 3],
    reuse_readers: [Option<ReuseReaders<T>>; 3],
) {
    let mut dest = T::default();
    let tl1 = codecs[0].1;
    let mut expected = Vec::new();
    tl1(source, &mut expected).unwrap();
    for ((name, write, read), reuse) in codecs.into_iter().zip(reuse_readers) {
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
        let mut scratch = T::default();
        if let Some((decode, in_place)) = reuse {
            in_place(&mut dest, &wire);
            actual.clear();
            tl1(&dest, &mut actual).unwrap();
            assert_eq!(actual, expected);
            decode(&mut dest, &wire, &mut scratch);
            actual.clear();
            tl1(&dest, &mut actual).unwrap();
            assert_eq!(actual, expected);
        }
        if mode == "verify" {
            continue;
        }
        if mode == "rotate" {
            let dirs =
                ["batch-3", "batch-24", "batch-192"].map(|case| dir.parent().unwrap().join(case));
            let wires = dirs
                .each_ref()
                .map(|d| fs::read(d.join(format!("go.{name}"))).unwrap());
            for (i, frame) in wires.iter().cycle().take(6).enumerate() {
                read(&mut dest, frame);
                actual.clear();
                tl1(&dest, &mut actual).unwrap();
                let expected = fs::read(dirs[i % 3].join("go.TL1")).unwrap();
                assert_eq!(actual, expected);
                if let Some((decode, in_place)) = reuse {
                    in_place(&mut dest, frame);
                    actual.clear();
                    tl1(&dest, &mut actual).unwrap();
                    assert_eq!(actual, expected);
                    decode(&mut dest, frame, &mut scratch);
                    actual.clear();
                    tl1(&dest, &mut actual).unwrap();
                    assert_eq!(actual, expected);
                }
            }
            let mut frames = wires.iter().cycle();
            let normal = measure(|| {
                read(black_box(&mut dest), black_box(frames.next().unwrap()));
                black_box(&dest);
            });
            let buffered = reuse.map(|(decode, _)| {
                let mut frames = wires.iter().cycle();
                measure(|| {
                    decode(
                        black_box(&mut dest),
                        black_box(frames.next().unwrap()),
                        black_box(&mut scratch),
                    );
                    black_box(&dest);
                })
            });
            let in_place = reuse.map(|(_, decode)| {
                let mut frames = wires.iter().cycle();
                measure(|| {
                    decode(black_box(&mut dest), black_box(frames.next().unwrap()));
                    black_box(&dest);
                })
            });
            println!(
                "{name},{normal:.3},{},{}",
                format_ns(buffered),
                format_ns(in_place)
            );
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
        let reuse_ns = reuse.map(|(decode, _)| {
            measure(|| {
                decode(
                    black_box(&mut dest),
                    black_box(&wire),
                    black_box(&mut scratch),
                );
                black_box(&dest);
            })
        });
        let in_place = reuse.map(|(_, decode)| {
            measure(|| {
                decode(black_box(&mut dest), black_box(&wire));
                black_box(&dest);
            })
        });
        println!(
            "{name},{size},{enc:.3},{dec:.3},{},{}",
            format_ns(reuse_ns),
            format_ns(in_place)
        );
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
            #[cfg(feature = "bench-reuse")]
            let readers: [Option<ReuseReaders<$ty>>; 3] = [
                Some((
                    |dest, mut input, scratch| {
                        dest.read_tl1_with_scratch(&mut input, scratch).unwrap();
                        assert!(input.is_empty());
                    },
                    |dest, mut input| {
                        dest.read_tl1_in_place(&mut input).unwrap();
                        assert!(input.is_empty());
                    },
                )),
                Some((
                    |dest, mut input, scratch| {
                        dest.read_tl2_with_scratch(&mut input, scratch).unwrap();
                        assert!(input.is_empty());
                    },
                    |dest, mut input| {
                        dest.read_tl2_in_place(&mut input).unwrap();
                        assert!(input.is_empty());
                    },
                )),
                None,
            ];
            #[cfg(not(feature = "bench-reuse"))]
            let readers = [None, None, None];
            run(&value, dir, &args[3], codecs!($ty), readers);
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
