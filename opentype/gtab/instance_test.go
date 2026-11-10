// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"runtime"
	"testing"

	"github.com/google/go-cmp/cmp"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/opentype/markarray"
)

// varIdx returns a VariationIndex table whose delta, as given by
// testResolve, is inner.
func varIdx(inner uint16) *device.Table {
	return &device.Table{DeltaFormat: device.VariationIndexFormat, InnerIndex: inner}
}

func testResolve(outer, inner uint16) funit.Int16 {
	return funit.Int16(inner)
}

func TestBakeVariationDevices(t *testing.T) {
	plain := &device.Table{StartSize: 8, EndSize: 8, Deltas: []int8{1}, DeltaFormat: 1}
	vr := &GposValueRecord{XAdvance: 10, XAdvanceDev: varIdx(5), YPlacementDev: plain}
	vrBaked := &GposValueRecord{XAdvance: 15, YPlacementDev: plain}
	a := &anchor.Table{X: 1, Y: 2, XDev: varIdx(3), YDev: varIdx(4)}
	aBaked := &anchor.Table{X: 4, Y: 6}
	marks := []markarray.Record{{Class: 0, Table: *a}}
	marksBaked := []markarray.Record{{Class: 0, Table: *aBaked}}
	ctx := &SeqContext1{Cov: coverage.Table{1: 0}, Rules: [][]*SeqRule{{{}}}}

	ll := LookupList{{
		Meta: &LookupMetaInfo{LookupType: 1},
		Subtables: []Subtable{
			&Gpos1_1{Cov: coverage.Table{1: 0}, Adjust: vr},
			&Gpos1_2{Cov: coverage.Table{1: 0}, Adjust: []*GposValueRecord{vr}},
			&Gpos2_1{Adjust: map[glyph.Pair]*PairAdjust{{Left: 1, Right: 2}: {First: vr}}},
			&Gpos2_2{Adjust: [][]*PairAdjust{{{First: vr, Second: vr}}}, PositionSecond: true},
			&Gpos3_1{Cov: coverage.Table{1: 0}, Records: []EntryExitRecord{{Entry: a}}},
			&Gpos4_1{MarkArray: marks, BaseArray: [][]*anchor.Table{{a, nil}}},
			&Gpos5_1{MarkArray: marks, LigArray: [][][]*anchor.Table{{{a}}}},
			&Gpos6_1{Mark1Array: marks, Mark2Array: [][]*anchor.Table{{a}}},
			ctx,
		},
	}}
	want := LookupList{{
		Meta: ll[0].Meta,
		Subtables: []Subtable{
			&Gpos1_1{Cov: coverage.Table{1: 0}, Adjust: vrBaked},
			&Gpos1_2{Cov: coverage.Table{1: 0}, Adjust: []*GposValueRecord{vrBaked}},
			&Gpos2_1{Adjust: map[glyph.Pair]*PairAdjust{{Left: 1, Right: 2}: {First: vrBaked}}},
			&Gpos2_2{Adjust: [][]*PairAdjust{{{First: vrBaked, Second: vrBaked}}}, PositionSecond: true},
			&Gpos3_1{Cov: coverage.Table{1: 0}, Records: []EntryExitRecord{{Entry: aBaked}}},
			&Gpos4_1{MarkArray: marksBaked, BaseArray: [][]*anchor.Table{{aBaked, nil}}},
			&Gpos5_1{MarkArray: marksBaked, LigArray: [][][]*anchor.Table{{{aBaked}}}},
			&Gpos6_1{Mark1Array: marksBaked, Mark2Array: [][]*anchor.Table{{aBaked}}},
			ctx,
		},
	}}

	got := ll.BakeVariationDevices(testResolve)
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unexpected result (-want +got):\n%s", diff)
	}

	// the input is not modified
	if vr.XAdvanceDev == nil || a.XDev == nil || marks[0].XDev == nil {
		t.Error("input modified")
	}
}

// TestBakeVariationDevicesShared checks that values shared within the input
// are baked once and remain shared.
func TestBakeVariationDevicesShared(t *testing.T) {
	const n = 5000
	const m = 2000

	// many ligatures with one shared LigatureAttach table
	comp := [][]*anchor.Table{make([]*anchor.Table, m)}
	for i := range comp[0] {
		comp[0][i] = &anchor.Table{XDev: varIdx(uint16(i))}
	}
	lig := &Gpos5_1{LigArray: make([][][]*anchor.Table, n)}
	for i := range lig.LigArray {
		lig.LigArray[i] = comp
	}

	// many pairs with one shared adjustment
	adj := &PairAdjust{First: &GposValueRecord{XAdvanceDev: varIdx(1)}}
	pairs := &Gpos2_1{Adjust: map[glyph.Pair]*PairAdjust{}}
	for i := range n {
		pairs.Adjust[glyph.Pair{Left: glyph.ID(i), Right: 1}] = adj
	}

	ll := LookupList{{Meta: &LookupMetaInfo{}, Subtables: []Subtable{lig, pairs}}}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	out := ll.BakeVariationDevices(testResolve)
	runtime.ReadMemStats(&after)

	// copying the shared LigatureAttach table per ligature would allocate
	// more than n*m*8 bytes
	if used := after.TotalAlloc - before.TotalAlloc; used > 2_000_000 {
		t.Errorf("baking allocated %d bytes", used)
	}

	lig2 := out[0].Subtables[0].(*Gpos5_1)
	for i := 1; i < n; i++ {
		if &lig2.LigArray[i][0][0] != &lig2.LigArray[0][0][0] {
			t.Fatal("shared LigatureAttach table copied")
		}
	}
	if lig2.LigArray[0][0][7].X != 7 || lig2.LigArray[0][0][7].XDev != nil {
		t.Error("anchor not baked")
	}

	pairs2 := out[0].Subtables[1].(*Gpos2_1)
	first := pairs2.Adjust[glyph.Pair{Left: 0, Right: 1}]
	if first.First.XAdvance != 1 {
		t.Error("value record not baked")
	}
	for pair, adj := range pairs2.Adjust {
		if adj != first {
			t.Fatalf("shared adjustment for %v copied", pair)
		}
	}
}

// TestBakeVariationDevicesEmpty checks that empty slices of different types
// are baked independently.  All empty slices have the same address.
func TestBakeVariationDevicesEmpty(t *testing.T) {
	ll := LookupList{{
		Meta: &LookupMetaInfo{},
		Subtables: []Subtable{
			&Gpos2_2{Adjust: [][]*PairAdjust{{}}},
			&Gpos4_1{MarkArray: []markarray.Record{}, BaseArray: [][]*anchor.Table{}},
		},
	}}
	out := ll.BakeVariationDevices(testResolve)
	if diff := cmp.Diff(ll, out); diff != "" {
		t.Errorf("baking changed the lookups (-want +got):\n%s", diff)
	}
}
