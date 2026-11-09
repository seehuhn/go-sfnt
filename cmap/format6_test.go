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

package cmap

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/glyph"
)

// format6Data builds a format 6 subtable from the glyph IDs of consecutive
// character codes, starting at first.
func format6Data(first uint16, gids ...uint16) []byte {
	L := 10 + 2*len(gids)
	data := []byte{0, 6, byte(L >> 8), byte(L), 0, 0, byte(first >> 8), byte(first), byte(len(gids) >> 8), byte(len(gids))}
	for _, g := range gids {
		data = append(data, byte(g>>8), byte(g))
	}
	return data
}

func TestFormat6(t *testing.T) {
	data := format6Data(0x41, 10, 0, 12)
	c, err := decodeFormat6(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := &Format6{FirstCode: 0x41, GlyphIDs: []glyph.ID{10, 0, 12}}
	if diff := cmp.Diff(want, c); diff != "" {
		t.Fatalf("wrong subtable (-want +got):\n%s", diff)
	}

	for r, want := range map[rune]glyph.ID{0x40: 0, 0x41: 10, 0x42: 0, 0x43: 12, 0x44: 0, -1: 0} {
		if got := c.Lookup(r); got != want {
			t.Errorf("Lookup(%U) = %d, want %d", r, got, want)
		}
	}
	if low, high := c.CodeRange(); low != 0x41 || high != 0x43 {
		t.Errorf("CodeRange() = %d, %d, want 65, 67", low, high)
	}
	if low, high := (&Format6{FirstCode: 5}).CodeRange(); low != 0 || high != 0 {
		t.Errorf("empty CodeRange() = %d, %d, want 0, 0", low, high)
	}

	if diff := cmp.Diff(data, c.Encode(0)); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// TestFormat6CodeOverflow checks that entries for character codes beyond
// 0xFFFF are dropped.
func TestFormat6CodeOverflow(t *testing.T) {
	c, err := decodeFormat6(format6Data(0xFFFE, 1, 2, 3, 4), nil)
	if err != nil {
		t.Fatal(err)
	}
	want := &Format6{FirstCode: 0xFFFE, GlyphIDs: []glyph.ID{1, 2}}
	if diff := cmp.Diff(want, c); diff != "" {
		t.Errorf("wrong subtable (-want +got):\n%s", diff)
	}
}

// TestFormat6Oversized checks that a subtable with more entries than its
// length field can describe is rejected.
func TestFormat6Oversized(t *testing.T) {
	gids := make([]uint16, maxFormat6Entries+1)
	if _, err := decodeFormat6(format6Data(0x20, gids...), nil); err == nil {
		t.Error("oversized subtable was accepted")
	}
}

// TestFormat6EncodeTooLarge checks that Encode panics for subtables which
// cannot be represented in the binary format.
func TestFormat6EncodeTooLarge(t *testing.T) {
	cases := []*Format6{
		{GlyphIDs: make([]glyph.ID, maxFormat6Entries+1)},
		{FirstCode: 0xFFFF, GlyphIDs: make([]glyph.ID, 2)},
	}
	for i, c := range cases {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%d: Encode did not panic", i)
				}
			}()
			c.Encode(0)
		}()
	}

	// the largest valid subtables
	(&Format6{GlyphIDs: make([]glyph.ID, maxFormat6Entries)}).Encode(0)
	(&Format6{FirstCode: 0xFFFF, GlyphIDs: make([]glyph.ID, 1)}).Encode(0)
}

// TestFormat6MacRoman checks that Table.Get translates the Mac Roman codes
// of a Macintosh format 6 subtable to Unicode, while GetNoLang keeps the raw
// codes.
func TestFormat6MacRoman(t *testing.T) {
	key := Key{PlatformID: 1, EncodingID: 0}
	tbl := Table{key: format6Data(0x7F, 10, 20)} // Mac Roman 0x80 is 'Ä'

	sub, err := tbl.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	for r, want := range map[rune]glyph.ID{0x7F: 10, 'Ä': 20, 0x80: 0} {
		if got := sub.Lookup(r); got != want {
			t.Errorf("Get: Lookup(%U) = %d, want %d", r, got, want)
		}
	}

	raw, err := tbl.GetNoLang(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := raw.(*Format6); !ok {
		t.Fatalf("GetNoLang: unexpected type %T", raw)
	}
	for r, want := range map[rune]glyph.ID{0x7F: 10, 0x80: 20} {
		if got := raw.Lookup(r); got != want {
			t.Errorf("GetNoLang: Lookup(%U) = %d, want %d", r, got, want)
		}
	}
}

func FuzzFormat6(f *testing.F) {
	f.Add(format6Data(0x41, 10, 0, 12))
	f.Add(format6Data(0xFFFE, 1, 2, 3))
	f.Add(append(format6Data(0, 5), 0, 0)) // excess 0x0000 at the end
	f.Add(format6Data(7))

	f.Fuzz(func(t *testing.T, data []byte) {
		c1, err := decodeFormat6(data, nil)
		if err != nil {
			return
		}
		f6, ok := c1.(*Format6)
		if !ok {
			return
		}

		data2 := f6.Encode(0)
		c2, err := decodeFormat6(data2, nil)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(c1, c2); diff != "" {
			t.Errorf("round trip failed (-want +got):\n%s", diff)
		}
	})
}
