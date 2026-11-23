// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

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
	data := mustEncode(t, table1)
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
		got := hex.EncodeToString(mustEncode(t, table))
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
	data := mustEncode(t, table1)

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
	data := mustEncode(t, table1)
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
	f.Add(mustEncode(f, table))
	table.GlyphClass = classdef.Table{
		2:  GlyphClassBase,
		3:  GlyphClassBase,
		4:  GlyphClassBase,
		10: GlyphClassLigature,
	}
	f.Add(mustEncode(f, table))
	table.MarkAttachClass = classdef.Table{
		5: 1,
		6: 2,
		7: 1,
	}
	f.Add(mustEncode(f, table))
	table.MarkGlyphSets = []coverage.Set{
		{12: true, 13: true, 14: true},
		{10: true, 15: true, 16: true},
	}
	f.Add(mustEncode(f, table))
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
	f.Add(mustEncode(f, table))
	table.ItemVarStore = goldenItemVarStore
	f.Add(mustEncode(f, table))

	f.Fuzz(func(t *testing.T, data []byte) {
		table1, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		if err != nil {
			return
		}

		data2 := mustEncode(t, table1)

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

// mustEncode returns the encoded table, and fails the test if the table
// cannot be encoded.
func mustEncode(tb testing.TB, table *Table) []byte {
	tb.Helper()
	data, err := table.Encode()
	if err != nil {
		tb.Fatal(err)
	}
	return data
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
			data := mustEncode(t, table)
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
		"mark attach":    {LigCaretList: bigCaret, MarkAttachClass: classdef.Table{1: 1}},
		"two big lists":  {GlyphClass: cd, MarkAttachClass: cd},
		"mark set count": {MarkGlyphSets: make([]coverage.Set, 0x10000)},
	}
	for name, table := range tooLarge {
		t.Run(name, func(t *testing.T) {
			if _, err := table.Encode(); err == nil {
				t.Error("Encode succeeded")
			}
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
		data := mustEncode(t, table)
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
	data, err := cd.Append(data)
	if err != nil {
		t.Fatal(err)
	}

	got, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}
	want := &Table{GlyphClass: cd}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unexpected result (-want +got):\n%s", diff)
	}

	// the result can be written
	data2 := mustEncode(t, got)
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
		if _, err := al.layout(); err == nil {
			t.Error("layout succeeded")
		}
	})
	t.Run("points", func(t *testing.T) {
		// the second point list lies behind a first one of about 80 kB
		al := &AttachList{
			Cov:    coverage.Table{1: 0, 2: 1},
			Points: [][]uint16{make([]uint16, 40000), {1}},
		}
		if _, err := al.layout(); err == nil {
			t.Error("layout succeeded")
		}
	})
}

func TestLigCaretListOffsetRange(t *testing.T) {
	t.Run("coverage", func(t *testing.T) {
		const n = 32768
		lcl := &LigCaretList{Cov: coverage.Table{}, Carets: make([][]CaretValue, n)}
		for i := range n {
			lcl.Cov[glyph.ID(i+1)] = i
		}
		if _, err := lcl.layout(); err == nil {
			t.Error("layout succeeded")
		}
	})
	t.Run("ligature glyph", func(t *testing.T) {
		// the second ligature glyph lies behind a first one of about 120 kB
		lcl := &LigCaretList{
			Cov:    coverage.Table{1: 0, 2: 1},
			Carets: [][]CaretValue{distinctCarets(20000), {{Coordinate: 1}}},
		}
		if _, err := lcl.layout(); err == nil {
			t.Error("layout succeeded")
		}
	})
	t.Run("caret value", func(t *testing.T) {
		// a single ligature glyph with so many carets that the last
		// caret values lie beyond 64 kB
		lcl := &LigCaretList{
			Cov:    coverage.Table{1: 0},
			Carets: [][]CaretValue{distinctCarets(11000)},
		}
		if _, err := lcl.layout(); err == nil {
			t.Error("layout succeeded")
		}
	})
	t.Run("within limit", func(t *testing.T) {
		lcl := &LigCaretList{
			Cov:    coverage.Table{1: 0},
			Carets: [][]CaretValue{distinctCarets(10000)},
		}
		table := &Table{LigCaretList: lcl}
		data := mustEncode(t, table)
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
	data := mustEncode(t, table)
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

// be returns the big-endian encoding of a sequence of 16-bit values.
func be(vals ...uint16) []byte {
	res := make([]byte, 0, 2*len(vals))
	for _, v := range vals {
		res = append(res, byte(v>>8), byte(v))
	}
	return res
}

// TestAttachListDuplicateGlyph checks that a coverage table which lists a
// glyph twice leaves the other glyphs with their own attachment points.
func TestAttachListDuplicateGlyph(t *testing.T) {
	covers := map[string][]byte{
		"format 1": be(1, 3, 5, 5, 7),
		"format 2": be(2, 3, 5, 5, 0, 5, 5, 1, 7, 7, 2),
	}
	for name, cov := range covers {
		t.Run(name, func(t *testing.T) {
			pointsPos := uint16(10 + len(cov))
			list := be(10, 3, pointsPos, pointsPos+4, pointsPos+8)
			list = append(list, cov...)
			list = append(list, be(1, 1, 1, 2, 1, 3)...) // points {1}, {2}, {3}

			table1 := readGdefBytes(t, gdefWithList(list, false))
			al := table1.AttachList
			idx, ok := al.Cov[7]
			if !ok {
				t.Fatal("glyph 7 is not covered")
			}
			if diff := cmp.Diff([]uint16{3}, al.Points[idx]); diff != "" {
				t.Errorf("wrong points for glyph 7 (-want +got):\n%s", diff)
			}

			table2 := readGdefBytes(t, mustEncode(t, table1))
			if diff := cmp.Diff(table1, table2); diff != "" {
				t.Errorf("round trip failed (-first +second):\n%s", diff)
			}
		})
	}
}

// TestLigCaretListDuplicateGlyph checks that a coverage table which lists a
// glyph twice leaves the other glyphs with their own caret values.
func TestLigCaretListDuplicateGlyph(t *testing.T) {
	covers := map[string][]byte{
		"format 1": be(1, 3, 5, 5, 7),
		"format 2": be(2, 3, 5, 5, 0, 5, 5, 1, 7, 7, 2),
	}
	for name, cov := range covers {
		t.Run(name, func(t *testing.T) {
			ligPos := uint16(10 + len(cov))
			list := be(10, 3, ligPos, ligPos+8, ligPos+16)
			list = append(list, cov...)
			for coord := range uint16(3) {
				// one caret, as a format 1 caret value at offset 4
				list = append(list, be(1, 4, 1, coord+1)...)
			}

			table1 := readGdefBytes(t, gdefWithList(list, true))
			lcl := table1.LigCaretList
			idx, ok := lcl.Cov[7]
			if !ok {
				t.Fatal("glyph 7 is not covered")
			}
			if diff := cmp.Diff([]CaretValue{{Coordinate: 3}}, lcl.Carets[idx]); diff != "" {
				t.Errorf("wrong carets for glyph 7 (-want +got):\n%s", diff)
			}

			table2 := readGdefBytes(t, mustEncode(t, table1))
			if diff := cmp.Diff(table1, table2); diff != "" {
				t.Errorf("round trip failed (-first +second):\n%s", diff)
			}
		})
	}
}

// TestListShapeError checks that Encode reports an error for an attachment
// point list or a ligature caret list whose coverage table is invalid or does
// not match the data.
func TestListShapeError(t *testing.T) {
	cases := []struct {
		name  string
		table *Table
	}{
		{"attach list too few points", &Table{AttachList: &AttachList{
			Cov:    coverage.Table{5: 0, 7: 1},
			Points: [][]uint16{{1}},
		}}},
		{"attach list invalid coverage", &Table{AttachList: &AttachList{
			Cov:    coverage.Table{5: 1, 7: 0},
			Points: [][]uint16{{1}, {2}},
		}}},
		{"caret list too many carets", &Table{LigCaretList: &LigCaretList{
			Cov:    coverage.Table{5: 0},
			Carets: [][]CaretValue{{{Coordinate: 1}}, {{Coordinate: 2}}},
		}}},
		{"caret list invalid coverage", &Table{LigCaretList: &LigCaretList{
			Cov:    coverage.Table{5: 0, 7: 2},
			Carets: [][]CaretValue{{{Coordinate: 1}}, {{Coordinate: 2}}},
		}}},
	}
	for _, tc := range cases {
		if _, err := tc.table.Encode(); err == nil {
			t.Errorf("%s: no error", tc.name)
		}
	}
}
