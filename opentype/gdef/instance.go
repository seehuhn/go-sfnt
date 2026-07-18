// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2026  Jochen Voss <voss@seehuhn.de>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package gdef

import (
	"seehuhn.de/go/postscript/funit"
)

// BakeVariationDevices returns a copy of table in which every VariationIndex
// table found in a ligature caret value has been folded into the caret
// coordinate and removed.  For each such table t, resolve(t.OuterIndex,
// t.InnerIndex) supplies the delta added to the caret Coordinate; the Device
// pointer is then cleared.  Plain (non-variation) Device tables are left in
// place unchanged.
//
// The returned table shares immutable structure with table: only the ligature
// caret list is deep-copied, so table itself is not modified.  ItemVarStore is
// carried over unchanged; the caller clears it once every reference has been
// baked out.  This is used when instantiating a variable font.
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
	for i, row := range lc.Carets {
		if row == nil {
			continue
		}
		carets[i] = make([]CaretValue, len(row))
		for j, cv := range row {
			carets[i][j] = cv
			if cv.Device != nil && cv.Device.IsVariationIndex() {
				carets[i][j].Coordinate += resolve(cv.Device.OuterIndex, cv.Device.InnerIndex)
				carets[i][j].Device = nil
			}
		}
	}
	return &LigCaretList{Cov: lc.Cov, Carets: carets}
}
