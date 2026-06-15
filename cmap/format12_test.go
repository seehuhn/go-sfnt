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

package cmap

import (
	"reflect"
	"testing"

	"seehuhn.de/go/sfnt/glyph"
)

func TestFormat12(t *testing.T) {
	in := []byte{
		0, 12, // uint16 format
		0, 0, // uint16 reserved
		0, 0, 0, 16 + 12 + 12, // uint32 length
		0, 0, 0, 0, // uint32 language
		0, 0, 0, 2, // uint32 numGroups

		0, 0, 0, 'A', // uint32 startCharCode
		0, 0, 0, 'Z', // uint32 endCharCode
		0, 0, 0, 2, // uint32 startGlyphID

		0, 0, 0, '~', // uint32 startCharCode
		0, 0, 0, '~', // uint32 endCharCode
		0, 0, 0, 1, // uint32 startGlyphID
	}
	cmap, err := decodeFormat12(in, nil)
	if err != nil {
		t.Fatal(err)
	}

	type testCase struct {
		code uint32
		gid  glyph.ID
	}
	cases := []testCase{
		{'A', 2},
		{'B', 3},
		{'Z', 27},
		{'a', 0},
		{'~', 1},
	}
	for _, c := range cases {
		gid := cmap.Lookup(rune(c.code))
		if gid != c.gid {
			t.Errorf("Lookup(%d)=%d, want %d", c.code, gid, c.gid)
		}
	}
}

// TestFormat12GlyphRange checks that segments whose glyph IDs do not fit in
// the uint16 glyph.ID range are rejected rather than silently wrapped around.
func TestFormat12GlyphRange(t *testing.T) {
	subtable := func(startGlyphID, endCharCode uint32) []byte {
		data := make([]byte, 28)
		data[1] = 12                     // format
		data[7] = 28                     // length
		data[15] = 1                     // numGroups
		put := func(off int, v uint32) { // big-endian uint32
			data[off] = byte(v >> 24)
			data[off+1] = byte(v >> 16)
			data[off+2] = byte(v >> 8)
			data[off+3] = byte(v)
		}
		put(16, 0)           // startCharCode
		put(20, endCharCode) // endCharCode
		put(24, startGlyphID)
		return data
	}

	cases := []struct {
		desc       string
		startGID   uint32
		endChar    uint32
		wantReject bool
	}{
		{"max valid glyph", 0xFFFF, 0, false},
		{"range up to max glyph", 0, 0xFFFF, false},
		{"start glyph overflows uint16", 0x1_0000, 0, true},
		{"range overflows uint16", 1, 0xFFFF, true},
	}
	for _, c := range cases {
		_, err := decodeFormat12(subtable(c.startGID, c.endChar), nil)
		if c.wantReject && err == nil {
			t.Errorf("%s: accepted, want rejected", c.desc)
		}
		if !c.wantReject && err != nil {
			t.Errorf("%s: rejected (%v), want accepted", c.desc, err)
		}
	}
}

func FuzzFormat12(f *testing.F) {
	f.Add(Format12{}.Encode(0))
	f.Add(Format12{
		1: 1,
	}.Encode(0))
	f.Add(Format12{
		65:  1,
		66:  2,
		67:  3,
		100: 4,
	}.Encode(0))
	f.Add(Format12{
		1: 3,
		2: 2,
		3: 1,
	}.Encode(0))

	f.Fuzz(func(t *testing.T, data []byte) {
		c1, err := decodeFormat12(data, nil)
		if err != nil {
			return
		}

		data2 := c1.Encode(0)
		if len(data2) > len(data) {
			t.Error("too long")
		}

		c2, err := decodeFormat12(data2, nil)
		if err != nil {
			t.Error(err)
		}

		if !reflect.DeepEqual(c1, c2) {
			t.Error("not equal")
		}
	})
}

var _ Subtable = Format12(nil)
