# TL

TL is a data description language and data storage format.

## Implementation details

We do not rush go version until we need some new feature or (more often) we need a fix
from our dependency, which requires newer go version.

## Overview

In general, interest in TL stems from a desire to serialize data and make RPC calls to servers that implement TL RPC, as well as to implement such servers

TL describes data structures, including RPC queriesand their responses, using syntax derived from functional programming languages. 

The TL format is characterized by compactness and high efficiency.

TL is schema-driven format. Tool caled `tlgen` is used to generate structs/classes and (de)serialization methods.


## Running without installation 

You can run tool without installation. This is recommended way for most use cases.

```
go run github.com/VKCOM/tl/cmd/tlgen@latest <options>
```

For build scripts, you can pin particular version instead of `latest`.

## Installation

Install `tlgen` with the following command

```
go install github.com/VKCOM/tl/cmd/tlgen@latest
```

## Rust code generation

`tl2gen` generates a Rust library from TL1 (`.tl`) and TL2 (`.tl2`) schemas.

```sh
go run ./cmd/tl2gen --language=rust --tl2WhiteList='*' --outdir=./gentl ./schema.tl
cargo check --manifest-path ./gentl/Cargo.toml
```

The generated library requires Rust 1.85 or newer. The package name defaults to
`gentl` and can be set with `--crate-name`. Use a dedicated output directory,
since regenerating replaces its contents.

Generated types live in `gentl::types`. Unions and enumerations use Rust
`enum`s, with typed payloads on variants that carry data. TL strings use
`Vec<u8>`, and TL2 optional fields use `Option<T>`.

- Use `read_tl1` and `write_tl1` for bare values and the `_boxed` methods
  for values with a constructor tag. TL1 field masks control TL1 presence.
- Use `read_tl2` and `write_tl2` for TL2 binary values. For TL1 schemas,
  `--tl2WhiteList` selects which types also get TL2 codecs.
- Use `read_json` and `write_json` with `JsonMode` to select TL1, legacy
  tagged TL1, or TL2 names. The default `tl-json` Cargo feature enables
  the JSON codecs. Build with `--no-default-features` for binary codecs
  using only the standard library.
- RPC functions have typed result codecs. `gentl::rpc::Request` and
  `Response` encode and dispatch RPC payloads, including legacy TL1
  `!X` wrappers. The application provides the network transport.

For streams of binary messages, keep a second value of the same type as scratch
storage. `value.read_tl1_with_scratch(&mut input, &mut scratch)?` and
`read_tl2_with_scratch` decode into that storage, then swap the values.
`read_tl1_boxed_with_scratch` also consumes the constructor tag. On error the
receiver and input cursor retain their original values. Scratch can contain a
partial message and can be used for the next read. This mode keeps up to two
decoded messages in memory. The ordinary readers also reuse compatible buffers
after validating the input.

When the caller can discard a partially decoded message after an error,
use `read_tl1_in_place`, `read_tl1_boxed_in_place` or `read_tl2_in_place`.
These methods decode in a single pass into one reusable value. On error the
input cursor stays at the start of the message, and the next read can reuse
the same value.

The Rust backend is experimental. Run `make testrust` to compile the generated
libraries and check binary and TL-JSON compatibility against the Go backend.

Run `python3 internal/puregen/genrust/testdata/bench/run.py` to benchmark both
backends on strings, numeric vectors, field masks and nested unions. Each reader
decodes the other writer's output before timing. Both readers then time the same
Go input with reusable destinations. The `rust_scratch_decode_ns` and
`rust_in_place_decode_ns` columns time the two explicit binary reuse modes.
Each writer reuses its output buffer.
The runner alternates languages between rounds and saves per-round medians
(seven samples each) and their means in `target/rust-bench/{samples,summary}.csv`.
Use `--verify-only` for the compatibility checks, `--rounds N` to repeat, or
`--baseline-generator /path/to/tl2gen` to include an earlier Rust backend.
Add `--rotating` to cycle through batches of 3, 24 and 192 events using
the same destination. Per-round read times are saved in `rotating.csv`.

## Documentation

- [Internals (TL1)](./docs/TLPrimer.pdf) (in Russian)
- [Internals (TL2)](./docs/TL2Primer.pdf) (in Russian)

## License

TL is licensed under the [Mozilla Public License Version 2.0](./LICENSE).
