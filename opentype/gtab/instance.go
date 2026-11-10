// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/opentype/internal/offsetpool"
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
// records, anchors, subtables and lookups that can carry a device table are
// copied, so ll itself is not modified.  Values which are shared within ll,
// as the readers return them for tables reached through several offsets, are
// copied once and remain shared in the result.  This is used when
// instantiating a variable font.
func (ll LookupList) BakeVariationDevices(resolve func(outer, inner uint16) funit.Int16) LookupList {
	b := &baker{
		resolve: resolve,
		values:  map[*GposValueRecord]*GposValueRecord{},
		pairs:   map[*PairAdjust]*PairAdjust{},
		anchors: map[*anchor.Table]*anchor.Table{},
		slices:  map[any]any{},
	}
	out := make(LookupList, len(ll))
	for i, lookup := range ll {
		subtables := make([]Subtable, len(lookup.Subtables))
		for j, st := range lookup.Subtables {
			subtables[j] = b.subtable(st)
		}
		out[i] = &LookupTable{
			Meta:      lookup.Meta,
			Subtables: subtables,
		}
	}
	return out
}

// baker bakes VariationIndex tables into the values which refer to them.
// Each value is baked once, so that shared values stay shared, and the
// memory used is proportional to the size of the input.
type baker struct {
	resolve func(outer, inner uint16) funit.Int16
	values  map[*GposValueRecord]*GposValueRecord
	pairs   map[*PairAdjust]*PairAdjust
	anchors map[*anchor.Table]*anchor.Table
	slices  map[any]any // keyed by [offsetpool.SliceKey]
}

// memoSlice returns the result of bake for the slice s, calling bake only
// once for each slice.
func memoSlice[T, U any](b *baker, s []T, bake func([]T) []U) []U {
	if s == nil {
		return nil
	}
	key := offsetpool.SliceKey(s)
	if res, ok := b.slices[key]; ok {
		return res.([]U)
	}
	res := bake(s)
	b.slices[key] = res
	return res
}

// subtable returns a copy of st with its VariationIndex device tables
// baked in.  Subtable types that cannot carry device tables (contextual
// lookups, GSUB subtables) are returned unchanged.
func (b *baker) subtable(st Subtable) Subtable {
	switch l := st.(type) {
	case *Gpos1_1:
		return &Gpos1_1{Cov: l.Cov, Adjust: b.valueRecord(l.Adjust)}
	case *Gpos1_2:
		adjust := make([]*GposValueRecord, len(l.Adjust))
		for i, vr := range l.Adjust {
			adjust[i] = b.valueRecord(vr)
		}
		return &Gpos1_2{Cov: l.Cov, Adjust: adjust}
	case *Gpos2_1:
		adjust := make(map[glyph.Pair]*PairAdjust, len(l.Adjust))
		for pair, adj := range l.Adjust {
			adjust[pair] = b.pairAdjust(adj)
		}
		return &Gpos2_1{Adjust: adjust, PositionSecond: l.PositionSecond}
	case *Gpos2_2:
		adjust := make([][]*PairAdjust, len(l.Adjust))
		for i, row := range l.Adjust {
			adjust[i] = memoSlice(b, row, func(row []*PairAdjust) []*PairAdjust {
				res := make([]*PairAdjust, len(row))
				for j, adj := range row {
					res[j] = b.pairAdjust(adj)
				}
				return res
			})
		}
		return &Gpos2_2{
			Cov:            l.Cov,
			Class1:         l.Class1,
			Class2:         l.Class2,
			Adjust:         adjust,
			PositionSecond: l.PositionSecond,
		}
	case *Gpos3_1:
		records := make([]EntryExitRecord, len(l.Records))
		for i, rec := range l.Records {
			records[i] = EntryExitRecord{
				Entry: b.anchor(rec.Entry),
				Exit:  b.anchor(rec.Exit),
			}
		}
		return &Gpos3_1{Cov: l.Cov, Records: records}
	case *Gpos4_1:
		return &Gpos4_1{
			MarkCov:   l.MarkCov,
			BaseCov:   l.BaseCov,
			MarkArray: b.markArray(l.MarkArray),
			BaseArray: b.anchorGrid(l.BaseArray),
		}
	case *Gpos5_1:
		ligArray := make([][][]*anchor.Table, len(l.LigArray))
		for i, comp := range l.LigArray {
			ligArray[i] = b.anchorGrid(comp)
		}
		return &Gpos5_1{
			MarkCov:   l.MarkCov,
			LigCov:    l.LigCov,
			MarkArray: b.markArray(l.MarkArray),
			LigArray:  ligArray,
		}
	case *Gpos6_1:
		return &Gpos6_1{
			Mark1Cov:   l.Mark1Cov,
			Mark2Cov:   l.Mark2Cov,
			Mark1Array: b.markArray(l.Mark1Array),
			Mark2Array: b.anchorGrid(l.Mark2Array),
		}
	default:
		return st
	}
}

func (b *baker) pairAdjust(adj *PairAdjust) *PairAdjust {
	if adj == nil {
		return nil
	}
	if res, ok := b.pairs[adj]; ok {
		return res
	}
	res := &PairAdjust{
		First:  b.valueRecord(adj.First),
		Second: b.valueRecord(adj.Second),
	}
	b.pairs[adj] = res
	return res
}

func (b *baker) markArray(marks []markarray.Record) []markarray.Record {
	return memoSlice(b, marks, func(marks []markarray.Record) []markarray.Record {
		out := make([]markarray.Record, len(marks))
		for i, rec := range marks {
			out[i] = markarray.Record{Class: rec.Class, Table: b.anchorValue(rec.Table)}
		}
		return out
	})
}

func (b *baker) anchorGrid(grid [][]*anchor.Table) [][]*anchor.Table {
	return memoSlice(b, grid, func(grid [][]*anchor.Table) [][]*anchor.Table {
		out := make([][]*anchor.Table, len(grid))
		for i, row := range grid {
			out[i] = memoSlice(b, row, func(row []*anchor.Table) []*anchor.Table {
				res := make([]*anchor.Table, len(row))
				for j, a := range row {
					res[j] = b.anchor(a)
				}
				return res
			})
		}
		return out
	})
}

func (b *baker) valueRecord(vr *GposValueRecord) *GposValueRecord {
	if vr == nil {
		return nil
	}
	if res, ok := b.values[vr]; ok {
		return res
	}
	c := *vr
	c.XPlacement += b.device(&c.XPlacementDev)
	c.YPlacement += b.device(&c.YPlacementDev)
	c.XAdvance += b.device(&c.XAdvanceDev)
	c.YAdvance += b.device(&c.YAdvanceDev)
	b.values[vr] = &c
	return &c
}

func (b *baker) anchor(a *anchor.Table) *anchor.Table {
	if a == nil {
		return nil
	}
	if res, ok := b.anchors[a]; ok {
		return res
	}
	c := b.anchorValue(*a)
	b.anchors[a] = &c
	return &c
}

func (b *baker) anchorValue(a anchor.Table) anchor.Table {
	a.X += b.device(&a.XDev)
	a.Y += b.device(&a.YDev)
	return a
}

// device returns the delta for a VariationIndex table and clears the
// pointer.  For a plain Device table (or nil) it returns 0 and leaves the
// pointer untouched.
func (b *baker) device(dev **device.Table) funit.Int16 {
	d := *dev
	if d == nil || !d.IsVariationIndex() {
		return 0
	}
	*dev = nil
	return b.resolve(d.OuterIndex, d.InnerIndex)
}
