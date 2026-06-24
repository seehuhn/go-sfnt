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
	"reflect"
	"testing"

	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/parser"
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
