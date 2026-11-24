// Copyright 2023 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package sfnt

import (
	"bytes"
	"io"

	"maps"
	"testing"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/vmtx"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/image/font/gofont/goregular"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/header"
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

// TestReadVerticalUnusableTops checks that a font whose glyph tops cannot be
// measured, because of a malformed font matrix, reads without vertical
// metrics rather than failing.  A NaN entry cannot be stored in a CFF font
// matrix; [TestGlyphTop] covers that case.
func TestReadVerticalUnusableTops(t *testing.T) {
	for _, c := range []struct {
		name string
		m    matrix.Matrix
	}{
		{"huge", matrix.Matrix{1e10, 0, 0, 1e10, 0, 0}},
		{"huge negative", matrix.Matrix{1, 0, 0, 1, 0, -1e10}},
	} {
		t.Run(c.name, func(t *testing.T) {
			font := cffFontWithMatrix(c.m)
			g := font.Outlines.(*cff.Outlines).Glyphs[0]
			g.Name = ".notdef"
			g.Width = 0 // fits "hmtx" whatever the matrix
			buf := &bytes.Buffer{}
			if _, err := font.Write(buf); err != nil {
				t.Fatal(err)
			}

			// vertical tables whose bearings are all zero
			info := &vmtx.Info{
				CaretSlopeRun: 1,
				Heights:       []funit.Uint16{1000},
				TSB:           []funit.Int16{0},
			}
			vheaData, vmtxData := info.Encode()
			tables := tablesOf(t, buf.Bytes())
			tables["vhea"] = vheaData
			tables["vmtx"] = vmtxData
			out := &bytes.Buffer{}
			if _, err := header.Write(out, header.ScalerTypeCFF, tables); err != nil {
				t.Fatal(err)
			}

			data := out.Bytes()
			got, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
			if err != nil {
				t.Fatal(err)
			}
			if got.glyphTop(0) != topLimit && got.glyphTop(0) != -topLimit {
				t.Fatalf("glyph top %d is not saturated", got.glyphTop(0))
			}
			if got.Vertical != nil {
				t.Error("vertical metrics kept for an unusable glyph top")
			}
		})
	}
}
