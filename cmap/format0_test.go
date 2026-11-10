// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

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

// TestFormat0MacRoman checks that Table.Get translates the Mac Roman codes
// of a Macintosh format 0 subtable to Unicode, as it does for the other
// formats, while GetNoLang keeps the raw codes.
func TestFormat0MacRoman(t *testing.T) {
	s := &Format0{}
	s.Data['A'] = 10
	s.Data[0x80] = 20 // Mac Roman 0x80 is 'Ä'
	key := Key{PlatformID: 1, EncodingID: 0}
	tbl := Table{key: s.Encode(0)}

	sub, err := tbl.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	for r, want := range map[rune]glyph.ID{'A': 10, 'Ä': 20, 0x80: 0} {
		if got := sub.Lookup(r); got != want {
			t.Errorf("Get: Lookup(%U) = %d, want %d", r, got, want)
		}
	}

	raw, err := tbl.GetNoLang(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	for r, want := range map[rune]glyph.ID{'A': 10, 0x80: 20} {
		if got := raw.Lookup(r); got != want {
			t.Errorf("GetNoLang: Lookup(%U) = %d, want %d", r, got, want)
		}
	}
}
