// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"fmt"

	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
)

func (wr *TypeRWWrapper) rustModule() string {
	return "crate::types::" + wr.goGlobalName
}

func (wr *TypeRWWrapper) rustType() string {
	return wr.TypeString2(false, &DirectImports{}, false, false)
}

func (wr *TypeRWWrapper) readTL2(buf string) string {
	switch typ := wr.trw.(type) {
	case *TypeRWPrimitive:
		switch typ.canonicalType {
		case "string":
			return "crate::tl2::read_sized(" + buf + ").map(<[u8]>::to_vec)"
		case "__function", "__function_result":
			return "Err(basictl::Error::UnsupportedFunction)"
		default:
			return fmt.Sprintf("(%s).read_%s()", buf, typ.goType)
		}
	case *TypeRWBool:
		return fmt.Sprintf("(%s).read_u8().map(|byte| byte != 0)", buf)
	case *TypeRWStruct:
		if typ.isUnwrapType() {
			return typ.Fields[0].t.readTL2(buf)
		}
	}
	return fmt.Sprintf("%s::decode_tl2(%s)", wr.rustModule(), buf)
}

// target is a mutable reference to the destination field.
func (wr *TypeRWWrapper) readTL2Into(cc *codecreator.RustCodeCreator, buf, target string) {
	switch typ := wr.trw.(type) {
	case *TypeRWStruct:
		if typ.isUnwrapType() {
			typ.Fields[0].t.readTL2Into(cc, buf, target)
			return
		}
	case *TypeRWPrimitive:
		if typ.canonicalType == "string" {
			cc.AddLinef("basictl::replace_bytes(%s, crate::tl2::read_sized(%s)?);", target, buf)
			return
		}
		cc.AddLinef("*(%s) = %s?;", target, wr.readTL2(buf))
		return
	case *TypeRWBool:
		cc.AddLinef("*(%s) = %s?;", target, wr.readTL2(buf))
		return
	}
	cc.AddLinef("%s::decode_tl2_into(%s, %s)?;", wr.rustModule(), buf, target)
}

func (wr *TypeRWWrapper) generateTL2ReadInto(cc *codecreator.RustCodeCreator, typ string, read func()) {
	generateTL2Decoder(cc, typ, func() {
		initial := "Default::default()"
		if _, sequence := wr.trw.(*TypeRWBrackets); sequence {
			initial = rustDefault(wr)
		}
		cc.AddLinef("let mut value = %s;", initial)
		cc.AddLinef("decode_tl2_into(buf, &mut value)?;")
		cc.AddLinef("Ok(value)")
	})
	cc.AddLinef("pub(crate) fn decode_tl2_into(buf: &mut &[u8], value: &mut %s) -> basictl::Result<()> {", typ)
	cc.FinishBlock(read, "}")
}

func (wr *TypeRWWrapper) writeTL2(buf, value, omit string) string {
	switch typ := wr.trw.(type) {
	case *TypeRWPrimitive:
		switch typ.canonicalType {
		case "string":
			return fmt.Sprintf("crate::tl2::write_bytes(%s, %s, %s)", buf, value, omit)
		case "__function", "__function_result":
			return "Err(basictl::Error::UnsupportedFunction)"
		default:
			return fmt.Sprintf("{ if !%s || *(%s) != %s::default() { %s.extend_from_slice(&(%s).to_le_bytes()); } Ok::<(), basictl::Error>(()) }",
				omit, value, typ.goType, buf, value)
		}
	case *TypeRWBool:
		return fmt.Sprintf("{ if !%s || *(%s) { %s.push(u8::from(*(%s))); } Ok::<(), basictl::Error>(()) }", omit, value, buf, value)
	case *TypeRWStruct:
		if typ.isUnwrapType() {
			return typ.Fields[0].t.writeTL2(buf, value, omit)
		}
	}
	return fmt.Sprintf("%s::encode_tl2(%s, %s, %s)", wr.rustModule(), value, buf, omit)
}

func (wr *TypeRWWrapper) skipTL2() string {
	switch typ := wr.trw.(type) {
	case *TypeRWPrimitive, *TypeRWBool:
		return wr.readTL2("&mut object.data")
	case *TypeRWStruct:
		if typ.isAlias() {
			return typ.Fields[0].t.skipTL2()
		}
	}
	return "crate::tl2::read_sized(&mut object.data)"
}

func (wr *TypeRWWrapper) generateTL2() string {
	cc := codecreator.NewRustCodeCreator()
	switch typ := wr.trw.(type) {
	case *TypeRWStruct:
		typ.generateTL2(cc)
	case *TypeRWUnion:
		typ.generateTL2(cc)
	case *TypeRWBrackets:
		wr.generateTL2Sequence(cc, typ.element.t, !typ.vectorLike && !typ.dynamicSize, "")
	case *TypeRWDict:
		key := typ.dictKeyField
		wr.generateTL2Sequence(cc, typ.element.t, false, key.t.keyAccess(key.goName))
	case *TypeRWMaybe:
		typ.generateTL2(cc)
	}
	return cc.Text()
}

func (wr *TypeRWWrapper) generateTL2Methods(cc *codecreator.RustCodeCreator) {
	cc.AddEmptyLine()
	cc.AddLinef("impl %s {", wr.goGlobalName)
	cc.FinishBlock(func() {
		cc.AddLinef("pub fn read_tl2(&mut self, buf: &mut &[u8]) -> basictl::Result<()> {")
		cc.FinishBlock(func() {
			cc.AddLinef("let mut input = *buf;")
			if !reuseVectorTL2(cc, wr) && !wr.stageRetainedRead(cc, "decode_tl2_into(&mut input, &mut pending)") {
				if wr.preflightRoot() {
					cc.AddLinef("let mut probe = input;")
					cc.If("preflight_tl2(&mut probe, self).is_some()", func() {
						cc.AddLinef("decode_tl2_into(&mut input, self)?;")
						cc.AddLinef("*buf = input;")
						cc.AddLinef("return Ok(());")
					})
				}
				cc.AddLinef("*self = decode_tl2(&mut input)?;")
			}
			cc.AddLinef("*buf = input;")
			cc.AddLinef("Ok(())")
		}, "}")
		cc.AddLinef("/// Reuse this value directly. On error the cursor is unchanged and the value may be partially updated.")
		cc.AddLinef("pub fn read_tl2_in_place(&mut self, buf: &mut &[u8]) -> basictl::Result<()> {")
		cc.FinishBlock(func() {
			cc.AddLinef("let mut input = *buf;")
			cc.AddLinef("decode_tl2_into(&mut input, self)?;")
			cc.AddLinef("*buf = input;")
			cc.AddLinef("Ok(())")
		}, "}")
		cc.AddLinef("/// Reuse scratch storage and replace this value only after a successful read.")
		cc.AddLinef("pub fn read_tl2_with_scratch(&mut self, buf: &mut &[u8], scratch: &mut Self) -> basictl::Result<()> {")
		cc.FinishBlock(func() {
			cc.AddLinef("scratch.read_tl2_in_place(buf)?;")
			cc.AddLinef("std::mem::swap(self, scratch);")
			cc.AddLinef("Ok(())")
		}, "}")
		cc.AddLinef("pub fn write_tl2(&self, buf: &mut Vec<u8>) -> basictl::Result<()> {")
		cc.FinishBlock(func() {
			writeOneStringVector(cc, wr, false)
			cc.AddLinef("basictl::append(buf, |buf| encode_tl2(self, buf, false))")
		}, "}")
	}, "}")
}

func generateTL2Decoder(cc *codecreator.RustCodeCreator, typ string, read func()) {
	cc.AddEmptyLine()
	cc.AddLinef("pub(crate) fn decode_tl2(buf: &mut &[u8]) -> basictl::Result<%s> {", typ)
	cc.FinishBlock(read, "}")
}

func generateTL2Encoder(cc *codecreator.RustCodeCreator, typ string, write func()) {
	cc.AddEmptyLine()
	cc.AddLinef("pub(crate) fn encode_tl2(value: &%s, buf: &mut Vec<u8>, omit_empty: bool) -> basictl::Result<()> {", typ)
	cc.FinishBlock(write, "}")
}
