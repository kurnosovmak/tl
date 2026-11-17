// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
)

func (trw *TypeRWStruct) generateJSON(cc *codecreator.RustCodeCreator) {
	wr := trw.wr
	wr.generateJSONMethods(cc)
	wr.generateJSONFunctions(cc, wr.goGlobalName, func() {
		if trw.isAlias() || trw.isTypedef() {
			field := trw.Fields[0]
			args := wr.formatNatArgs(trw.Fields, field.NatArgs())
			expr := field.t.decodeJSON("input", args) + "?"
			cc.AddLinef("Ok(%s(%s))", wr.goGlobalName, field.wrapDecoded(expr))
			return
		}
		trw.decodeJSONFields(cc)
	}, func() {
		if trw.isAlias() || trw.isTypedef() {
			field := trw.Fields[0]
			args := wr.formatNatArgs(trw.Fields, field.NatArgs())
			if field.hasPresence() {
				cc.AddLinef("let field = value.0.as_deref().ok_or(basictl::Error::MissingField(%q))?;", field.OriginalName())
				cc.AddLinef("%s", field.t.encodeJSON("field", args))
			} else {
				cc.AddLinef("%s", field.t.encodeJSON("&value.0", args))
			}
			return
		}
		trw.encodeJSONFields(cc)
	})
}

func (trw *TypeRWStruct) jsonFieldBit(field Field) string {
	mask := trw.wr.formatNatArg(trw.Fields, *field.FieldMask())
	return fmt.Sprintf("(%s & (1 << %d) != 0)", mask, field.BitNumber())
}

func (trw *TypeRWStruct) decodeJSONFields(cc *codecreator.RustCodeCreator) {
	cc.AddLinef("let object = json::object(input)?;")
	var properties []string
	for _, field := range trw.Fields {
		if !field.IsTL2Omitted() {
			properties = append(properties, strconv.Quote(field.OriginalName()))
		}
	}
	cc.AddLinef("json::check_fields(object, &[%s])?;", strings.Join(properties, ", "))
	cc.AddLinef("let mut value = %s::default();", trw.wr.goGlobalName)

	// The mask fields are read first. A JSON property can also set its
	// legacy presence bit or carry an independent TL2 presence flag.
	for _, field := range trw.Fields {
		if field.IsTL2Omitted() {
			continue
		}
		cc.AddLinef("let prop_%s = object.get(%q);", field.goName, field.OriginalName())
		if field.IsBit() {
			cc.AddLinef("value.%s = json::read_bool(prop_%s)?;", field.goName, field.goName)
		} else if !field.IsTypeDependsFromLocalFields() {
			trw.decodeJSONField(cc, field, "prop_"+field.goName)
		}
	}
	for _, field := range trw.Fields {
		if field.IsTL2Omitted() || field.FieldMask() == nil {
			continue
		}
		cc.AddLinef("let initial_%s = %s;", field.goName, trw.jsonFieldBit(field))
	}
	for _, field := range trw.Fields {
		if field.IsTL2Omitted() || field.FieldMask() == nil {
			continue
		}
		present := "prop_" + field.goName + ".is_some()"
		if field.IsBit() {
			present = "value." + field.goName
		}
		cc.If(present, func() {
			for current := field; current.FieldMask() != nil; {
				mask := *current.FieldMask()
				if !mask.IsField() {
					if !trw.wr.HasTL2() {
						cc.If("!"+trw.jsonFieldBit(current), func() {
							cc.AddLinef("return Err(json::error(%q));", "field "+field.OriginalName()+" has a disabled external mask bit")
						})
					}
					break
				}
				ancestor := trw.Fields[mask.FieldIndex()]
				target := "value." + ancestor.goName
				if ancestor.hasPresence() {
					cc.AddLinef("*%s.get_or_insert(0) |= 1 << %d;", target, current.BitNumber())
				} else {
					cc.AddLinef("%s |= 1 << %d;", target, current.BitNumber())
				}
				current = ancestor
			}
		})
	}
	for _, field := range trw.Fields {
		if field.IsTL2Omitted() {
			continue
		}
		prop := "prop_" + field.goName
		if field.IsBit() {
			if field.FieldMask() != nil {
				cc.If(prop+".is_none()", func() {
					cc.AddLinef("value.%s = initial_%s;", field.goName, field.goName)
				})
				if !trw.wr.HasTL2() {
					cc.If(fmt.Sprintf("%s.is_some() && !value.%s && %s", prop, field.goName, trw.jsonFieldBit(field)), func() {
						cc.AddLinef("return Err(json::error(%q));", "field "+field.OriginalName()+" contradicts its mask bit")
					})
				}
			}
			continue
		}
		if field.IsTypeDependsFromLocalFields() {
			trw.decodeJSONField(cc, field, prop)
		}
		if len(field.NatArgs()) != 0 || field.FieldMask() != nil {
			present := "true"
			if field.FieldMask() != nil {
				present = "initial_" + field.goName
			}
			cc.If(fmt.Sprintf("%s.is_none() && %s", prop, present), func() {
				// An inferred mask may have already initialized this nat field.
				if !field.IsTypeDependsFromLocalFields() && len(field.NatArgs()) == 0 {
					if field.hasPresence() {
						if field.recursive {
							cc.AddLinef("value.%s.get_or_insert_with(|| Box::new(%s));", field.goName, rustDefault(field.t))
						} else {
							cc.AddLinef("value.%s.get_or_insert_with(|| %s);", field.goName, rustDefault(field.t))
						}
					}
					return
				}
				trw.assignJSONField(cc, field, "None")
			})
		}
	}
	cc.AddLinef("Ok(value)")
}

func (trw *TypeRWStruct) decodeJSONField(cc *codecreator.RustCodeCreator, field Field, prop string) {
	cc.If(prop+".is_some()", func() { trw.assignJSONField(cc, field, prop) })
}

func (trw *TypeRWStruct) assignJSONField(cc *codecreator.RustCodeCreator, field Field, prop string) {
	args := trw.wr.formatNatArgs(trw.Fields, field.NatArgs())
	expr := field.t.decodeJSON(prop, args) + "?"
	cc.AddLinef("value.%s = %s;", field.goName, field.wrapDecoded(expr))
}

func (trw *TypeRWStruct) encodeJSONFields(cc *codecreator.RustCodeCreator) {
	cc.AddLinef("let mut object = json::Map::new();")
	for _, field := range trw.Fields {
		if field.IsTL2Omitted() {
			continue
		}
		access := "value." + field.goName
		if field.IsBit() {
			present := access
			if field.FieldMask() != nil && !trw.wr.HasTL2() {
				present = trw.jsonFieldBit(field)
			}
			cc.If(present, func() {
				cc.AddLinef("object.insert(%q.into(), true.into());", field.OriginalName())
			})
			continue
		}
		if field.t.IsTrueType() {
			continue
		}
		args := trw.wr.formatNatArgs(trw.Fields, field.NatArgs())
		encode := func(borrowed string, required bool) {
			property := field.t.encodeJSON(borrowed, args) + "?"
			if !required && len(args) != 0 {
				// A TL1 tuple must agree with its nat length even when it is empty.
				cc.AddLinef("let property = %s;", property)
				property = "property"
			}
			insert := func() { cc.AddLinef("object.insert(%q.into(), %s);", field.OriginalName(), property) }
			if required {
				insert()
			} else {
				cc.If(field.t.jsonNonEmpty(borrowed), insert)
			}
		}
		if field.hasPresence() {
			borrow := ifString(field.recursive, "as_deref", "as_ref")
			cc.If(fmt.Sprintf("let Some(field) = %s.%s()", access, borrow), func() {
				encode("field", field.MaskTL2Bit() != nil || field.FieldMask() != nil)
			})
		} else if field.FieldMask() != nil {
			cc.If(trw.jsonFieldBit(field), func() { encode("&"+access, true) })
		} else {
			encode("&"+access, false)
		}
	}
	cc.AddLinef("Ok(json::Value::Object(object))")
}
