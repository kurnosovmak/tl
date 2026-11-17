// Copyright 2025 V Kontakte LLC
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/TwiN/go-color"
	"github.com/VKCOM/tl/internal/pure"
	"github.com/VKCOM/tl/internal/puregen"
	"github.com/VKCOM/tl/internal/tlast"
	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
)

//go:embed basictl.rs
var basicTL string

//go:embed tl2.rs
var tl2Runtime string

//go:embed json.rs
var jsonRuntime string

//go:embed json_vector.rs
var jsonVectorRuntime string

type genRust struct {
	kernel  *pure.Kernel
	options *puregen.Options

	bytesWhiteList      pure.Whitelist
	rawHandlerWhileList pure.Whitelist

	generatedTypes     map[string]*TypeRWWrapper
	generatedTypesList []*TypeRWWrapper // we need more deterministic order than sort predicate can establish

	globalDec  puregen.Deconflicter
	Namespaces map[string]*Namespace // Handlers Code is inside
}

func Generate(kernel *pure.Kernel, options *puregen.Options) error {
	gen, err := compileRust(kernel, options)
	if err != nil {
		return err
	}
	if err := gen.generateCode(); err != nil {
		return err
	}
	if err := gen.bytesWhiteList.UnusedWarning(); err != nil {
		fmt.Printf("%v\n", color.InYellow(err))
	}
	if err := gen.rawHandlerWhileList.UnusedWarning(); err != nil {
		fmt.Printf("%v\n", color.InYellow(err))
	}
	fmt.Printf("%s\n", color.InYellow("warning: rust generator is very experimental and will quickly evolve with no backward compatibility."))
	return nil
}

func compileRust(kernel *pure.Kernel, options *puregen.Options) (*genRust, error) {
	options.Kernel.InstantiateConstants = true
	if err := kernel.Compile(); err != nil {
		return nil, err
	}
	gen := genRust{
		kernel:         kernel,
		options:        options,
		Namespaces:     map[string]*Namespace{},
		generatedTypes: map[string]*TypeRWWrapper{},
	}
	gen.bytesWhiteList = pure.NewWhiteList("--generateByteVersions", options.BytesWhiteList)
	gen.rawHandlerWhileList = pure.NewWhiteList("--rawHandlerWhiteList", options.Go.RawHandlerWhileList)
	// Reserve names that generated declarations could shadow inside their modules.
	for _, name := range []string{"Self", "Default", "Option", "Vec", "Box", "Some", "None", "Ok", "Err"} {
		gen.globalDec.DeconflictName(name)
	}
	if err := gen.compile(); err != nil {
		return nil, err
	}
	return &gen, nil
}

func (gen *genRust) generateCode() error {
	var outdir puregen.OutDir
	var filenames puregen.Deconflicter
	modules := codecreator.NewRustCodeCreator()
	for _, typeRw := range gen.generatedTypesList {
		code := typeRw.trw.GenerateCode(false, &DirectImports{})
		if typeRw.HasTL2() {
			code += typeRw.generateTL2()
			code += typeRw.generatePreflight(true)
		}
		code += typeRw.generatePreflight(false)
		code += typeRw.generateJSON()
		if function, ok := typeRw.trw.(*TypeRWStruct); ok && function.ResultType != nil {
			code += function.generateRPC()
		}
		if code == "" {
			continue
		}
		name := typeRw.goGlobalName
		// Schema identifiers are case-sensitive. Keep the public Rust names but
		// avoid case-only filename differences on filesystems such as APFS.
		filename := filenames.DeconflictName("tl_"+strings.ToLower(name)) + ".rs"
		modules.AddLinef("#[path = %q]", filename)
		modules.AddLinef("pub mod %s;", name)
		if err := outdir.AddCodeFile(filepath.Join("src/types", filename), gen.options.CopyrightText+"\n"+code); err != nil {
			return err
		}
	}
	if gen.options.Kernel.Verbose {
		fmt.Printf("generation of serialization code finished, %d types generated\n", len(gen.generatedTypesList))
	}
	code := fmt.Sprintf(`[package]
name = "%s"
version = "0.1.0"
edition = "2024"
rust-version = "1.85"

[lints.rust]
non_snake_case = "allow"
unused = "allow"

[features]
default = ["tl-json"]
tl-json = ["dep:base64", "dep:itoa", "dep:memchr", "dep:serde", "dep:serde_json"]

[dependencies]
base64 = { version = "0.22", optional = true }
itoa = { version = "1", optional = true }
memchr = { version = "2", optional = true }
serde = { version = "1", optional = true }
serde_json = { version = "1", features = ["float_roundtrip"], optional = true }
`, gen.options.Rust.CrateName)
	if err := outdir.AddCodeFile("Cargo.toml", code); err != nil {
		return err
	}
	code = `/target
`
	if err := outdir.AddCodeFile(".gitignore", code); err != nil {
		return err
	}
	code = `pub mod basictl;
mod tl2;
pub mod types;

#[cfg(feature = "tl-json")]
mod json;
#[cfg(feature = "tl-json")]
mod json_vector;
#[cfg(feature = "tl-json")]
pub use json::JsonMode;
`
	if rpc := gen.generateRPC(); rpc != "" {
		code += "\npub mod rpc;\n"
		if err := outdir.AddCodeFile("src/rpc.rs", gen.options.CopyrightText+"\n"+rpc); err != nil {
			return err
		}
	}
	if err := outdir.AddCodeFile("src/lib.rs", code); err != nil {
		return err
	}
	if err := outdir.AddCodeFile("src/basictl.rs", basicTL); err != nil {
		return err
	}

	if err := outdir.AddCodeFile("src/types/mod.rs", modules.Text()); err != nil {
		return err
	}

	if err := outdir.AddCodeFile("src/tl2.rs", tl2Runtime); err != nil {
		return err
	}

	if err := outdir.AddCodeFile("src/json.rs", jsonRuntime); err != nil {
		return err
	}
	if err := outdir.AddCodeFile("src/json_vector.rs", jsonVectorRuntime); err != nil {
		return err
	}

	return outdir.Write(gen.options, "Cargo.toml")
}

func (gen *genRust) InternalPrefix() string {
	if gen.options.Go.SplitInternal {
		return "internal."
	}
	return ""
}

func (gen *genRust) getNamespace(n string) *Namespace {
	na, ok := gen.Namespaces[n]
	if !ok {
		na = &Namespace{name: n}
		gen.Namespaces[n] = na
		// TODO - ALL golang-specific names
		na.decGo.DeconflictName("Handler")
		na.decGo.DeconflictName("Handle")
		na.decGo.DeconflictName("Client")
	}
	return na
}

func (gen *genRust) getType(t pure.TypeInstance) (*TypeRWWrapper, error) {
	result, ok := gen.generatedTypes[t.CanonicalName()]
	if !ok {
		return nil, fmt.Errorf("internal error: type %q not found", t.CanonicalName())
	}
	return result, nil
}

func (gen *genRust) getTypeMust(t pure.TypeInstance) *TypeRWWrapper {
	result, ok := gen.generatedTypes[t.CanonicalName()]
	if !ok {
		panic(fmt.Errorf("internal error: type instance %q not found", t.CanonicalName()))
	}
	return result
}

func (gen *genRust) getTypeWrapperMust(rt tlast.TL2TypeRef) (*TypeRWWrapper, bool) {
	ref, fieldBare, err := gen.kernel.GetInstance(rt)
	if err != nil {
		panic(fmt.Errorf("internal error: cannot get type of argument %s: %w", rt.String(), err))
	}
	return gen.getTypeMust(ref), fieldBare
}
