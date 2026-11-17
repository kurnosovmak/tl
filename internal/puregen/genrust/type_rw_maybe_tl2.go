// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import "github.com/VKCOM/tl/internal/tlcodegen/codecreator"

func (trw *TypeRWMaybe) generateTL2(cc *codecreator.RustCodeCreator) {
	typ := trw.wr.rustType()
	generateTL2Decoder(cc, typ, func() {
		cc.AddLinef("let (index, mut object) = crate::tl2::read_object(buf)?;")
		cc.AddLinef("match index {")
		cc.FinishBlock(func() {
			cc.AddLinef("0 => Ok(None),")
			cc.AddLinef("1 if object.mask & 2 != 0 => Ok(Some(%s?)),", trw.element.t.readTL2("&mut object.data"))
			cc.AddLinef("1 => Ok(Some(%s)),", rustDefault(trw.element.t))
			cc.AddLinef("index => Err(basictl::Error::UnexpectedConstructor(index)),")
		}, "}")
	})
	generateTL2Encoder(cc, typ, func() {
		cc.AddLinef("crate::tl2::write_object(buf, usize::from(value.is_some()), omit_empty, |object| {")
		cc.FinishBlock(func() {
			cc.If("let Some(element) = value", func() {
				cc.AddLinef("object.field(2, false, |buf| %s)?;", trw.element.t.writeTL2("buf", "element", "true"))
			})
			cc.AddLinef("Ok(())")
		}, "})")
	})
}
