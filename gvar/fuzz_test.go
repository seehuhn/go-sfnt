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

package gvar

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/variation"
)

func FuzzGvar(f *testing.F) {
	// short-offset seed
	if enc, err := (&Table{
		AxisCount:    2,
		SharedTuples: sharedGolden,
		PerGlyph: []GlyphData{
			{Data: []byte{0x00, 0x00, 0x00, 0x02}},
			{},
			{Data: []byte{0x00, 0x00}},
		},
	}).Encode(); err == nil {
		f.Add(enc)
	}

	// long-offset seed with an odd-length block
	if enc, err := (&Table{
		AxisCount: 1,
		PerGlyph: []GlyphData{
			{Data: []byte{0x01, 0x02, 0x03}},
			{Data: []byte{0x04, 0x05}},
		},
	}).Encode(); err == nil {
		f.Add(enc)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// budget proportional to the input bounds memory use
		t1, err := Decode(data, parser.NewBudget(int64(len(data))))
		if err != nil {
			return
		}

		encoded, err := t1.Encode()
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}
		t2, err := Decode(encoded, parser.NewBudget(int64(len(data))))
		if err != nil {
			t.Fatalf("re-decode failed: %v", err)
		}
		if diff := cmp.Diff(t1, t2); diff != "" {
			t.Errorf("round trip failed (-first +second):\n%s", diff)
		}
	})
}

// FuzzApply decodes arbitrary gvar data and applies it to a tiny synthetic
// glyph slice at fixed coordinates under a small work budget.  The contract is
// that Apply never panics and honours the work budget on malformed input.
func FuzzApply(f *testing.F) {
	if enc, err := (&Table{
		AxisCount:    2,
		SharedTuples: sharedGolden,
		PerGlyph:     []GlyphData{{Data: buildBlock0(f)}, {}, {}},
	}).Encode(); err == nil {
		f.Add(enc)
	}

	// synthetic glyphs: a simple triangle, a composite, and an empty glyph
	simple := &glyf.SimpleUnpacked{Contours: []glyf.Contour{{
		{X: 0, Y: 0, OnCurve: true},
		{X: 100, Y: 0, OnCurve: true},
		{X: 50, Y: 100, OnCurve: true},
	}}}
	simpleGlyph := simple.AsGlyph()
	composite := &glyf.Glyph{
		Data: glyf.CompositeGlyph{Components: []glyf.GlyphComponent{
			{Flags: glyf.FlagArgsAreXYValues, GlyphIndex: 0, Data: []byte{0x7f, 0x00}},
			{Flags: 0, GlyphIndex: 0, Data: []byte{0x01, 0x02}},
		}},
	}
	glyphs := glyf.Glyphs{&simpleGlyph, composite, nil}
	widths := []funit.Uint16{100, 200, 300}

	f.Fuzz(func(t *testing.T, data []byte) {
		tbl, err := Decode(data, parser.NewBudget(int64(len(data))))
		if err != nil {
			return
		}
		coords := make([]variation.F2Dot14, tbl.AxisCount)
		for i := range coords {
			coords[i] = 0x2000
		}
		for gid := range glyphs {
			wb := int64(64)
			budget := parser.NewBudget(int64(len(data)) + 1<<16)
			_, _ = tbl.Apply(glyphs, widths, glyph.ID(gid), coords, budget, &wb)
		}
	})
}

// buildBlock0 encodes glyph 0's tuples for the FuzzApply seed.
func buildBlock0(f *testing.F) []byte {
	f.Helper()
	b, err := variation.EncodeTupleData(glyph0Tuples, 2, 2, glyph0Points, sharedGolden)
	if err != nil {
		f.Fatal(err)
	}
	return b
}
