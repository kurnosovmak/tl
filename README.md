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

`tl2gen` can generate a standalone Rust library for TL1 binary data.

```sh
go run ./cmd/tl2gen --language=rust --outdir=./gentl ./schema.tl
cargo check --manifest-path ./gentl/Cargo.toml
```

The generated library requires Rust 1.85 or newer and uses only the standard
library. The package name defaults to `gentl` and can be set with
`--crate-name`. Use a dedicated output directory, since regenerating replaces
its contents.

Types are available in `gentl::types`. Use `read_tl1` and `write_tl1` for
bare values and the `_boxed` methods for values with a constructor tag.
TL strings are represented as `Vec<u8>` so they can preserve arbitrary bytes.

The Rust backend is experimental. TL2 binary codecs, TL-JSON, generic
function wrappers, and RPC client/server generation are outside its current
scope. Run `make testrust` to compile the generated libraries and test TL1
compatibility with the Go backend.

## Documentation

- [Internals (TL1)](./docs/TLPrimer.pdf) (in Russian)
- [Internals (TL2)](./docs/TL2Primer.pdf) (in Russian)

## License

TL is licensed under the [Mozilla Public License Version 2.0](./LICENSE).
