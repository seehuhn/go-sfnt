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

package gdef

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/variation"
)

func TestGdefListsRoundTrip(t *testing.T) {
	cp := uint16(2)
	table1 := &Table{
		AttachList: &AttachList{
			Cov:    coverage.Table{2: 0, 4: 1},
			Points: [][]uint16{{1, 3}, {5}},
		},
		LigCaretList: &LigCaretList{
			Cov: coverage.Table{10: 0, 11: 1},
			Carets: [][]CaretValue{
				{
					{Coordinate: 100},   // format 1
					{ContourPoint: &cp}, // format 2
					{Coordinate: 200, Device: &device.Table{ // format 3
						StartSize: 8, EndSize: 9, Deltas: []int8{1, -1}, DeltaFormat: 1}},
				},
				nil, // ligature with no carets
			},
		},
	}
	data := table1.Encode()
	table2, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(table1, table2) {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", table2, table1)
	}
}

// TestGdefEncodeNoStoreByteIdentical pins the encoder's output for
// tables without an item variation store to the bytes produced before
// GDEF 1.3 support was added. It guards against the version-1.3 header
// changes accidentally altering the version-1.0/1.2 encoding.
func TestGdefEncodeNoStoreByteIdentical(t *testing.T) {
	table := &Table{}
	check := func(want string) {
		t.Helper()
		got := hex.EncodeToString(table.Encode())
		if got != want {
			t.Errorf("got  %s\nwant %s", got, want)
		}
	}
	check("000100000000000000000000")

	table.GlyphClass = classdef.Table{
		2:  GlyphClassBase,
		3:  GlyphClassBase,
		4:  GlyphClassBase,
		10: GlyphClassLigature,
	}
	check("00010000000c00000000000000020002000200040001000a000a0002")

	table.MarkAttachClass = classdef.Table{
		5: 1,
		6: 2,
		7: 1,
	}
	check("00010000000c00000000001c00020002000200040001000a000a0002000100050003000100020001")

	table.MarkGlyphSets = []coverage.Set{
		{12: true, 13: true, 14: true},
		{10: true, 15: true, 16: true},
	}
	check("00010002000e00000000001e002a00020002000200040001000a000a0002000100050003000100020001000100020000000c0000001600010003000c000d000e00010003000a000f0010")

	cp := uint16(2)
	table.AttachList = &AttachList{
		Cov:    coverage.Table{2: 0, 4: 1},
		Points: [][]uint16{{1, 3}, {5}},
	}
	table.LigCaretList = &LigCaretList{
		Cov: coverage.Table{10: 0},
		Carets: [][]CaretValue{
			{
				{Coordinate: 100},
				{ContourPoint: &cp},
				{Coordinate: 200, Device: &device.Table{StartSize: 8, EndSize: 9, Deltas: []int8{1, -1}, DeltaFormat: 1}},
			},
		},
	}
	check("00010002000e001e00380062006e00020002000200040001000a000a0002000800020010001600010002000200040002000100030001000500060001000c00010001000a00030008000c00100001006400020002000300c800060008000900017000000100050003000100020001000100020000000c0000001600010003000c000d000e00010003000a000f0010")
}

var goldenItemVarStore = &variation.ItemVariationStore{
	Regions: []variation.Region{
		{{Start: 0, Peak: 0x4000, End: 0x4000}},
	},
	Data: []*variation.ItemVariationData{
		{
			RegionIndexes: []uint16{0},
			Deltas:        [][]int32{{100}, {200}},
		},
	},
}

// TestGdefItemVarStoreRoundTrip checks that a GDEF 1.3 table with an item
// variation store, and no mark glyph sets, round-trips.
func TestGdefItemVarStoreRoundTrip(t *testing.T) {
	table1 := &Table{
		GlyphClass:   classdef.Table{2: GlyphClassBase},
		ItemVarStore: goldenItemVarStore,
	}
	data := table1.Encode()

	major := uint16(data[0])<<8 | uint16(data[1])
	minor := uint16(data[2])<<8 | uint16(data[3])
	if major != 1 || minor != 3 {
		t.Fatalf("version = %d.%d, want 1.3", major, minor)
	}

	table2, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(table1, table2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// TestGdefItemVarStoreWithMarkGlyphSetsRoundTrip checks the combination of
// an item variation store and mark glyph sets, which exercises the full
// 1.3 header (both trailing offset fields present).
func TestGdefItemVarStoreWithMarkGlyphSetsRoundTrip(t *testing.T) {
	table1 := &Table{
		MarkGlyphSets: []coverage.Set{
			{12: true, 13: true},
		},
		ItemVarStore: goldenItemVarStore,
	}
	data := table1.Encode()
	table2, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(table1, table2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

func FuzzGdef(f *testing.F) {
	table := &Table{}
	f.Add(table.Encode())
	table.GlyphClass = classdef.Table{
		2:  GlyphClassBase,
		3:  GlyphClassBase,
		4:  GlyphClassBase,
		10: GlyphClassLigature,
	}
	f.Add(table.Encode())
	table.MarkAttachClass = classdef.Table{
		5: 1,
		6: 2,
		7: 1,
	}
	f.Add(table.Encode())
	table.MarkGlyphSets = []coverage.Set{
		{12: true, 13: true, 14: true},
		{10: true, 15: true, 16: true},
	}
	f.Add(table.Encode())
	cp := uint16(2)
	table.AttachList = &AttachList{
		Cov:    coverage.Table{2: 0, 4: 1},
		Points: [][]uint16{{1, 3}, {5}},
	}
	table.LigCaretList = &LigCaretList{
		Cov: coverage.Table{10: 0},
		Carets: [][]CaretValue{
			{
				{Coordinate: 100},
				{ContourPoint: &cp},
				{Coordinate: 200, Device: &device.Table{StartSize: 8, EndSize: 9, Deltas: []int8{1, -1}, DeltaFormat: 1}},
			},
		},
	}
	f.Add(table.Encode())
	table.ItemVarStore = goldenItemVarStore
	f.Add(table.Encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		table1, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		if err != nil {
			return
		}

		data2 := table1.Encode()

		// The compact re-encoding may be smaller than data, giving it a
		// smaller input-proportional budget; reuse data's allowance so a
		// wide ClassDef/Coverage range that fit the first read does not trip
		// the budget on the second.
		table2, err := Read(bytes.NewReader(data2), parser.NewBudget(int64(len(data))))
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(table1, table2) {
			t.Error("different")
		}
	})
}

func assertPanics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("expected panic, got none")
		}
	}()
	fn()
}

// bigClassDef returns a class definition table of more than 64 kB.
func bigClassDef() classdef.Table {
	cd := classdef.Table{}
	for g := 1; g <= 40000; g++ {
		cd[glyph.ID(g)] = uint16(1 + g%2)
	}
	return cd
}

// distinctCarets returns n caret values which all differ.
func distinctCarets(n int) []CaretValue {
	row := make([]CaretValue, n)
	for i := range row {
		row[i].Coordinate = funit.Int16(i)
	}
	return row
}

// bigAttachList returns an attachment point list of more than 64 kB.
func bigAttachList() *AttachList {
	return &AttachList{
		Cov:    coverage.Table{1: 0},
		Points: [][]uint16{make([]uint16, 40000)},
	}
}

func TestGdefEncodeOffsetRange(t *testing.T) {
	cd := bigClassDef()
	if cd.AppendLen() <= 0xFFFF {
		t.Fatalf("class definition too short: %d bytes", cd.AppendLen())
	}
	smallCaret := &LigCaretList{
		Cov:    coverage.Table{1: 0},
		Carets: [][]CaretValue{{{Coordinate: 1}}},
	}
	bigCaret := &LigCaretList{
		Cov:    coverage.Table{1: 0},
		Carets: [][]CaretValue{distinctCarets(20000)},
	}

	// In each case, the tables only fit if the large table is stored last.
	fitting := map[string]*Table{
		"attach list":      {GlyphClass: cd, AttachList: &AttachList{Cov: coverage.Table{1: 0}, Points: [][]uint16{{1}}}},
		"ligature carets":  {AttachList: bigAttachList(), LigCaretList: smallCaret},
		"mark glyph sets":  {MarkAttachClass: cd, MarkGlyphSets: []coverage.Set{{1: true}}},
		"mark sets, v 1.3": {MarkAttachClass: cd, MarkGlyphSets: []coverage.Set{{1: true}}, ItemVarStore: &variation.ItemVariationStore{}},
	}
	for name, table := range fitting {
		t.Run(name, func(t *testing.T) {
			data := table.Encode()
			got, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(table, got); diff != "" {
				t.Errorf("round trip failed (-want +got):\n%s", diff)
			}
		})
	}

	tooLarge := map[string]*Table{
		"mark attach":   {LigCaretList: bigCaret, MarkAttachClass: classdef.Table{1: 1}},
		"two big lists": {GlyphClass: cd, MarkAttachClass: cd},
	}
	for name, table := range tooLarge {
		t.Run(name, func(t *testing.T) {
			assertPanics(t, func() { table.Encode() })
		})
	}

	t.Run("within limit", func(t *testing.T) {
		table := &Table{
			AttachList: &AttachList{
				Cov:    coverage.Table{1: 0},
				Points: [][]uint16{make([]uint16, 20000)},
			},
			LigCaretList:    smallCaret,
			MarkAttachClass: classdef.Table{1: 1},
			MarkGlyphSets:   []coverage.Set{{1: true}},
		}
		data := table.Encode()
		got, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(table, got); diff != "" {
			t.Errorf("round trip failed (-want +got):\n%s", diff)
		}
	})
}

// TestGdefReadDropsUnreachable checks that optional tables which cannot be
// reached from the header of the encoded table are dropped on read.  Both
// class definition offsets point to the same table of more than 64 kB,
// which is written out twice.
func TestGdefReadDropsUnreachable(t *testing.T) {
	cd := bigClassDef()
	data := []byte{
		0, 1, 0, 0, // version 1.0
		0, 12, // glyph class definition
		0, 0, // attachment point list
		0, 0, // ligature caret list
		0, 12, // mark attachment class definition
	}
	data = cd.Append(data)

	got, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}
	want := &Table{GlyphClass: cd}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unexpected result (-want +got):\n%s", diff)
	}

	// the result can be written
	data2 := got.Encode()
	got2, err := Read(bytes.NewReader(data2), parser.NewBudget(int64(len(data2))))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(got, got2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

func TestAttachListOffsetRange(t *testing.T) {
	t.Run("coverage", func(t *testing.T) {
		const n = 32768
		al := &AttachList{Cov: coverage.Table{}, Points: make([][]uint16, n)}
		for i := range n {
			al.Cov[glyph.ID(i+1)] = i
		}
		assertPanics(t, func() { mustFit(al.layout()) })
	})
	t.Run("points", func(t *testing.T) {
		// the second point list lies behind a first one of about 80 kB
		al := &AttachList{
			Cov:    coverage.Table{1: 0, 2: 1},
			Points: [][]uint16{make([]uint16, 40000), {1}},
		}
		assertPanics(t, func() { mustFit(al.layout()) })
	})
}

func TestLigCaretListOffsetRange(t *testing.T) {
	t.Run("coverage", func(t *testing.T) {
		const n = 32768
		lcl := &LigCaretList{Cov: coverage.Table{}, Carets: make([][]CaretValue, n)}
		for i := range n {
			lcl.Cov[glyph.ID(i+1)] = i
		}
		assertPanics(t, func() { mustFit(lcl.layout()) })
	})
	t.Run("ligature glyph", func(t *testing.T) {
		// the second ligature glyph lies behind a first one of about 120 kB
		lcl := &LigCaretList{
			Cov:    coverage.Table{1: 0, 2: 1},
			Carets: [][]CaretValue{distinctCarets(20000), {{Coordinate: 1}}},
		}
		assertPanics(t, func() { mustFit(lcl.layout()) })
	})
	t.Run("caret value", func(t *testing.T) {
		// a single ligature glyph with so many carets that the last
		// caret values lie beyond 64 kB
		lcl := &LigCaretList{
			Cov:    coverage.Table{1: 0},
			Carets: [][]CaretValue{distinctCarets(11000)},
		}
		assertPanics(t, func() { mustFit(lcl.layout()) })
	})
	t.Run("within limit", func(t *testing.T) {
		lcl := &LigCaretList{
			Cov:    coverage.Table{1: 0},
			Carets: [][]CaretValue{distinctCarets(10000)},
		}
		table := &Table{LigCaretList: lcl}
		data := table.Encode()
		got, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(table, got); diff != "" {
			t.Errorf("round trip failed (-want +got):\n%s", diff)
		}
	})
}

// sequentialCoverage returns a coverage table for glyphs 1, ..., n.
func sequentialCoverage(n int) coverage.Table {
	cov := coverage.Table{}
	for i := range n {
		cov[glyph.ID(i+1)] = i
	}
	return cov
}

func roundTripTable(t *testing.T, table *Table) []byte {
	t.Helper()
	data := table.Encode()
	got, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(table, got); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
	return data
}

func TestAttachListSharing(t *testing.T) {
	// without sharing, the point lists would need 176 kB
	const n = 8000
	pts := []uint16{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	al := &AttachList{Cov: sequentialCoverage(n), Points: make([][]uint16, n)}
	for i := range al.Points {
		al.Points[i] = append([]uint16(nil), pts...)
	}
	data := roundTripTable(t, &Table{AttachList: al})
	if len(data) > 12+4+2*n+len(al.Cov.Encode())+2+2*len(pts) {
		t.Errorf("point lists not shared: %d bytes", len(data))
	}

	// only identical lists share
	for i := range al.Points {
		al.Points[i][0] = uint16(i % 3)
	}
	data = roundTripTable(t, &Table{AttachList: al})
	want := 12 + 4 + 2*n + len(al.Cov.Encode()) + 3*(2+2*len(pts))
	if len(data) != want {
		t.Errorf("got %d bytes, want %d", len(data), want)
	}
}

func TestLigCaretListSharing(t *testing.T) {
	// without sharing, the ligature glyphs would need 186 kB
	const n = 3000
	lcl := &LigCaretList{Cov: sequentialCoverage(n), Carets: make([][]CaretValue, n)}
	for i := range lcl.Carets {
		lcl.Carets[i] = distinctCarets(10)
	}
	data := roundTripTable(t, &Table{LigCaretList: lcl})
	if len(data) > 12+4+2*n+len(lcl.Cov.Encode())+2+2*10+4*10 {
		t.Errorf("ligature glyphs not shared: %d bytes", len(data))
	}

	// only identical ligature glyphs share
	for i := range lcl.Carets {
		lcl.Carets[i][0].Coordinate = funit.Int16(100 + i%2)
	}
	data = roundTripTable(t, &Table{LigCaretList: lcl})
	want := 12 + 4 + 2*n + len(lcl.Cov.Encode()) + 2*(2+2*10+4*10)
	if len(data) != want {
		t.Errorf("got %d bytes, want %d", len(data), want)
	}

	// caret values within a ligature glyph
	cp := uint16(3)
	row := make([]CaretValue, 15000)
	for i := range row {
		if i%2 == 0 {
			row[i] = CaretValue{Coordinate: 7}
		} else {
			row[i] = CaretValue{ContourPoint: &cp}
		}
	}
	lcl = &LigCaretList{Cov: coverage.Table{1: 0}, Carets: [][]CaretValue{row}}
	data = roundTripTable(t, &Table{LigCaretList: lcl})
	want = 12 + 4 + 2 + len(lcl.Cov.Encode()) + 2 + 2*len(row) + 2*4
	if len(data) != want {
		t.Errorf("got %d bytes, want %d", len(data), want)
	}
}

// gdefWithList returns the bytes of a GDEF table with either an attachment
// point list (ligCaret false) or a ligature caret list, given as the bytes of
// the list.
func gdefWithList(list []byte, ligCaret bool) []byte {
	data := []byte{0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	if ligCaret {
		data[9] = 12
	} else {
		data[7] = 12
	}
	return append(data, list...)
}

func readGdefBytes(t *testing.T, data []byte) *Table {
	t.Helper()
	table, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}
	return table
}

// TestAttachListUnwritable checks that an attachment point list is dropped
// when its AttachPoint tables overlap so much that they cannot be
// re-encoded.  AttachPoint i starts at word i of a shared array and has
// base+i points.
func TestAttachListUnwritable(t *testing.T) {
	const n = 100
	const base = 2000

	covLen := 10
	regionPos := 4 + 2*n + covLen
	list := []byte{byte((4 + 2*n) >> 8), byte(4 + 2*n), 0, n}
	for i := range n {
		off := regionPos + 2*i
		list = append(list, byte(off>>8), byte(off))
	}
	list = append(list, 0, 2, 0, 1, 0, 1, 0, n, 0, 0) // glyphs 1, ..., n
	for j := range 2*n + base + 10 {
		w := base + j
		list = append(list, byte(w>>8), byte(w))
	}

	table := readGdefBytes(t, gdefWithList(list, false))
	if table.AttachList != nil {
		t.Error("unwritable attachment point list was kept")
	}
}

// TestLigCaretListUnwritable checks that a ligature caret list is dropped
// when its LigGlyph tables are too numerous to be re-encoded.  Each LigGlyph
// table has one caret, and its CaretValue table overlaps the LigGlyph
// table in the input.
func TestLigCaretListUnwritable(t *testing.T) {
	n := 7000

	covLen := 10
	regionPos := 4 + 2*n + covLen
	list := []byte{byte((4 + 2*n) >> 8), byte(4 + 2*n), byte(n >> 8), byte(n)}
	for i := range n {
		off := regionPos + 6*i
		list = append(list, byte(off>>8), byte(off))
	}
	list = append(list, 0, 2, 0, 1, 0, 1, byte(n>>8), byte(n), 0, 0)
	for k := range n {
		// count 1, caret offset 2, then the caret value (format 2, point k)
		list = append(list, 0, 1, 0, 2, byte(k>>8), byte(k))
	}

	table := readGdefBytes(t, gdefWithList(list, true))
	if table.LigCaretList != nil {
		t.Error("unwritable ligature caret list was kept")
	}
}
