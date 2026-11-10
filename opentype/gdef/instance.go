// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gdef

import (
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/opentype/internal/offsetpool"
)

// BakeVariationDevices returns a copy of table in which every VariationIndex
// table found in a ligature caret value has been folded into the caret
// coordinate and removed.  For each such table t, resolve(t.OuterIndex,
// t.InnerIndex) supplies the delta added to the caret Coordinate; the Device
// pointer is then cleared.  Plain (non-variation) Device tables are left in
// place unchanged.
//
// The returned table shares immutable structure with table: only the ligature
// caret list is copied, so table itself is not modified.  Rows of caret
// values which are shared within the list, as the reader returns them for
// tables reached through several offsets, are copied once and remain shared
// in the result.  ItemVarStore is carried over unchanged; the caller clears
// it once every reference has been baked out.  This is used when
// instantiating a variable font.
func (table *Table) BakeVariationDevices(resolve func(outer, inner uint16) funit.Int16) *Table {
	c := *table
	c.LigCaretList = bakeLigCaretList(table.LigCaretList, resolve)
	return &c
}

func bakeLigCaretList(lc *LigCaretList, resolve func(uint16, uint16) funit.Int16) *LigCaretList {
	if lc == nil {
		return nil
	}
	carets := make([][]CaretValue, len(lc.Carets))
	baked := map[any][]CaretValue{} // keyed by [offsetpool.SliceKey]
	for i, row := range lc.Carets {
		if row == nil {
			continue
		}
		key := offsetpool.SliceKey(row)
		if res, ok := baked[key]; ok {
			carets[i] = res
			continue
		}
		res := make([]CaretValue, len(row))
		for j, cv := range row {
			res[j] = cv
			if cv.Device != nil && cv.Device.IsVariationIndex() {
				res[j].Coordinate += resolve(cv.Device.OuterIndex, cv.Device.InnerIndex)
				res[j].Device = nil
			}
		}
		baked[key] = res
		carets[i] = res
	}
	return &LigCaretList{Cov: lc.Cov, Carets: carets}
}
