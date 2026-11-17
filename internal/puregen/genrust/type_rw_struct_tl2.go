// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"fmt"

	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
)

func (trw *TypeRWStruct) generateTL2(cc *codecreator.RustCodeCreator) {
	name := trw.wr.goGlobalName
	trw.wr.generateTL2Methods(cc)
	if trw.isAlias() {
		field := trw.Fields[0]
		generateTL2Decoder(cc, name, func() {
			cc.AddLinef("Ok(%s(%s?))", name, field.t.readTL2("buf"))
		})
		generateTL2Encoder(cc, name, func() {
			cc.AddLinef("%s", field.t.writeTL2("buf", "&value.0", "omit_empty"))
		})
		return
	}

	index := trw.pureTypeStruct.UnionIndex()
	generateTL2Decoder(cc, name, func() {
		cc.AddLinef("let (index, object) = crate::tl2::read_object(buf)?;")
		cc.If(fmt.Sprintf("index != %d", index), func() {
			cc.AddLinef("return Err(basictl::Error::UnexpectedConstructor(index));")
		})
		cc.AddLinef("decode_tl2_fields(object)")
	})
	cc.AddEmptyLine()
	cc.AddLinef("pub(crate) fn decode_tl2_fields(mut object: crate::tl2::ObjectReader<'_>) -> basictl::Result<%s> {", name)
	cc.FinishBlock(func() {
		cc.AddLinef("let mut value = %s::default();", name)
		for i, field := range trw.Fields {
			if (i+1)%8 == 0 {
				cc.AddLinef("object.next_block();")
			}
			mask := 1 << ((i + 1) % 8)
			if field.IsBit() {
				if !field.IsTL2Omitted() {
					cc.AddLinef("value.%s = object.mask & %d != 0;", field.goName, mask)
				}
				continue
			}
			cc.If(fmt.Sprintf("object.mask & %d != 0", mask), func() {
				if field.IsTL2Omitted() {
					cc.AddLinef("%s?;", field.t.skipTL2())
					return
				}
				expr := field.t.readTL2("&mut object.data") + "?"
				cc.AddLinef("value.%s = %s;", field.goName, field.wrapDecoded(expr))
			})
		}
		cc.AddLinef("Ok(value)")
	}, "}")

	generateTL2Encoder(cc, name, func() {
		if index == 0 && len(trw.Fields) == 1 {
			field := trw.Fields[0]
			if !field.hasPresence() && !field.IsBit() && !field.IsTL2Omitted() {
				value := "&value." + field.goName
				if size := field.t.tl2KnownSize(value, "true"); size != "" {
					cc.AddLinef("let size = %s;", size)
					cc.AddLinef("crate::tl2::write_single_field(buf, size, omit_empty, |buf| %s)", field.t.writeTL2("buf", value, "true"))
					return
				}
			}
		}
		cc.AddLinef("crate::tl2::write_object(buf, %d, omit_empty, |object| {", index)
		cc.FinishBlock(func() {
			for i, field := range trw.Fields {
				if (i+1)%8 == 0 {
					cc.AddLinef("object.next_block();")
				}
				if field.IsTL2Omitted() {
					continue
				}
				mask := 1 << ((i + 1) % 8)
				access := "value." + field.goName
				if field.IsBit() {
					cc.If(access, func() { cc.AddLinef("object.flag(%d);", mask) })
				} else if field.hasPresence() {
					borrow := ifString(field.recursive, "as_deref", "as_ref")
					cc.If(fmt.Sprintf("let Some(field) = %s.%s()", access, borrow), func() {
						omit := ifString(field.MaskTL2Bit() == nil, "true", "false")
						cc.AddLinef("object.field(%d, !%s, |buf| %s)?;", mask, omit, field.t.writeTL2("buf", "field", omit))
					})
				} else {
					cc.AddLinef("object.field(%d, false, |buf| %s)?;", mask, field.t.writeTL2("buf", "&"+access, "true"))
				}
			}
			cc.AddLinef("Ok(())")
		}, "})")
	})
}
