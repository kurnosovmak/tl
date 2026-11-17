// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"fmt"

	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
)

// On a repeated layout, verify the wire and the destination's capacities before
// updating it. On a new layout the transactional reader builds a fresh value.
func (wr *TypeRWWrapper) preflightRoot() bool {
	if typ, ok := wr.trw.(*TypeRWStruct); ok && (typ.isUnwrapType() || len(typ.Fields) == 0) {
		return false
	}
	return wr.supportsDirectRead(binaryRead, make(map[*TypeRWWrapper]bool))
}

func (wr *TypeRWWrapper) preflightField(cc *codecreator.RustCodeCreator, buf, value string, bare, tl2 bool) {
	_, union := wr.trw.(*TypeRWUnion)
	_, primitive := wr.trw.(*TypeRWPrimitive)
	if !tl2 && !bare && !union && !primitive {
		cc.AddLinef("(%s).read_exact_tag(0x%08x).ok()?;", buf, wr.TLTag())
	}
	switch typ := wr.trw.(type) {
	case *TypeRWPrimitive:
		if typ.canonicalType == "string" {
			reader := ifString(tl2, "crate::tl2::read_sized", "basictl::read_string_slice")
			cc.AddLinef("%s(%s).ok()?;", reader, buf)
		} else {
			cc.AddLinef("basictl::take(%s, %d).ok()?;", buf, wr.tl2FixedWidth())
		}
		return
	case *TypeRWStruct:
		if typ.isUnwrapType() {
			f := typ.Fields[0]
			f.t.preflightField(cc, buf, value, f.Bare(), tl2)
			return
		}
	}
	cc.AddLinef("%s::preflight_tl%s(%s, %s)?;", wr.rustModule(), ifString(tl2, "2", "1"), buf, value)
}

func (wr *TypeRWWrapper) generatePreflight(tl2 bool) string {
	if !wr.supportsDirectRead(binaryRead, make(map[*TypeRWWrapper]bool)) {
		return ""
	}
	switch typ := wr.trw.(type) {
	case *TypeRWPrimitive:
		return ""
	case *TypeRWStruct:
		if typ.isUnwrapType() {
			return ""
		}
	}
	cc := codecreator.NewRustCodeCreator()
	cc.AddEmptyLine()
	cc.AddLinef("pub(crate) fn preflight_tl%s(buf: &mut &[u8], value: &%s) -> Option<()> {", ifString(tl2, "2", "1"), wr.rustType())
	cc.FinishBlock(func() {
		switch typ := wr.trw.(type) {
		case *TypeRWBrackets:
			if tl2 {
				cc.AddLinef("let mut data = crate::tl2::read_sized(buf).ok()?;")
				cc.If("data.is_empty()", func() { cc.AddLinef("return Some(());") })
				cc.AddLinef("let buf = &mut data;")
				cc.AddLinef("let count = crate::tl2::read_size(buf).ok()?;")
			} else {
				cc.AddLinef("let count = buf.read_u32().ok()? as usize;")
			}
			if width := typ.element.t.tl2FixedWidth(); typ.element.t.fixedSequencePrimitive() != "" && (tl2 || typ.element.Bare()) {
				cc.If("count > value.capacity()", func() { cc.AddLinef("return None;") })
				cc.AddLinef("basictl::take(buf, count.checked_mul(%d)?).ok()?;", width)
			} else {
				cc.For("slot", "value.get(..count)?", "", func() { typ.element.t.preflightField(cc, "buf", "slot", typ.element.Bare(), tl2) })
			}
		case *TypeRWStruct:
			if tl2 {
				cc.AddLinef("let (index, object) = crate::tl2::read_object(buf).ok()?;")
				cc.If(fmt.Sprintf("index != %d", typ.pureTypeStruct.UnionIndex()), func() { cc.AddLinef("return None;") })
				cc.AddLinef("return preflight_fields(object, value);")
				return
			}
			typ.preflightFields(cc, false)
		case *TypeRWUnion:
			tag := "buf.read_u32().ok()?"
			if tl2 {
				cc.AddLinef("let (index, object) = crate::tl2::read_object(buf).ok()?;")
				tag = "index"
			}
			cc.AddLinef("match (%s, value) {", tag)
			cc.FinishBlock(func() {
				for index, f := range typ.Fields {
					tag := fmt.Sprintf("0x%08x", f.t.TLTag())
					if tl2 {
						tag = fmt.Sprint(index)
					}
					if f.t.IsTrueType() {
						cc.AddLinef("(%s, %s::%s) => {},", tag, wr.goGlobalName, f.goName)
						continue
					}
					probe := ifString(tl2, "preflight_fields(object, slot)", "preflight_tl1(buf, slot)")
					cc.AddLinef("(%s, %s::%s(slot)) => %s::%s?,", tag, wr.goGlobalName, f.goName, f.t.rustModule(), probe)
				}
				cc.AddLinef("_ => return None,")
			}, "}")
		}
		cc.AddLinef("Some(())")
	}, "}")
	if typ, ok := wr.trw.(*TypeRWStruct); ok && tl2 {
		cc.AddLinef("pub(crate) fn preflight_fields(mut object: crate::tl2::ObjectReader<'_>, value: &%s) -> Option<()> {", wr.rustType())
		cc.FinishBlock(func() { typ.preflightFields(cc, true); cc.AddLinef("Some(())") }, "}")
	}
	return cc.Text()
}

func (typ *TypeRWStruct) preflightFields(cc *codecreator.RustCodeCreator, tl2 bool) {
	masks := map[int]bool{}
	for _, f := range typ.Fields {
		if m := f.FieldMask(); m != nil {
			masks[m.FieldIndex()] = true
		}
	}
	for i, f := range typ.Fields {
		buf := "buf"
		if tl2 {
			buf = "&mut object.data"
			if (i+1)%8 == 0 {
				cc.AddLinef("object.next_block();")
			}
		}
		if !tl2 && masks[i] {
			if _, primitive := f.t.trw.(*TypeRWPrimitive); !f.Bare() && !primitive {
				cc.AddLinef("buf.read_exact_tag(0x%08x).ok()?;", f.t.TLTag())
			}
			cc.AddLinef("let mask_%d = buf.read_u32().ok()?;", i)
			continue
		}
		check := func() {
			slot := "&value." + f.goName
			if f.hasPresence() {
				slot = "value." + f.goName + ".as_ref()?"
			}
			f.t.preflightField(cc, buf, slot, f.Bare(), tl2)
		}
		if tl2 {
			cc.If(fmt.Sprintf("object.mask & %d != 0", 1<<((i+1)%8)), check)
		} else if m := f.FieldMask(); m != nil {
			cc.If(fmt.Sprintf("mask_%d & (1 << %d) != 0", m.FieldIndex(), f.BitNumber()), check)
		} else {
			check()
		}
	}
}
