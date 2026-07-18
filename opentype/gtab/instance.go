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

package gtab

import (
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/opentype/markarray"
)

// BakeVariationDevices returns a copy of ll in which every VariationIndex
// table found in a GPOS value record or anchor has been folded into its paired
// base coordinate and removed.  For each such table t, resolve(t.OuterIndex,
// t.InnerIndex) supplies the delta added to the paired coordinate
// (XPlacement/YPlacement/XAdvance/YAdvance for value records, X/Y for anchors);
// the device pointer is then cleared.  Plain (non-variation) Device tables are
// left in place unchanged.
//
// The returned LookupList shares immutable structure with ll: only the value
// records, anchors, subtables and lookups that carry a device table are
// deep-copied, so ll itself is not modified.  This is used when instantiating a
// variable font.
func (ll LookupList) BakeVariationDevices(resolve func(outer, inner uint16) funit.Int16) LookupList {
	out := make(LookupList, len(ll))
	for i, lookup := range ll {
		out[i] = &LookupTable{
			Meta:      lookup.Meta,
			Subtables: bakeSubtables(lookup.Subtables, resolve),
		}
	}
	return out
}

func bakeSubtables(ss []Subtable, resolve func(uint16, uint16) funit.Int16) []Subtable {
	out := make([]Subtable, len(ss))
	for i, st := range ss {
		out[i] = bakeSubtable(st, resolve)
	}
	return out
}

// bakeSubtable returns a copy of st with its VariationIndex device tables
// baked in.  Subtable types that cannot carry device tables (contextual
// lookups, GSUB subtables) are returned unchanged.
func bakeSubtable(st Subtable, resolve func(uint16, uint16) funit.Int16) Subtable {
	switch l := st.(type) {
	case *Gpos1_1:
		return &Gpos1_1{Cov: l.Cov, Adjust: bakeValueRecord(l.Adjust, resolve)}
	case *Gpos1_2:
		adjust := make([]*GposValueRecord, len(l.Adjust))
		for i, vr := range l.Adjust {
			adjust[i] = bakeValueRecord(vr, resolve)
		}
		return &Gpos1_2{Cov: l.Cov, Adjust: adjust}
	case Gpos2_1:
		out := make(Gpos2_1, len(l))
		for pair, adj := range l {
			out[pair] = bakePairAdjust(adj, resolve)
		}
		return out
	case *Gpos2_2:
		adjust := make([][]*PairAdjust, len(l.Adjust))
		for i, row := range l.Adjust {
			adjust[i] = make([]*PairAdjust, len(row))
			for j, adj := range row {
				adjust[i][j] = bakePairAdjust(adj, resolve)
			}
		}
		return &Gpos2_2{Cov: l.Cov, Class1: l.Class1, Class2: l.Class2, Adjust: adjust}
	case *Gpos3_1:
		records := make([]EntryExitRecord, len(l.Records))
		for i, rec := range l.Records {
			records[i] = EntryExitRecord{
				Entry: bakeAnchor(rec.Entry, resolve),
				Exit:  bakeAnchor(rec.Exit, resolve),
			}
		}
		return &Gpos3_1{Cov: l.Cov, Records: records}
	case *Gpos4_1:
		return &Gpos4_1{
			MarkCov:   l.MarkCov,
			BaseCov:   l.BaseCov,
			MarkArray: bakeMarkArray(l.MarkArray, resolve),
			BaseArray: bakeAnchorGrid(l.BaseArray, resolve),
		}
	case *Gpos5_1:
		ligArray := make([][][]*anchor.Table, len(l.LigArray))
		for i, comp := range l.LigArray {
			ligArray[i] = bakeAnchorGrid(comp, resolve)
		}
		return &Gpos5_1{
			MarkCov:   l.MarkCov,
			LigCov:    l.LigCov,
			MarkArray: bakeMarkArray(l.MarkArray, resolve),
			LigArray:  ligArray,
		}
	case *Gpos6_1:
		return &Gpos6_1{
			Mark1Cov:   l.Mark1Cov,
			Mark2Cov:   l.Mark2Cov,
			Mark1Array: bakeMarkArray(l.Mark1Array, resolve),
			Mark2Array: bakeAnchorGrid(l.Mark2Array, resolve),
		}
	default:
		return st
	}
}

func bakePairAdjust(adj *PairAdjust, resolve func(uint16, uint16) funit.Int16) *PairAdjust {
	if adj == nil {
		return nil
	}
	return &PairAdjust{
		First:  bakeValueRecord(adj.First, resolve),
		Second: bakeValueRecord(adj.Second, resolve),
	}
}

func bakeMarkArray(marks []markarray.Record, resolve func(uint16, uint16) funit.Int16) []markarray.Record {
	if marks == nil {
		return nil
	}
	out := make([]markarray.Record, len(marks))
	for i, rec := range marks {
		out[i] = markarray.Record{Class: rec.Class, Table: bakeAnchorValue(rec.Table, resolve)}
	}
	return out
}

func bakeAnchorGrid(grid [][]*anchor.Table, resolve func(uint16, uint16) funit.Int16) [][]*anchor.Table {
	if grid == nil {
		return nil
	}
	out := make([][]*anchor.Table, len(grid))
	for i, row := range grid {
		out[i] = make([]*anchor.Table, len(row))
		for j, a := range row {
			out[i][j] = bakeAnchor(a, resolve)
		}
	}
	return out
}

func bakeValueRecord(vr *GposValueRecord, resolve func(uint16, uint16) funit.Int16) *GposValueRecord {
	if vr == nil {
		return nil
	}
	c := *vr
	c.XPlacement += bakeDevice(&c.XPlacementDev, resolve)
	c.YPlacement += bakeDevice(&c.YPlacementDev, resolve)
	c.XAdvance += bakeDevice(&c.XAdvanceDev, resolve)
	c.YAdvance += bakeDevice(&c.YAdvanceDev, resolve)
	return &c
}

func bakeAnchor(a *anchor.Table, resolve func(uint16, uint16) funit.Int16) *anchor.Table {
	if a == nil {
		return nil
	}
	c := bakeAnchorValue(*a, resolve)
	return &c
}

func bakeAnchorValue(a anchor.Table, resolve func(uint16, uint16) funit.Int16) anchor.Table {
	a.X += bakeDevice(&a.XDev, resolve)
	a.Y += bakeDevice(&a.YDev, resolve)
	return a
}

// bakeDevice returns the delta for a VariationIndex table and clears the
// pointer.  For a plain Device table (or nil) it returns 0 and leaves the
// pointer untouched.
func bakeDevice(dev **device.Table, resolve func(uint16, uint16) funit.Int16) funit.Int16 {
	d := *dev
	if d == nil || !d.IsVariationIndex() {
		return 0
	}
	*dev = nil
	return resolve(d.OuterIndex, d.InnerIndex)
}
