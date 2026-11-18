// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"fmt"

	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
)

func (trw *TypeRWBrackets) generateJSON(cc *codecreator.RustCodeCreator) {
	wr := trw.wr
	element := trw.element.t
	args := wr.formatNatArgs(nil, trw.element.NatArgs())
	fixed := !trw.vectorLike && !trw.dynamicSize
	wr.generateJSONFunctions(cc, wr.rustType(), func() {
		cc.AddLinef("let items = json::array(input)?;")
		if fixed {
			cc.If("input.is_none()", func() { cc.AddLinef("return Ok(%s);", rustDefault(wr)) })
			cc.If(fmt.Sprintf("items.len() != %d", trw.size), func() { cc.AddLinef("return Err(basictl::Error::InvalidLength);") })
			cc.AddLinef("let mut value = %s;", rustDefault(wr))
			cc.For("(item, dest)", "items.iter().zip(value.iter_mut())", "", func() {
				cc.AddLinef("*dest = %s?;", element.decodeJSON("Some(item)", args))
			})
			cc.AddLinef("Ok(value)")
		} else {
			trw.checkJSONLength(cc, "items")
			cc.AddLinef("items.iter().map(|item| %s).collect()", element.decodeJSON("Some(item)", args))
		}
	}, func() {
		trw.checkJSONLength(cc, "value")
		cc.AddLinef("let items = value.iter().map(|item| %s).collect::<basictl::Result<_>>()?;", element.encodeJSON("item", args))
		cc.AddLinef("Ok(json::Value::Array(items))")
	})
}

func (trw *TypeRWBrackets) checkJSONLength(cc *codecreator.RustCodeCreator, value string) {
	if trw.dynamicSize {
		cc.If(fmt.Sprintf("mode != JsonMode::Tl2 && %s.len() != nat_%s as usize", value, trw.wr.NatParams()[0]), func() {
			cc.AddLinef("return Err(basictl::Error::InvalidLength);")
		})
	}
}

func (trw *TypeRWDict) generateJSON(cc *codecreator.RustCodeCreator) {
	wr := trw.wr
	key, value := trw.dictKeyField, trw.dictValueField
	elementArgs := wr.formatNatArgs(nil, trw.element.NatArgs())
	keyArgs := trw.structElement.pureTypeStruct.ReplaceUnwrapArgs(0, elementArgs)
	valueArgs := trw.structElement.pureTypeStruct.ReplaceUnwrapArgs(1, elementArgs)
	wr.generateJSONFunctions(cc, wr.rustType(), func() {
		cc.AddLinef("let object = json::object(input)?;")
		cc.AddLinef("let mut entries = Vec::with_capacity(object.len());")
		cc.For("(key, value)", "object", "", func() {
			cc.AddLinef("let key = json::Value::String(key.clone());")
			cc.AddLinef("let mut entry: %s = %s;", trw.element.t.rustType(), rustDefault(trw.element.t))
			cc.AddLinef("entry.%s = %s?;", key.goName, key.t.decodeJSON("Some(&key)", keyArgs))
			cc.AddLinef("entry.%s = %s?;", value.goName, value.t.decodeJSON("Some(value)", valueArgs))
			cc.AddLinef("entries.push(entry);")
		})
		cc.AddLinef("Ok(entries)")
	}, func() {
		cc.AddLinef("let mut object = json::Map::new();")
		cc.For("entry", "value", "", func() {
			cc.AddLinef("let key = json::key(%s?)?;", key.t.encodeJSON("&entry."+key.goName, keyArgs))
			cc.AddLinef("let value = %s?;", value.t.encodeJSON("&entry."+value.goName, valueArgs))
			cc.If("object.insert(key, value).is_some()", func() {
				cc.AddLinef("return Err(json::error(\"duplicate dictionary key\"));")
			})
		})
		cc.AddLinef("Ok(json::Value::Object(object))")
	})
}
