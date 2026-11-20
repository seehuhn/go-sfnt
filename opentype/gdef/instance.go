// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gdef

import (
	"iter"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/internal/vardelta"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/opentype/internal/offsetpool"
)

// BakeVariationDevices returns a copy of table in which every VariationIndex
// table found in a ligature caret value has been folded into the caret
// coordinate and removed.  For each such table t, resolve(t.OuterIndex,
// t.InnerIndex) supplies the delta added to the caret Coordinate; the Device
// pointer is then cleared.  Plain (non-variation) Device tables are left in
// place unchanged.  If a coordinate with its delta added does not fit into
// 16 bits, BakeVariationDevices returns an error.
//
// The returned table shares immutable structure with table: only the ligature
// caret list is copied, so table itself is not modified.  Rows of caret
// values which are shared within the list, as the reader returns them for
// tables reached through several offsets, are copied once and remain shared
// in the result.  ItemVarStore is carried over unchanged; the caller clears
// it once every reference has been baked out.  This is used when
// instantiating a variable font.
func (table *Table) BakeVariationDevices(resolve func(outer, inner uint16) int64) (*Table, error) {
	return table.mapDevices(func(v funit.Int16, d *device.Table) (funit.Int16, *device.Table, error) {
		if !d.IsVariationIndex() {
			return v, d, nil
		}
		x, err := vardelta.AddInt16(v, resolve(d.OuterIndex, d.InnerIndex))
		return x, nil, err
	})
}

// MapDevices returns a copy of table in which the device table t of every
// ligature caret value has been replaced by fn(t).  fn is called only for
// device tables which are present.  If fn returns nil, the device table is
// removed and the caret coordinate is kept.
//
// The receiver is not modified.  Rows of caret values which are shared
// within the ligature caret list are shared in the result.
func (table *Table) MapDevices(fn func(*device.Table) *device.Table) *Table {
	res, _ := table.mapDevices(func(v funit.Int16, d *device.Table) (funit.Int16, *device.Table, error) {
		return v, fn(d), nil
	})
	return res
}

// Devices yields the device tables of the ligature caret values in table,
// in no particular order.  A row of caret values which is shared within the
// ligature caret list is visited once, so that the work done is
// proportional to the memory used by table.
func (table *Table) Devices() iter.Seq[*device.Table] {
	return func(yield func(*device.Table) bool) {
		if table.LigCaretList == nil {
			return
		}
		seen := map[any]bool{} // keyed by [offsetpool.SliceKey]
		for _, row := range table.LigCaretList.Carets {
			if len(row) == 0 {
				continue
			}
			key := offsetpool.SliceKey(row)
			if seen[key] {
				continue
			}
			seen[key] = true
			for _, cv := range row {
				if cv.Device != nil && !yield(cv.Device) {
					return
				}
			}
		}
	}
}

// mapDevices returns a copy of table in which every caret coordinate v with
// a device table d has been replaced by adjust(v, d).
func (table *Table) mapDevices(adjust func(v funit.Int16, d *device.Table) (funit.Int16, *device.Table, error)) (*Table, error) {
	lc, err := mapLigCaretList(table.LigCaretList, adjust)
	if err != nil {
		return nil, err
	}
	c := *table
	c.LigCaretList = lc
	return &c, nil
}

func mapLigCaretList(lc *LigCaretList, adjust func(funit.Int16, *device.Table) (funit.Int16, *device.Table, error)) (*LigCaretList, error) {
	if lc == nil {
		return nil, nil
	}
	carets := make([][]CaretValue, len(lc.Carets))
	mapped := map[any][]CaretValue{} // keyed by [offsetpool.SliceKey]
	for i, row := range lc.Carets {
		if row == nil {
			continue
		}
		key := offsetpool.SliceKey(row)
		if res, ok := mapped[key]; ok {
			carets[i] = res
			continue
		}
		res := make([]CaretValue, len(row))
		for j, cv := range row {
			res[j] = cv
			if cv.Device != nil {
				x, d, err := adjust(cv.Coordinate, cv.Device)
				if err != nil {
					return nil, err
				}
				res[j].Coordinate, res[j].Device = x, d
			}
		}
		mapped[key] = res
		carets[i] = res
	}
	return &LigCaretList{Cov: lc.Cov, Carets: carets}, nil
}
