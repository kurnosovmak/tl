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
)

func addTL1Import(cc *codecreator.RustCodeCreator) {
	cc.AddLinef("use crate::basictl::{self, TLRead as _};")
	cc.AddEmptyLine()
}

func readTag(cc *codecreator.RustCodeCreator, bare bool, tag uint32) {
	if !bare {
		cc.AddLinef("buf.read_exact_tag(0x%08x)?;", tag)
	}
}

func writeTag(bare bool, tag uint32) string {
	if bare {
		return ""
	}
	return fmt.Sprintf("basictl::write_tag(buf, 0x%08x);\n", tag)
}

func noTL1(cc *codecreator.RustCodeCreator, wrapper *TypeRWWrapper) {
	cc.AddLinef("Err(basictl::Error::NoTL1(%q))", wrapper.pureType.CanonicalName())
}

// Rust's Default implementation for arrays covers only lengths up to 32.
func rustDefault(wrapper *TypeRWWrapper) string {
	switch typ := wrapper.trw.(type) {
	case *TypeRWBrackets:
		if !typ.vectorLike && !typ.dynamicSize {
			return "std::array::from_fn(|_| " + rustDefault(typ.element.t) + ")"
		}
	case *TypeRWStruct:
		if typ.isUnwrapType() {
			return rustDefault(typ.Fields[0].t)
		}
	}
	return "Default::default()"
}

func (wr *TypeRWWrapper) generateTL1Methods(cc *codecreator.RustCodeCreator, publicSuffix, internalSuffix string, withTag bool) {
	natDecl := wr.formatNatArgsDecl() + wr.fetcherDecl()
	natCall := wr.formatNatArgsDeclCall() + wr.fetcherCall()
	cc.AddLinef("pub fn read_tl1%s(&mut self, buf: &mut &[u8]%s) -> basictl::Result<()> {", publicSuffix, natDecl)
	cc.FinishBlock(func() {
		cc.AddLinef("let mut input = *buf;")
		if withTag && !wr.OriginTL2() {
			cc.AddLinef("input.read_exact_tag(0x%08x)?;", wr.TLTag())
		}
		if !reuseVectorTL1(cc, wr) {
			if wr.preflightRoot() {
				cc.AddLinef("let mut probe = input;")
				cc.If("preflight_tl1(&mut probe, self).is_some()", func() {
					cc.AddLinef("self::read_tl1%s(self, &mut input%s)?;", internalSuffix, natCall)
					cc.AddLinef("*buf = input;")
					cc.AddLinef("return Ok(());")
				})
			}
			cc.AddLinef("let mut value = Self::default();")
			cc.AddLinef("self::read_tl1%s(&mut value, &mut input%s)?;", internalSuffix, natCall)
			cc.AddLinef("*self = value;")
		}
		cc.AddLinef("*buf = input;")
		cc.AddLinef("Ok(())")
	}, "}")
	cc.AddLinef("/// Reuse this value directly. On error the cursor is unchanged and the value may be partially updated.")
	cc.AddLinef("pub fn read_tl1%s_in_place(&mut self, buf: &mut &[u8]%s) -> basictl::Result<()> {", publicSuffix, natDecl)
	cc.FinishBlock(func() {
		cc.AddLinef("let mut input = *buf;")
		if withTag && !wr.OriginTL2() {
			cc.AddLinef("input.read_exact_tag(0x%08x)?;", wr.TLTag())
		}
		cc.AddLinef("self::read_tl1%s(self, &mut input%s)?;", internalSuffix, natCall)
		cc.AddLinef("*buf = input;")
		cc.AddLinef("Ok(())")
	}, "}")
	cc.AddLinef("/// Reuse scratch storage and replace this value only after a successful read.")
	cc.AddLinef("pub fn read_tl1%s_with_scratch(&mut self, buf: &mut &[u8], scratch: &mut Self%s) -> basictl::Result<()> {", publicSuffix, natDecl)
	cc.FinishBlock(func() {
		cc.AddLinef("scratch.read_tl1%s_in_place(buf%s)?;", publicSuffix, natCall)
		cc.AddLinef("std::mem::swap(self, scratch);")
		cc.AddLinef("Ok(())")
	}, "}")
	cc.AddLinef("pub fn write_tl1%s(&self, buf: &mut Vec<u8>%s) -> basictl::Result<()> {", publicSuffix, natDecl)
	cc.FinishBlock(func() {
		cc.AddLinef("basictl::append(buf, |buf| {")
		cc.FinishBlock(func() {
			if withTag && !wr.OriginTL2() {
				cc.AddLinef("basictl::write_tag(buf, 0x%08x);", wr.TLTag())
			}
			writeOneStringVector(cc, wr, true)
			cc.AddLinef("self::write_tl1%s(self, buf%s)", internalSuffix, natCall)
		}, "})")
	}, "}")
}

func (wr *TypeRWWrapper) generateTL1Functions(cc *codecreator.RustCodeCreator, suffix, typeName string, read, write func()) {
	natDecl := wr.formatNatArgsDecl() + wr.fetcherDecl()
	cc.AddLinef("pub(crate) fn read_tl1%s(value: &mut %s, buf: &mut &[u8]%s) -> basictl::Result<()> {", suffix, typeName, natDecl)
	cc.FinishBlock(func() {
		if wr.OriginTL2() {
			noTL1(cc, wr)
			return
		}
		read()
	}, "}")
	cc.AddEmptyLine()
	cc.AddLinef("pub(crate) fn write_tl1%s(value: &%s, buf: &mut Vec<u8>%s) -> basictl::Result<()> {", suffix, typeName, natDecl)
	cc.FinishBlock(func() {
		if wr.OriginTL2() {
			noTL1(cc, wr)
			return
		}
		write()
	}, "}")
}

func (wr *TypeRWWrapper) fixedSequencePrimitive() string {
	if typ, ok := wr.trw.(*TypeRWPrimitive); ok && wr.tl2FixedWidth() != 0 {
		return typ.goType
	}
	return ""
}

func writeFixedSequence(cc *codecreator.RustCodeCreator, primitive string) {
	if primitive == "u8" {
		cc.AddLinef("buf.extend_from_slice(value);")
		cc.AddLinef("Ok(())")
		return
	}
	cc.AddLinef("basictl::write_fixed_sequence(buf, value, %s::to_le_bytes)", primitive)
}

func generateSequence(wrapper *TypeRWWrapper, element Field, prefixed bool, length string, bytesVersion bool, imports *DirectImports) string {
	cc := codecreator.NewRustCodeCreator()
	addTL1Import(cc)
	natArgs := wrapper.formatNatArgs(nil, element.NatArgs())
	typeName := wrapper.TypeString2(bytesVersion, imports, false, false)
	wrapper.generateTL1Functions(cc, "", typeName, func() {
		count := length
		if prefixed {
			cc.AddLinef("let count = buf.read_u32()?;")
			count = "count"
		}
		if count == "" {
			cc.For("element", "value.iter_mut()", "", func() {
				element.t.TypeReadingCode(cc, bytesVersion, imports, "element", element.Bare(), natArgs, true)
			})
		} else if primitive := element.t.fixedSequencePrimitive(); primitive != "" && element.Bare() {
			cc.AddLinef("basictl::read_fixed_sequence_into(buf, %s as usize, value, %s::from_le_bytes)?;", count, primitive)
		} else {
			cc.AddLinef("basictl::read_vector_into(value, %s as usize, || %s, |element| {", count, rustDefault(element.t))
			cc.FinishBlock(func() {
				element.t.TypeReadingCode(cc, bytesVersion, imports, "element", element.Bare(), natArgs, true)
				cc.AddLinef("Ok(())")
			}, "})?;")
		}
		cc.AddLinef("Ok(())")
	}, func() {
		if prefixed {
			cc.AddLinef("let count = u32::try_from(value.len()).map_err(|_| basictl::Error::InvalidLength)?;")
			cc.AddLinef("basictl::write_tag(buf, count);")
		} else if length != "" {
			cc.If("value.len() != "+length+" as usize", func() {
				cc.AddLinef("return Err(basictl::Error::InvalidLength);")
			})
		}
		if primitive := element.t.fixedSequencePrimitive(); primitive != "" && element.Bare() {
			writeFixedSequence(cc, primitive)
			return
		}
		cc.For("element", "value", "", func() {
			cc.AddLines(strings.Split(element.t.TypeWritingCode("element", element.Bare(), natArgs, true), "\n")...)
		})
		cc.AddLinef("Ok(())")
	})
	return cc.Text()
}
