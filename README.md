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

The Rust backend is experimental. Run `make testrust` to compile the generated
libraries and check binary and TL-JSON compatibility against the Go backend.

## Documentation

- [Internals (TL1)](./docs/TLPrimer.pdf) (in Russian)
- [Internals (TL2)](./docs/TL2Primer.pdf) (in Russian)

## License

TL is licensed under the [Mozilla Public License Version 2.0](./LICENSE).
