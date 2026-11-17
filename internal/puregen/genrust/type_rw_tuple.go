// Copyright 2025 V Kontakte LLC
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"fmt"

	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
)

// check that brackets cannot be function return type

type TypeRWBrackets struct {
	wr          *TypeRWWrapper
	vectorLike  bool   // # [T], because # has no reference name
	dynamicSize bool   // with passed nat param
	size        uint32 // if !dynamicSize
	element     Field
}

var _ TypeRW = &TypeRWBrackets{}

func (trw *TypeRWBrackets) typeString2(bytesVersion bool, directImports *DirectImports, isLocal bool, skipAlias bool) string {
	if trw.vectorLike || trw.dynamicSize {
		return fmt.Sprintf("Vec<%s>", trw.element.t.TypeString2(bytesVersion, directImports, isLocal, skipAlias))
	}
	return fmt.Sprintf("[%s; %d]", trw.element.t.TypeString2(bytesVersion, directImports, isLocal, skipAlias), trw.size)
}

func (trw *TypeRWBrackets) markHasBytesVersion(visitedNodes map[*TypeRWWrapper]bool) bool {
	return trw.element.t.MarkHasBytesVersion(visitedNodes)
}

func (trw *TypeRWBrackets) markHasRepairMasks(visitedNodes map[*TypeRWWrapper]bool) bool {
	return trw.element.t.MarkHasRepairMasks(visitedNodes)
}

func (trw *TypeRWBrackets) markWriteHasError(visitedNodes map[*TypeRWWrapper]bool) bool {
	if trw.dynamicSize {
		return true
	}
	return trw.element.t.MarkWriteHasError(visitedNodes)
}

func (trw *TypeRWBrackets) markWantsBytesVersion(visitedNodes map[*TypeRWWrapper]bool) {
	trw.element.t.MarkWantsBytesVersion(visitedNodes)
}

func (trw *TypeRWBrackets) FillRecursiveChildren(visitedNodes map[*TypeRWWrapper]int, generic bool) {
}

func (trw *TypeRWBrackets) ContainsUnion(visitedNodes map[*TypeRWWrapper]bool) bool {
	return trw.element.t.containsUnion(visitedNodes)
}

func (trw *TypeRWBrackets) BeforeCodeGenerationStep1() {
}

func (trw *TypeRWBrackets) BeforeCodeGenerationStep2() {
}

func (trw *TypeRWBrackets) fillRecursiveChildren(visitedNodes map[*TypeRWWrapper]bool) {
	if trw.vectorLike || trw.dynamicSize {
		return
	}
	trw.element.t.FillRecursiveChildren(visitedNodes)
}

func (trw *TypeRWBrackets) typeResettingCode(cc *codecreator.RustCodeCreator, bytesVersion bool, directImports *DirectImports, val string, ref bool) {
	if trw.dynamicSize || trw.vectorLike {
		cc.AddLinef("%s.clear();", val)
		return
	}
	cc.AddLinef("crate::types::%s::reset(%s);", trw.wr.goGlobalName, addMutableRef(ref, val))
}

func (trw *TypeRWBrackets) typeRandomCode(bytesVersion bool, directImports *DirectImports, val string, natArgs []string, ref bool) string {
	goGlobalName := addBytes(trw.wr.goGlobalName, bytesVersion)
	return fmt.Sprintf("%sFillRandom(rg, %s%s)", goGlobalName, addAmpersand(ref, val), joinWithCommas(natArgs))
}

func (trw *TypeRWBrackets) typeRepairMasksCode(bytesVersion bool, directImports *DirectImports, val string, natArgs []string, ref bool) string {
	goGlobalName := addBytes(trw.wr.goGlobalName, bytesVersion)
	return fmt.Sprintf("%sRepairMasks(%s%s)", goGlobalName, addAmpersand(ref, val), joinWithCommas(natArgs))
}

func (trw *TypeRWBrackets) typeWritingCode(val string, bare bool, natArgs []string, ref bool) string {
	return writeTag(bare, trw.wr.TLTag()) + fmt.Sprintf("crate::types::%s::write_tl1(%s, buf%s%s)?;", trw.wr.goGlobalName, addAmpersand(ref, val), joinWithCommas(natArgs), trw.wr.fetcherCall())
}

func (trw *TypeRWBrackets) typeReadingCode(cc *codecreator.RustCodeCreator, bytesVersion bool, directImports *DirectImports, val string, bare bool, natArgs []string, ref bool) {
	readTag(cc, bare, trw.wr.TLTag())
	cc.AddLinef("crate::types::%s::read_tl1(%s, buf%s%s)?;", trw.wr.goGlobalName, addMutableRef(ref, val), joinWithCommas(natArgs), trw.wr.fetcherCall())
}

func (trw *TypeRWBrackets) GenerateCode(bytesVersion bool, directImports *DirectImports) string {
	length := ""
	if trw.dynamicSize {
		length = "nat_" + trw.wr.NatParams()[0]
	}
	return generateSequence(trw.wr, trw.element, trw.vectorLike, length, bytesVersion, directImports)
}
