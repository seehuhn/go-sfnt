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

	"seehuhn.de/go/sfnt/glyph"
)

func TestFormat0(t *testing.T) {
	s := &Format0{}
	s.Data[0] = 0
	s.Data[65] = 10
	s.Data[200] = 20
	s.Data[255] = 30

	t.Run("lookup", func(t *testing.T) {
		// each byte code maps to the glyph in Data[code]; codes above 255 map
		// to .notdef (0)
		checks := []struct {
			r    rune
			want glyph.ID
		}{
			{0, 0}, {65, 10}, {200, 20}, {255, 30},
			{256, 0}, {0x10000, 0},
		}
		for _, c := range checks {
			if got := s.Lookup(c.r); got != c.want {
				t.Errorf("Lookup(%d) = %d, want %d", c.r, got, c.want)
			}
		}
	})

	t.Run("code range", func(t *testing.T) {
		if low, high := s.CodeRange(); low != 0 || high != 255 {
			t.Errorf("CodeRange() = (%d, %d), want (0, 255)", low, high)
		}
	})

	t.Run("round trip via public Decode", func(t *testing.T) {
		tbl := Table{
			{PlatformID: 1, EncodingID: 0}: s.Encode(0),
		}
		dec, err := Decode(tbl.Encode())
		if err != nil {
			t.Fatal(err)
		}
		sub, err := dec.GetNoLang(1, 0)
		if err != nil {
			t.Fatal(err)
		}
		// every code must map to the same glyph after the round trip
		for code := rune(0); code <= 255; code++ {
			if got, want := sub.Lookup(code), s.Lookup(code); got != want {
				t.Errorf("after round trip, Lookup(%d) = %d, want %d", code, got, want)
			}
		}
	})

	t.Run("rejects wrong-size subtable", func(t *testing.T) {
		// a format-0 subtable must hold exactly 256 glyph bytes after the
		// 6-byte header
		short := make([]byte, 6+255)
		if _, err := decodeFormat0(short, nil); err == nil {
			t.Error("expected an error for a 255-byte glyph array")
		}
	})
}
