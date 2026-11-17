// Copyright 2025 V Kontakte LLC
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

//{% import "github.com/VKCOM/tl/internal/utils" %}
//{% import "strings" %}
//{% import "sort" %}

import (
	"fmt"
	"strings"

	"github.com/VKCOM/tl/internal/pure"
	"github.com/VKCOM/tl/internal/puregen"
	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
)

type TypeRWStruct struct {
	wr             *TypeRWWrapper
	pureTypeStruct *pure.TypeInstanceStruct
	Fields         []Field

	ResultType    *TypeRWWrapper
	ResultNatArgs []pure.ActualNatArg

	unionParent *TypeRWUnion // a bit hackish, but simple
	unionIndex  int

	fieldsDec  puregen.Deconflicter // TODO - add all generated methods here
	setNames   []string             // method names should be the same for bytes and normal versions, so we remember them here
	clearNames []string
	isSetNames []string
}

var _ TypeRW = &TypeRWStruct{}

//lint:ignore U1000 will be used later
func (trw *TypeRWStruct) resultHasFetcher() bool {
	return trw.ResultType != nil && trw.ResultType.HasFetcher()
}

func (trw *TypeRWStruct) isTypedef() bool {
	return trw.pureTypeStruct.IsTypedef()
}

func (trw *TypeRWStruct) isAlias() bool {
	return trw.pureTypeStruct.IsAlias()
}

func (trw *TypeRWStruct) isUnwrapType() bool {
	return trw.pureTypeStruct.IsUnwrap()
}

func (trw *TypeRWStruct) typeString2(bytesVersion bool, directImports *DirectImports, isLocal bool, skipAlias bool) string {
	if !skipAlias && trw.isUnwrapType() {
		return trw.Fields[0].t.TypeString2(bytesVersion, directImports, isLocal, skipAlias)
	}
	if isLocal {
		return addBytes(trw.wr.goLocalName, bytesVersion)
	}
	return "crate::types::" + trw.wr.goGlobalName + "::" + addBytes(trw.wr.goGlobalName, bytesVersion)
}

func (trw *TypeRWStruct) markHasBytesVersion(visitedNodes map[*TypeRWWrapper]bool) bool {
	result := false
	for _, f := range trw.Fields {
		result = result || f.t.MarkHasBytesVersion(visitedNodes)
	}
	if trw.ResultType != nil {
		result = result || trw.ResultType.MarkHasBytesVersion(visitedNodes)
	}
	return result
}

func (trw *TypeRWStruct) markHasRepairMasks(visitedNodes map[*TypeRWWrapper]bool) bool {
	result := false
	for _, field := range trw.Fields {
		result = result || (field.MaskTL2Bit() != nil && !trw.wr.OriginTL2())
	}
	for _, f := range trw.Fields {
		result = result || f.t.MarkHasRepairMasks(visitedNodes)
	}
	// result type does not affect this
	return result
}

func (trw *TypeRWStruct) markWriteHasError(visitedNodes map[*TypeRWWrapper]bool) bool {
	result := false
	for _, f := range trw.Fields {
		result = result || f.t.MarkWriteHasError(visitedNodes)
	}
	if trw.ResultType != nil {
		result = result || trw.ResultType.MarkWriteHasError(visitedNodes)
	}
	return result
}

func (trw *TypeRWStruct) markWantsBytesVersion(visitedNodes map[*TypeRWWrapper]bool) {
	for _, f := range trw.Fields {
		f.t.MarkWantsBytesVersion(visitedNodes)
	}
	if trw.ResultType != nil {
		trw.ResultType.MarkWantsBytesVersion(visitedNodes)
	}
}

func (trw *TypeRWStruct) ContainsUnion(visitedNodes map[*TypeRWWrapper]bool) bool {
	for _, f := range trw.Fields {
		if f.t.containsUnion(visitedNodes) {
			return true
		}
	}
	return false
}

func (trw *TypeRWStruct) FillRecursiveChildren(visitedNodes map[*TypeRWWrapper]int, generic bool) {
	if visitedNodes[trw.wr] != 0 {
		return
	}
	visitedNodes[trw.wr] = 1
}

func (trw *TypeRWStruct) BeforeCodeGenerationStep1() {
	for i, f := range trw.Fields {
		visitedNodes := map[*TypeRWWrapper]bool{}
		f.t.trw.fillRecursiveChildren(visitedNodes)
		trw.Fields[i].recursive = visitedNodes[trw.wr]
	}

	trw.setNames = make([]string, len(trw.Fields))
	trw.clearNames = make([]string, len(trw.Fields))
	trw.isSetNames = make([]string, len(trw.Fields))
}

func (trw *TypeRWStruct) GetAllLocallyAffectedByTrueTypeFieldMasks() []Field {
	nats := make([]Field, 0)
	containingNats := make(map[int]bool)

	for _, field := range trw.Fields {
		if field.IsAffectedByLocalFieldMask() && field.t.IsTrueType() {
			index := field.FieldMask().FieldIndex()
			if _, contains := containingNats[index]; !contains {
				nats = append(nats, trw.Fields[index])
				containingNats[index] = true
			}
		}
	}

	return nats
}

func (trw *TypeRWStruct) GetAllLocallyAffectedFieldMasks() []Field {
	nats := make([]Field, 0)
	containingNats := make(map[int]bool)

	for _, field := range trw.Fields {
		if field.IsAffectedByLocalFieldMask() {
			index := field.FieldMask().FieldIndex()
			if _, contains := containingNats[index]; !contains {
				nats = append(nats, trw.Fields[index])
				containingNats[index] = true
			}
		}
	}

	return nats
}

// AllAffectedFieldMasks f must be from trw.Fields
func (trw *TypeRWStruct) AllAffectedFieldMasks(f Field) (nats []Field, bits []uint32) {
	curField := f
	for curField.IsAffectedByLocalFieldMask() {
		ancestor := trw.Fields[curField.FieldMask().FieldIndex()]
		nats = append(nats, ancestor)
		bits = append(bits, curField.BitNumber())
		curField = ancestor
	}

	return
}

func (trw *TypeRWStruct) BeforeCodeGenerationStep2() {
	//if trw.wr.gen.options.Language == "cpp" { // TODO - temporary solution to benchmark combined tl
	//	var nf []Field
	//	for _, f := range trw.Fields {
	//		if !f.recursive {
	//			nf = append(nf, f)
	//  // panic("recursive field in union " + trw.wr.tlName.String())
	//}
	//}
	//trw.Fields = nf
	//}
}

func (trw *TypeRWStruct) fillRecursiveChildren(visitedNodes map[*TypeRWWrapper]bool) {
	for _, f := range trw.Fields {
		if !f.recursive {
			f.t.FillRecursiveChildren(visitedNodes)
		}
	}
}

func (trw *TypeRWStruct) typeResettingCode(cc *codecreator.RustCodeCreator, bytesVersion bool, directImports *DirectImports, val string, ref bool) {
	if trw.isUnwrapType() {
		trw.Fields[0].t.TypeResettingCode(cc, bytesVersion, directImports, val, ref)
		return
	}
	cc.AddLinef("%s.reset();", val)
}

func (trw *TypeRWStruct) typeRandomCode(bytesVersion bool, directImports *DirectImports, val string, natArgs []string, ref bool) string {
	if trw.isUnwrapType() {
		return trw.Fields[0].t.TypeRandomCode(bytesVersion, directImports, val, trw.pureTypeStruct.ReplaceUnwrapArgs(0, natArgs), ref)
	}
	return fmt.Sprintf("%s.FillRandom(rg %s)", val, joinWithCommas(natArgs))
}

func (trw *TypeRWStruct) typeRepairMasksCode(bytesVersion bool, directImports *DirectImports, val string, natArgs []string, ref bool) string {
	if trw.isUnwrapType() {
		return trw.Fields[0].t.TypeRepairMasksCode(bytesVersion, directImports, val, trw.pureTypeStruct.ReplaceUnwrapArgs(0, natArgs), ref)
	}
	return fmt.Sprintf("%s.RepairMasks(%s)", val, strings.Join(natArgs, ","))
}

func (trw *TypeRWStruct) typeWritingCode(val string, bare bool, natArgs []string, ref bool) string {
	prefix := writeTag(bare, trw.wr.TLTag())
	if trw.isUnwrapType() {
		return prefix + trw.Fields[0].t.TypeWritingCode(val, trw.Fields[0].Bare(), trw.pureTypeStruct.ReplaceUnwrapArgs(0, natArgs), ref)
	}
	return prefix + fmt.Sprintf("crate::types::%s::write_tl1(%s, buf%s%s)?;", trw.wr.goGlobalName, addAmpersand(ref, val), joinWithCommas(natArgs), trw.wr.fetcherCall())
}

func (trw *TypeRWStruct) typeReadingCode(cc *codecreator.RustCodeCreator, bytesVersion bool, directImports *DirectImports, val string, bare bool, natArgs []string, ref bool) {
	readTag(cc, bare, trw.wr.TLTag())
	if trw.isUnwrapType() {
		trw.Fields[0].t.TypeReadingCode(cc, bytesVersion, directImports, val, trw.Fields[0].Bare(), trw.pureTypeStruct.ReplaceUnwrapArgs(0, natArgs), ref)
		return
	}
	cc.AddLinef("crate::types::%s::read_tl1(%s, buf%s%s)?;", trw.wr.goGlobalName, addMutableRef(ref, val), joinWithCommas(natArgs), trw.wr.fetcherCall())
}

func (trw *TypeRWStruct) GenerateCode(bytesVersion bool, directImports *DirectImports) string {
	cc := codecreator.NewRustCodeCreator()
	addTL1Import(cc)
	name := trw.wr.goGlobalName
	tupleStruct := trw.isAlias() || trw.isTypedef()
	fieldType := func(field Field) string {
		typ := field.t.TypeString2(bytesVersion, directImports, false, false)
		if field.recursive {
			typ = "Box<" + typ + ">"
		}
		if field.hasPresence() {
			typ = "Option<" + typ + ">"
		}
		if field.IsBit() {
			return "bool"
		}
		return typ
	}
	cc.AddLinef("#[derive(Debug)]")
	if tupleStruct {
		cc.AddLinef("pub struct %s(pub %s);", name, fieldType(trw.Fields[0]))
	} else {
		cc.AddLinef("pub struct %s {", name)
		cc.FinishBlock(func() {
			for _, field := range trw.Fields {
				if !field.IsTL2Omitted() {
					cc.AddLinef("pub %s: %s,", field.goName, fieldType(field))
				}
			}
		}, "}")
	}
	cc.AddEmptyLine()
	cc.AddLinef("impl Default for %s {", name)
	cc.FinishBlock(func() {
		cc.AddLinef("fn default() -> Self {")
		cc.FinishBlock(func() {
			if tupleStruct {
				defaultValue := "None"
				if !trw.Fields[0].hasPresence() {
					defaultValue = rustDefault(trw.Fields[0].t)
				}
				cc.AddLinef("Self(%s)", defaultValue)
				return
			}
			cc.AddLinef("Self {")
			cc.FinishBlock(func() {
				for _, field := range trw.Fields {
					if field.IsTL2Omitted() {
						continue
					}
					defaultValue := "None"
					if field.IsBit() {
						defaultValue = "false"
					} else if !field.hasPresence() {
						defaultValue = rustDefault(field.t)
					}
					cc.AddLinef("%s: %s,", field.goName, defaultValue)
				}
			}, "}")
		}, "}")
	}, "}")
	cc.AddEmptyLine()
	cc.AddLinef("impl %s {", name)
	cc.FinishBlock(func() {
		trw.wr.generateTL1Methods(cc, "", "", false)
		if trw.wr.TLTag() != 0 || trw.wr.OriginTL2() {
			trw.wr.generateTL1Methods(cc, "_boxed", "", true)
		}
		cc.AddLinef("pub fn reset(&mut self) { *self = Self::default(); }")
	}, "}")
	cc.AddEmptyLine()
	trw.wr.generateTL1Functions(cc, "", name, func() {
		for _, field := range trw.Fields {
			trw.generateTL1Field(cc, field, false, bytesVersion, directImports)
		}
		cc.AddLinef("Ok(())")
	}, func() {
		for _, field := range trw.Fields {
			trw.generateTL1Field(cc, field, true, bytesVersion, directImports)
		}
		cc.AddLinef("Ok(())")
	})
	return cc.Text()
}

func (trw *TypeRWStruct) generateTL1Field(cc *codecreator.RustCodeCreator, field Field, write bool, bytesVersion bool, directImports *DirectImports) {
	if field.IsBit() && field.Bare() && write {
		return
	}
	body := func() {
		if field.IsBit() {
			if write {
				cc.AddLinef("basictl::write_tag(buf, 0x%08x);", field.t.TLTag())
			} else {
				readTag(cc, field.Bare(), field.t.TLTag())
				cc.AddLinef("value.%s = true;", field.goName)
			}
			return
		}
		fieldAccess := "value." + field.goName
		natArgs := trw.wr.formatNatArgs(trw.Fields, field.NatArgs())
		if write && field.hasPresence() {
			access := ifString(field.recursive, "as_deref", "as_ref")
			cc.AddLinef("let field = %s.%s().ok_or(basictl::Error::MissingField(%q))?;", fieldAccess, access, field.OriginalName())
			cc.AddLines(strings.Split(field.t.TypeWritingCode("field", field.Bare(), natArgs, true), "\n")...)
		} else if write {
			cc.AddLines(strings.Split(field.t.TypeWritingCode(fieldAccess, field.Bare(), natArgs, false), "\n")...)
		} else if field.hasPresence() {
			cc.AddLinef("let mut field = %s;", rustDefault(field.t))
			field.t.TypeReadingCode(cc, bytesVersion, directImports, "field", field.Bare(), natArgs, false)
			cc.AddLinef("%s = %s;", fieldAccess, field.wrapDecoded("field"))
		} else {
			field.t.TypeReadingCode(cc, bytesVersion, directImports, fieldAccess, field.Bare(), natArgs, false)
		}
	}
	if field.FieldMask() == nil {
		body()
		return
	}
	mask := trw.wr.formatNatArg(trw.Fields, *field.FieldMask())
	cc.If(fmt.Sprintf("%s & (1 << %d) != 0", mask, field.BitNumber()), body)
}
