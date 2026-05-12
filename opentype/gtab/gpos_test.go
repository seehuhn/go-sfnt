// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2022  Jochen Voss <voss@seehuhn.de>
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
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
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
		Cov:    coverage.Set{1: true, 12: true},
		Class1: classdef.Table{1: 1, 2: 1, 12: 2},
		Class2: classdef.Table{3: 1, 4: 2},
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
	p := parser.New(bytes.NewReader(data))
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
				Table: anchor.Table{
					X: 1,
					Y: 2,
				},
			},
		},
		BaseArray: [][]anchor.Table{
			{
				{
					X: 3,
					Y: 4,
				},
			},
		},
	}
	data := l1.encode()
	p := parser.New(bytes.NewReader(data))
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
	p := parser.New(bytes.NewReader(data))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	if _, err := readGpos4_1(p, 0); err == nil {
		t.Errorf("expected error for out-of-range mark class")
	}
}

func TestGpos5_1(t *testing.T) {
	l1 := &Gpos5_1{
		MarkCov: coverage.Table{10: 0, 11: 1},
		LigCov:  coverage.Table{20: 0, 21: 1},
		MarkArray: []markarray.Record{
			{Class: 0, Table: anchor.Table{X: 1, Y: 2}},
			{Class: 1, Table: anchor.Table{X: 3, Y: 4}},
		},
		LigArray: [][][]anchor.Table{
			// ligature 20: two components, two mark classes
			{
				{
					{X: 10, Y: 20}, // comp 0, class 0
					{X: 30, Y: 40}, // comp 0, class 1
				},
				{
					{X: 50, Y: 60}, // comp 1, class 0
					{},             // comp 1, class 1 (NULL)
				},
			},
			// ligature 21: one component, two mark classes
			{
				{
					{},              // comp 0, class 0 (NULL)
					{X: 70, Y: -10}, // comp 0, class 1
				},
			},
		},
	}
	data := l1.encode()
	if len(data) != l1.encodeLen() {
		t.Fatalf("encode length mismatch: encode=%d encodeLen=%d", len(data), l1.encodeLen())
	}
	p := parser.New(bytes.NewReader(data))
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
	p := parser.New(bytes.NewReader(data))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	if _, err := readGpos6_1(p, 0); err == nil {
		t.Errorf("expected error for out-of-range mark class")
	}
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
	l := Gpos2_1{}
	f.Add(l.encode())
	l = Gpos2_1{
		glyph.Pair{Left: 1, Right: 2}: &PairAdjust{
			First: &GposValueRecord{XAdvance: -10},
		},
	}
	f.Add(l.encode())
	l = Gpos2_1{
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
	}
	f.Add(l.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		doFuzz(t, 2, 1, readGpos2_1, data)
	})
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
				Entry: anchor.Table{X: 1, Y: 2},
				Exit:  anchor.Table{X: 3, Y: 4},
			},
			{
				Entry: anchor.Table{X: -1, Y: -2},
				Exit:  anchor.Table{X: -3, Y: -4},
			},
			{
				Entry: anchor.Table{X: 0, Y: 1},
			},
			{
				Exit: anchor.Table{X: 1, Y: 0},
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
				Table: anchor.Table{
					X: -32768,
					Y: 0,
				},
			},
			{
				Class: 1,
				Table: anchor.Table{
					X: 32767,
					Y: 0,
				},
			},
			{
				Class: 0,
				Table: anchor.Table{
					X: -1,
					Y: 1,
				},
			},
		},
		BaseArray: [][]anchor.Table{
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
				Table: anchor.Table{
					X: -32768,
					Y: 0,
				},
			},
			{
				Class: 1,
				Table: anchor.Table{
					X: 32767,
					Y: 0,
				},
			},
			{
				Class: 0,
				Table: anchor.Table{
					X: -1,
					Y: 1,
				},
			},
		},
		Mark2Array: [][]anchor.Table{
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
			MarkArray: []markarray.Record{{Class: 0, Table: anchor.Table{X: 1}}},
			BaseArray: [][]anchor.Table{
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
			MarkArray: []markarray.Record{{Class: 5, Table: anchor.Table{X: 1}}},
			BaseArray: [][]anchor.Table{{{X: 1}}}, // markClassCount = 1
		}
		assertPanics(t, func() { _ = l.encodeLen() })
	})

	t.Run("Gpos5_1 inconsistent LigArray row width", func(t *testing.T) {
		l := &Gpos5_1{
			MarkCov:   coverage.Table{1: 0},
			LigCov:    coverage.Table{2: 0, 3: 1},
			MarkArray: []markarray.Record{{Class: 0, Table: anchor.Table{X: 1}}},
			LigArray: [][][]anchor.Table{
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
			MarkArray: []markarray.Record{{Class: 5, Table: anchor.Table{X: 1}}},
			LigArray:  [][][]anchor.Table{{{{X: 1}}}}, // markClassCount = 1
		}
		assertPanics(t, func() { _ = l.encodeLen() })
	})

	t.Run("Gpos6_1 inconsistent Mark2Array row width", func(t *testing.T) {
		l := &Gpos6_1{
			Mark1Cov:   coverage.Table{1: 0},
			Mark2Cov:   coverage.Table{2: 0, 3: 1},
			Mark1Array: []markarray.Record{{Class: 0, Table: anchor.Table{X: 1}}},
			Mark2Array: [][]anchor.Table{
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
			Mark1Array: []markarray.Record{{Class: 5, Table: anchor.Table{X: 1}}},
			Mark2Array: [][]anchor.Table{{{X: 1}}}, // markClassCount = 1
		}
		assertPanics(t, func() { _ = l.encodeLen() })
	})
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
		Cov:    coverage.Set{1: true},
		Class1: classdef.Table{1: 1},
		Class2: classdef.Table{1: 1},
		Adjust: adj,
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
	p := parser.New(bytes.NewReader(encoded))
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
