// Copyright 2025 V Kontakte LLC
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"fmt"
	"strings"

	"github.com/VKCOM/tl/internal/pure"
	"github.com/VKCOM/tl/internal/puregen"
	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
)

type Variant struct {
	t           *TypeRWWrapper
	variantName string
	goName      string
	recursive   bool
}

type TypeRWUnion struct {
	wr       *TypeRWWrapper
	pureType *pure.TypeInstanceUnion
	Fields   []Variant
	IsEnum   bool

	fieldsDec puregen.Deconflicter // TODO - add all generated methods here
}

func (trw *TypeRWUnion) ElementNatArgs() []pure.ActualNatArg {
	return trw.pureType.ElementNatArgs()
}

var _ TypeRW = &TypeRWUnion{}

func (trw *TypeRWUnion) typeString2(bytesVersion bool, directImports *DirectImports, isLocal bool, skipAlias bool) string {
	if isLocal {
		return addBytes(trw.wr.goLocalName, bytesVersion)
	}
	return "crate::types::" + trw.wr.goGlobalName + "::" + addBytes(trw.wr.goGlobalName, bytesVersion)
}

func (trw *TypeRWUnion) markHasBytesVersion(visitedNodes map[*TypeRWWrapper]bool) bool {
	result := false
	for _, f := range trw.Fields {
		result = result || f.t.MarkHasBytesVersion(visitedNodes)
	}
	return result
}

func (trw *TypeRWUnion) markHasRepairMasks(visitedNodes map[*TypeRWWrapper]bool) bool {
	result := false
	for _, f := range trw.Fields {
		result = result || f.t.MarkHasRepairMasks(visitedNodes)
	}
	return result
}

func (trw *TypeRWUnion) markWriteHasError(visitedNodes map[*TypeRWWrapper]bool) bool {
	result := false
	for _, f := range trw.Fields {
		result = result || f.t.MarkWriteHasError(visitedNodes)
	}
	return result
}

func (trw *TypeRWUnion) markWantsBytesVersion(visitedNodes map[*TypeRWWrapper]bool) {
	for _, f := range trw.Fields {
		f.t.MarkWantsBytesVersion(visitedNodes)
	}
}

func (trw *TypeRWUnion) FillRecursiveChildren(visitedNodes map[*TypeRWWrapper]int, generic bool) {
	if visitedNodes[trw.wr] != 0 {
		return
	}
	visitedNodes[trw.wr] = 1
	for _, f := range trw.Fields {
		if f.recursive {
			continue
		}
		f.t.trw.FillRecursiveChildren(visitedNodes, generic)
	}
	visitedNodes[trw.wr] = 2
}

func (trw *TypeRWUnion) ContainsUnion(visitedNodes map[*TypeRWWrapper]bool) bool {
	return true
}

func (trw *TypeRWUnion) BeforeCodeGenerationStep1() {
}

func (trw *TypeRWUnion) BeforeCodeGenerationStep2() {
	for i, f := range trw.Fields {
		visitedNodes := map[*TypeRWWrapper]bool{}
		f.t.trw.fillRecursiveChildren(visitedNodes)
		trw.Fields[i].recursive = visitedNodes[trw.wr]
	}
}

func (trw *TypeRWUnion) fillRecursiveChildren(visitedNodes map[*TypeRWWrapper]bool) {
}

func (trw *TypeRWUnion) typeResettingCode(cc *codecreator.RustCodeCreator, bytesVersion bool, directImports *DirectImports, val string, ref bool) {
	cc.AddLinef("%s.reset();", val)
}

func (trw *TypeRWUnion) typeRandomCode(bytesVersion bool, directImports *DirectImports, val string, natArgs []string, ref bool) string {
	return fmt.Sprintf("%s.FillRandom(rg%s)", val, joinWithCommas(natArgs))
}

func (trw *TypeRWUnion) typeRepairMasksCode(bytesVersion bool, directImports *DirectImports, val string, natArgs []string, ref bool) string {
	return fmt.Sprintf("%s.RepairMasks(%s)", val, strings.Join(natArgs, ","))
}

func (trw *TypeRWUnion) typeWritingCode(val string, bare bool, natArgs []string, ref bool) string {
	return fmt.Sprintf("crate::types::%s::write_tl1_boxed(%s, buf%s)?;", trw.wr.goGlobalName, addAmpersand(ref, val), joinWithCommas(natArgs))
}

func (trw *TypeRWUnion) typeReadingCode(cc *codecreator.RustCodeCreator, bytesVersion bool, directImports *DirectImports, val string, bare bool, natArgs []string, ref bool) {
	cc.AddLinef("crate::types::%s::read_tl1_boxed(%s, buf%s)?;", trw.wr.goGlobalName, addMutableRef(ref, val), joinWithCommas(natArgs))
}

func (trw *TypeRWUnion) typeJSONEmptyCondition(bytesVersion bool, val string, ref bool) string {
	return ""
}

func (trw *TypeRWUnion) typeJSONWritingCode(bytesVersion bool, directImports *DirectImports, val string, natArgs []string, ref bool, needError bool) string {
	if needError {
		return fmt.Sprintf("if w, err = %s.WriteJSONOpt(jctx, w %s); err != nil { return w, err }", val, joinWithCommas(natArgs))
	} else {
		return fmt.Sprintf("w = %s.WriteJSONOpt(jctx, w %s)", val, joinWithCommas(natArgs))
	}
}

func (trw *TypeRWUnion) typeJSON2ReadingCode(bytesVersion bool, directImports *DirectImports, jvalue string, val string, natArgs []string, ref bool) string {
	return fmt.Sprintf("if err := %s.ReadJSONGeneral(jctx, %s %s); err != nil { return err }", val, jvalue, joinWithCommas(natArgs))
}

// TODO - remove with long adapters
func (trw *TypeRWUnion) HasShortFieldCollision(wr *TypeRWWrapper) bool {
	//messages.peerId peer_id:int = messages.ChatId;
	//messagesLong.peerId peer_id:long = messages.ChatId;
	//
	//messages.globalChatId#07a5893d chat_id:long = messages.ChatId;
	//messagesLong.globalChatId global_id:messagesLong.GlobalId = messages.ChatId;

	for _, field := range trw.Fields {
		if field.t == wr {
			return true
		}
	}
	return false
}

func (trw *TypeRWUnion) GenerateCode(bytesVersion bool, directImports *DirectImports) string {
	cc := codecreator.NewRustCodeCreator()
	addTL1Import(cc)
	name := trw.wr.goGlobalName
	cc.AddLinef("#[derive(Debug)]")
	cc.AddLinef("pub enum %s {", name)
	cc.FinishBlock(func() {
		for _, field := range trw.Fields {
			if field.t.IsTrueType() {
				cc.AddLinef("%s,", field.goName)
				continue
			}
			fieldType := field.t.TypeString2(bytesVersion, directImports, false, false)
			if field.recursive {
				fieldType = "Box<" + fieldType + ">"
			}
			cc.AddLinef("%s(%s),", field.goName, fieldType)
		}
	}, "}")
	cc.AddEmptyLine()
	cc.AddLinef("impl Default for %s {", name)
	cc.FinishBlock(func() {
		cc.AddLinef("fn default() -> Self {")
		cc.FinishBlock(func() {
			field := trw.Fields[0]
			if field.t.IsTrueType() {
				cc.AddLinef("Self::%s", field.goName)
			} else {
				defaultValue := rustDefault(field.t)
				if field.recursive {
					defaultValue = "Box::new(" + defaultValue + ")"
				}
				cc.AddLinef("Self::%s(%s)", field.goName, defaultValue)
			}
		}, "}")
	}, "}")
	cc.AddEmptyLine()
	cc.AddLinef("impl %s {", name)
	cc.FinishBlock(func() {
		trw.wr.generateTL1Methods(cc, "_boxed", "_boxed", false)
		cc.AddLinef("pub fn reset(&mut self) { *self = Self::default(); }")
	}, "}")
	cc.AddEmptyLine()
	natArgs := trw.wr.formatNatArgs(nil, trw.ElementNatArgs())
	trw.wr.generateTL1Functions(cc, "_boxed", name, func() {
		cc.AddLinef("*value = match buf.read_u32()? {")
		cc.FinishBlock(func() {
			for _, field := range trw.Fields {
				cc.AddLinef("0x%08x => {", field.t.TLTag())
				cc.FinishBlock(func() {
					if field.t.IsTrueType() {
						cc.AddLinef("%s::%s", name, field.goName)
						return
					}
					cc.AddLinef("let mut payload = %s;", rustDefault(field.t))
					field.t.TypeReadingCode(cc, bytesVersion, directImports, "payload", true, natArgs, false)
					payload := "payload"
					if field.recursive {
						payload = "Box::new(payload)"
					}
					cc.AddLinef("%s::%s(%s)", name, field.goName, payload)
				}, "}")
			}
			cc.AddLinef("actual => return Err(basictl::Error::UnexpectedMagic(actual)),")
		}, "};")
		cc.AddLinef("Ok(())")
	}, func() {
		cc.AddLinef("match value {")
		cc.FinishBlock(func() {
			for _, field := range trw.Fields {
				payload := ""
				if !field.t.IsTrueType() {
					payload = "(payload)"
				}
				cc.AddLinef("%s::%s%s => {", name, field.goName, payload)
				cc.FinishBlock(func() {
					cc.AddLinef("basictl::write_tag(buf, 0x%08x);", field.t.TLTag())
					if payload != "" {
						value := "payload"
						if field.recursive {
							value = "payload.as_ref()"
						}
						cc.AddLines(strings.Split(field.t.TypeWritingCode(value, true, natArgs, true), "\n")...)
					}
				}, "}")
			}
		}, "}")
		cc.AddLinef("Ok(())")
	})
	return cc.Text()
}
