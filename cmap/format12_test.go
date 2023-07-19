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
