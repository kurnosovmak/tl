// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"fmt"
	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
)

func (trw *TypeRWUnion) generateTL2(cc *codecreator.RustCodeCreator) {
	name := trw.wr.goGlobalName
	trw.wr.generateTL2Methods(cc)
	trw.wr.generateTL2ReadInto(cc, name, func() {
		cc.AddLinef("let (index, object) = crate::tl2::read_object(buf)?;")
		cc.AddLinef("match index {")
		cc.FinishBlock(func() {
			for index, field := range trw.Fields {
				variant := name + "::" + field.goName
				cc.AddLinef("%d => {", index)
				cc.FinishBlock(func() {
					if field.t.IsTrueType() {
						cc.AddLinef("*value = %s;", variant)
					} else {
						cc.If(fmt.Sprintf("!matches!(value, %s(_))", variant), func() {
							cc.AddLinef("*value = %s(Default::default());", variant)
						})
						cc.AddLinef("let %s(payload) = value else { unreachable!() };", variant)
						cc.AddLinef("%s::decode_tl2_fields(object, payload)?;", field.t.rustModule())
					}
					cc.AddLinef("Ok(())")
				}, "}")
			}
			cc.AddLinef("index => Err(basictl::Error::UnexpectedConstructor(index)),")
		}, "}")
	})
	generateTL2Encoder(cc, name, func() {
		cc.AddLinef("match value {")
		cc.FinishBlock(func() {
			for index, field := range trw.Fields {
				if field.t.IsTrueType() {
					cc.AddLinef("%s::%s => crate::tl2::write_object(buf, %d, omit_empty, |_| Ok(())),", name, field.goName, index)
					continue
				}
				cc.AddLinef("%s::%s(payload) => %s,", name, field.goName, field.t.writeTL2("buf", "payload", "omit_empty"))
			}
		}, "}")
	})
}
