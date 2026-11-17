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

	goType         string
	resetValue     string
	randomValue    string
	writeValue     string
	readValue      string
	writeJSONValue string
	readJSON2Value string
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
	case "__function", "__function_result":
		return "return Err(basictl::Error::UnsupportedFunction);"
	default:
		return fmt.Sprintf("buf.extend_from_slice(&(%s).to_le_bytes());", val)
	}
}

func (trw *TypeRWPrimitive) typeJSONEmptyCondition(bytesVersion bool, val string, ref bool) string {
	if trw.canonicalType == "string" {
		return fmt.Sprintf("len(%s) != 0", addAsterisk(ref, val))
	}
	if trw.canonicalType == "__function" || trw.canonicalType == "__function_result" {
		return "true"
	}
	return fmt.Sprintf("%s != 0", addAsterisk(ref, val))
}

func (trw *TypeRWPrimitive) typeReadingCode(cc *codecreator.RustCodeCreator, bytesVersion bool, directImports *DirectImports, val string, bare bool, natArgs []string, ref bool) {
	if trw.canonicalType == "__function" || trw.canonicalType == "__function_result" {
		cc.AddLinef("return Err(basictl::Error::UnsupportedFunction);")
		return
	}
	cc.AddLinef(trw.readValue, addAsterisk(ref, val))
}

func (trw *TypeRWPrimitive) typeJSONWritingCode(bytesVersion bool, directImports *DirectImports, val string, natArgs []string, ref bool, needError bool) string {
	writeJSONValue := trw.writeJSONValue
	if bytesVersion {
		writeJSONValue += "Bytes"
	}
	return fmt.Sprintf("w = %s(w, %s)", writeJSONValue, addAsterisk(ref, val))
}

func (trw *TypeRWPrimitive) typeJSON2ReadingCode(bytesVersion bool, directImports *DirectImports, jvalue string, val string, natArgs []string, ref bool) string {
	readJSONValue := trw.readJSON2Value
	if bytesVersion {
		readJSONValue += "Bytes"
	}
	return wrapLast(false, fmt.Sprintf("%s(%s, %s)", readJSONValue, jvalue, addAmpersand(ref, val)))
}

func (trw *TypeRWPrimitive) GenerateCode(byteVersion bool, directImports *DirectImports) string {
	return ""
}
