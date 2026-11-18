// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"

	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
)

// Stream common field types without building a serde_json::Value per field.
func (wr *TypeRWWrapper) streamJSON(value string, args []string) string {
	switch typ := wr.trw.(type) {
	case *TypeRWPrimitive:
		switch typ.canonicalType {
		case "string":
			return fmt.Sprintf("crate::json_vector::write_bytes(buf, %s)", value)
		case "int32", "uint32", "int64", "uint64", "byte":
			return fmt.Sprintf("json::serialize(%s, buf)", value)
		}
	case *TypeRWBool:
		return fmt.Sprintf("json::serialize(%s, buf)", value)
	case *TypeRWStruct, *TypeRWUnion:
		if alias, ok := typ.(*TypeRWStruct); ok && alias.isUnwrapType() {
			return alias.Fields[0].t.streamJSON(value, alias.pureTypeStruct.ReplaceUnwrapArgs(0, args))
		}
		return fmt.Sprintf("(%s).write_json(buf, mode%s%s)", value, joinWithCommas(args), wr.fetcherCall())
	case *TypeRWBrackets:
		if typ.vectorLike && len(typ.element.NatArgs()) == 0 {
			return fmt.Sprintf("json::write_array(buf, %s, |buf, item| %s)", value, typ.element.t.streamJSON("item", nil))
		}
	}
	return fmt.Sprintf("json::serialize(&(%s?), buf)", wr.encodeJSON(value, args))
}

func (wr *TypeRWWrapper) generateJSONStream(cc *codecreator.RustCodeCreator) bool {
	switch typ := wr.trw.(type) {
	case *TypeRWStruct:
		if typ.isAlias() || typ.isTypedef() {
			return false
		}
		// Dependent fields must be validated even when omitted from JSON.
		for _, f := range typ.Fields {
			if len(f.NatArgs()) != 0 {
				return false
			}
		}
		typ.streamJSONObject(cc)
	case *TypeRWUnion:
		typ.streamJSONUnion(cc)
	default:
		return false
	}
	return true
}

func (typ *TypeRWStruct) streamJSONObject(cc *codecreator.RustCodeCreator) {
	cc.AddLinef("let value = self;")
	cc.AddLinef("basictl::append(buf, |buf| {")
	cc.FinishBlock(func() {
		cc.AddLinef("buf.push(b'{');")
		cc.AddLinef("let start = buf.len();")
		// Preserve the key order of serde_json::Map.
		fields := slices.Clone(typ.Fields)
		slices.SortFunc(fields, func(a, b Field) int { return cmp.Compare(a.OriginalName(), b.OriginalName()) })
		typ.visitJSONFields(cc, fields, func(field Field, borrowed string, args []string, required bool) {
			emit := func() {
				cc.AddLinef("json::field_name(buf, start, %q.as_bytes());", strconv.Quote(field.OriginalName())+":")
				if field.IsBit() {
					cc.AddLinef("buf.extend_from_slice(b\"true\");")
				} else {
					cc.AddLinef("%s?;", field.t.streamJSON(borrowed, args))
				}
			}
			if required {
				emit()
			} else {
				cc.If(field.t.jsonNonEmpty(borrowed), emit)
			}
		})
		cc.AddLinef("buf.push(b'}');")
		cc.AddLinef("Ok(())")
	}, "})")
}

func (typ *TypeRWUnion) streamJSONUnion(cc *codecreator.RustCodeCreator) {
	args := typ.wr.formatNatArgs(nil, typ.ElementNatArgs())
	cc.AddLinef("basictl::append(buf, |buf| match self {")
	cc.FinishBlock(func() {
		for _, variant := range typ.Fields {
			pattern := ifString(variant.t.IsTrueType(), "", "(payload)")
			cc.AddLinef("Self::%s%s => {", variant.goName, pattern)
			cc.FinishBlock(func() {
				cc.AddLinef("let constructor = %s;", typ.jsonConstructor(variant))
				if typ.IsEnum {
					cc.AddLinef("json::serialize(constructor, buf)")
					return
				}
				cc.AddLinef("buf.extend_from_slice(br#\"{\"type\":\"#);")
				cc.AddLinef("json::serialize(constructor, buf)?;")
				if !variant.t.IsTrueType() {
					payload := ifString(variant.recursive, "payload.as_ref()", "payload")
					cc.If(variant.t.jsonNonEmpty(payload), func() {
						cc.AddLinef("buf.extend_from_slice(br#\",\"value\":\"#);")
						cc.AddLinef("%s?;", variant.t.streamJSON(payload, args))
					})
				}
				cc.AddLinef("buf.push(b'}');")
				cc.AddLinef("Ok(())")
			}, "}")
		}
	}, "})")
}
