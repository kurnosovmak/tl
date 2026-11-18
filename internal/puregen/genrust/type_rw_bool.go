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

type TypeRWBool struct {
	wr          *TypeRWWrapper
	falseGoName string
	trueGoName  string
	falseTag    uint32
	trueTag     uint32

	isBit bool
}

var _ TypeRW = &TypeRWBool{}

func (trw *TypeRWBool) typeString2(bytesVersion bool, directImports *DirectImports, isLocal bool, skipAlias bool) string {
	if !skipAlias {
		return "bool"
	}
	if isLocal {
		return addBytes(trw.wr.goLocalName, bytesVersion)
	}
	return addBytes(trw.wr.goGlobalName, bytesVersion)
}

func (trw *TypeRWBool) markHasBytesVersion(visitedNodes map[*TypeRWWrapper]bool) bool {
	return false
}

func (trw *TypeRWBool) markHasRepairMasks(visitedNodes map[*TypeRWWrapper]bool) bool {
	return false
}

func (trw *TypeRWBool) markWriteHasError(visitedNodes map[*TypeRWWrapper]bool) bool {
	return false
}

func (trw *TypeRWBool) markWantsBytesVersion(visitedNodes map[*TypeRWWrapper]bool) {
}

func (trw *TypeRWBool) ContainsUnion(visitedNodes map[*TypeRWWrapper]bool) bool {
	return false
}

func (trw *TypeRWBool) FillRecursiveChildren(visitedNodes map[*TypeRWWrapper]int, generic bool) {
}

func (trw *TypeRWBool) BeforeCodeGenerationStep1() {
}

func (trw *TypeRWBool) BeforeCodeGenerationStep2() {
}

func (trw *TypeRWBool) fillRecursiveChildren(visitedNodes map[*TypeRWWrapper]bool) {
}

func (trw *TypeRWBool) typeResettingCode(cc *codecreator.RustCodeCreator, bytesVersion bool, directImports *DirectImports, val string, ref bool) {
	cc.AddLinef("%s = false", addAsterisk(ref, val))
}

func (trw *TypeRWBool) typeRandomCode(bytesVersion bool, directImports *DirectImports, val string, natArgs []string, ref bool) string {
	return fmt.Sprintf("%s = basictl.RandomUint(rg) & 1 == 1", addAsterisk(ref, val))
}

func (trw *TypeRWBool) typeRepairMasksCode(bytesVersion bool, directImports *DirectImports, val string, natArgs []string, ref bool) string {
	return ""
}

func (trw *TypeRWBool) typeWritingCode(val string, bare bool, natArgs []string, ref bool) string {
	if trw.isBit {
		return `return Err(basictl::Error::NoTL1("bit"));`
	}
	return fmt.Sprintf("crate::types::%s::write_tl1(%s, buf)?;", trw.wr.goGlobalName, addAsterisk(ref, val))
}

func (trw *TypeRWBool) typeReadingCode(cc *codecreator.RustCodeCreator, bytesVersion bool, directImports *DirectImports, val string, bare bool, natArgs []string, ref bool) {
	if trw.isBit {
		cc.AddLinef(`return Err(basictl::Error::NoTL1("bit"));`)
		return
	}
	cc.AddLinef("%s = crate::types::%s::read_tl1(buf)?;", addAsterisk(ref, val), trw.wr.goGlobalName)
}

func (trw *TypeRWBool) GenerateCode(bytesVersion bool, directImports *DirectImports) string {
	if trw.isBit {
		return ""
	}
	cc := codecreator.NewRustCodeCreator()
	addTL1Import(cc)
	cc.AddLinef("pub(crate) fn read_tl1(buf: &mut &[u8]) -> basictl::Result<bool> {")
	cc.FinishBlock(func() {
		if trw.wr.OriginTL2() {
			noTL1(cc, trw.wr)
			return
		}
		cc.AddLinef("buf.read_bool(0x%08x, 0x%08x)", trw.falseTag, trw.trueTag)
	}, "}")
	cc.AddEmptyLine()
	cc.AddLinef("pub(crate) fn write_tl1(value: bool, buf: &mut Vec<u8>) -> basictl::Result<()> {")
	cc.FinishBlock(func() {
		if trw.wr.OriginTL2() {
			noTL1(cc, trw.wr)
			return
		}
		cc.AddLinef("basictl::write_tag(buf, if value { 0x%08x } else { 0x%08x });", trw.trueTag, trw.falseTag)
		cc.AddLinef("Ok(())")
	}, "}")
	return cc.Text()
}
