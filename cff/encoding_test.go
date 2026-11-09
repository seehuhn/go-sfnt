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

package cff

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
)

func FuzzEncoding(f *testing.F) {
	ss := &cffStrings{}
	var cc []int32

	var glyphs []*Glyph
	for i := range 258 {
		var name string
		if i == 0 {
			name = ".notdef"
		} else if i >= 'A' && i <= 'Z' {
			name = string([]rune{rune(i)})
		} else {
			name = fmt.Sprintf("%d", i)
		}
		glyphs = append(glyphs, &Glyph{Name: name})
		cc = append(cc, ss.lookup(name))
	}

	f.Add([]byte{0, 3, 'A', 'B', 'A'})                          // duplicate code, format 0
	f.Add([]byte{1, 2, 'A', 1, 'B', 1})                         // overlapping ranges, format 1
	f.Add([]byte{0x80, 1, 'A', 1, 'A', 0, 'B'})                 // supplement for assigned code
	f.Add([]byte{0x81, 1, 'A', 0, 2, 'C', 0, 'B', 'C', 0, 'B'}) // repeated supplement code

	f.Fuzz(func(t *testing.T, data1 []byte) {
		p := parser.New(bytes.NewReader(data1), parser.NewBudget(int64(len(data1))))
		enc1, err := readEncoding(p, cc)
		if err != nil {
			return
		}

		var enc2 []glyph.ID
		var data2 []byte
		if isStandardEncoding(enc1, glyphs) {
			enc2 = StandardEncoding(glyphs)
		} else if isExpertEncoding(enc1, glyphs) {
			enc2 = expertEncoding(glyphs)
		} else {
			data2, err = encodeEncoding(enc1, cc)
			if err != nil {
				t.Fatal(err)
			}

			p = parser.New(bytes.NewReader(data2), parser.NewBudget(int64(len(data2))))
			enc2, err = readEncoding(p, cc)
			if err != nil {
				t.Fatal(err)
			}
		}

		if diff := cmp.Diff(enc1, enc2); diff != "" {
			t.Errorf("different (-old +new):\n%s", diff)
		}
	})
}

// TestEncodingSparse checks that encodings in which unencoded glyphs sit
// between encoded ones survive a round trip.
func TestEncodingSparse(t *testing.T) {
	ss := &cffStrings{}
	var names []int32
	for i := range 300 {
		name := ".notdef"
		if i > 0 {
			name = fmt.Sprintf("g%d", i)
		}
		names = append(names, ss.lookup(name))
	}

	makeEnc := func(m map[int]glyph.ID) []glyph.ID {
		enc := make([]glyph.ID, 256)
		for code, gid := range m {
			enc[code] = gid
		}
		return enc
	}
	allCodes := func(gid func(code int) glyph.ID) []glyph.ID {
		enc := make([]glyph.ID, 256)
		for code := range enc {
			enc[code] = gid(code)
		}
		return enc
	}

	cases := []struct {
		name string
		enc  []glyph.ID
	}{
		{"empty", make([]glyph.ID, 256)},
		{"gap", makeEnc(map[int]glyph.ID{65: 1, 66: 3, 67: 4})},
		{"gap with multiple codes", makeEnc(map[int]glyph.ID{65: 1, 66: 3, 67: 3, 200: 1, 10: 7})},
		{"first glyph unencoded", makeEnc(map[int]glyph.ID{32: 2, 33: 3, 34: 4})},
		{"reverse order", makeEnc(map[int]glyph.ID{1: 5, 2: 4, 3: 2, 4: 1})},
		{"high glyph IDs", makeEnc(map[int]glyph.ID{0: 1, 1: 2, 2: 299, 255: 298})},
		{"all codes, in order", allCodes(func(c int) glyph.ID { return glyph.ID(c + 1) })},
		{"all codes, reversed", allCodes(func(c int) glyph.ID { return glyph.ID(256 - c) })},
		{"all codes, one glyph", allCodes(func(int) glyph.ID { return 1 })},
		{"all codes, glyph 1 unencoded, 255 codes", allCodes(func(c int) glyph.ID {
			if c == 0 {
				return 0
			}
			return glyph.ID(c + 1)
		})},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data, err := encodeEncoding(c.enc, names)
			if err != nil {
				t.Fatal(err)
			}
			p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
			got, err := readEncoding(p, names)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(c.enc, got); diff != "" {
				t.Errorf("round trip failed (-want +got):\n%s", diff)
			}
		})
	}
}

// TestEncodingUnrepresentable checks that encodings which the CFF encoding
// formats cannot express are rejected.
func TestEncodingUnrepresentable(t *testing.T) {
	ss := &cffStrings{}
	names := []int32{
		ss.lookup(".notdef"),
		ss.lookup("A"),
		ss.lookup("dup"),
		ss.lookup("dup"),
	}

	cases := []struct {
		name string
		enc  map[int]glyph.ID
	}{
		// glyph 2 shares its name with glyph 3, so a supplement naming it
		// would resolve to glyph 3
		{"supplement for shadowed name", map[int]glyph.ID{65: 1, 66: 2, 67: 2}},
		{"shadowed name beyond gap", map[int]glyph.ID{65: 2}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			enc := make([]glyph.ID, 256)
			for code, gid := range c.enc {
				enc[code] = gid
			}
			_, err := encodeEncoding(enc, names)
			if err == nil {
				t.Error("expected an error")
			}
		})
	}

	// 256 supplements do not fit
	enc := make([]glyph.ID, 256)
	for code := range enc {
		enc[code] = 2
	}
	_, err := encodeEncoding(enc, names[:3])
	if err == nil {
		t.Error("expected an error for 256 supplements")
	}
}

// TestEncodingDuplicateCodes checks that the first assignment of a code wins
// and that the result survives a round trip.
func TestEncodingDuplicateCodes(t *testing.T) {
	names := []int32{0, 1, 2, 3, 4}
	cases := []struct {
		name string
		data []byte
		want map[int]glyph.ID
	}{
		{"format 0", []byte{0, 3, 'A', 'B', 'A'}, map[int]glyph.ID{'A': 1, 'B': 2}},
		{"format 1", []byte{1, 2, 'A', 1, 'B', 1}, map[int]glyph.ID{'A': 1, 'B': 2, 'C': 4}},
		{"supplement", []byte{0x80, 1, 'A', 1, 'A', 0, 2}, map[int]glyph.ID{'A': 1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := parser.New(bytes.NewReader(c.data), parser.NewBudget(int64(len(c.data))))
			enc, err := readEncoding(p, names)
			if err != nil {
				t.Fatal(err)
			}
			want := make([]glyph.ID, 256)
			for code, gid := range c.want {
				want[code] = gid
			}
			if d := cmp.Diff(want, enc); d != "" {
				t.Errorf("wrong encoding (-want +got):\n%s", d)
			}

			data, err := encodeEncoding(enc, names)
			if err != nil {
				t.Fatal(err)
			}
			p = parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
			enc2, err := readEncoding(p, names)
			if err != nil {
				t.Fatal(err)
			}
			if d := cmp.Diff(enc, enc2); d != "" {
				t.Errorf("round trip failed (-want +got):\n%s", d)
			}
		})
	}
}
