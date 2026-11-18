// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"fmt"

	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
)

// Type read by Serde and expression that converts it to the public TL type.
func (wr *TypeRWWrapper) jsonInput(item string) (string, string) {
	switch typ := wr.trw.(type) {
	case *TypeRWPrimitive:
		if typ.canonicalType == "string" {
			return "json::Bytes", item + ".0"
		}
		return typ.goType, item
	case *TypeRWStruct:
		if typ.isUnwrapType() {
			return typ.Fields[0].t.jsonInput(item)
		}
	case *TypeRWBrackets:
		elem, convert := typ.element.t.jsonInput("x")
		if convert == "x" {
			if wr.retainedVector() {
				return "Vec<" + elem + ">", item + ".into()"
			}
			return "Vec<" + elem + ">", item
		}
		return "Vec<" + elem + ">", item + ".into_iter().map(|x| " + convert + ").collect()"
	}
	return "json::Input<" + wr.rustType() + ">", item + ".0"
}

func (wr *TypeRWWrapper) directJSONRoot() bool {
	switch typ := wr.trw.(type) {
	case *TypeRWStruct:
		if typ.isUnwrapType() {
			return false
		}
	case *TypeRWUnion:
	default:
		return false
	}
	return wr.supportsDirectRead(jsonRead, make(map[*TypeRWWrapper]bool))
}

func (wr *TypeRWWrapper) generateDirectJSON(cc *codecreator.RustCodeCreator) {
	if !wr.directJSONRoot() {
		return
	}
	name := wr.goGlobalName
	cc.AddLinef(jsonFeature)
	cc.AddLinef("impl<'de> serde::Deserialize<'de> for json::Input<%s> {", name)
	cc.FinishBlock(func() {
		cc.AddLinef("fn deserialize<D: serde::Deserializer<'de>>(decoder: D) -> std::result::Result<Self, D::Error> {")
		cc.FinishBlock(func() {
			cc.AddLinef("struct Visitor;")
			cc.AddLinef("impl<'de> serde::de::Visitor<'de> for Visitor {")
			cc.FinishBlock(func() {
				cc.AddLinef("type Value = json::Input<self::%s>;", name)
				cc.AddLinef("fn expecting(&self, fmt: &mut std::fmt::Formatter<'_>) -> std::fmt::Result { fmt.write_str(\"TL object\") }")
				cc.AddLinef("fn visit_map<A: serde::de::MapAccess<'de>>(self, mut map: A) -> std::result::Result<Self::Value, A::Error> {")
				cc.FinishBlock(func() {
					cc.AddLinef("use serde::de::Error as _;")
					switch typ := wr.trw.(type) {
					case *TypeRWStruct:
						typ.directJSONFields(cc)
					case *TypeRWUnion:
						typ.directJSONVariant(cc)
					}
				}, "}")
			}, "}")
			cc.AddLinef("decoder.deserialize_map(Visitor)")
		}, "}")
	}, "}")
}

func (typ *TypeRWStruct) directJSONFields(cc *codecreator.RustCodeCreator) {
	cc.AddLinef("let mut value = self::%s::default();", typ.wr.goGlobalName)
	if len(typ.Fields) == 0 {
		cc.If("map.next_key::<&str>()?.is_some()", func() { cc.AddLinef("return Err(A::Error::custom(\"unknown TL field\"));") })
		cc.AddLinef("Ok(json::Input(value))")
		return
	}
	cc.AddLinef("let mut seen = 0u64;")
	cc.AddLinef("while let Some(key) = map.next_key::<&str>()? {")
	cc.FinishBlock(func() {
		cc.AddLinef("let bit = match key {")
		cc.FinishBlock(func() {
			for i, f := range typ.Fields {
				cc.AddLinef("%q => {", f.OriginalName())
				cc.FinishBlock(func() {
					wire, convert := f.t.jsonInput("field")
					cc.AddLinef("let field = map.next_value::<%s>()?;", wire)
					cc.AddLinef("value.%s = %s;", f.goName, f.wrapDecoded(convert))
					cc.AddLinef("1u64 << %d", i)
				}, "}")
			}
			cc.AddLinef("_ => return Err(A::Error::custom(\"unknown TL field\")),")
		}, "};")
		cc.If("seen & bit != 0", func() { cc.AddLinef("return Err(A::Error::custom(\"duplicate TL field\"));") })
		cc.AddLinef("seen |= bit;")
	}, "}")
	// Capture fields implied by the explicit mask before inferring any bits.
	for _, f := range typ.Fields {
		if f.FieldMask() != nil && f.hasPresence() {
			cc.If(typ.jsonFieldBit(f), func() { cc.AddLinef("value.%s.get_or_insert_with(Default::default);", f.goName) })
		}
	}
	for i, f := range typ.Fields {
		if m := f.FieldMask(); m != nil {
			cc.If(fmt.Sprintf("seen & (1u64 << %d) != 0", i), func() { cc.AddLinef("value.%s |= 1 << %d;", typ.Fields[m.FieldIndex()].goName, f.BitNumber()) })
		}
	}
	cc.AddLinef("Ok(json::Input(value))")
}

func (typ *TypeRWUnion) directJSONVariant(cc *codecreator.RustCodeCreator) {
	cc.If("map.next_key::<&str>()? != Some(\"type\")", func() { cc.AddLinef("return Err(A::Error::custom(\"expected constructor\"));") })
	cc.AddLinef("let tag = map.next_value::<&str>()?;")
	cc.If("map.next_key::<&str>()? != Some(\"value\")", func() { cc.AddLinef("return Err(A::Error::custom(\"expected payload\"));") })
	cc.AddLinef("let value = match tag {")
	cc.FinishBlock(func() {
		for _, f := range typ.Fields {
			if f.t.IsTrueType() {
				cc.AddLinef("%s => return Err(A::Error::custom(\"unexpected constructor payload\")),", typ.jsonReadTags(f))
				continue
			}
			wire, convert := f.t.jsonInput("payload")
			cc.AddLinef("%s => { let payload = map.next_value::<%s>()?; self::%s::%s(%s) }", typ.jsonReadTags(f), wire, typ.wr.goGlobalName, f.goName, convert)
		}
		cc.AddLinef("_ => return Err(A::Error::custom(\"unknown constructor\")),")
	}, "};")
	cc.If("map.next_key::<&str>()?.is_some()", func() { cc.AddLinef("return Err(A::Error::custom(\"unexpected union field\"));") })
	cc.AddLinef("Ok(json::Input(value))")
}
