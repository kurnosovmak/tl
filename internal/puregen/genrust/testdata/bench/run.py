# This Source Code Form is subject to the terms of the Mozilla Public
# License, v. 2.0. If a copy of the MPL was not distributed with this
# file, You can obtain one at https://mozilla.org/MPL/2.0/.

"""Compare generated Go/Rust codecs. Run from any directory with Python 3."""
import argparse
import csv
import json
import shutil
import statistics
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[4]
OUT = ROOT / "target/rust-bench"
GO_PKG = "github.com/VKCOM/tl/target/rust-bench/go_codec"


def build_rust(generator, name, retain=False):
    crate = OUT / name
    subprocess.run([generator, "--language=rust", "--tl2WhiteList=*", "--outdir=" + str(crate),
                    *(["--retain-vector-buffers"] if retain else []), HERE / "schema.tl"], cwd=ROOT, check=True)
    examples = crate / "examples"
    examples.mkdir(exist_ok=True)
    shutil.copyfile(HERE / "compare.rs", examples / "compare.rs")
    manifest = crate / "Cargo.toml"
    manifest.write_text(manifest.read_text().replace("[features]", "[features]\nbench-reuse = []", 1))
    has_reuse = any("pub fn read_tl2_in_place(" in module.read_text()
                    for module in (crate / "src/types").glob("*.rs"))
    features = ["--features", "bench-reuse"] if has_reuse else []
    target = OUT / (name + "-target")
    subprocess.run(["cargo", "build", "--release", "--example", "compare", *features,
                    "--target-dir", target], cwd=crate, check=True)
    return target / "release/examples/compare"


def build(baseline, retain=False):
    OUT.mkdir(parents=True, exist_ok=True)
    generator = OUT / "tl2gen"
    subprocess.run(["go", "build", "-o", generator, "./cmd/tl2gen"], cwd=ROOT, check=True)
    subprocess.run([generator, "--language=go", "--tl2WhiteList=*", "--generateByteVersions=bench.",
                    "--basicPkgPath=", "--pkgPath=" + GO_PKG + "/tl", "--outdir=" + str(OUT / "go_codec"),
                    HERE / "schema.tl"], cwd=ROOT, check=True)
    subprocess.run(["go", "build", "-o", OUT / "go-bench", HERE / "main.go"], cwd=ROOT, check=True)
    binaries = {"go": OUT / "go-bench", "rust": build_rust(generator, "rust", retain)}
    if baseline:
        binaries["previous"] = build_rust(baseline.resolve(), "previous")
    return binaries


def row(index, pattern):
    fields = {"id": index + 1}
    if pattern == "empty":
        return fields
    if pattern == "present-zero":
        return dict(fields, label="", samples=[], sequence=0)
    if pattern in ("sparse", "full"):
        fields.update(detail="high bit", sequence=index + 2)
    if pattern == "full":
        fields.update(label="sensor-" + str(index), score=0.125 + index, timestamp=42000 + index,
                      samples=[index - j for j in range(16)], tags=["east", "west"])
    return fields


def cases():
    for size in (16, 1024, 65536, 262144):
        yield f"string-{size}", "bytes", {"values": [("abcdefghijklmnopqrstuvwxyz" * (size // 26 + 1))[:size]]}
        yield f"int32-{size}", "ints", {"values": [i % 200000 - 100000 for i in range(size // 4)]}
    for pattern in ("empty", "sparse", "full", "present-zero"):
        yield "mask-" + pattern, "row", row(0, pattern)
    for count in (3, 24, 192):
        events = []
        for i in range(count):
            if i % 3 == 0:
                events.append({"type": "bench.tick", "value": {"value": i + 1}})
            elif i % 3 == 1:
                events.append({"type": "bench.update", "value": {"row": row(i, "sparse")}})
            else:
                rows = [row(i + j, p) for j, p in enumerate(("full", "empty", "present-zero"))]
                events.append({"type": "bench.group", "value": {"name": "group", "rows": rows}})
        yield f"batch-{count}", "batch", {"epoch": 17, "events": events}
    yield "strings-mixed", "bytes", {"values": ["", "esc\n\t\"\\", "Москва", {"base64": "/wA="}] * 32}
    yield "strings-boundaries", "bytes", {"values": ["x" * n for n in (0, 1, 252, 253, 254, 255, 65789, 65790)]}


def invoke(binary, kind, directory, mode):
    return subprocess.check_output([binary, kind, directory, mode], text=True)


def verify(binaries, name, kind, payload):
    directory = OUT / "cases" / name
    directory.mkdir(parents=True, exist_ok=True)
    (directory / "input.json").write_text(json.dumps(payload, ensure_ascii=False), encoding="utf-8")
    for lang in ("go", "rust"):
        invoke(binaries[lang], kind, directory, "prepare")
    for codec in ("TL1", "TL2", "JSON"):
        go, rust = [(directory / f"{lang}.{codec}").read_bytes() for lang in ("go", "rust")]
        equal = json.loads(go) == json.loads(rust) if codec == "JSON" else go == rust
        if not equal:
            raise ValueError(f"{name} {codec}: Go and Rust outputs differ")
    for binary in binaries.values():
        invoke(binary, kind, directory, "verify")
    return directory


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--verify-only", action="store_true")
    parser.add_argument("--rounds", type=int, default=2)
    parser.add_argument("--baseline-generator", type=Path,
                        help="also time Rust code from this earlier tl2gen executable")
    parser.add_argument("--rotating", action="store_true", help="also cycle through three batch sizes while reading")
    parser.add_argument("--retain-vector-buffers", action="store_true", help="retain nested Rust vector slots across reads")
    parser.add_argument("--rotating-rounds", type=int, help="run this many rotating rounds independently of --rounds")
    args = parser.parse_args()
    if args.rounds < 1:
        parser.error("--rounds must be positive")
    if args.rotating_rounds is not None and args.rotating_rounds < 1:
        parser.error("--rotating-rounds must be positive")
    binaries = build(args.baseline_generator, args.retain_vector_buffers)
    jobs = [(name, kind, verify(binaries, name, kind, data)) for name, kind, data in cases()]
    print(f"Verified {len(jobs) * 3} codec cases in both directions.", file=sys.stderr)
    if args.verify_only:
        return
    samples = {}
    reuse_samples = {}
    with (OUT / "samples.csv").open("w", newline="") as file:
        writer = csv.writer(file)
        writer.writerow(["round", "case", "language", "codec", "bytes", "encode_ns", "decode_ns", "scratch_decode_ns", "in_place_decode_ns"])
        for index in range(args.rounds):
            for name, kind, directory in jobs:
                for lang in (list(binaries) if index % 2 == 0 else reversed(binaries)):
                    result = invoke(binaries[lang], kind, directory, "bench")
                    for codec, size, enc, dec, *extra in csv.reader(result.splitlines()):
                        scratch, in_place = extra or ("", "")
                        if scratch and lang == "rust":
                            reuse_samples.setdefault((name, codec), []).append((float(scratch), float(in_place)))
                        writer.writerow([index + 1, name, lang, codec, size, enc, dec, scratch, in_place])
                        file.flush()
                        samples.setdefault((name, codec, lang), []).append((int(size), float(enc), float(dec)))
    previous = "previous" in binaries
    with (OUT / "summary.csv").open("w", newline="") as file:
        writer = csv.writer(file)
        writer.writerow(["case", "codec", "go_bytes", "rust_bytes", "go_encode_ns", "rust_encode_ns",
                         "go_decode_ns", "rust_decode_ns", "rust_scratch_decode_ns", "rust_in_place_decode_ns"] + (["previous_rust_encode_ns", "previous_rust_decode_ns"] if previous else []))
        for name, _, _ in jobs:
            for codec in ("TL1", "TL2", "JSON"):
                go, rust = [[statistics.mean(col) for col in zip(*samples[name, codec, lang])]
                            for lang in ("go", "rust")]
                row = [name, codec, int(go[0]), int(rust[0]), go[1], rust[1], go[2], rust[2]]
                metrics = reuse_samples.get((name, codec))
                row.extend(map(statistics.mean, zip(*metrics)) if metrics else ("", ""))
                if previous:
                    _, enc, dec = zip(*samples[name, codec, "previous"])
                    row.extend((statistics.mean(enc), statistics.mean(dec)))
                writer.writerow(row)
    print(OUT / "summary.csv")
    if args.rotating or args.rotating_rounds:
        with (OUT / "rotating.csv").open("w", newline="") as file:
            writer = csv.writer(file)
            writer.writerow(["round", "language", "codec", "decode_ns", "scratch_decode_ns", "in_place_decode_ns"])
            for index in range(args.rotating_rounds or args.rounds):
                for lang in (list(binaries) if index % 2 == 0 else reversed(binaries)):
                    result = invoke(binaries[lang], "batch", OUT / "cases/batch-192", "rotate")
                    for row in csv.reader(result.splitlines()):
                        writer.writerow([index + 1, lang, *row])
        print(OUT / "rotating.csv")


if __name__ == "__main__":
    main()
