// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import "github.com/VKCOM/tl/internal/tlcodegen/codecreator"

func (trw *TypeRWMaybe) generateJSON(cc *codecreator.RustCodeCreator) {
	wr := trw.wr
	args := wr.formatNatArgs(nil, trw.element.NatArgs())
	wr.generateJSONFunctions(cc, wr.rustType(), func() {
		cc.AddLinef("let (present, payload) = json::read_maybe(input)?;")
		cc.If("!present", func() { cc.AddLinef("return Ok(None);") })
		cc.AddLinef("Ok(Some(%s?))", trw.element.t.decodeJSON("payload", args))
	}, func() {
		cc.AddLinef("let Some(payload) = value else { return Ok(json::write_maybe(false, None)); };")
		cc.AddLinef("let payload = if %s { Some(%s?) } else { None };", trw.element.t.jsonNonEmpty("payload"), trw.element.t.encodeJSON("payload", args))
		cc.AddLinef("Ok(json::write_maybe(true, payload))")
	})
}
