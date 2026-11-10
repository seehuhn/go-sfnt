// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"bytes"
	"errors"
	"maps"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"seehuhn.de/go/membudget"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/opentype/markarray"
	"seehuhn.de/go/sfnt/parser"
)

func TestGpos2_2(t *testing.T) {
	l1 := &Gpos2_2{
		PositionSecond: true,
		Cov:            coverage.Set{1: true, 12: true},
		Class1:         classdef.Table{1: 1, 2: 1, 12: 2},
		Class2:         classdef.Table{3: 1, 4: 2},
		Adjust: [][]*PairAdjust{
			{
				{
					First: &GposValueRecord{
						XPlacement: 1,
						YPlacement: 2,
						XAdvance:   3,
						YAdvance:   4,
					},
					Second: &GposValueRecord{
						XPlacement: 5,
						YPlacement: 6,
						XAdvance:   7,
						YAdvance:   8,
					},
				},
				{
					First: &GposValueRecord{
						XPlacement: 9,
						YPlacement: 10,
						XAdvance:   11,
						YAdvance:   12,
					},
					Second: &GposValueRecord{
						XPlacement: 13,
						YPlacement: 14,
						XAdvance:   15,
						YAdvance:   16,
					},
				},
				{
					First: &GposValueRecord{
						XPlacement: 1000,
						YPlacement: 2000,
						XAdvance:   3000,
						YAdvance:   4000,
					},
					Second: &GposValueRecord{
						XPlacement: 5000,
						YPlacement: 6000,
						XAdvance:   7000,
						YAdvance:   8000,
					},
				},
			},
		},
	}
	data := l1.encode()
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	err := p.Discard(2)
	if err != nil {
		t.Fatal(err)
	}
	l2, err := readGpos2_2(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if d := cmp.Diff(l1, l2); d != "" {
		t.Errorf("mismatch (-want +got):\n%s", d)
	}
}

func TestGpos4_1(t *testing.T) {
	l1 := &Gpos4_1{
		MarkCov: coverage.Table{1: 0},
		BaseCov: coverage.Table{2: 0},
		MarkArray: []markarray.Record{
			{
				Class: 0,
				X:     1,
				Y:     2,
			},
		},
		BaseArray: [][]*anchor.Table{
			{
				{
					X: 3,
					Y: 4,
				},
			},
		},
	}
	data := l1.encode()
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	err := p.Discard(2)
	if err != nil {
		t.Fatal(err)
	}
	l2, err := readGpos4_1(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if d := cmp.Diff(l1, l2); d != "" {
		t.Errorf("mismatch (-want +got):\n%s", d)
	}
}

// TestGpos4_1VariableAnchors round-trips a subtable mixing a format-2
// (contour-point) mark anchor with a format-3 (device) base anchor, exercising
// the variable-size offset arithmetic in the encoder.
func TestGpos4_1VariableAnchors(t *testing.T) {
	cp := uint16(3)
	l1 := &Gpos4_1{
		MarkCov: coverage.Table{1: 0},
		BaseCov: coverage.Table{2: 0},
		MarkArray: []markarray.Record{
			{Class: 0, X: 1, Y: 2, ContourPoint: &cp},
		},
		BaseArray: [][]*anchor.Table{
			{{X: 3, Y: 4, XDev: &device.Table{StartSize: 8, EndSize: 9, Deltas: []int8{1, -1}, DeltaFormat: 1}}},
		},
	}
	data := l1.encode()
	if len(data) != l1.encodeLen() {
		t.Fatalf("encode length mismatch: encode=%d encodeLen=%d", len(data), l1.encodeLen())
	}
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	l2, err := readGpos4_1(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if d := cmp.Diff(l1, l2); d != "" {
		t.Errorf("round-trip mismatch (-want +got):\n%s", d)
	}
}

// TestGpos4_1MarkClassOutOfRange checks that a mark record whose Class
// is greater than or equal to markClassCount is rejected at read time
// rather than triggering a panic later in apply. The byte layout is
// hand-crafted because the encoder rejects this configuration.
func TestGpos4_1MarkClassOutOfRange(t *testing.T) {
	data := []byte{
		// header
		0x00, 0x01, // posFormat = 1
		0x00, 0x0c, // markCoverageOffset = 12
		0x00, 0x12, // baseCoverageOffset = 18
		0x00, 0x01, // markClassCount = 1
		0x00, 0x18, // markArrayOffset = 24
		0x00, 0x24, // baseArrayOffset = 36
		// markCov: format-1, count=1, gid=1
		0x00, 0x01, 0x00, 0x01, 0x00, 0x01,
		// baseCov: format-1, count=1, gid=2
		0x00, 0x01, 0x00, 0x01, 0x00, 0x02,
		// markArray
		0x00, 0x01, // markCount = 1
		0x00, 0x01, // class = 1 — INVALID, markClassCount = 1
		0x00, 0x06, // markAnchorOffset = 6
		0x00, 0x01, 0x00, 0x01, 0x00, 0x02, // mark anchor (format=1, X=1, Y=2)
		// baseArray
		0x00, 0x01, // baseCount = 1
		0x00, 0x04, // baseAnchorOffset = 4
		0x00, 0x01, 0x00, 0x03, 0x00, 0x04, // base anchor (format=1, X=3, Y=4)
	}
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	if _, err := readGpos4_1(p, 0); err == nil {
		t.Errorf("expected error for out-of-range mark class")
	}
}

// TestGpos4_1Apply checks mark-to-base positioning: the mark's anchor is
// aligned with the preceding base's anchor, reduced by the advances of the
// glyphs in between.  The placement is absolute — it replaces any offset the
// mark already carries rather than accumulating onto it.
func TestGpos4_1Apply(t *testing.T) {
	const (
		baseGID glyph.ID    = 50
		markGID glyph.ID    = 10
		baseAdv funit.Int16 = 1000
	)
	l := &Gpos4_1{
		MarkCov: coverage.Table{markGID: 0},
		BaseCov: coverage.Table{baseGID: 0},
		MarkArray: []markarray.Record{
			{Class: 0, X: 5, Y: 5},
		},
		BaseArray: [][]*anchor.Table{
			{{X: 200, Y: 300}}, // base index 0, class 0
		},
	}
	lookupList := []*LookupTable{
		{Meta: &LookupMetaInfo{LookupType: 4}, Subtables: []Subtable{l}},
	}
	// the mark's anchor (5,5) meets the base's anchor (200,300), less the
	// base's advance
	wantX, wantY := funit.Int16(200-5-baseAdv), funit.Int16(300-5)

	t.Run("attaches to preceding base", func(t *testing.T) {
		seq := []glyph.Info{
			{GID: baseGID, Advance: baseAdv},
			{GID: markGID},
		}
		out := NewContext(lookupList, nil, []LookupIndex{0}).Apply(seq)
		if out[1].XOffset != wantX || out[1].YOffset != wantY {
			t.Errorf("mark offset = (%d, %d), want (%d, %d)", out[1].XOffset, out[1].YOffset, wantX, wantY)
		}
	})

	t.Run("absolute placement replaces a prior mark offset", func(t *testing.T) {
		// a preceding lookup may already have shifted the mark; mark-to-base
		// placement is absolute and must overwrite that, not add to it
		seq := []glyph.Info{
			{GID: baseGID, Advance: baseAdv},
			{GID: markGID, XOffset: 999, YOffset: -999},
		}
		out := NewContext(lookupList, nil, []LookupIndex{0}).Apply(seq)
		if out[1].XOffset != wantX || out[1].YOffset != wantY {
			t.Errorf("mark offset = (%d, %d), want (%d, %d); prior offset should be replaced",
				out[1].XOffset, out[1].YOffset, wantX, wantY)
		}
	})
}

func TestGpos5_1(t *testing.T) {
	l1 := &Gpos5_1{
		MarkCov: coverage.Table{10: 0, 11: 1},
		LigCov:  coverage.Table{20: 0, 21: 1},
		MarkArray: []markarray.Record{
			{Class: 0, X: 1, Y: 2},
			{Class: 1, X: 3, Y: 4},
		},
		LigArray: [][][]*anchor.Table{
			// ligature 20: two components, two mark classes
			{
				{
					{X: 10, Y: 20}, // comp 0, class 0
					{X: 30, Y: 40}, // comp 0, class 1
				},
				{
					{X: 50, Y: 60}, // comp 1, class 0
					nil,            // comp 1, class 1 (NULL)
				},
			},
			// ligature 21: one component, two mark classes
			{
				{
					nil,             // comp 0, class 0 (NULL)
					{X: 70, Y: -10}, // comp 0, class 1
				},
			},
		},
	}
	data := l1.encode()
	if len(data) != l1.encodeLen() {
		t.Fatalf("encode length mismatch: encode=%d encodeLen=%d", len(data), l1.encodeLen())
	}
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	l2, err := readGpos5_1(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if d := cmp.Diff(l1, l2); d != "" {
		t.Errorf("round-trip mismatch (-want +got):\n%s", d)
	}
}

// TestGpos5_1Apply checks that mark-to-ligature positioning attaches a mark to
// the ligature component recorded on it (via LigID/LigComp), falling back to
// the last component for marks not tagged for this ligature.  The anchor offset
// is reduced by the advances of the glyphs between the ligature and the mark.
func TestGpos5_1Apply(t *testing.T) {
	const (
		ligGID  glyph.ID    = 100
		markGID glyph.ID    = 10
		ligID   uint16      = 7
		ligAdv  funit.Int16 = 1000
	)
	// one mark class; the ligature has two components with distinct anchors
	l := &Gpos5_1{
		MarkCov: coverage.Table{markGID: 0},
		LigCov:  coverage.Table{ligGID: 0},
		MarkArray: []markarray.Record{
			{Class: 0, X: 5, Y: 5},
		},
		LigArray: [][][]*anchor.Table{
			{
				{{X: 200, Y: 300}}, // component 0, class 0
				{{X: 400, Y: 500}}, // component 1, class 0
			},
		},
	}
	lookupList := []*LookupTable{
		{Meta: &LookupMetaInfo{LookupType: 5}, Subtables: []Subtable{l}},
	}

	cases := []struct {
		name         string
		markLigID    uint16
		markLigComp  uint16
		wantX, wantY funit.Int16
	}{
		// component 0 anchor (200,300): X = 200 - 5 - ligAdv, Y = 300 - 5
		{"component 0", ligID, 0, 200 - 5 - ligAdv, 300 - 5},
		// component 1 anchor (400,500)
		{"component 1", ligID, 1, 400 - 5 - ligAdv, 500 - 5},
		// untagged mark (LigID 0) falls back to the last component
		{"untagged falls back to last", 0, 0, 400 - 5 - ligAdv, 500 - 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			seq := []glyph.Info{
				{GID: ligGID, Advance: ligAdv, LigID: ligID},
				{GID: markGID, LigID: tc.markLigID, LigComp: tc.markLigComp},
			}
			out := NewContext(lookupList, nil, []LookupIndex{0}).Apply(seq)
			if out[1].XOffset != tc.wantX || out[1].YOffset != tc.wantY {
				t.Errorf("mark offset = (%d, %d), want (%d, %d)",
					out[1].XOffset, out[1].YOffset, tc.wantX, tc.wantY)
			}
		})
	}
}

// TestGpos6_1MarkClassOutOfRange — analogous check for Gpos6_1.
func TestGpos6_1MarkClassOutOfRange(t *testing.T) {
	// Gpos6_1 has the same on-disk layout as Gpos4_1 (mark1 plays the role
	// of mark, mark2 plays the role of base), so the bytes are identical.
	data := []byte{
		0x00, 0x01, // posFormat = 1
		0x00, 0x0c, // mark1CoverageOffset = 12
		0x00, 0x12, // mark2CoverageOffset = 18
		0x00, 0x01, // markClassCount = 1
		0x00, 0x18, // mark1ArrayOffset = 24
		0x00, 0x24, // mark2ArrayOffset = 36
		0x00, 0x01, 0x00, 0x01, 0x00, 0x01, // mark1Cov
		0x00, 0x01, 0x00, 0x01, 0x00, 0x02, // mark2Cov
		0x00, 0x01, // mark1Count = 1
		0x00, 0x01, // class = 1 — INVALID
		0x00, 0x06, // mark1AnchorOffset = 6
		0x00, 0x01, 0x00, 0x01, 0x00, 0x02,
		0x00, 0x01, // mark2Count = 1
		0x00, 0x04, // mark2AnchorOffset = 4
		0x00, 0x01, 0x00, 0x03, 0x00, 0x04,
	}
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	if _, err := readGpos6_1(p, 0); err == nil {
		t.Errorf("expected error for out-of-range mark class")
	}
}

// TestGpos6_1Apply checks mark-to-mark positioning: the attaching mark (in
// Mark1Cov) is placed so its anchor meets the anchor of the preceding base
// mark (in Mark2Cov), reduced by the advances of the glyphs in between.  A NULL
// (empty) anchor or a missing base mark leaves the mark unmoved.
func TestGpos6_1Apply(t *testing.T) {
	const (
		baseMark glyph.ID    = 50 // the mark being attached to (Mark2Cov)
		attMark  glyph.ID    = 10 // the attaching mark (Mark1Cov)
		baseAdv  funit.Int16 = 1000
	)
	mk := func(mark2Anchor *anchor.Table) *Gpos6_1 {
		return &Gpos6_1{
			Mark1Cov: coverage.Table{attMark: 0},
			Mark2Cov: coverage.Table{baseMark: 0},
			Mark1Array: []markarray.Record{
				{Class: 0, X: 5, Y: 5},
			},
			Mark2Array: [][]*anchor.Table{
				{mark2Anchor}, // mark2 index 0, class 0
			},
		}
	}

	t.Run("attaches to preceding mark", func(t *testing.T) {
		l := mk(&anchor.Table{X: 200, Y: 300})
		seq := []glyph.Info{
			{GID: baseMark, Advance: baseAdv},
			{GID: attMark},
		}
		out := apply6(l, seq)
		// the attaching mark's anchor (5,5) meets the base mark's anchor
		// (200,300), less the base mark's advance
		if wantX, wantY := funit.Int16(200-5-baseAdv), funit.Int16(300-5); out[1].XOffset != wantX || out[1].YOffset != wantY {
			t.Errorf("mark offset = (%d, %d), want (%d, %d)", out[1].XOffset, out[1].YOffset, wantX, wantY)
		}
	})

	t.Run("null anchor leaves the mark unmoved", func(t *testing.T) {
		l := mk(nil) // absent anchor
		seq := []glyph.Info{
			{GID: baseMark, Advance: baseAdv},
			{GID: attMark},
		}
		out := apply6(l, seq)
		if out[1].XOffset != 0 || out[1].YOffset != 0 {
			t.Errorf("mark offset = (%d, %d), want (0, 0)", out[1].XOffset, out[1].YOffset)
		}
	})

	t.Run("no preceding mark leaves the mark unmoved", func(t *testing.T) {
		l := mk(&anchor.Table{X: 200, Y: 300})
		seq := []glyph.Info{
			{GID: 999}, // not in Mark2Cov
			{GID: attMark},
		}
		out := apply6(l, seq)
		if out[1].XOffset != 0 || out[1].YOffset != 0 {
			t.Errorf("mark offset = (%d, %d), want (0, 0)", out[1].XOffset, out[1].YOffset)
		}
	})
}

// apply6 runs a single Gpos6_1 lookup over seq through the public Apply path.
func apply6(l *Gpos6_1, seq []glyph.Info) []glyph.Info {
	lookupList := []*LookupTable{
		{Meta: &LookupMetaInfo{LookupType: 6}, Subtables: []Subtable{l}},
	}
	return NewContext(lookupList, nil, []LookupIndex{0}).Apply(seq)
}

func FuzzGpos1_1(f *testing.F) {
	l := &Gpos1_1{
		Cov: map[glyph.ID]int{8: 0, 9: 1},
		Adjust: &GposValueRecord{
			XAdvance: 100,
		},
	}
	f.Add(l.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 1, 1, readGpos1_1, data)
	})
}

func FuzzGpos1_2(f *testing.F) {
	l := &Gpos1_2{}
	f.Add(l.encode())
	l = &Gpos1_2{
		Cov: map[glyph.ID]int{8: 0, 9: 1},
		Adjust: []*GposValueRecord{
			{XAdvance: 100},
			{XAdvance: 50, XPlacement: -50},
		},
	}
	f.Add(l.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 1, 2, readGpos1_2, data)
	})
}

func FuzzGpos2_1(f *testing.F) {
	l := &Gpos2_1{Adjust: map[glyph.Pair]*PairAdjust{}}
	f.Add(l.encode())
	l = &Gpos2_1{Adjust: map[glyph.Pair]*PairAdjust{
		glyph.Pair{Left: 1, Right: 2}: &PairAdjust{
			First: &GposValueRecord{XAdvance: -10},
		},
	}}
	f.Add(l.encode())
	l = &Gpos2_1{PositionSecond: true, Adjust: map[glyph.Pair]*PairAdjust{
		glyph.Pair{Left: 1, Right: 2}: &PairAdjust{
			First: &GposValueRecord{XAdvance: -10},
		},
		glyph.Pair{Left: 1, Right: 4}: &PairAdjust{
			First:  &GposValueRecord{XAdvance: -10},
			Second: &GposValueRecord{XPlacement: 5},
		},
		glyph.Pair{Left: 1, Right: 6}: &PairAdjust{
			First: &GposValueRecord{
				XAdvance: -10,
			},
			Second: &GposValueRecord{
				XPlacement: 1,
				YPlacement: 2,
				XAdvance:   3,
				YAdvance:   4,
				XPlacementDev: &device.Table{
					StartSize: 8, EndSize: 11,
					Deltas: []int8{-1, 0, 1, 2}, DeltaFormat: 2,
				},
				YPlacementDev: &device.Table{
					StartSize: 9, EndSize: 9,
					Deltas: []int8{1}, DeltaFormat: 1,
				},
				XAdvanceDev: &device.Table{
					OuterIndex: 7, InnerIndex: 3,
					DeltaFormat: device.VariationIndexFormat,
				},
				YAdvanceDev: &device.Table{
					StartSize: 10, EndSize: 12,
					Deltas: []int8{0, 1, -1}, DeltaFormat: 3,
				},
			},
		},
	}}
	f.Add(l.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 2, 1, readGpos2_1, data)
	})
}

func FuzzGpos2_2(f *testing.F) {
	l := &Gpos2_2{
		Cov:    coverage.Set{1: true, 2: true},
		Class1: classdef.Table{2: 1},
		Class2: classdef.Table{3: 1},
		Adjust: [][]*PairAdjust{
			{{}, {First: &GposValueRecord{XAdvance: -10}}},
			{{}, {First: &GposValueRecord{XAdvance: 20}}},
		},
	}
	f.Add(l.encode())
	l = &Gpos2_2{
		Cov:    coverage.Set{1: true},
		Class1: classdef.Table{},
		Class2: classdef.Table{3: 1, 4: 2},
		Adjust: [][]*PairAdjust{{
			{},
			{First: &GposValueRecord{XAdvance: -10}, Second: &GposValueRecord{YPlacement: 5}},
			{Second: &GposValueRecord{
				XAdvanceDev: &device.Table{
					OuterIndex: 7, InnerIndex: 3,
					DeltaFormat: device.VariationIndexFormat,
				},
			}},
		}},
		PositionSecond: true,
	}
	f.Add(l.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 2, 2, readGpos2_2, data)
	})
}

// TestGpos2_1PairSetDeviceBase feeds a spec-conformant PairPosFormat1
// subtable whose value-record device offset is measured from the PairSet
// table.  Resolving it from the subtable start instead would read the wrong
// bytes, so this pins the offset base independently of the round trip.
func TestGpos2_1PairSetDeviceBase(t *testing.T) {
	dev := &device.Table{StartSize: 8, EndSize: 9, Deltas: []int8{1, -1}, DeltaFormat: 1}
	covBytes := coverage.Table{5: 0}.Encode()
	devBytes := dev.Encode()

	// device follows the single PairValueRecord; the offset is relative to
	// the PairSet table start: pairValueCount + secondGlyph + value record
	deviceOffset := 2 + 2 + (2 + 2)
	pairSetOffset := 12 + len(covBytes)

	var data []byte
	data = append(data,
		0x00, 0x01, // posFormat
		0x00, 0x0c, // coverageOffset = 12
		0x00, 0x11, // valueFormat1 = X_PLACEMENT | X_PLACEMENT_DEVICE
		0x00, 0x00, // valueFormat2
		0x00, 0x01, // pairSetCount
		byte(pairSetOffset>>8), byte(pairSetOffset),
	)
	data = append(data, covBytes...)
	data = append(data,
		0x00, 0x01, // pairValueCount
		0x00, 0x09, // secondGlyph
		0x00, 0x64, // xPlacement = 100
		byte(deviceOffset>>8), byte(deviceOffset),
	)
	data = append(data, devBytes...)

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if _, err := p.ReadUint16(); err != nil { // consume posFormat
		t.Fatal(err)
	}
	sub, err := readGpos2_1(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := sub.(*Gpos2_1).Adjust[glyph.Pair{Left: 5, Right: 9}]
	want := &PairAdjust{First: &GposValueRecord{XPlacement: 100, XPlacementDev: dev}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("device resolved from wrong base (-want +got):\n%s", diff)
	}
}

func FuzzGpos3_1(f *testing.F) {
	l := &Gpos3_1{}
	f.Add(l.encode())
	l = &Gpos3_1{
		Cov: coverage.Table{
			1: 0,
			3: 1,
			5: 2,
			6: 3,
		},
		Records: []EntryExitRecord{
			{
				Entry: &anchor.Table{X: 1, Y: 2},
				Exit:  &anchor.Table{X: 3, Y: 4},
			},
			{
				Entry: &anchor.Table{X: -1, Y: -2},
				Exit:  &anchor.Table{X: -3, Y: -4},
			},
			{
				Entry: &anchor.Table{X: 0, Y: 1},
			},
			{
				Exit: &anchor.Table{X: 1, Y: 0},
			},
		},
	}
	f.Add(l.encode())
	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 3, 1, readGpos3_1, data)
	})
}

func FuzzGpos4_1(f *testing.F) {
	l := &Gpos4_1{}
	f.Add(l.encode())
	l = &Gpos4_1{
		MarkCov: coverage.Table{
			1: 0,
			3: 1,
			9: 2,
		},
		BaseCov: coverage.Table{
			2: 0,
			4: 1,
			6: 2,
		},
		MarkArray: []markarray.Record{
			{
				Class: 0,
				X:     -32768,
				Y:     0,
			},
			{
				Class: 1,
				X:     32767,
				Y:     0,
			},
			{
				Class: 0,
				X:     -1,
				Y:     1,
			},
		},
		BaseArray: [][]*anchor.Table{
			{
				{X: -2, Y: -1},
				{X: 0, Y: 1},
			},
			{
				{X: 2, Y: 3},
				{X: 4, Y: 5},
			},
			{
				{X: 6, Y: 7},
				{X: 8, Y: 255},
			},
		},
	}
	f.Add(l.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 4, 1, readGpos4_1, data)
	})
}

func FuzzGpos5_1(f *testing.F) {
	l := &Gpos5_1{}
	f.Add(l.encode())
	l = &Gpos5_1{
		MarkCov: coverage.Table{
			1: 0,
			3: 1,
			9: 2,
		},
		LigCov: coverage.Table{
			2: 0,
			4: 1,
			6: 2,
		},
		MarkArray: []markarray.Record{
			{
				Class: 0,
				X:     -32768,
				Y:     0,
			},
			{
				Class: 1,
				X:     32767,
				Y:     0,
			},
			{
				Class: 0,
				X:     -1,
				Y:     1,
			},
		},
		// ligatures of one, two and three components; nil entries leave a
		// mark class unattached, and the repeated anchor exercises the pool
		LigArray: [][][]*anchor.Table{
			{
				{{X: -2, Y: -1}, nil},
			},
			{
				{{X: 2, Y: 3}, {X: 4, Y: 5}},
				{nil, {X: 4, Y: 5}},
			},
			{
				{{X: 6, Y: 7}, {X: 8, Y: 255}},
				{{X: 6, Y: 7}, nil},
				{nil, nil},
			},
		},
	}
	f.Add(l.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 5, 1, readGpos5_1, data)
	})
}

func FuzzGpos6_1(f *testing.F) {
	l := &Gpos6_1{}
	f.Add(l.encode())
	l = &Gpos6_1{
		Mark1Cov: coverage.Table{
			1: 0,
			3: 1,
			9: 2,
		},
		Mark2Cov: coverage.Table{
			2: 0,
			4: 1,
			6: 2,
		},
		Mark1Array: []markarray.Record{
			{
				Class: 0,
				X:     -32768,
				Y:     0,
			},
			{
				Class: 1,
				X:     32767,
				Y:     0,
			},
			{
				Class: 0,
				X:     -1,
				Y:     1,
			},
		},
		Mark2Array: [][]*anchor.Table{
			{
				{X: -2, Y: -1},
				{X: 0, Y: 1},
			},
			{
				{X: 2, Y: 3},
				{X: 4, Y: 5},
			},
			{
				{X: 6, Y: 7},
				{X: 8, Y: 255},
			},
		},
	}
	f.Add(l.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 6, 1, readGpos6_1, data)
	})
}

// TestCountMarkClassesCatchesInconsistency confirms that
// countMarkClasses validates structural invariants for Gpos4_1, Gpos5_1
// and Gpos6_1, so both encodeLen and encode see the same panic instead
// of encodeLen happily returning a size for unencodable data.
func TestCountMarkClassesCatchesInconsistency(t *testing.T) {
	t.Run("Gpos4_1 inconsistent BaseArray row width", func(t *testing.T) {
		l := &Gpos4_1{
			MarkCov:   coverage.Table{1: 0},
			BaseCov:   coverage.Table{2: 0, 3: 1},
			MarkArray: []markarray.Record{{Class: 0, X: 1}},
			BaseArray: [][]*anchor.Table{
				{{X: 1}, {X: 2}}, // width 2
				{{X: 3}},         // width 1 — inconsistent
			},
		}
		assertPanics(t, func() { _ = l.encodeLen() })
	})

	t.Run("Gpos4_1 mark class out of range", func(t *testing.T) {
		l := &Gpos4_1{
			MarkCov:   coverage.Table{1: 0},
			BaseCov:   coverage.Table{2: 0},
			MarkArray: []markarray.Record{{Class: 5, X: 1}},
			BaseArray: [][]*anchor.Table{{{X: 1}}}, // markClassCount = 1
		}
		assertPanics(t, func() { _ = l.encodeLen() })
	})

	t.Run("Gpos5_1 inconsistent LigArray row width", func(t *testing.T) {
		l := &Gpos5_1{
			MarkCov:   coverage.Table{1: 0},
			LigCov:    coverage.Table{2: 0, 3: 1},
			MarkArray: []markarray.Record{{Class: 0, X: 1}},
			LigArray: [][][]*anchor.Table{
				{{{X: 1}, {X: 2}}}, // width 2
				{{{X: 3}}},         // width 1 — inconsistent
			},
		}
		assertPanics(t, func() { _ = l.encodeLen() })
	})

	t.Run("Gpos5_1 mark class out of range", func(t *testing.T) {
		l := &Gpos5_1{
			MarkCov:   coverage.Table{1: 0},
			LigCov:    coverage.Table{2: 0},
			MarkArray: []markarray.Record{{Class: 5, X: 1}},
			LigArray:  [][][]*anchor.Table{{{{X: 1}}}}, // markClassCount = 1
		}
		assertPanics(t, func() { _ = l.encodeLen() })
	})

	t.Run("Gpos6_1 inconsistent Mark2Array row width", func(t *testing.T) {
		l := &Gpos6_1{
			Mark1Cov:   coverage.Table{1: 0},
			Mark2Cov:   coverage.Table{2: 0, 3: 1},
			Mark1Array: []markarray.Record{{Class: 0, X: 1}},
			Mark2Array: [][]*anchor.Table{
				{{X: 1}, {X: 2}}, // width 2
				{{X: 3}},         // width 1 — inconsistent
			},
		}
		assertPanics(t, func() { _ = l.encodeLen() })
	})

	t.Run("Gpos6_1 mark class out of range", func(t *testing.T) {
		l := &Gpos6_1{
			Mark1Cov:   coverage.Table{1: 0},
			Mark2Cov:   coverage.Table{2: 0},
			Mark1Array: []markarray.Record{{Class: 5, X: 1}},
			Mark2Array: [][]*anchor.Table{{{X: 1}}}, // markClassCount = 1
		}
		assertPanics(t, func() { _ = l.encodeLen() })
	})
}

// TestCountClassesCatchesInconsistency confirms that countClasses rejects a
// ragged Adjust.  The class-pair records are written as one rectangular
// block, and the coverage table, the two classdefs and every Device table are
// placed after it, so a short row would displace all of them.
func TestCountClassesCatchesInconsistency(t *testing.T) {
	l := &Gpos2_2{
		Cov:    coverage.Set{1: true, 2: true},
		Class1: classdef.Table{1: 1},
		Class2: classdef.Table{2: 1},
		Adjust: [][]*PairAdjust{
			{{First: &GposValueRecord{XAdvance: 1}}, {First: &GposValueRecord{XAdvance: 2}}}, // width 2
			{{First: &GposValueRecord{XAdvance: 3}}},                                         // width 1 — inconsistent
		},
	}
	assertPanics(t, func() { _ = l.encodeLen() })
	assertPanics(t, func() { _ = l.encode() })
}

// TestSubtableSizeOverflow confirms that the encoder panics when a
// subtable's offsets would no longer fit in uint16, instead of
// silently truncating offsets and producing corrupt output.
func TestSubtableSizeOverflow(t *testing.T) {
	// A Gpos2_2 with class1Count=200, class2Count=200 and a 4-byte
	// PairValueRecord (XPlacement on both sides) produces 200*200*4 =
	// 160 000 bytes of records before any header/coverage/classdef —
	// well past the 64 KiB uint16 limit.
	const n = 200
	oneAdj := &PairAdjust{
		First:  &GposValueRecord{XPlacement: 1},
		Second: &GposValueRecord{XPlacement: 1},
	}
	adj := make([][]*PairAdjust, n)
	for i := range adj {
		adj[i] = make([]*PairAdjust, n)
		for j := range adj[i] {
			adj[i][j] = oneAdj
		}
	}
	l := &Gpos2_2{
		Cov:            coverage.Set{1: true},
		Class1:         classdef.Table{1: 1},
		Class2:         classdef.Table{1: 1},
		Adjust:         adj,
		PositionSecond: true,
	}
	assertPanics(t, func() { _ = l.encode() })
}

// TestDevicePoolDeduplicates confirms that GPOS encoders content-
// dedupe Device/VariationIndex tables: a subtable that references the
// same VariationIndex from many value records emits exactly one copy.
//
// Concretely, build a Gpos1_2 whose 16 adjustments all point at the
// same VariationIndex.  Without dedup the encoded subtable carries 16
// copies of the 6-byte table; with dedup it carries one.
func TestDevicePoolDeduplicates(t *testing.T) {
	shared := &device.Table{
		OuterIndex:  3,
		InnerIndex:  7,
		DeltaFormat: device.VariationIndexFormat,
	}
	const n = 16
	adjusts := make([]*GposValueRecord, n)
	cov := coverage.Table{}
	for i := range adjusts {
		adjusts[i] = &GposValueRecord{
			XAdvance:    funit.Int16(i + 1),
			XAdvanceDev: shared,
		}
		cov[glyph.ID(i+1)] = i
	}
	l := &Gpos1_2{Cov: cov, Adjust: adjusts}

	encoded := l.encode()
	if len(encoded) != l.encodeLen() {
		t.Fatalf("encode/encodeLen mismatch: %d vs %d", len(encoded), l.encodeLen())
	}

	// Count occurrences of the shared VariationIndex's encoded bytes
	// inside the subtable.  The dedup'd subtable contains exactly one
	// copy; pre-fix it would have contained n.
	needle := shared.Encode()
	count := bytes.Count(encoded, needle)
	if count != 1 {
		t.Errorf("expected 1 copy of shared VariationIndex, found %d (subtable size %d)",
			count, len(encoded))
	}

	// Re-read and confirm every record still points at a content-
	// equivalent VariationIndex.
	p := parser.New(bytes.NewReader(encoded), parser.NewBudget(int64(len(encoded))))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	got, err := readGpos1_2(p, 0)
	if err != nil {
		t.Fatalf("re-read failed: %v", err)
	}
	for i, adj := range got.(*Gpos1_2).Adjust {
		if adj.XAdvanceDev == nil {
			t.Errorf("adjust %d lost its VariationIndex", i)
			continue
		}
		if d := cmp.Diff(shared, adj.XAdvanceDev); d != "" {
			t.Errorf("adjust %d VariationIndex mismatch (-want +got):\n%s", i, d)
		}
	}
}

// TestGpos2_2LongSubtable checks that a Gpos2_2 subtable longer than
// 64 KiB still encodes, as long as every offset it stores fits in a
// uint16.  Real fonts contain such subtables: the class-pair records
// and the Device tables they point at come first, and the tables which
// are reached through a single offset each sit at the end, where they
// may extend past the limit.
func TestGpos2_2LongSubtable(t *testing.T) {
	const (
		class1Count = 85
		class2Count = 134
		numDevices  = 2778
	)

	adjust := make([][]*PairAdjust, class1Count)
	k := 0
	for i := range adjust {
		adjust[i] = make([]*PairAdjust, class2Count)
		for j := range adjust[i] {
			adjust[i][j] = &PairAdjust{
				First: &GposValueRecord{
					XAdvance: funit.Int16(k%100 - 50),
					XAdvanceDev: &device.Table{
						OuterIndex:  uint16(k % numDevices),
						DeltaFormat: device.VariationIndexFormat,
					},
				},
			}
			k++
		}
	}

	cov := coverage.Set{}
	for gid := glyph.ID(1); gid <= 916; gid++ {
		cov[gid] = true
	}
	class1 := classdef.Table{}
	for gid := glyph.ID(1); gid <= 900; gid++ {
		class1[gid] = uint16(gid)%(class1Count-1) + 1
	}
	class2 := classdef.Table{}
	for gid := glyph.ID(1); gid <= 1000; gid++ {
		class2[gid] = uint16(gid)%(class2Count-1) + 1
	}

	l1 := &Gpos2_2{
		Cov:    cov,
		Class1: class1,
		Class2: class2,
		Adjust: adjust,
	}

	data := l1.encode()
	if len(data) != l1.encodeLen() {
		t.Errorf("encode/encodeLen mismatch: %d vs %d", len(data), l1.encodeLen())
	}
	if len(data) <= 0xFFFF {
		t.Fatalf("subtable is only %d bytes, test does not exercise long subtables", len(data))
	}

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	l2, err := readGpos2_2(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if d := cmp.Diff(l1, l2); d != "" {
		t.Errorf("mismatch (-want +got):\n%s", d)
	}
}

// TestAnchorPoolDeduplicates confirms that the mark attachment encoders
// write one copy of an anchor shared by many glyphs.  Fonts reuse anchors
// heavily, and a copy per reference makes the arrays outgrow the offsets
// which address them.
func TestAnchorPoolDeduplicates(t *testing.T) {
	shared := &anchor.Table{X: 100, Y: 200}
	const n = 50

	baseCov := coverage.Table{}
	baseArray := make([][]*anchor.Table, n)
	for i := range baseArray {
		baseArray[i] = []*anchor.Table{shared}
		baseCov[glyph.ID(i+1)] = i
	}
	l1 := &Gpos4_1{
		MarkCov:   coverage.Table{1000: 0},
		BaseCov:   baseCov,
		MarkArray: []markarray.Record{{Class: 0, X: 1, Y: 2}},
		BaseArray: baseArray,
	}

	data := l1.encode()
	if len(data) != l1.encodeLen() {
		t.Errorf("encode/encodeLen mismatch: %d vs %d", len(data), l1.encodeLen())
	}
	needle := shared.Append(nil)
	if count := bytes.Count(data, needle); count != 1 {
		t.Errorf("expected 1 copy of the shared anchor, found %d", count)
	}

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	l2, err := readGpos4_1(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if d := cmp.Diff(l1, l2); d != "" {
		t.Errorf("mismatch (-want +got):\n%s", d)
	}
}

// TestAnchorPoolDeduplicatesCursive confirms that cursive attachment writes
// one copy of an anchor shared by several glyphs.  Joining scripts attach many
// glyphs to the same entry and exit points.
func TestAnchorPoolDeduplicatesCursive(t *testing.T) {
	entry := &anchor.Table{X: 100, Y: 200}
	exit := &anchor.Table{X: 700, Y: 200}
	const n = 50

	cov := coverage.Table{}
	records := make([]EntryExitRecord, n)
	for i := range records {
		records[i] = EntryExitRecord{Entry: entry, Exit: exit}
		cov[glyph.ID(i+1)] = i
	}
	l1 := &Gpos3_1{Cov: cov, Records: records}

	data := l1.encode()
	if len(data) != l1.encodeLen() {
		t.Errorf("encode/encodeLen mismatch: %d vs %d", len(data), l1.encodeLen())
	}
	for _, a := range []*anchor.Table{entry, exit} {
		if count := bytes.Count(data, a.Append(nil)); count != 1 {
			t.Errorf("expected 1 copy of anchor %v, found %d", *a, count)
		}
	}

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	l2, err := readGpos3_1(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if d := cmp.Diff(l1, l2); d != "" {
		t.Errorf("mismatch (-want +got):\n%s", d)
	}
}

// TestAnchorPoolDeduplicatesLig confirms that a LigatureAttach table writes
// one copy of an anchor shared by several of its components.
func TestAnchorPoolDeduplicatesLig(t *testing.T) {
	shared := &anchor.Table{X: 100, Y: 200}
	const n = 50

	lig := make([][]*anchor.Table, n)
	for i := range lig {
		lig[i] = []*anchor.Table{shared}
	}
	l1 := &Gpos5_1{
		MarkCov:   coverage.Table{1000: 0},
		LigCov:    coverage.Table{1: 0},
		MarkArray: []markarray.Record{{Class: 0, X: 1, Y: 2}},
		LigArray:  [][][]*anchor.Table{lig},
	}

	data := l1.encode()
	if len(data) != l1.encodeLen() {
		t.Errorf("encode/encodeLen mismatch: %d vs %d", len(data), l1.encodeLen())
	}
	needle := shared.Append(nil)
	if count := bytes.Count(data, needle); count != 1 {
		t.Errorf("expected 1 copy of the shared anchor, found %d", count)
	}

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	l2, err := readGpos5_1(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if d := cmp.Diff(l1, l2); d != "" {
		t.Errorf("mismatch (-want +got):\n%s", d)
	}
}

// TestAnchorPoolDeduplicatesMark2 confirms that the mark to mark encoder
// writes one copy of an anchor shared by many mark2 glyphs.
func TestAnchorPoolDeduplicatesMark2(t *testing.T) {
	shared := &anchor.Table{X: 100, Y: 200}
	const n = 50

	mark2Cov := coverage.Table{}
	mark2Array := make([][]*anchor.Table, n)
	for i := range mark2Array {
		mark2Array[i] = []*anchor.Table{shared}
		mark2Cov[glyph.ID(i+1)] = i
	}
	l1 := &Gpos6_1{
		Mark1Cov:   coverage.Table{1000: 0},
		Mark2Cov:   mark2Cov,
		Mark1Array: []markarray.Record{{Class: 0, X: 1, Y: 2}},
		Mark2Array: mark2Array,
	}

	data := l1.encode()
	if len(data) != l1.encodeLen() {
		t.Errorf("encode/encodeLen mismatch: %d vs %d", len(data), l1.encodeLen())
	}
	needle := shared.Append(nil)
	if count := bytes.Count(data, needle); count != 1 {
		t.Errorf("expected 1 copy of the shared anchor, found %d", count)
	}

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	l2, err := readGpos6_1(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if d := cmp.Diff(l1, l2); d != "" {
		t.Errorf("mismatch (-want +got):\n%s", d)
	}
}

// TestGpos5_1LigArrayTooLong checks that a ligature array whose last
// LigatureAttach sits beyond the reach of an Offset16 is refused, instead of
// being written with a truncated offset.  Each LigatureAttach table is small
// here; only their total exceeds the limit.
func TestGpos5_1LigArrayTooLong(t *testing.T) {
	// each LigatureAttach takes 10 bytes, and the array header a further
	// 2 bytes per ligature, so 6000 ligatures put the last offset at 71992
	const n = 6000

	ligCov := coverage.Table{}
	ligArray := make([][][]*anchor.Table, n)
	for i := range ligArray {
		ligArray[i] = [][]*anchor.Table{{{X: funit.Int16(i), Y: 200}}}
		ligCov[glyph.ID(i+1)] = i
	}
	l := &Gpos5_1{
		MarkCov:   coverage.Table{60000: 0},
		LigCov:    ligCov,
		MarkArray: []markarray.Record{{Class: 0, X: 1, Y: 2}},
		LigArray:  ligArray,
	}

	assertPanics(t, func() { l.encode() })
}

// assertPanics runs fn and reports a t.Errorf if fn does not panic.
func assertPanics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("expected panic, got none")
		}
	}()
	fn()
}

// TestPoolsCheckOffsetsWhenSizing confirms that an over-long pool is refused
// by the sizing pass as well as by encode.  encodeLen fills the same pools but
// never lays out the subtable around them, so a check left to the encoder
// would not run on this path at all, and the pool would hand out truncated
// offsets unnoticed.
func TestPoolsCheckOffsetsWhenSizing(t *testing.T) {
	t.Run("anchors", func(t *testing.T) {
		// 2 + 2*n bytes of offsets followed by 6*n bytes of anchors
		const n = 9000

		baseCov := coverage.Table{}
		baseArray := make([][]*anchor.Table, n)
		for i := range baseArray {
			baseArray[i] = []*anchor.Table{{X: funit.Int16(i)}}
			baseCov[glyph.ID(i+1)] = i
		}
		l := &Gpos4_1{
			MarkCov:   coverage.Table{60000: 0},
			BaseCov:   baseCov,
			MarkArray: []markarray.Record{{Class: 0, X: 1, Y: 2}},
			BaseArray: baseArray,
		}

		assertPanics(t, func() { _ = l.encodeLen() })
		assertPanics(t, func() { _ = l.encode() })
	})

	t.Run("devices", func(t *testing.T) {
		// 8 + 2*n bytes of value records and coverage, then 6*n bytes of
		// VariationIndex tables
		const n = 12000

		cov := coverage.Table{}
		adjust := make([]*GposValueRecord, n)
		for i := range adjust {
			cov[glyph.ID(i+1)] = i
			adjust[i] = &GposValueRecord{
				XAdvanceDev: &device.Table{
					OuterIndex:  uint16(i),
					DeltaFormat: device.VariationIndexFormat,
				},
			}
		}
		l := &Gpos1_2{Cov: cov, Adjust: adjust}

		assertPanics(t, func() { _ = l.encodeLen() })
		assertPanics(t, func() { _ = l.encode() })
	})
}

// TestGposLongSubtables checks that a GPOS subtable longer than 64 KiB
// encodes and reads back unchanged, as long as every offset it stores fits in
// a uint16.  These subtables end in a coverage table which carries no offsets
// of its own, so only the offset reaching it is bounded; the table itself may
// run past the limit.  Gpos2_2 has a test of its own, since its layout ends in
// two classdefs rather than a coverage table.
func TestGposLongSubtables(t *testing.T) {
	// glyphs spaced two apart, so the coverage table cannot collapse them
	// into a range and grows with the number of entries
	gid := func(i int) glyph.ID { return glyph.ID(2*i + 1) }
	covTable := func(n int) coverage.Table {
		c := coverage.Table{}
		for i := range n {
			c[gid(i)] = i
		}
		return c
	}

	// a handful of distinct VariationIndex tables, so the Device pool is not
	// empty and its offsets are exercised in their new position
	dev := func(outer uint16) *device.Table {
		return &device.Table{
			OuterIndex:  outer,
			DeltaFormat: device.VariationIndexFormat,
		}
	}

	cases := []struct {
		name string
		make func() Subtable
		read func(*parser.Parser, int64) (Subtable, error)
	}{
		{"Gpos1_1", func() Subtable {
			// one shared adjustment over a very large coverage table
			return &Gpos1_1{
				Cov: covTable(32767),
				Adjust: &GposValueRecord{
					XAdvance:    10,
					XAdvanceDev: dev(0),
				},
			}
		}, readGpos1_1},

		{"Gpos1_2", func() Subtable {
			const n = 16000
			adj := make([]*GposValueRecord, n)
			for i := range adj {
				adj[i] = &GposValueRecord{
					XAdvance:    funit.Int16(i%100 - 50),
					XAdvanceDev: dev(uint16(i % 7)),
				}
			}
			return &Gpos1_2{Cov: covTable(n), Adjust: adj}
		}, readGpos1_2},

		{"Gpos2_1", func() Subtable {
			const n = 6000
			l := &Gpos2_1{Adjust: map[glyph.Pair]*PairAdjust{}}
			for i := range n {
				l.Adjust[glyph.Pair{Left: gid(i), Right: gid(i) + 1}] = &PairAdjust{
					First: &GposValueRecord{
						XAdvance:    funit.Int16(i%100 - 50),
						XAdvanceDev: dev(uint16(i % 7)),
					},
				}
			}
			return l
		}, readGpos2_1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l1 := c.make()

			data := l1.encode()
			if len(data) != l1.encodeLen() {
				t.Errorf("encode/encodeLen mismatch: %d vs %d", len(data), l1.encodeLen())
			}
			if len(data) <= 0xFFFF {
				t.Fatalf("subtable is only %d bytes, the case does not test long subtables",
					len(data))
			}

			p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
			if err := p.Discard(2); err != nil {
				t.Fatal(err)
			}
			l2, err := c.read(p, 0)
			if err != nil {
				t.Fatal(err)
			}
			if d := cmp.Diff(l1, l2); d != "" {
				t.Errorf("round trip (-want +got):\n%s", d)
			}
		})
	}
}

// TestGpos2PositionSecondRequired checks that encoding panics when a subtable
// without second value records is given a non-nil Second.
func TestGpos2PositionSecondRequired(t *testing.T) {
	adj := &PairAdjust{
		First:  &GposValueRecord{XAdvance: 1},
		Second: &GposValueRecord{XAdvance: 2},
	}
	l1 := &Gpos2_1{
		Adjust: map[glyph.Pair]*PairAdjust{{Left: 1, Right: 2}: adj},
	}
	l2 := &Gpos2_2{
		Cov:    coverage.Set{1: true},
		Class1: classdef.Table{1: 1},
		Class2: classdef.Table{2: 1},
		Adjust: [][]*PairAdjust{{{}, {}}, {{}, adj}},
	}
	for _, l := range []Subtable{l1, l2} {
		assertPanics(t, func() { _ = l.encodeLen() })
		assertPanics(t, func() { _ = l.encode() })
	}
}

// TestGpos2_1ApplyPositionSecond checks which glyphs a pair adjustment
// consumes: with PositionSecond the second glyph is used up, without it the
// second glyph can start the next match.
func TestGpos2_1ApplyPositionSecond(t *testing.T) {
	// pairs (1,2) and (2,3) both match the sequence 1 2 3
	pairs := func(second bool) *Gpos2_1 {
		adj := func() *PairAdjust {
			return &PairAdjust{First: &GposValueRecord{XAdvance: 10}}
		}
		return &Gpos2_1{
			PositionSecond: second,
			Adjust: map[glyph.Pair]*PairAdjust{
				{Left: 1, Right: 2}: adj(),
				{Left: 2, Right: 3}: adj(),
			},
		}
	}
	run := func(l *Gpos2_1) []glyph.Info {
		lookupList := []*LookupTable{
			{Meta: &LookupMetaInfo{LookupType: 2}, Subtables: []Subtable{l}},
		}
		seq := []glyph.Info{{GID: 1}, {GID: 2}, {GID: 3}}
		return NewContext(lookupList, nil, []LookupIndex{0}).Apply(seq)
	}

	out := run(pairs(true))
	if out[0].Advance != 10 || out[1].Advance != 0 {
		t.Errorf("PositionSecond=true: advances %d, %d; want 10, 0",
			out[0].Advance, out[1].Advance)
	}
	out = run(pairs(false))
	if out[0].Advance != 10 || out[1].Advance != 10 {
		t.Errorf("PositionSecond=false: advances %d, %d; want 10, 10",
			out[0].Advance, out[1].Advance)
	}
}

// TestGpos2_1SharedPairSets checks that pair sets shared between first glyphs
// in the input stay shared when the subtable is written back out, so that a
// subtable which fits into a font can be re-encoded.
func TestGpos2_1SharedPairSets(t *testing.T) {
	const n = 400 // first glyphs, all sharing one pair set
	const m = 40  // pairs in the shared pair set
	pairSetPos := 10 + 2*n
	covPos := pairSetPos + 2 + 4*m
	data := []byte{
		0, 1, // posFormat
		byte(covPos >> 8), byte(covPos & 0xFF),
		0, 4, // valueFormat1: XAdvance
		0, 0, // valueFormat2
		byte(n >> 8), byte(n & 0xFF), // pairSetCount
	}
	for range n {
		data = append(data, byte(pairSetPos>>8), byte(pairSetPos))
	}
	data = append(data, 0, m) // pairValueCount
	for j := range m {
		data = append(data, 0, byte(j+1), 0xFF, 0xF0) // second glyph, XAdvance -16
	}
	data = append(data, 0, 2, 0, 1, 0, 1, byte(n>>8), byte(n&0xFF), 0, 0) // glyphs 1..n

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	sub, err := readGpos2_1(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(sub.(*Gpos2_1).Adjust); got != n*m {
		t.Fatalf("got %d pairs, want %d", got, n*m)
	}

	enc := sub.encode()
	if len(enc) != sub.encodeLen() {
		t.Fatalf("encode/encodeLen mismatch: %d vs %d", len(enc), sub.encodeLen())
	}
	if len(enc) > len(data) {
		t.Errorf("re-encoded subtable has %d bytes, input has %d", len(enc), len(data))
	}

	p = parser.New(bytes.NewReader(enc), parser.NewBudget(int64(len(enc))))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	sub2, err := readGpos2_1(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(sub, sub2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// TestGpos2_1SharedPairSetsBudget checks that a pair set shared by many first
// glyphs is expanded once per glyph in the flat map of pairs, and that this is
// charged to the memory budget.
func TestGpos2_1SharedPairSetsBudget(t *testing.T) {
	const n = 3000 // first glyphs, all sharing one pair set
	const m = 3000 // pairs in the shared pair set
	pairSetPos := 10 + 2*n
	covPos := pairSetPos + 2 + 2*m
	data := markBE16(1, covPos, 0, 0, n)
	for range n {
		data = append(data, markBE16(pairSetPos)...)
	}
	data = append(data, markBE16(m)...)
	for j := range m {
		data = append(data, markBE16(j+1)...)
	}
	data = append(data, markBE16(2, 1, 1, n, 0)...)

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	if _, err := readGpos2_1(p, 0); !errors.Is(err, membudget.ErrExceeded) {
		t.Errorf("got error %v, want budget exceeded", err)
	}
}

// TestGpos2_1PairSetSharing checks that only pair sets with identical
// content are shared.
func TestGpos2_1PairSetSharing(t *testing.T) {
	dev := &device.Table{StartSize: 8, EndSize: 9, Deltas: []int8{1, -1}, DeltaFormat: 1}
	l := &Gpos2_1{Adjust: map[glyph.Pair]*PairAdjust{
		{Left: 1, Right: 5}: {First: &GposValueRecord{XAdvance: -10, XAdvanceDev: dev}},
		{Left: 2, Right: 5}: {First: &GposValueRecord{XAdvance: -10, XAdvanceDev: dev}},
		{Left: 3, Right: 5}: {First: &GposValueRecord{XAdvance: -11, XAdvanceDev: dev}},
		{Left: 4, Right: 6}: {First: &GposValueRecord{XAdvance: -10, XAdvanceDev: dev}},
		{Left: 5, Right: 5}: {First: &GposValueRecord{XAdvance: -10}},
	}}
	ps, err := l.layout()
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]int{0, 0, 1, 2, 3}, ps.setOf); diff != "" {
		t.Errorf("pair set assignment (-want +got):\n%s", diff)
	}

	enc := l.encode()
	if len(enc) != l.encodeLen() {
		t.Fatalf("encode/encodeLen mismatch: %d vs %d", len(enc), l.encodeLen())
	}
	p := parser.New(bytes.NewReader(enc), parser.NewBudget(int64(len(enc))))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	got, err := readGpos2_1(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(l, got); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

func markBE16(vals ...int) []byte {
	var buf []byte
	for _, v := range vals {
		buf = append(buf, byte(v>>8), byte(v))
	}
	return buf
}

// TestGpos5_1SharedLigatureAttach checks that a subtable whose ligature
// offsets all reach the same LigatureAttach table can be read and written
// back.  Without sharing, the Go data would exceed the memory budget and the
// output would need more than 64 KiB.
func TestGpos5_1SharedLigatureAttach(t *testing.T) {
	const n = 200     // ligatures, all sharing one LigatureAttach table
	const comps = 100 // components in the shared table
	const classes = 8 // mark classes

	markCov := markBE16(1, 1, 7) // format 1, glyph 7
	ligCov := markBE16(2, 1, 1, n, 0)
	markArray := markBE16(1, classes-1, 6, 1, 3, 4) // one mark, anchor at 6

	// every cell refers to the same anchor
	cells := comps * classes
	anchorOff := 2 + 2*cells
	attach := markBE16(comps)
	for range cells {
		attach = append(attach, markBE16(anchorOff)...)
	}
	attach = append(attach, markBE16(1, 5, 6)...)

	markCovOff := 12
	ligCovOff := markCovOff + len(markCov)
	markArrayOff := ligCovOff + len(ligCov)
	ligArrayOff := markArrayOff + len(markArray)
	data := markBE16(1, markCovOff, ligCovOff, classes, markArrayOff, ligArrayOff)
	data = append(data, markCov...)
	data = append(data, ligCov...)
	data = append(data, markArray...)
	attachOff := 2 + 2*n
	data = append(data, markBE16(n)...)
	for range n {
		data = append(data, markBE16(attachOff)...)
	}
	data = append(data, attach...)

	sub := checkSharedRoundTrip(t, readGpos5_1, data).(*Gpos5_1)
	if len(sub.LigArray) != n || len(sub.LigArray[0]) != comps {
		t.Fatalf("got %d ligatures, want %d", len(sub.LigArray), n)
	}
	if &sub.LigArray[0][0] != &sub.LigArray[n-1][0] {
		t.Error("LigatureAttach table not shared")
	}
	if n*len(attach) <= 0xFFFF {
		t.Fatal("test data too small")
	}
}

// checkSharedRoundTripDeep is like checkSharedRoundTrip, but compares the two
// reads with [reflect.DeepEqual].  Unlike cmp.Diff, it visits a pointer shared
// by many references only once, which keeps tests with large shared Device
// tables fast.
func checkSharedRoundTripDeep(t *testing.T, read func(*parser.Parser, int64) (Subtable, error), data []byte) Subtable {
	t.Helper()
	sub := readSubtable(t, read, data)
	enc := sub.encode()
	if len(enc) != sub.encodeLen() {
		t.Fatalf("encode/encodeLen mismatch: %d vs %d", len(enc), sub.encodeLen())
	}
	if len(enc) > len(data) {
		t.Errorf("re-encoded subtable has %d bytes, input has %d", len(enc), len(data))
	}
	if sub2 := readSubtable(t, read, enc); !reflect.DeepEqual(sub, sub2) {
		t.Error("round trip failed")
	}
	return sub
}

// sharedDeviceProbe returns an encoded Device table which is large enough
// that reading it many times exceeds the default memory budget.
func sharedDeviceProbe(t *testing.T) []byte {
	t.Helper()
	const sizes = 4000
	dev := &device.Table{StartSize: 1, EndSize: sizes, Deltas: make([]int8, sizes), DeltaFormat: 2}
	return dev.Encode()
}

// TestGpos3_1SharedAnchors checks that many entry and exit offsets which
// reach one anchor, with a large Device table, can be read and written back.
func TestGpos3_1SharedAnchors(t *testing.T) {
	const n = 2000
	devEnc := sharedDeviceProbe(t)

	headerLen := 6 + 4*n
	covOff := headerLen + 10 + len(devEnc)
	data := markBE16(1, covOff, n)
	for range 2 * n {
		data = append(data, markBE16(headerLen)...)
	}
	data = append(data, markBE16(3, 1, 2, 10, 10)...)
	data = append(data, devEnc...)
	data = append(data, markBE16(2, 1, 1, n, 0)...)

	sub := checkSharedRoundTripDeep(t, readGpos3_1, data).(*Gpos3_1)
	if len(sub.Records) != n {
		t.Fatalf("got %d records, want %d", len(sub.Records), n)
	}
	if sub.Records[0].Entry != sub.Records[n-1].Exit {
		t.Error("anchors not shared")
	}
	if sub.Records[0].Entry.XDev != sub.Records[0].Entry.YDev {
		t.Error("Device tables not shared")
	}
}

// TestMarkAttachSharedAnchors checks that base and mark2 arrays whose cells
// all reach one anchor, with a large Device table, can be read and written
// back.
func TestMarkAttachSharedAnchors(t *testing.T) {
	const bases = 200
	const classes = 2
	devEnc := sharedDeviceProbe(t)

	markCov := markBE16(1, 1, 7)
	baseCov := markBE16(2, 1, 1, bases, 0)
	markArray := markBE16(1, classes-1, 6, 1, 3, 4)

	cells := bases * classes
	anchorOff := 2 + 2*cells
	baseArray := markBE16(bases)
	for range cells {
		baseArray = append(baseArray, markBE16(anchorOff)...)
	}
	baseArray = append(baseArray, markBE16(3, 1, 2, 10, 10)...)
	baseArray = append(baseArray, devEnc...)

	markCovOff := 12
	baseCovOff := markCovOff + len(markCov)
	markArrayOff := baseCovOff + len(baseCov)
	baseArrayOff := markArrayOff + len(markArray)
	data := markBE16(1, markCovOff, baseCovOff, classes, markArrayOff, baseArrayOff)
	data = append(data, markCov...)
	data = append(data, baseCov...)
	data = append(data, markArray...)
	data = append(data, baseArray...)

	t.Run("Gpos4_1", func(t *testing.T) {
		sub := checkSharedRoundTripDeep(t, readGpos4_1, data).(*Gpos4_1)
		if len(sub.BaseArray) != bases || sub.BaseArray[0][0] != sub.BaseArray[bases-1][classes-1] {
			t.Error("anchors not shared")
		}
	})
	t.Run("Gpos6_1", func(t *testing.T) {
		sub := checkSharedRoundTripDeep(t, readGpos6_1, data).(*Gpos6_1)
		if len(sub.Mark2Array) != bases || sub.Mark2Array[0][0] != sub.Mark2Array[bases-1][classes-1] {
			t.Error("anchors not shared")
		}
	})
}

// TestMarkArraySharedAnchors checks that mark records which reach one anchor
// share its Device table.
func TestMarkArraySharedAnchors(t *testing.T) {
	const marks = 600
	devEnc := sharedDeviceProbe(t)

	markCov := markBE16(2, 1, 1, marks, 0)
	baseCov := markBE16(1, 1, 7)
	anchorOff := 2 + 4*marks
	markArray := markBE16(marks)
	for range marks {
		markArray = append(markArray, markBE16(0, anchorOff)...)
	}
	markArray = append(markArray, markBE16(3, 1, 2, 10, 10)...)
	markArray = append(markArray, devEnc...)
	baseArray := markBE16(1, 4, 1, 1, 1) // one base, one anchor at 4

	markCovOff := 12
	baseCovOff := markCovOff + len(markCov)
	markArrayOff := baseCovOff + len(baseCov)
	baseArrayOff := markArrayOff + len(markArray)
	data := markBE16(1, markCovOff, baseCovOff, 1, markArrayOff, baseArrayOff)
	data = append(data, markCov...)
	data = append(data, baseCov...)
	data = append(data, markArray...)
	data = append(data, baseArray...)

	sub := checkSharedRoundTripDeep(t, readGpos4_1, data).(*Gpos4_1)
	if len(sub.MarkArray) != marks {
		t.Fatalf("got %d marks, want %d", len(sub.MarkArray), marks)
	}
	if sub.MarkArray[0].XDev != sub.MarkArray[marks-1].YDev {
		t.Error("Device tables not shared")
	}
}

// TestValueRecordSharedDevice checks that value records which reach one large
// Device table can be read and written back.
func TestValueRecordSharedDevice(t *testing.T) {
	const n = 300
	devEnc := sharedDeviceProbe(t)

	t.Run("Gpos1_2", func(t *testing.T) {
		devOff := 8 + 2*n
		covOff := devOff + len(devEnc)
		data := markBE16(2, covOff, 0x0040, n)
		for range n {
			data = append(data, markBE16(devOff)...)
		}
		data = append(data, devEnc...)
		data = append(data, markBE16(2, 1, 1, n, 0)...)

		sub := checkSharedRoundTripDeep(t, readGpos1_2, data).(*Gpos1_2)
		if sub.Adjust[0].XAdvanceDev != sub.Adjust[n-1].XAdvanceDev {
			t.Error("Device tables not shared")
		}
	})

	t.Run("Gpos2_1", func(t *testing.T) {
		psOff := 12
		devOff := 2 + 4*n
		covOff := psOff + devOff + len(devEnc)
		data := markBE16(1, covOff, 0x0040, 0, 1, psOff)
		data = append(data, markBE16(n)...)
		for j := range n {
			data = append(data, markBE16(j+1, devOff)...)
		}
		data = append(data, devEnc...)
		data = append(data, markBE16(1, 1, 1)...)

		sub := checkSharedRoundTripDeep(t, readGpos2_1, data).(*Gpos2_1)
		if len(sub.Adjust) != n {
			t.Fatalf("got %d pairs, want %d", len(sub.Adjust), n)
		}
		a := sub.Adjust[glyph.Pair{Left: 1, Right: 1}].First.XAdvanceDev
		b := sub.Adjust[glyph.Pair{Left: 1, Right: n}].First.XAdvanceDev
		if a == nil || a != b {
			t.Error("Device tables not shared")
		}
	})

	t.Run("Gpos2_2", func(t *testing.T) {
		classDef := markBE16(2, 0) // format 2, no ranges
		devOff := 16 + 2*n
		cdOff := devOff + len(devEnc)
		covOff := cdOff + 2*len(classDef)
		data := markBE16(2, covOff, 0x0040, 0, cdOff, cdOff+len(classDef), 1, n)
		for range n {
			data = append(data, markBE16(devOff)...)
		}
		data = append(data, devEnc...)
		data = append(data, classDef...)
		data = append(data, classDef...)
		data = append(data, markBE16(1, 1, 1)...)

		sub := checkSharedRoundTripDeep(t, readGpos2_2, data).(*Gpos2_2)
		row := sub.Adjust[0]
		if len(row) != n || row[0].First.XAdvanceDev != row[n-1].First.XAdvanceDev {
			t.Error("Device tables not shared")
		}
	})
}

// TestGpos3_1SharedDevice checks that an anchor whose two Device offsets
// reach the same 32 KiB table is written back with one copy of that table.
// Without sharing, the anchor which follows it would lie beyond the reach of
// an Offset16.
func TestGpos3_1SharedDevice(t *testing.T) {
	const sizes = 65524 // four bits each, so 6+2*16381 bytes
	dev := &device.Table{StartSize: 1, EndSize: sizes, Deltas: make([]int8, sizes), DeltaFormat: 2}
	devEnc := dev.Encode()
	if len(devEnc) != 32768 {
		t.Fatalf("device table has %d bytes", len(devEnc))
	}

	a1 := append(markBE16(3, 1, 0, 10, 10), devEnc...)
	a2 := markBE16(1, 2, 0)

	a1Off := 10
	a2Off := a1Off + len(a1)
	covOff := a2Off + len(a2)
	data := markBE16(1, covOff, 1, a1Off, a2Off)
	data = append(data, a1...)
	data = append(data, a2...)
	data = append(data, markBE16(1, 1, 1)...)

	sub := checkSharedRoundTrip(t, readGpos3_1, data).(*Gpos3_1)
	rec := sub.Records[0]
	if rec.Entry == nil || rec.Exit == nil || rec.Entry.XDev == nil || rec.Entry.YDev == nil {
		t.Fatal("anchors not read")
	}
	if want := 10 + len(devEnc); rec.Entry.EncodeLen() != want {
		t.Errorf("anchor length %d, want %d", rec.Entry.EncodeLen(), want)
	}
	if 20+2*len(devEnc) <= 0xFFFF {
		t.Fatal("test data too small")
	}
}

// TestMarkAttachCoverageSharing checks that identical coverage tables are
// stored once, and that different ones are not.
func TestMarkAttachCoverageSharing(t *testing.T) {
	cov := coverage.Table{3: 0, 5: 1}
	other := coverage.Table{3: 0, 6: 1}
	covLen := cov.EncodeLen()
	if other.EncodeLen() != covLen {
		t.Fatal("test coverages differ in length")
	}

	anchors := func() []*anchor.Table { return []*anchor.Table{{X: 1, Y: 2}} }
	marks := []markarray.Record{
		{Class: 0, X: 1, Y: 2},
		{Class: 0, X: 3, Y: 4},
	}
	base := [][]*anchor.Table{anchors(), anchors()}

	cases := []struct {
		name string
		make func(c1, c2 coverage.Table) Subtable
		read func(*parser.Parser, int64) (Subtable, error)
	}{
		{"Gpos4_1", func(c1, c2 coverage.Table) Subtable {
			return &Gpos4_1{MarkCov: c1, BaseCov: c2, MarkArray: marks, BaseArray: base}
		}, readGpos4_1},
		{"Gpos5_1", func(c1, c2 coverage.Table) Subtable {
			return &Gpos5_1{MarkCov: c1, LigCov: c2, MarkArray: marks,
				LigArray: [][][]*anchor.Table{{anchors()}, {anchors()}}}
		}, readGpos5_1},
		{"Gpos6_1", func(c1, c2 coverage.Table) Subtable {
			return &Gpos6_1{Mark1Cov: c1, Mark2Cov: c2, Mark1Array: marks, Mark2Array: base}
		}, readGpos6_1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			same := c.make(cov, cov)
			diff := c.make(cov, other)
			if got := diff.encodeLen() - same.encodeLen(); got != covLen {
				t.Errorf("sharing saves %d bytes, want %d", got, covLen)
			}
			for _, l := range []Subtable{same, diff} {
				enc := l.encode()
				if len(enc) != l.encodeLen() {
					t.Errorf("encode/encodeLen mismatch: %d vs %d", len(enc), l.encodeLen())
				}
				got := readSubtable(t, c.read, enc)
				if d := cmp.Diff(l, got); d != "" {
					t.Errorf("round trip failed (-want +got):\n%s", d)
				}
			}
		})
	}
}

// TestGpos5_1LigatureAttachSharing checks that only identical LigatureAttach
// tables are shared.
func TestGpos5_1LigatureAttachSharing(t *testing.T) {
	lig := func(x int) [][]*anchor.Table {
		return [][]*anchor.Table{{{X: funit.Int16(x)}}}
	}
	mk := func(a, b int) *Gpos5_1 {
		return &Gpos5_1{
			MarkCov:   coverage.Table{9: 0},
			LigCov:    coverage.Table{1: 0, 2: 1},
			MarkArray: []markarray.Record{{Class: 0, X: 1, Y: 2}},
			LigArray:  [][][]*anchor.Table{lig(a), lig(b)},
		}
	}
	same, diff := mk(5, 5), mk(5, 6)
	if got := diff.encodeLen() - same.encodeLen(); got != 2+2+6 {
		t.Errorf("sharing saves %d bytes, want 10", got)
	}
	for _, l := range []*Gpos5_1{same, diff} {
		got := readSubtable(t, readGpos5_1, l.encode())
		if d := cmp.Diff(l, got); d != "" {
			t.Errorf("round trip failed (-want +got):\n%s", d)
		}
	}
}

// TestGpos2_2TableSharing checks that identical class definitions are stored
// once.
func TestGpos2_2TableSharing(t *testing.T) {
	mk := func(c2 classdef.Table) *Gpos2_2 {
		return &Gpos2_2{
			Cov:    coverage.Set{1: true, 2: true},
			Class1: classdef.Table{1: 1, 2: 1},
			Class2: c2,
			Adjust: [][]*PairAdjust{
				{{First: &GposValueRecord{XAdvance: 3}}, {First: &GposValueRecord{XAdvance: 4}}},
				{{First: &GposValueRecord{XAdvance: 1}}, {First: &GposValueRecord{XAdvance: 2}}},
			},
		}
	}
	same := mk(classdef.Table{1: 1, 2: 1})
	diff := mk(classdef.Table{1: 1, 3: 1})
	if got := diff.encodeLen() - same.encodeLen(); got != diff.Class2.AppendLen() {
		t.Errorf("sharing saves %d bytes, want %d", got, diff.Class2.AppendLen())
	}
	for _, l := range []*Gpos2_2{same, diff} {
		enc := l.encode()
		if len(enc) != l.encodeLen() {
			t.Errorf("encode/encodeLen mismatch: %d vs %d", len(enc), l.encodeLen())
		}
		got := readSubtable(t, readGpos2_2, enc)
		if d := cmp.Diff(l, got); d != "" {
			t.Errorf("round trip failed (-want +got):\n%s", d)
		}
	}
}

// TestGposLayoutErrors checks that layout reports an offset which does not
// fit, instead of panicking.
func TestGposLayoutErrors(t *testing.T) {
	const n = 12000
	cov := coverage.Table{}
	records := make([]EntryExitRecord, n)
	bases := make([][]*anchor.Table, n)
	adjust := make([]*GposValueRecord, n)
	pairs := map[glyph.Pair]*PairAdjust{}
	for i := range n {
		cov[glyph.ID(i+1)] = i
		a := &anchor.Table{X: funit.Int16(i)}
		records[i] = EntryExitRecord{Entry: a}
		bases[i] = []*anchor.Table{a}
		dev := &device.Table{OuterIndex: uint16(i), DeltaFormat: device.VariationIndexFormat}
		adjust[i] = &GposValueRecord{XAdvanceDev: dev}
		pairs[glyph.Pair{Left: glyph.ID(i + 1), Right: 1}] = &PairAdjust{First: adjust[i]}
	}
	marks := []markarray.Record{{Class: 0, X: 1, Y: 2}}

	cases := []struct {
		name string
		err  func() error
	}{
		{"Gpos1_2", func() error { _, err := (&Gpos1_2{Cov: cov, Adjust: adjust}).layout(); return err }},
		{"Gpos2_1", func() error { _, err := (&Gpos2_1{Adjust: pairs}).layout(); return err }},
		{"Gpos3_1", func() error { _, err := (&Gpos3_1{Cov: cov, Records: records}).layout(); return err }},
		{"Gpos4_1", func() error {
			_, err := (&Gpos4_1{MarkCov: coverage.Table{60000: 0}, BaseCov: cov, MarkArray: marks, BaseArray: bases}).layout()
			return err
		}},
		{"Gpos6_1", func() error {
			_, err := (&Gpos6_1{Mark1Cov: coverage.Table{60000: 0}, Mark2Cov: cov, Mark1Array: marks, Mark2Array: bases}).layout()
			return err
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.err() == nil {
				t.Error("expected an error")
			}
		})
	}
}

// TestGpos2_2ClassCountRange checks that a class count which does not fit
// into a uint16 is reported, even when the class records are empty.
func TestGpos2_2ClassCountRange(t *testing.T) {
	l := &Gpos2_2{
		Cov:    coverage.Set{1: true},
		Adjust: make([][]*PairAdjust, 0x10000),
	}
	if err := l.fits(); err == nil {
		t.Error("expected error")
	}
}

// TestLayoutOffsetErrors checks that the layout of a subtable reports
// tables which cannot be reached, at each stage where this can happen.
func TestLayoutOffsetErrors(t *testing.T) {
	const n = 12000
	varDev := func(i int) *device.Table {
		return &device.Table{OuterIndex: uint16(i), DeltaFormat: device.VariationIndexFormat}
	}
	// a Device table of about 64 KiB
	largeDev := &device.Table{EndSize: 65534, Deltas: make([]int8, 65535), DeltaFormat: 3}
	largeDev2 := &device.Table{EndSize: 65533, Deltas: make([]int8, 65534), DeltaFormat: 3}

	// coverage tables with n glyphs and with close to 32768 glyphs, which
	// cannot be stored as ranges
	cov := coverage.Table{}
	for i := range n {
		cov[glyph.ID(2*i+1)] = i
	}
	const m = 32760
	wide := coverage.Table{}
	for i := range m {
		wide[glyph.ID(2*i+1)] = i
	}

	wider := maps.Clone(wide)
	for i := m; i < m+4; i++ {
		wider[glyph.ID(2*i+1)] = i
	}

	// one long per-glyph table, with all other glyphs sharing a short one
	long := make([]glyph.ID, 25000)
	short := []glyph.ID{1}
	const k = 20000
	kCov := coverage.Table{}
	seqs := make([][]glyph.ID, k)
	ligs := make([][]Ligature, k)
	shortLigs := []Ligature{{In: short, Out: 2}}
	for i := range k {
		kCov[glyph.ID(2*i+1)] = i
		seqs[i] = short
		ligs[i] = shortLigs
	}
	seqs[0] = long
	ligs[0] = []Ligature{{In: long, Out: 1}}

	secondDevs := map[glyph.Pair]*PairAdjust{}
	firstDevs := map[glyph.Pair]*PairAdjust{}
	exits := make([]EntryExitRecord, n)
	marks := make([]markarray.Record, n)
	rows := make([][]*anchor.Table, n)
	for i := range n {
		pair := glyph.Pair{Left: 1, Right: glyph.ID(i)}
		firstDevs[pair] = &PairAdjust{First: &GposValueRecord{XAdvanceDev: varDev(i)}}
		secondDevs[pair] = &PairAdjust{Second: &GposValueRecord{XAdvanceDev: varDev(i)}}
		a := &anchor.Table{X: funit.Int16(i)}
		exits[i] = EntryExitRecord{Exit: a}
		marks[i] = markarray.Record{Table: *a}
		rows[i] = []*anchor.Table{a}
	}
	classDevs := func(second bool) [][]*PairAdjust {
		res := make([][]*PairAdjust, 100)
		for i := range res {
			res[i] = make([]*PairAdjust, 120)
			for j := range res[i] {
				vr := &GposValueRecord{XAdvanceDev: varDev(120*i + j)}
				if second {
					res[i][j] = &PairAdjust{Second: vr}
				} else {
					res[i][j] = &PairAdjust{First: vr}
				}
			}
		}
		return res
	}
	oneMark := []markarray.Record{{X: 1}}
	shared := &anchor.Table{X: 1}
	wideRows := make([][]*anchor.Table, m)
	for i := range wideRows {
		wideRows[i] = []*anchor.Table{shared}
	}

	cases := []struct {
		name string
		sub  Subtable
	}{
		{"Gsub2_1 sequence", &Gsub2_1{
			Cov:  coverage.Table{1: 0, 2: 1},
			Repl: [][]glyph.ID{make([]glyph.ID, 40000), make([]glyph.ID, 40001)},
		}},
		{"Gsub2_1 coverage", &Gsub2_1{Cov: kCov, Repl: seqs}},
		{"Gsub3_1 coverage", &Gsub3_1{Cov: kCov, Alternates: seqs}},
		{"Gsub4_1 coverage", &Gsub4_1{Cov: kCov, Repl: ligs}},
		{"Gpos1_1 device", &Gpos1_1{
			Cov: coverage.Table{1: 0},
			Adjust: &GposValueRecord{
				XPlacementDev: largeDev,
				YPlacementDev: largeDev2,
				XAdvanceDev:   varDev(1),
			},
		}},
		{"Gpos1_1 coverage", &Gpos1_1{
			Cov:    wider,
			Adjust: &GposValueRecord{XAdvanceDev: largeDev},
		}},
		{"Gpos1_2 coverage", &Gpos1_2{
			Cov:    kCov,
			Adjust: append(make([]*GposValueRecord, k-1), &GposValueRecord{XAdvanceDev: largeDev}),
		}},
		{"Gpos2_1 first device", &Gpos2_1{Adjust: firstDevs}},
		{"Gpos2_1 second device", &Gpos2_1{Adjust: secondDevs, PositionSecond: true}},
		{"Gpos2_2 first device", &Gpos2_2{Cov: coverage.Set{1: true}, Adjust: classDevs(false)}},
		{"Gpos2_2 second device", &Gpos2_2{
			Cov:            coverage.Set{1: true},
			Adjust:         classDevs(true),
			PositionSecond: true,
		}},
		{"Gpos3_1 exit anchor", &Gpos3_1{Cov: cov, Records: exits}},
		{"Gpos3_1 coverage", &Gpos3_1{
			Cov:     cov,
			Records: append(make([]EntryExitRecord, n-1), EntryExitRecord{Entry: &anchor.Table{XDev: largeDev}}),
		}},
		{"Gpos4_1 mark array", &Gpos4_1{
			MarkCov:   cov,
			BaseCov:   coverage.Table{1: 0},
			MarkArray: marks,
			BaseArray: [][]*anchor.Table{{shared}},
		}},
		{"Gpos4_1 header", &Gpos4_1{
			MarkCov:   coverage.Table{60000: 0},
			BaseCov:   wide,
			MarkArray: oneMark,
			BaseArray: wideRows,
		}},
		{"Gpos5_1 mark array", &Gpos5_1{
			MarkCov:   cov,
			LigCov:    coverage.Table{1: 0},
			MarkArray: marks,
			LigArray:  [][][]*anchor.Table{{{shared}}},
		}},
		{"Gpos5_1 ligature attach", &Gpos5_1{
			MarkCov:   coverage.Table{60000: 0},
			LigCov:    coverage.Table{1: 0},
			MarkArray: oneMark,
			LigArray:  [][][]*anchor.Table{rows},
		}},
		{"Gpos6_1 mark array", &Gpos6_1{
			Mark1Cov:   cov,
			Mark2Cov:   coverage.Table{1: 0},
			Mark1Array: marks,
			Mark2Array: [][]*anchor.Table{{shared}},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.sub.fits(); err == nil {
				t.Error("expected an error")
			}
		})
	}
}
