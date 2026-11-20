// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"iter"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/internal/vardelta"
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
// left in place unchanged.  If a coordinate with its delta added does not fit
// into 16 bits, BakeVariationDevices returns an error.
//
// The returned LookupList shares immutable structure with ll: only the value
// records, anchors, subtables and lookups that can carry a device table are
// copied, so ll itself is not modified.  Values which are shared within ll,
// as the readers return them for tables reached through several offsets, are
// copied once and remain shared in the result.  This is used when
// instantiating a variable font.
func (ll LookupList) BakeVariationDevices(resolve func(outer, inner uint16) int64) (LookupList, error) {
	return ll.mapDevices(func(v funit.Int16, d *device.Table) (funit.Int16, *device.Table, error) {
		if !d.IsVariationIndex() {
			return v, d, nil
		}
		x, err := vardelta.AddInt16(v, resolve(d.OuterIndex, d.InnerIndex))
		return x, nil, err
	})
}

// MapDevices returns a copy of ll in which every device table t of a GPOS
// value record or anchor has been replaced by fn(t).  fn is called only for
// device tables which are present.  If fn returns nil, the device table is
// removed and the coordinate it adjusts is kept.
//
// The receiver is not modified.  Values which are shared within ll are
// shared in the result.
func (ll LookupList) MapDevices(fn func(*device.Table) *device.Table) LookupList {
	res, _ := ll.mapDevices(func(v funit.Int16, d *device.Table) (funit.Int16, *device.Table, error) {
		return v, fn(d), nil
	})
	return res
}

// Devices yields the device tables of the GPOS value records and anchors
// in ll, in no particular order.  Values, anchors and lists of them which
// are shared within ll are visited once, so that the work done is
// proportional to the memory used by ll.
func (ll LookupList) Devices() iter.Seq[*device.Table] {
	return func(yield func(*device.Table) bool) {
		w := &deviceWalker{yield: yield, seen: map[any]bool{}}
		for _, lookup := range ll {
			if !w.first(lookup) {
				continue
			}
			for _, st := range lookup.Subtables {
				if !w.subtable(st) {
					return
				}
			}
		}
	}
}

// deviceWalker visits the device tables of a lookup list for
// [LookupList.Devices].  Its methods return false once yield has asked to
// stop.
type deviceWalker struct {
	yield func(*device.Table) bool
	seen  map[any]bool // pointers and [offsetpool.SliceKey] keys
}

// first reports whether key is seen for the first time.
func (w *deviceWalker) first(key any) bool {
	if w.seen[key] {
		return false
	}
	w.seen[key] = true
	return true
}

// firstSlice reports whether the slice s is seen for the first time.
func firstSlice[T any](w *deviceWalker, s []T) bool {
	return len(s) > 0 && w.first(offsetpool.SliceKey(s))
}

func (w *deviceWalker) subtable(st Subtable) bool {
	switch l := st.(type) {
	case *Gpos1_1:
		if !w.first(l) {
			return true
		}
		return w.valueRecord(l.Adjust)
	case *Gpos1_2:
		if !w.first(l) {
			return true
		}
		for _, vr := range l.Adjust {
			if !w.valueRecord(vr) {
				return false
			}
		}
	case *Gpos2_1:
		if !w.first(l) {
			return true
		}
		for _, adj := range l.Adjust {
			if !w.pairAdjust(adj) {
				return false
			}
		}
	case *Gpos2_2:
		if !w.first(l) {
			return true
		}
		for _, row := range l.Adjust {
			if !firstSlice(w, row) {
				continue
			}
			for _, adj := range row {
				if !w.pairAdjust(adj) {
					return false
				}
			}
		}
	case *Gpos3_1:
		if !w.first(l) {
			return true
		}
		for _, rec := range l.Records {
			if !w.anchor(rec.Entry) || !w.anchor(rec.Exit) {
				return false
			}
		}
	case *Gpos4_1:
		if !w.first(l) {
			return true
		}
		return w.markArray(l.MarkArray) && w.anchorGrid(l.BaseArray)
	case *Gpos5_1:
		if !w.first(l) {
			return true
		}
		if !w.markArray(l.MarkArray) {
			return false
		}
		for _, comp := range l.LigArray {
			if !w.anchorGrid(comp) {
				return false
			}
		}
	case *Gpos6_1:
		if !w.first(l) {
			return true
		}
		return w.markArray(l.Mark1Array) && w.anchorGrid(l.Mark2Array)
	}
	return true
}

func (w *deviceWalker) pairAdjust(adj *PairAdjust) bool {
	if adj == nil || !w.first(adj) {
		return true
	}
	return w.valueRecord(adj.First) && w.valueRecord(adj.Second)
}

func (w *deviceWalker) valueRecord(vr *GposValueRecord) bool {
	if vr == nil || !w.first(vr) {
		return true
	}
	return w.device(vr.XPlacementDev) && w.device(vr.YPlacementDev) &&
		w.device(vr.XAdvanceDev) && w.device(vr.YAdvanceDev)
}

func (w *deviceWalker) markArray(marks []markarray.Record) bool {
	if !firstSlice(w, marks) {
		return true
	}
	for _, rec := range marks {
		if !w.anchorValue(&rec.Table) {
			return false
		}
	}
	return true
}

func (w *deviceWalker) anchorGrid(grid [][]*anchor.Table) bool {
	if !firstSlice(w, grid) {
		return true
	}
	for _, row := range grid {
		if !firstSlice(w, row) {
			continue
		}
		for _, a := range row {
			if !w.anchor(a) {
				return false
			}
		}
	}
	return true
}

func (w *deviceWalker) anchor(a *anchor.Table) bool {
	if a == nil || !w.first(a) {
		return true
	}
	return w.anchorValue(a)
}

func (w *deviceWalker) anchorValue(a *anchor.Table) bool {
	return w.device(a.XDev) && w.device(a.YDev)
}

func (w *deviceWalker) device(d *device.Table) bool {
	return d == nil || w.yield(d)
}

// mapDevices returns a copy of ll in which every coordinate v with a device
// table d has been replaced by adjust(v, d).
func (ll LookupList) mapDevices(adjust func(v funit.Int16, d *device.Table) (funit.Int16, *device.Table, error)) (LookupList, error) {
	b := &mapper{
		adjust:  adjust,
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
	// the first error is reported; the partial result is discarded
	if b.err != nil {
		return nil, b.err
	}
	return out, nil
}

// mapper applies a function to the coordinates of a lookup list which carry
// device tables.  Each value is visited once, so that shared values stay
// shared, and the memory used is proportional to the size of the input.
type mapper struct {
	adjust  func(v funit.Int16, d *device.Table) (funit.Int16, *device.Table, error)
	err     error // the first error returned by adjust
	values  map[*GposValueRecord]*GposValueRecord
	pairs   map[*PairAdjust]*PairAdjust
	anchors map[*anchor.Table]*anchor.Table
	slices  map[any]any // keyed by [offsetpool.SliceKey]
}

// memoSlice returns the result of fn for the slice s, calling fn only
// once for each slice.
func memoSlice[T, U any](b *mapper, s []T, fn func([]T) []U) []U {
	if s == nil {
		return nil
	}
	key := offsetpool.SliceKey(s)
	if res, ok := b.slices[key]; ok {
		return res.([]U)
	}
	res := fn(s)
	b.slices[key] = res
	return res
}

// subtable returns a copy of st with its device tables adjusted.  Subtable
// types that cannot carry device tables (contextual lookups, GSUB
// subtables) are returned unchanged.
func (b *mapper) subtable(st Subtable) Subtable {
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

func (b *mapper) pairAdjust(adj *PairAdjust) *PairAdjust {
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

func (b *mapper) markArray(marks []markarray.Record) []markarray.Record {
	return memoSlice(b, marks, func(marks []markarray.Record) []markarray.Record {
		out := make([]markarray.Record, len(marks))
		for i, rec := range marks {
			out[i] = markarray.Record{Class: rec.Class, Table: b.anchorValue(rec.Table)}
		}
		return out
	})
}

func (b *mapper) anchorGrid(grid [][]*anchor.Table) [][]*anchor.Table {
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

func (b *mapper) valueRecord(vr *GposValueRecord) *GposValueRecord {
	if vr == nil {
		return nil
	}
	if res, ok := b.values[vr]; ok {
		return res
	}
	c := *vr
	b.apply(&c.XPlacement, &c.XPlacementDev)
	b.apply(&c.YPlacement, &c.YPlacementDev)
	b.apply(&c.XAdvance, &c.XAdvanceDev)
	b.apply(&c.YAdvance, &c.YAdvanceDev)
	b.values[vr] = &c
	return &c
}

func (b *mapper) anchor(a *anchor.Table) *anchor.Table {
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

func (b *mapper) anchorValue(a anchor.Table) anchor.Table {
	b.apply(&a.X, &a.XDev)
	b.apply(&a.Y, &a.YDev)
	return a
}

// apply replaces the coordinate v and its device table by the result of
// b.adjust.  A coordinate without a device table is left untouched.  An
// error is recorded in b.err.
func (b *mapper) apply(v *funit.Int16, dev **device.Table) {
	if *dev == nil {
		return
	}
	x, d, err := b.adjust(*v, *dev)
	if err != nil {
		if b.err == nil {
			b.err = err
		}
		return
	}
	*v, *dev = x, d
}
