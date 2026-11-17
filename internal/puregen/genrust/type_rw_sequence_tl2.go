// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"fmt"

	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
)

func (wr *TypeRWWrapper) keyAccess(field string) string {
	if alias, ok := wr.trw.(*TypeRWStruct); ok && alias.isAlias() {
		if !alias.isUnwrapType() {
			field += ".0"
		}
		return alias.Fields[0].t.keyAccess(field)
	}
	return field
}

func (wr *TypeRWWrapper) tl2FixedWidth() int {
	switch typ := wr.trw.(type) {
	case *TypeRWPrimitive:
		switch typ.canonicalType {
		case "byte":
			return 1
		case "int32", "uint32", "float32":
			return 4
		case "int64", "uint64", "float64":
			return 8
		}
	case *TypeRWBool:
		return 1
	case *TypeRWStruct:
		if typ.isAlias() {
			return typ.Fields[0].t.tl2FixedWidth()
		}
	}
	return 0
}

// Return the cost of the sequence elements, excluding the two size prefixes.
// Use a sizing pass only when it can avoid walking nested objects.
func tl2SequencePayloadSize(element *TypeRWWrapper, packed bool) string {
	if packed {
		return "value.len().div_ceil(8)"
	}
	if width := element.tl2FixedWidth(); width != 0 {
		return fmt.Sprintf("value.len() * %d", width)
	}
	if element.pureType.CanonicalName() == "string" {
		return "value.iter().map(|item| crate::tl2::bytes_size(item, false)).sum::<usize>()"
	}
	return ""
}

func (wr *TypeRWWrapper) tl2KnownSize(value, omit string) string {
	switch typ := wr.trw.(type) {
	case *TypeRWPrimitive:
		if typ.canonicalType == "string" {
			return fmt.Sprintf("crate::tl2::bytes_size(%s, %s)", value, omit)
		}
	case *TypeRWBrackets:
		bit, isBool := typ.element.t.trw.(*TypeRWBool)
		if tl2SequencePayloadSize(typ.element.t, isBool && bit.isBit) != "" {
			return fmt.Sprintf("%s::size_tl2(%s, %s)", wr.rustModule(), value, omit)
		}
	case *TypeRWStruct:
		if typ.isUnwrapType() {
			return typ.Fields[0].t.tl2KnownSize(value, omit)
		}
	}
	return ""
}

func (wr *TypeRWWrapper) generateTL2Sequence(cc *codecreator.RustCodeCreator, element *TypeRWWrapper, fixed bool, dictKey string) {
	typ := wr.rustType()
	bit, isBool := element.trw.(*TypeRWBool)
	packed := isBool && bit.isBit
	size := tl2SequencePayloadSize(element, packed)
	if size != "" {
		cc.AddEmptyLine()
		cc.AddLinef("fn payload_size_tl2(value: &%s) -> usize {", typ)
		cc.FinishBlock(func() { cc.AddLinef("%s", size) }, "}")
		cc.AddLinef("pub(crate) fn size_tl2(value: &%s, omit_empty: bool) -> usize {", typ)
		cc.FinishBlock(func() {
			cc.AddLinef("crate::tl2::sequence_size(value.len(), payload_size_tl2(value), omit_empty)")
		}, "}")
	}
	wr.generateTL2ReadInto(cc, typ, func() {
		cc.AddLinef("let mut data = crate::tl2::read_sized(buf)?;")
		cc.AddLinef("let count = if data.is_empty() { 0 } else { crate::tl2::read_size(&mut data)? };")
		if primitive := element.fixedSequencePrimitive(); primitive != "" && !fixed {
			cc.AddLinef("basictl::read_fixed_sequence_into(&mut data, count, value, %s::from_le_bytes)", primitive)
			return
		}
		if fixed {
			cc.AddLinef("let count = count.min(value.len());")
			cc.AddLinef("for slot in &mut value[count..] { *slot = %s; }", rustDefault(element))
		} else {
			needed := ifString(packed, "count.div_ceil(8)", "count")
			cc.If(needed+" > data.len()", func() { cc.AddLinef("return Err(basictl::Error::InvalidLength);") })
			cc.AddLinef("value.try_reserve_exact(count.saturating_sub(value.len())).map_err(|_| basictl::Error::InvalidLength)?;")
			if !packed {
				cc.AddLinef("basictl::read_vector_into(value, count, || %s, |slot| {", rustDefault(element))
				cc.FinishBlock(func() {
					element.readTL2Into(cc, "&mut data", "slot")
					cc.AddLinef("Ok(())")
				}, "})")
				return
			}
			cc.AddLinef("value.resize(count, false);")
		}
		if packed {
			cc.AddLinef("let bits = basictl::take(&mut data, count.div_ceil(8))?;")
			cc.For("(i, slot)", "value[..count].iter_mut().enumerate()", "", func() {
				cc.AddLinef("*slot = bits[i / 8] & (1 << (i %% 8)) != 0;")
			})
		} else {
			cc.For("slot", "&mut value[..count]", "", func() { element.readTL2Into(cc, "&mut data", "slot") })
		}
		cc.AddLinef("Ok(())")
	})
	generateTL2Encoder(cc, typ, func() {
		sizeArg := ifString(size != "", "Some(payload_size_tl2(value))", "None")
		cc.AddLinef("crate::tl2::write_sequence(buf, value.len(), %s, omit_empty, |buf| {", sizeArg)
		cc.FinishBlock(func() {
			if primitive := element.fixedSequencePrimitive(); primitive != "" && dictKey == "" {
				writeFixedSequence(cc, primitive)
				return
			}
			if packed {
				cc.For("chunk", "value.chunks(8)", "", func() {
					cc.AddLinef("let byte = chunk.iter().enumerate().fold(0, |byte, (i, flag)| byte | (u8::from(*flag) << i));")
					cc.AddLinef("buf.push(byte);")
				})
			} else {
				elements := "value"
				if dictKey != "" {
					// TL2 dictionaries use the same key order as the Go encoder.
					cc.AddLinef("let mut entries: Vec<_> = value.iter().collect();")
					cc.AddLinef("entries.sort_by(|a, b| a.%[1]s.cmp(&b.%[1]s));", dictKey)
					elements = "entries"
				}
				cc.For("element", elements, "", func() {
					cc.AddLinef("%s?;", element.writeTL2("buf", "element", "false"))
				})
			}
			cc.AddLinef("Ok(())")
		}, "})")
	})
}
