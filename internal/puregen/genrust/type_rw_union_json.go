// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
)

func (trw *TypeRWUnion) generateJSON(cc *codecreator.RustCodeCreator) {
	wr, name := trw.wr, trw.wr.goGlobalName
	args := wr.formatNatArgs(nil, trw.ElementNatArgs())
	wr.generateJSONMethods(cc)
	wr.generateJSONFunctions(cc, name, func() {
		cc.AddLinef("let (tag, payload) = json::read_variant(input)?;")
		cc.AddLinef("match tag {")
		cc.FinishBlock(func() {
			for _, variant := range trw.Fields {
				tlName := variant.t.TLName().String()
				if !wr.OriginTL2() {
					legacy := fmt.Sprintf("#%08x", variant.t.TLTag())
					cc.AddLinef("%q | %q if mode == JsonMode::Legacy => {", tlName+legacy, legacy)
					cc.FinishBlock(func() { trw.decodeJSONVariant(cc, variant, args) }, "}")
				}
				cc.AddLinef("%s => {", trw.jsonReadTags(variant))
				cc.FinishBlock(func() { trw.decodeJSONVariant(cc, variant, args) }, "}")
			}
			cc.AddLinef("_ => Err(json::error(format!(\"unknown union variant {tag:?}\"))),")
		}, "}")
	}, func() {
		cc.AddLinef("match value {")
		cc.FinishBlock(func() {
			for _, variant := range trw.Fields {
				pattern := ""
				if !variant.t.IsTrueType() {
					pattern = "(payload)"
				}
				cc.AddLinef("%s::%s%s => {", name, variant.goName, pattern)
				cc.FinishBlock(func() {
					cc.AddLinef("let constructor = %s;", trw.jsonConstructor(variant))
					switch {
					case trw.IsEnum:
						cc.AddLinef("Ok(constructor.into())")
					case variant.t.IsTrueType():
						cc.AddLinef("Ok(json::write_variant(constructor, None))")
					default:
						payload := ifString(variant.recursive, "payload.as_ref()", "payload")
						cc.AddLinef("let payload = if %s { Some(%s?) } else { None };", variant.t.jsonNonEmpty(payload), variant.t.encodeJSON(payload, args))
						cc.AddLinef("Ok(json::write_variant(constructor, payload))")
					}
				}, "}")
			}
		}, "}")
	})
}

func (trw *TypeRWUnion) jsonReadTags(variant Variant) string {
	tlName := variant.t.TLName().String()
	var tags []string
	if trw.wr.HasTL2() || strings.Contains(tlName, "__") {
		tags = append(tags, strconv.Quote(variant.variantName))
	}
	if !strings.Contains(tlName, "__") && !slices.Contains(tags, strconv.Quote(tlName)) {
		tags = append(tags, strconv.Quote(tlName))
	}
	return strings.Join(tags, " | ")
}

func (trw *TypeRWUnion) decodeJSONVariant(cc *codecreator.RustCodeCreator, variant Variant, args []string) {
	name := trw.wr.goGlobalName + "::" + variant.goName
	if variant.t.IsTrueType() {
		cc.AddLinef("Ok(%s)", name)
		return
	}
	payload := variant.t.decodeJSON("payload", args) + "?"
	if variant.recursive {
		payload = "Box::new(" + payload + ")"
	}
	cc.AddLinef("Ok(%s(%s))", name, payload)
}

func (trw *TypeRWUnion) jsonConstructor(variant Variant) string {
	wr := trw.wr
	tlName := variant.t.TLName().String()
	constructor := strconv.Quote(tlName)
	if strings.Contains(tlName, "__") {
		constructor = strconv.Quote(variant.variantName)
	} else {
		if !wr.OriginTL2() {
			legacy := fmt.Sprintf("%s#%08x", tlName, variant.t.TLTag())
			constructor = fmt.Sprintf("if mode == JsonMode::Legacy { %q } else { %s }", legacy, constructor)
		}
		if wr.HasTL2() {
			constructor = fmt.Sprintf("if mode == JsonMode::Tl2 { %q } else { %s }", variant.variantName, constructor)
		}
	}
	return constructor
}
