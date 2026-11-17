// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/VKCOM/tl/internal/pure"
	"github.com/VKCOM/tl/internal/puregen"
	"github.com/VKCOM/tl/internal/tlcodegen/test/gen/casesTL1/tlcases"
	"github.com/VKCOM/tl/internal/tlcodegen/test/gen/casesTL1/tlcases_bytes"
	"github.com/VKCOM/tl/pkg/basictl"
	"github.com/stretchr/testify/require"
)

func TestTL1Interop(t *testing.T) {
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
	require.NoError(t, kernel.AddFileTL1("../../tlcodegen/test/tls/cases.tl"))
	require.NoError(t, Generate(kernel, &options))

	tests := filepath.Join(options.Outdir, "tests")
	require.NoError(t, os.Mkdir(tests, 0755))
	rustTests, err := os.ReadFile("testdata/tl1.rs")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(tests, "tl1.rs"), rustTests, 0644))

	type sample interface {
		FillRandom(*basictl.RandGenerator)
		WriteTL1BoxedGeneral([]byte) ([]byte, error)
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
	} {
		var wire []byte
		for range 16 {
			fixture.value.FillRandom(random)
			wire, err = fixture.value.WriteTL1BoxedGeneral(wire)
			require.NoError(t, err, fixture.name)
		}
		require.NoError(t, os.WriteFile(filepath.Join(tests, fixture.name+".bin"), wire, 0644))
	}

	// Force both the short and medium string encodings. TL strings may contain non-UTF-8 bytes.
	strings := tlcases_bytes.TestVector{Arr: []string{"a\xff", string(make([]byte, 254))}}
	require.NoError(t, os.WriteFile(filepath.Join(tests, "strings.bin"), strings.WriteTL1Boxed(nil), 0644))

	cmd := exec.CommandContext(t.Context(), cargo, "test", "--offline")
	cmd.Dir = options.Outdir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
}
