// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/VKCOM/tl/internal/pure"
	"github.com/VKCOM/tl/internal/puregen"
	"github.com/VKCOM/tl/internal/tlcodegen/test/codegen_test/golang/common"
	"github.com/VKCOM/tl/internal/tlcodegen/test/gen/goldmaster/factory"
	"github.com/VKCOM/tl/internal/utils"
	"github.com/VKCOM/tl/pkg/basictl"
	"github.com/stretchr/testify/require"
)

func TestGoldmasterInterop(t *testing.T) {
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
	for _, name := range []string{"goldmaster.tl", "goldmaster2.tl", "goldmaster3.tl"} {
		require.NoError(t, kernel.AddFileTL1(filepath.Join("../../tlcodegen/test/tls", name)))
	}
	gen, err := compileRust(kernel, &options)
	require.NoError(t, err)
	require.NoError(t, gen.generateCode())

	fixtures, err := os.ReadFile("../../tlcodegen/test/codegen_test/data/test-stress-data-goldmaster.json")
	require.NoError(t, err)
	var corpus common.AllTestsBytes
	require.NoError(t, json.Unmarshal(fixtures, &corpus))
	require.NotEmpty(t, corpus.Tests)

	tests := filepath.Join(options.Outdir, "tests")
	require.NoError(t, os.Mkdir(tests, 0755))
	harness, err := os.ReadFile("testdata/goldmaster.rs")
	require.NoError(t, err)
	var rust strings.Builder
	rust.Write(harness)
	for index, name := range slices.Sorted(maps.Keys(corpus.Tests)) {
		group := corpus.Tests[name]
		require.NotEmpty(t, group.Successes, name)
		instance := kernel.GetObjectInstanceForTests(group.TestingType)
		require.NotNil(t, instance, name)
		wrapper, err := gen.getType(instance)
		require.NoError(t, err, name)
		typeName := fmt.Sprintf("gentl::types::%[1]s::%[1]s", wrapper.goGlobalName)
		var records []byte
		for _, fixture := range group.Successes {
			tl1 := utils.ParseHexToBytes(fixture.Bytes)
			tl2 := utils.ParseHexToBytesTL2(fixture.BytesTL2)
			value := factory.CreateObjectFromName(group.TestingType)
			require.NotNil(t, value, name)
			if fixture.IsTLBytesBoxed {
				_, err = value.ReadTL1Boxed(tl1)
			} else {
				_, err = value.ReadTL1(tl1)
			}
			require.NoError(t, err, name)
			jsonValue, err := value.WriteJSONGeneral(nil, nil)
			require.NoError(t, err, name)
			// The checked-in TL2 samples also cover independent TL2 presence
			// flags. Use the live Go encoder as the reference for conversion.
			transcoded := value.WriteTL2(nil, nil)
			for _, wire := range [][]byte{tl1, tl2, transcoded, jsonValue} {
				records = basictl.NatWrite(records, uint32(len(wire)))
				records = append(records, wire...)
			}
		}
		filename := fmt.Sprintf("goldmaster_%d.bin", index)
		require.NoError(t, os.WriteFile(filepath.Join(tests, filename), records, 0644))
		boxed := group.Successes[0].IsTLBytesBoxed
		for _, sample := range group.Successes {
			require.Equal(t, boxed, sample.IsTLBytesBoxed, name)
		}
		method := "tl1"
		if boxed {
			method = "tl1_boxed"
		}
		fmt.Fprintf(&rust, "\ngoldmaster!(group_%d, %s, %q, %q, read_%s, write_%s);\n",
			index, typeName, name, filename, method, method)
	}
	require.NoError(t, os.WriteFile(filepath.Join(tests, "goldmaster.rs"), []byte(rust.String()), 0644))
	runCargoTests(t, cargo, options.Outdir)
}
