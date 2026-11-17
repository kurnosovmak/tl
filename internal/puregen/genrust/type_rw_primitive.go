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

type TypeRWPrimitive struct {
	canonicalType string

	historicalName string // go generator historically uses TL1-style names

	goType      string
	resetValue  string
	randomValue string
	writeValue  string
	readValue   string
}

var _ TypeRW = &TypeRWPrimitive{}

func (trw *TypeRWPrimitive) typeString2(bytesVersion bool, directImports *DirectImports, isLocal bool, skipAlias bool) string {
	if isLocal {
		return addBytes(trw.goType, bytesVersion)
	}
	if bytesVersion {
		return "[]byte"
	}
	return trw.goType
}

func (trw *TypeRWPrimitive) markHasBytesVersion(visitedNodes map[*TypeRWWrapper]bool) bool {
	return trw.canonicalType == "string"
}

func (trw *TypeRWPrimitive) markHasRepairMasks(visitedNodes map[*TypeRWWrapper]bool) bool {
	return false
}

func (trw *TypeRWPrimitive) markWriteHasError(visitedNodes map[*TypeRWWrapper]bool) bool {
	return false
}

func (trw *TypeRWPrimitive) markWantsBytesVersion(visitedNodes map[*TypeRWWrapper]bool) {
}

func (trw *TypeRWPrimitive) ContainsUnion(visitedNodes map[*TypeRWWrapper]bool) bool {
	return false
}

func (trw *TypeRWPrimitive) FillRecursiveChildren(visitedNodes map[*TypeRWWrapper]int, generic bool) {
}

func (trw *TypeRWPrimitive) BeforeCodeGenerationStep1() {
}

func (trw *TypeRWPrimitive) BeforeCodeGenerationStep2() {
}

func (trw *TypeRWPrimitive) fillRecursiveChildren(visitedNodes map[*TypeRWWrapper]bool) {
}

func (trw *TypeRWPrimitive) typeResettingCode(cc *codecreator.RustCodeCreator, bytesVersion bool, directImports *DirectImports, val string, ref bool) {
	if bytesVersion {
		if ref {
			cc.AddLinef("*%[1]s = (*%[1]s)[:0]", val)
			return
		}
		cc.AddLinef("%[1]s = %[1]s[:0]", val)
		return
	}
	cc.AddLinef(trw.resetValue, addAsterisk(ref, val))
}

func (trw *TypeRWPrimitive) typeRandomCode(bytesVersion bool, directImports *DirectImports, val string, natArgs []string, ref bool) string {
	if bytesVersion {
		return fmt.Sprintf("%s = %sBytes(rg)", addAsterisk(ref, val), trw.randomValue)
	}
	return fmt.Sprintf("%s = %s(rg)", addAsterisk(ref, val), trw.randomValue)
}

func (trw *TypeRWPrimitive) typeRepairMasksCode(bytesVersion bool, directImports *DirectImports, val string, natArgs []string, ref bool) string {
	return ""
}

func (trw *TypeRWPrimitive) typeWritingCode(val string, bare bool, natArgs []string, ref bool) string {
	switch trw.canonicalType {
	case "string":
		return fmt.Sprintf("basictl::write_string(buf, %s)?;", addAmpersand(ref, val))
	case "__function":
		return fmt.Sprintf("(%s).as_deref().ok_or(basictl::Error::MissingRpcPayload)?.write_tl1(buf)?;", val)
	case "__function_result":
		return fmt.Sprintf("request.write_response_tl1((%s).as_deref().ok_or(basictl::Error::MissingRpcPayload)?, buf)?;", val)
	default:
		return fmt.Sprintf("buf.extend_from_slice(&(%s).to_le_bytes());", val)
	}
}

func (trw *TypeRWPrimitive) typeReadingCode(cc *codecreator.RustCodeCreator, bytesVersion bool, directImports *DirectImports, val string, bare bool, natArgs []string, ref bool) {
	target := addAsterisk(ref, val)
	switch trw.canonicalType {
	case "__function":
		cc.AddLinef("%s = Some(Box::new(crate::rpc::Request::read_tl1(buf)?));", target)
	case "__function_result":
		cc.AddLinef("%s = Some(Box::new(request.read_response_tl1(buf)?));", target)
	default:
		cc.AddLinef(trw.readValue, target)
	}
}

func (trw *TypeRWPrimitive) GenerateCode(byteVersion bool, directImports *DirectImports) string {
	return ""
}
