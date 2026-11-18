// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this file,
// You can obtain one at https://mozilla.org/MPL/2.0/.

package genrust

import "github.com/VKCOM/tl/internal/tlcodegen/codecreator"

// When all owned buffers live in required retained vectors, their spare banks
// can stage the whole record. All fields then commit together in one pass.
func (wr *TypeRWWrapper) stageRetainedRead(cc *codecreator.RustCodeCreator, read string) bool {
	if !wr.gen.options.Rust.RetainVectorBuffers {
		return false
	}
	record, ok := wr.trw.(*TypeRWStruct)
	if !ok || record.isAlias() || record.isTypedef() || record.isUnwrapType() {
		return false
	}
	var vectors []string
	for _, f := range record.Fields {
		if f.IsTL2Omitted() || f.IsBit() {
			continue
		}
		if f.t.retainedVector() && !f.hasPresence() {
			vectors = append(vectors, f.goName)
			continue
		}
		_, flag := f.t.trw.(*TypeRWBool)
		if f.recursive || (!flag && f.t.fixedSequencePrimitive() == "") {
			return false
		}
	}
	if len(vectors) == 0 {
		return false
	}
	exchange := func() {
		for _, field := range vectors {
			cc.AddLinef("self.%[1]s.exchange_spare(&mut pending.%[1]s);", field)
		}
	}
	cc.AddLinef("let mut pending = Self::default();")
	exchange()
	cc.AddLinef("let result = %s;", read)
	cc.If("result.is_ok()", func() { cc.AddLinef("std::mem::swap(self, &mut pending);") })
	exchange()
	cc.AddLinef("result?;")
	return true
}
