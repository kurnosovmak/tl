// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import (
	"fmt"

	"github.com/VKCOM/tl/internal/tlcodegen/codecreator"
)

type jsonVector struct {
	field  Field
	prefix string
	read   string
	write  string
}

func (wr *TypeRWWrapper) jsonVector() (jsonVector, bool) {
	trw, ok := wr.trw.(*TypeRWStruct)
	if !ok {
		return jsonVector{}, false
	}
	vector := trw.independentVector()
	field, element := vector.field, vector.element
	if element == nil {
		return jsonVector{}, false
	}
	path := jsonVector{field: field, prefix: fmt.Sprintf("{%q:[", field.OriginalName())}
	switch element.goType {
	case "Vec<u8>":
		path.read = "read_string"
		path.write = "crate::json_vector::write_bytes(buf, element)"
	case "u8", "u32", "u64", "i32", "i64":
		path.read = "read_integers"
		path.write = ""
	default:
		return jsonVector{}, false
	}
	return path, true
}

func (path jsonVector) generateRead(cc *codecreator.RustCodeCreator) {
	cc.If(fmt.Sprintf("crate::json_vector::%s(input, %q.as_bytes(), &mut self.%s)", path.read, path.prefix, path.field.goName), func() {
		cc.AddLinef("return Ok(());")
	})
}

func (path jsonVector) generateWrite(cc *codecreator.RustCodeCreator) {
	if path.write == "" {
		cc.AddLinef("crate::json_vector::write_integers(buf, %q.as_bytes(), &self.%s)", path.prefix, path.field.goName)
		return
	}
	cc.AddLinef("crate::json_vector::write_object(buf, %q.as_bytes(), &self.%s, |buf, element| %s)", path.prefix, path.field.goName, path.write)
}
