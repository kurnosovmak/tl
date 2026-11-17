// Copyright 2025 V Kontakte LLC
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"fmt"
	"strings"

	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
	"github.com/VKCOM/tl/internal/utils"
)

type TypeRW interface {
	// methods below are target language independent
	markWantsBytesVersion(visitedNodes map[*TypeRWWrapper]bool)
	markHasBytesVersion(visitedNodes map[*TypeRWWrapper]bool) bool
	markHasRepairMasks(visitedNodes map[*TypeRWWrapper]bool) bool
	markWriteHasError(visitedNodes map[*TypeRWWrapper]bool) bool

	FillRecursiveChildren(visitedNodes map[*TypeRWWrapper]int, generic bool)
	ContainsUnion(visitedNodes map[*TypeRWWrapper]bool) bool

	BeforeCodeGenerationStep1() // during first phase, struct fields recursive bit is set
	BeforeCodeGenerationStep2() // during second phase, union fields recursive bit is set

	fillRecursiveChildren(visitedNodes map[*TypeRWWrapper]bool)
	typeString2(bytesVersion bool, directImports *DirectImports, isLocal bool, skipAlias bool) string
	typeResettingCode(cc *codecreator.RustCodeCreator, bytesVersion bool, directImports *DirectImports, val string, ref bool)
	typeRandomCode(bytesVersion bool, directImports *DirectImports, val string, natArgs []string, ref bool) string
	typeRepairMasksCode(bytesVersion bool, directImports *DirectImports, val string, natArgs []string, ref bool) string
	typeWritingCode(val string, bare bool, natArgs []string, ref bool) string
	typeReadingCode(cc *codecreator.RustCodeCreator, bytesVersion bool, directImports *DirectImports, val string, bare bool, natArgs []string, ref bool)
	GenerateCode(bytesVersion bool, directImports *DirectImports) string
}

// simply adds commas, natArgs are already fully formatted. Difference to strings.Join is leading comma
func joinWithCommas(natArgs []string) string {
	var s strings.Builder
	for _, arg := range natArgs {
		s.WriteString(fmt.Sprintf(", %s", arg))
	}
	return s.String()
}

func addBytes(val string, bytesVersion bool) string {
	return ifString(bytesVersion, val+"Bytes", val)
}

func addAmpersand(ref bool, val string) string {
	return ifString(ref, val, "&"+val)
}

func addAsterisk(ref bool, val string) string {
	return ifString(ref, "*"+val, val)
}

func addMutableRef(ref bool, val string) string {
	return ifString(ref, val, "&mut "+val)
}

func ifString(value bool, t string, f string) string {
	if value {
		return t
	}
	return f
}

func ToUpperFirst(str string) string {
	return utils.ToUpperFirst(str)
}
