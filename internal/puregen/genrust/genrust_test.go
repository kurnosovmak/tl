// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"bytes"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/VKCOM/tl/internal/pure"
	"github.com/VKCOM/tl/internal/puregen"
	"github.com/VKCOM/tl/internal/tlcodegen/test/gen/cases/tlcases"
	"github.com/VKCOM/tl/internal/tlcodegen/test/gen/cases/tlcasesTL2"
	"github.com/VKCOM/tl/internal/tlcodegen/test/gen/cases/tlcases_bytes"
	"github.com/VKCOM/tl/pkg/basictl"
	"github.com/stretchr/testify/require"
)

func TestCodecInterop(t *testing.T) {
	cargo, err := exec.LookPath("cargo")
	if err != nil {
		t.Skip("cargo is required to test generated Rust")
	}
	options := puregen.Options{
		Language:    "rust",
		Outdir:      t.TempDir(),
		Rust:        puregen.OptionsRust{CrateName: "gentl"},
		ErrorWriter: io.Discard,
		Kernel:      pure.OptionsKernel{TypesWhiteList: "*", TL2WhiteList: "*"},
	}
	require.NoError(t, options.Validate())
	kernel := pure.NewKernel(&options.Kernel)
	require.NoError(t, kernel.AddFileTL1("../../tlcodegen/test/tls/cases.tl"))
	require.NoError(t, kernel.AddFileTL1("testdata/rpc.tl"))
	require.NoError(t, Generate(kernel, &options))

	tests := filepath.Join(options.Outdir, "tests")
	require.NoError(t, os.Mkdir(tests, 0755))
	for _, file := range []string{"interop.rs", "rpc.rs"} {
		copyRustTest(t, tests, file)
	}

	type sample interface {
		FillRandom(*basictl.RandGenerator)
		WriteTL1BoxedGeneral([]byte) ([]byte, error)
		WriteTL2([]byte, *basictl.TL2WriteContext) []byte
		WriteJSONGeneral(*basictl.JSONWriteContext, []byte) ([]byte, error)
	}
	random := basictl.NewRandGenerator(rand.New(rand.NewSource(10)))
	for _, fixture := range []struct {
		name  string
		value sample
	}{
		{"cycle", &tlcases.MyCycle1{}},
		{"array", &tlcases.TestArray{}},
		{"vector", &tlcases.TestVector{}},
		{"tuple", &tlcases.TestTuple{}},
		{"dictionaries", &tlcases.TestAllDicts{}},
		{"maybe", &tlcases.TestMaybe{}},
		{"union", &tlcases.TestUnionContainer{}},
		{"enum", &tlcases.TestEnumContainer{}},
		{"masks", &tlcases.TestAllPossibleFieldConfigsContainer{}},
		{"nat_args", &tlcases.TestInplaceStructArgs2{}},
		{"bools", &tlcasesTL2.TestArrayFixedBool{}},
	} {
		var wire, wire2 []byte
		var json bytes.Buffer
		for range 16 {
			fixture.value.FillRandom(random)
			wire, err = fixture.value.WriteTL1BoxedGeneral(wire)
			require.NoError(t, err, fixture.name)
			wire2 = fixture.value.WriteTL2(wire2, nil)
			encoded, err := fixture.value.WriteJSONGeneral(nil, nil)
			require.NoError(t, err, fixture.name)
			json.Write(encoded)
			json.WriteByte('\n')
		}
		require.NoError(t, os.WriteFile(filepath.Join(tests, fixture.name+".bin"), wire, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(tests, fixture.name+".tl2"), wire2, 0644))
		require.NoError(t, os.WriteFile(filepath.Join(tests, fixture.name+".jsonl"), json.Bytes(), 0644))
	}

	// Force both the short and medium string encodings. TL strings may contain non-UTF-8 bytes.
	strings := tlcases_bytes.TestVector{Arr: []string{"a\xff", string(make([]byte, 254))}}
	require.NoError(t, os.WriteFile(filepath.Join(tests, "strings.bin"), strings.WriteTL1Boxed(nil), 0644))
	jsonStrings, err := strings.WriteJSONGeneral(nil, nil)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(tests, "strings.jsonl"), jsonStrings, 0644))

	function := tlcasesTL2.TestFunctionNoDep1{X: 42}
	rpcResult1, err := function.WriteResultTL1(nil, -17)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(tests, "rpc_result.bin"), rpcResult1, 0644))
	rpcResult2 := function.WriteResultTL2(nil, nil, -17)
	require.NoError(t, os.WriteFile(filepath.Join(tests, "rpc_result.tl2"), rpcResult2, 0644))

	runCargoTests(t, cargo, options.Outdir)
}

func copyRustTest(t *testing.T, dest, file string) {
	t.Helper()
	rustTests, err := os.ReadFile(filepath.Join("testdata", file))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dest, file), rustTests, 0644))
}

func runCargoTests(t *testing.T, cargo, dir string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), cargo, "test")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
}

func TestNativeTL2(t *testing.T) {
	cargo, err := exec.LookPath("cargo")
	if err != nil {
		t.Skip("cargo is required to test generated Rust")
	}
	options := puregen.Options{
		Language:    "rust",
		Outdir:      t.TempDir(),
		Rust:        puregen.OptionsRust{CrateName: "gentl"},
		ErrorWriter: io.Discard,
		Kernel:      pure.OptionsKernel{TypesWhiteList: "*"},
	}
	require.NoError(t, options.Validate())
	kernel := pure.NewKernel(&options.Kernel)
	require.NoError(t, kernel.AddFileTL2("../../tlcodegen/test/tls/cases.tl2"))
	require.NoError(t, kernel.AddFileTL2("testdata/native.tl2"))
	require.NoError(t, Generate(kernel, &options))

	tests := filepath.Join(options.Outdir, "tests")
	require.NoError(t, os.Mkdir(tests, 0755))
	copyRustTest(t, tests, "native.rs")
	for _, legacy := range []bool{false, true} {
		mask := byte(0b10000110)
		if legacy {
			mask |= 8
		}
		body := basictl.IntWrite([]byte{mask}, 0)
		if legacy {
			body = basictl.StringWriteTL2(body, "discard")
		}
		body = basictl.IntWrite(body, -17)
		body = basictl.IntWrite(append(body, 5), 42)
		body = append(body, 3, 9, 0x81, 1)
		wire := append(basictl.TL2WriteSize(nil, len(body)), body...)
		name := "record.tl2"
		if legacy {
			name = "record_with_old_field.tl2"
		}
		require.NoError(t, os.WriteFile(filepath.Join(tests, name), wire, 0644))
	}
	runCargoTests(t, cargo, options.Outdir)
}
