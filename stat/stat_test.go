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

package stat

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/parser"
)

func sampleTable() *Table {
	return &Table{
		DesignAxes: []DesignAxis{
			{Tag: "wght", NameID: 256, Ordering: 0},
			{Tag: "wdth", NameID: 257, Ordering: 1},
		},
		AxisValues: []AxisValue{
			&Format1{AxisIndex: 0, Flags: 0, NameID: 258, Value: 400},
			&Format2{AxisIndex: 0, Flags: 0, NameID: 259, Nominal: 400, Min: 100, Max: 900},
			&Format3{AxisIndex: 0, Flags: 2, NameID: 260, Value: 700, LinkedValue: 900},
			&Format4{Flags: 0, NameID: 261, Values: []AxisValueEntry{
				{AxisIndex: 0, Value: 400},
				{AxisIndex: 1, Value: 100},
			}},
		},
		ElidedFallbackNameID: 2,
	}
}

func TestRoundTrip(t *testing.T) {
	t1 := sampleTable()
	data := t1.Encode()

	t2, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(t1, t2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

func TestEncodeVersion(t *testing.T) {
	data := sampleTable().Encode()
	if len(data) < 4 {
		t.Fatalf("encoded table too short: %d bytes", len(data))
	}
	major := uint16(data[0])<<8 | uint16(data[1])
	minor := uint16(data[2])<<8 | uint16(data[3])
	if major != 1 || minor != 2 {
		t.Errorf("version = %d.%d, want 1.2", major, minor)
	}
}

// TestMinorVersion0 checks that a minor-version-0 table (no
// elidedFallbackNameID field) decodes with a synthesized zero ID.
func TestMinorVersion0(t *testing.T) {
	var buf []byte
	buf = appendU16(buf, 1)  // majorVersion
	buf = appendU16(buf, 0)  // minorVersion
	buf = appendU16(buf, 8)  // designAxisSize
	buf = appendU16(buf, 1)  // designAxisCount
	buf = appendU32(buf, 18) // designAxesOffset
	buf = appendU16(buf, 0)  // axisValueCount
	buf = appendU32(buf, 0)  // offsetToAxisValueOffsets

	// design axis record
	buf = appendTag(buf, "wght")
	buf = appendU16(buf, 256)
	buf = appendU16(buf, 0)

	tbl, err := Read(bytes.NewReader(buf), parser.NewBudget(int64(len(buf))))
	if err != nil {
		t.Fatal(err)
	}
	if tbl.ElidedFallbackNameID != 0 {
		t.Errorf("ElidedFallbackNameID = %d, want 0", tbl.ElidedFallbackNameID)
	}
	want := []DesignAxis{{Tag: "wght", NameID: 256, Ordering: 0}}
	if diff := cmp.Diff(want, tbl.DesignAxes); diff != "" {
		t.Errorf("design axes mismatch (-want +got):\n%s", diff)
	}
}

// TestUnknownAxisValueFormatSkipped checks that an axis value record with
// an unrecognized format is dropped rather than causing a read error.
func TestUnknownAxisValueFormatSkipped(t *testing.T) {
	// header: majorVersion, minorVersion, designAxisSize, designAxisCount,
	// designAxesOffset, axisValueCount, offsetToAxisValueOffsets,
	// elidedFallbackNameID
	const headerLen = 20
	const offsetsOffset = headerLen // no design axes
	const avOffsetsLen = 2 * 2      // two axis value offsets

	var buf []byte
	buf = appendU16(buf, 1) // majorVersion
	buf = appendU16(buf, 2) // minorVersion
	buf = appendU16(buf, 8) // designAxisSize
	buf = appendU16(buf, 0) // designAxisCount
	buf = appendU32(buf, 0) // designAxesOffset
	buf = appendU16(buf, 2) // axisValueCount
	buf = appendU32(buf, offsetsOffset)
	buf = appendU16(buf, 3) // elidedFallbackNameID

	if len(buf) != headerLen {
		t.Fatalf("header length = %d, want %d", len(buf), headerLen)
	}

	// axis value offsets, relative to offsetsOffset
	firstOff := avOffsetsLen
	secondOff := avOffsetsLen + 99 // unknown format record, 6 bytes below
	buf = appendU16(buf, uint16(firstOff))
	buf = appendU16(buf, uint16(secondOff))

	// first record: recognized format 1
	buf = appendU16(buf, 1) // format
	buf = appendU16(buf, 0) // axisIndex
	buf = appendU16(buf, 0) // flags
	buf = appendU16(buf, 5) // nameID
	buf = appendU32(buf, uint32(400<<16))

	// pad up to the second record's declared offset
	for len(buf) < headerLen+secondOff {
		buf = append(buf, 0)
	}

	// second record: unrecognized format 99
	buf = appendU16(buf, 99) // format
	buf = appendU16(buf, 0)
	buf = appendU16(buf, 0)
	buf = appendU16(buf, 0)

	tbl, err := Read(bytes.NewReader(buf), parser.NewBudget(int64(len(buf))))
	if err != nil {
		t.Fatal(err)
	}
	if len(tbl.AxisValues) != 1 {
		t.Fatalf("len(AxisValues) = %d, want 1", len(tbl.AxisValues))
	}
	f1, ok := tbl.AxisValues[0].(*Format1)
	if !ok {
		t.Fatalf("AxisValues[0] has type %T, want *Format1", tbl.AxisValues[0])
	}
	if f1.NameID != 5 || f1.Value != 400 {
		t.Errorf("got %+v, want NameID=5 Value=400", f1)
	}
}
