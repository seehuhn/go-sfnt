// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2023  Jochen Voss <voss@seehuhn.de>
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

package sfnt

import (
	"bytes"
	"io"
	"maps"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/image/font/gofont/goregular"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/header"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/parser"
)

// TestEmptyGDEF tests that we can read a font with an empty GDEF table.
// This is a regression test for https://github.com/seehuhn/go-sfnt/issues/1 .
func TestEmptyGDEF(t *testing.T) {
	// create a TrueType font with an empty GDEF table
	r := bytes.NewReader(goregular.TTF)
	info, err := header.Read(r)
	if err != nil {
		t.Fatal(err)
	}
	tables := make(map[string][]byte)
	for name := range info.Toc {
		data, err := info.ReadTableBytes(r, name)
		if err != nil {
			t.Fatal(err)
		}
		tables[name] = data
	}
	tables["GDEF"] = []byte{}
	w := &bytes.Buffer{}
	_, err = header.Write(w, info.ScalerType, tables)
	if err != nil {
		t.Fatal(err)
	}

	// read the font again
	data := w.Bytes()
	r = bytes.NewReader(data)
	_, err = Read(r, parser.NewBudget(int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}
}

// TestShortPostNames tests that a "post" table listing fewer names than the
// font has glyphs is accepted, and that the missing names are left empty.
func TestShortPostNames(t *testing.T) {
	r := bytes.NewReader(goregular.TTF)
	info, err := header.Read(r)
	if err != nil {
		t.Fatal(err)
	}
	tables := make(map[string][]byte)
	for name := range info.Toc {
		data, err := info.ReadTableBytes(r, name)
		if err != nil {
			t.Fatal(err)
		}
		tables[name] = data
	}

	// A version 1.0 "post" table names the 258 glyphs of the standard
	// Macintosh ordering, but the font has more glyphs than that.
	tables["post"] = append([]byte{0x00, 0x01, 0x00, 0x00}, tables["post"][4:32]...)

	w := &bytes.Buffer{}
	_, err = header.Write(w, info.ScalerType, tables)
	if err != nil {
		t.Fatal(err)
	}

	data := w.Bytes()
	font, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}

	outlines, ok := font.Outlines.(*glyf.Outlines)
	if !ok {
		t.Fatalf("unexpected outline type %T", font.Outlines)
	}
	if len(outlines.Names) != font.NumGlyphs() {
		t.Errorf("got %d names, want %d", len(outlines.Names), font.NumGlyphs())
	}
	if font.NumGlyphs() <= 258 {
		t.Fatal("test font has too few glyphs")
	}
	if name := font.GlyphName(glyph.ID(font.NumGlyphs() - 1)); name != "" {
		t.Errorf("unnamed glyph: got %q, want \"\"", name)
	}

	// the font must still be writable
	if _, err := font.Write(io.Discard); err != nil {
		t.Error(err)
	}
}

// TestPutFormat4 checks where putFormat4 stores mappings which do or do not
// fit into a format 4 subtable.
func TestPutFormat4(t *testing.T) {
	small := cmap.Format4{'A': 1}
	// irregular glyph IDs, which format 4 stores in its glyph ID array
	large := cmap.Format4{}
	for i := range 40000 {
		large[uint16(0x20+i)] = glyph.ID(1 + i%2)
	}

	k10 := cmap.Key{PlatformID: 1, EncodingID: 0}
	k31 := cmap.Key{PlatformID: 3, EncodingID: 1}
	k310 := cmap.Key{PlatformID: 3, EncodingID: 10}
	other := []byte{0, 12}

	cases := []struct {
		name string
		key  cmap.Key
		c    cmap.Format4
		orig cmap.Table
		out  cmap.Table
		want map[cmap.Key]uint16 // the format of each subtable in out
	}{
		{"fits", k31, small, cmap.Table{}, cmap.Table{},
			map[cmap.Key]uint16{k31: 4}},
		{"moved", k31, large, cmap.Table{}, cmap.Table{},
			map[cmap.Key]uint16{k310: 12}},
		{"same key", k310, large, cmap.Table{}, cmap.Table{},
			map[cmap.Key]uint16{k310: 12}},
		{"taken in orig", k31, large, cmap.Table{k310: other}, cmap.Table{},
			map[cmap.Key]uint16{}},
		{"taken in out", k31, large, cmap.Table{}, cmap.Table{k310: other},
			map[cmap.Key]uint16{k310: 12}},
		{"no format 12 key", k10, large, cmap.Table{}, cmap.Table{},
			map[cmap.Key]uint16{}},
		{"no format 12 key, symbol", cmap.Key{PlatformID: 3, EncodingID: 0}, large,
			cmap.Table{}, cmap.Table{}, map[cmap.Key]uint16{}},
		{"unicode platform", cmap.Key{PlatformID: 0, EncodingID: 3}, large,
			cmap.Table{}, cmap.Table{},
			map[cmap.Key]uint16{{PlatformID: 0, EncodingID: 4}: 12}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := maps.Clone(tc.out)
			putFormat4(tc.out, tc.key, tc.c, tc.orig)
			got := map[cmap.Key]uint16{}
			for key, data := range tc.out {
				got[key] = uint16(data[0])<<8 | uint16(data[1])
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("wrong subtables (-want +got):\n%s", diff)
			}
			for key, data := range before {
				if !bytes.Equal(tc.out[key], data) {
					t.Errorf("subtable %v was replaced", key)
				}
			}
		})
	}
}

// TestDropInvalidGsubGlyphs checks that substitutions producing glyphs
// outside the font are removed, while valid substitutions are kept.
func TestDropInvalidGsubGlyphs(t *testing.T) {
	const n = 10 // glyphs 0, ..., 9 exist
	subtables := []gtab.Subtable{
		&gtab.Gsub1_1{Cov: coverage.Set{1: true, 2: true, 8: true}, Delta: 2},
		&gtab.Gsub1_2{
			Cov:                coverage.Table{1: 0, 2: 1, 3: 2},
			SubstituteGlyphIDs: []glyph.ID{4, n, 5},
		},
		&gtab.Gsub2_1{
			Cov:  coverage.Table{1: 0, 2: 1},
			Repl: [][]glyph.ID{{3, n + 1}, {4, 5}},
		},
		&gtab.Gsub3_1{
			Cov:        coverage.Table{1: 0, 2: 1, 3: 2},
			Alternates: [][]glyph.ID{{n, 4, n + 1, 5}, {n}, {6}},
		},
		&gtab.Gsub4_1{
			Cov: coverage.Table{1: 0, 2: 1},
			Repl: [][]gtab.Ligature{
				{{In: []glyph.ID{2}, Out: n}, {In: []glyph.ID{3}, Out: 6}},
				{{In: []glyph.ID{n}, Out: n + 2}},
			},
		},
		&gtab.Gsub8_1{
			Input:              coverage.Table{1: 0, 2: 1},
			Backtrack:          []coverage.Table{{3: 0}},
			SubstituteGlyphIDs: []glyph.ID{n, 4},
		},
	}
	// subtables which are left empty are removed, others are kept
	valid := &gtab.Gsub1_2{Cov: coverage.Table{1: 0}, SubstituteGlyphIDs: []glyph.ID{2}}
	context := &gtab.SeqContext1{Cov: coverage.Table{}}
	mixed := []gtab.Subtable{
		&gtab.Gsub1_1{Cov: coverage.Set{9: true}, Delta: 1},
		valid,
		&gtab.Gsub1_2{Cov: coverage.Table{1: 0}, SubstituteGlyphIDs: []glyph.ID{n}},
		&gtab.Gsub2_1{Cov: coverage.Table{1: 0}, Repl: [][]glyph.ID{{n}}},
		&gtab.Gsub3_1{Cov: coverage.Table{1: 0}, Alternates: [][]glyph.ID{{n}}},
		&gtab.Gsub4_1{Cov: coverage.Table{1: 0}, Repl: [][]gtab.Ligature{{{Out: n}}}},
		&gtab.Gsub8_1{Input: coverage.Table{1: 0}, SubstituteGlyphIDs: []glyph.ID{n}},
		context,
	}

	info := &gtab.Info{LookupList: gtab.LookupList{
		{Meta: &gtab.LookupMetaInfo{}, Subtables: subtables},
		{Meta: &gtab.LookupMetaInfo{}, Subtables: mixed},
	}}

	dropInvalidGsubGlyphs(info, n)

	if got := info.LookupList[1].Subtables; len(got) != 2 || got[0] != valid || got[1] != context {
		t.Errorf("wrong subtables kept: %v", got)
	}

	want := []gtab.Subtable{
		&gtab.Gsub1_1{Cov: coverage.Set{1: true, 2: true}, Delta: 2},
		&gtab.Gsub1_2{
			Cov:                coverage.Table{1: 0, 3: 1},
			SubstituteGlyphIDs: []glyph.ID{4, 5},
		},
		&gtab.Gsub2_1{
			Cov:  coverage.Table{2: 0},
			Repl: [][]glyph.ID{{4, 5}},
		},
		&gtab.Gsub3_1{
			Cov:        coverage.Table{1: 0, 3: 1},
			Alternates: [][]glyph.ID{{4, 5}, {6}},
		},
		&gtab.Gsub4_1{
			Cov:  coverage.Table{1: 0},
			Repl: [][]gtab.Ligature{{{In: []glyph.ID{3}, Out: 6}}},
		},
		&gtab.Gsub8_1{
			Input:              coverage.Table{2: 0},
			Backtrack:          []coverage.Table{{3: 0}},
			SubstituteGlyphIDs: []glyph.ID{4},
		},
	}
	if diff := cmp.Diff(want, []gtab.Subtable(info.LookupList[0].Subtables)); diff != "" {
		t.Errorf("wrong subtables (-want +got):\n%s", diff)
	}
}

// TestDropInvalidGsubGlyphsUnchanged checks that subtables without invalid
// outputs are left as they are.
func TestDropInvalidGsubGlyphsUnchanged(t *testing.T) {
	sub := &gtab.Gsub3_1{
		Cov:        coverage.Table{1: 0, 2: 1},
		Alternates: [][]glyph.ID{{3, 4}, {5}},
	}
	cov, alts := sub.Cov, sub.Alternates
	info := &gtab.Info{LookupList: gtab.LookupList{{
		Meta:      &gtab.LookupMetaInfo{},
		Subtables: []gtab.Subtable{sub},
	}}}

	dropInvalidGsubGlyphs(info, 6)

	if !maps.Equal(sub.Cov, cov) || &sub.Alternates[0] != &alts[0] {
		t.Error("valid subtable was modified")
	}
	dropInvalidGsubGlyphs(nil, 6) // must not panic
}
