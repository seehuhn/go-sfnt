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
