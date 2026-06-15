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
	"fmt"
	"math"
	"testing"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/postscript/cid"
	"seehuhn.de/go/postscript/type1"
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/header"
	"seehuhn.de/go/sfnt/hmtx"
	"seehuhn.de/go/sfnt/parser"
)

// TestFDSelect tests that the FDSelect function in subsetted fonts is correct.
func TestFDSelect(t *testing.T) {
	// Construct a CID-keyed CFF font with several FDs.
	o1 := &cff.Outlines{}
	for i := range 10 {
		g := cff.NewGlyph(fmt.Sprintf("orig%d", i), 100*float64(i))
		g.MoveTo(0, 0)
		g.LineTo(100*float64(i), 0)
		g.LineTo(100*float64(i), 500)
		g.LineTo(0, 500)
		o1.Glyphs = append(o1.Glyphs, g)
		o1.Private = append(o1.Private, &type1.PrivateDict{
			StdHW: float64(10*i + 1),
		})
		o1.FontMatrices = append(o1.FontMatrices, matrix.Identity)
	}
	o1.FDSelect = func(gid glyph.ID) int {
		return int(gid) % 10
	}
	o1.ROS = &cid.SystemInfo{
		Registry:   "Seehuhn",
		Ordering:   "Sonderbar",
		Supplement: 0,
	}
	o1.GIDToCID = make([]cid.CID, 10)
	for i := range 10 {
		o1.GIDToCID[i] = cid.CID(i)
	}
	i1 := &Font{
		FamilyName: "Test",
		UnitsPerEm: 1000,
		Outlines:   o1,
	}

	ss := []glyph.ID{0, 3, 5, 4}
	i2 := i1.Subset(ss)
	o2 := i2.Outlines.(*cff.Outlines)

	if len(o2.Private) != len(ss) {
		t.Fatalf("expected %d FDs, got %d", len(ss), len(o2.Private))
	}
	if len(o2.GIDToCID) != len(ss) {
		t.Fatalf("expected %d Gid2Cid entries, got %d", len(ss), len(o2.GIDToCID))
	}
	for i, info := range []*Font{i1, i2} {
		o := info.Outlines.(*cff.Outlines)
		for gidInt, cid := range o.GIDToCID {
			gid := glyph.ID(gidInt)
			w := info.GlyphWidth(gid)
			if math.Abs(w-100*float64(cid)) > 0.5 {
				t.Errorf("%d: wrong glyph %s@%d, expected width %v, got %v",
					i, o.Glyphs[gid].Name, gid, 100*cid, w)
				continue
			}
			fd := o.Private[o.FDSelect(gid)]
			if fd.StdHW != float64(10*int(cid)+1) {
				t.Errorf("%d: wrong FD for glyph %s@%d, expected StdHW %d, got %f",
					i, o.Glyphs[gid].Name, gid, 10*int(cid)+1, fd.StdHW)
			}
		}
	}
}

// TestCIDPerFDMatrixRoundTrip checks that a CID-keyed CFF font whose FDs
// use distinct FontMatrices survives an OpenType round-trip with correct
// hmtx widths.  The hmtx table is in UnitsPerEm; before per-FD matrices
// were composed in WidthsPDF/makeHmtx, hmtx widths were emitted in
// FD-local CFF units instead, and glyphs in any FD whose matrix differed
// from the typical 1/1000 had wrong advances.
//
// A pure round-trip would be symmetrically broken (read inverts write), so
// this test inspects the intermediate hmtx table to confirm the encoded
// widths are in UnitsPerEm.  The glyph widths in CFF design units are
// also checked after re-reading.
func TestCIDPerFDMatrixRoundTrip(t *testing.T) {
	// FD 0: standard 1/1000 matrix.
	// FD 1: 2/1000 matrix -- a CFF design unit in FD 1 spans twice as much
	// text space as a CFF design unit in FD 0.
	o := &cff.Outlines{
		ROS: &cid.SystemInfo{
			Registry:   "Test",
			Ordering:   "Identity",
			Supplement: 0,
		},
		Private: []*type1.PrivateDict{
			{StdHW: 50},
			{StdHW: 50},
		},
		FontMatrices: []matrix.Matrix{
			{0.001, 0, 0, 0.001, 0, 0},
			{0.002, 0, 0, 0.002, 0, 0},
		},
		FDSelect: func(gid glyph.ID) int {
			return int(gid) & 1
		},
	}
	// Width values chosen so that every glyph advances 0.5 em in text space,
	// i.e. hmtx widths should all be 500 for UnitsPerEm = 1000.
	cffWidths := []float64{500, 250, 500, 250}
	for i, w := range cffWidths {
		g := cff.NewGlyph(fmt.Sprintf("c%d", i), w)
		g.MoveTo(0, 0)
		g.LineTo(w, 0)
		g.LineTo(w, 100)
		g.LineTo(0, 100)
		o.Glyphs = append(o.Glyphs, g)
	}
	o.GIDToCID = []cid.CID{0, 1, 2, 3}

	src := &Font{
		FamilyName: "Test",
		UnitsPerEm: 1000,
		FontMatrix: matrix.Identity,
		Ascent:     800,
		Descent:    -200,
		Outlines:   o,
	}

	var buf bytes.Buffer
	if _, err := src.Write(&buf); err != nil {
		t.Fatal(err)
	}

	// inspect intermediate hmtx
	dir, err := header.Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	hheaData, err := dir.ReadTableBytes(bytes.NewReader(buf.Bytes()), "hhea")
	if err != nil {
		t.Fatal(err)
	}
	hmtxData, err := dir.ReadTableBytes(bytes.NewReader(buf.Bytes()), "hmtx")
	if err != nil {
		t.Fatal(err)
	}
	hm, err := hmtx.Decode(hheaData, hmtxData)
	if err != nil {
		t.Fatal(err)
	}
	for gid := range cffWidths {
		// expected = round(text-space advance * UnitsPerEm) = 0.5 * 1000
		if hm.Widths[gid] != 500 {
			t.Errorf("hmtx[%d] = %d, want 500", gid, hm.Widths[gid])
		}
	}

	// round-trip
	dstData := buf.Bytes()
	dst, err := Read(bytes.NewReader(dstData), parser.NewBudget(int64(len(dstData))))
	if err != nil {
		t.Fatal(err)
	}
	dstOutlines, ok := dst.Outlines.(*cff.Outlines)
	if !ok {
		t.Fatalf("expected CFF outlines after round trip, got %T", dst.Outlines)
	}
	if !dstOutlines.IsCIDKeyed() {
		t.Fatal("CID-keyed structure lost on round trip")
	}
	for gid, want := range cffWidths {
		if got := dstOutlines.Glyphs[gid].Width; math.Abs(got-want) > 0.5 {
			t.Errorf("gid %d: CFF width %g, want %g", gid, got, want)
		}
	}

	// per-glyph PDF advance preserved
	for gid := range cffWidths {
		want := src.GlyphWidthPDF(glyph.ID(gid))
		got := dst.GlyphWidthPDF(glyph.ID(gid))
		if math.Abs(got-want) > 0.5 {
			t.Errorf("gid %d: GlyphWidthPDF = %g, want %g", gid, got, want)
		}
	}
}

// TestIsFixedPitchCIDPerFDMatrix checks that IsFixedPitch sees through
// per-FD font matrices: glyphs whose CFF design-unit widths differ but
// whose text-space advances are equal must count as fixed pitch.
func TestIsFixedPitchCIDPerFDMatrix(t *testing.T) {
	o := &cff.Outlines{
		ROS: &cid.SystemInfo{Registry: "T", Ordering: "I"},
		Private: []*type1.PrivateDict{
			{StdHW: 50},
			{StdHW: 50},
		},
		FontMatrices: []matrix.Matrix{
			{0.001, 0, 0, 0.001, 0, 0},
			{0.002, 0, 0, 0.002, 0, 0},
		},
		FDSelect: func(gid glyph.ID) int { return int(gid) & 1 },
	}
	for i, w := range []float64{500, 250, 500, 250} {
		o.Glyphs = append(o.Glyphs, cff.NewGlyph(fmt.Sprintf("c%d", i), w))
	}
	o.GIDToCID = []cid.CID{0, 1, 2, 3}
	f := &Font{
		FamilyName: "Test",
		UnitsPerEm: 1000,
		FontMatrix: matrix.Identity,
		Outlines:   o,
	}
	if !f.IsFixedPitch() {
		t.Errorf("IsFixedPitch=false, want true (all glyphs advance 0.5 em)")
	}

	// Sanity check: a varying-width version should be detected as variable.
	o.Glyphs[2].Width = 600
	if f.IsFixedPitch() {
		t.Errorf("IsFixedPitch=true, want false (gid 2 advances 0.6 em)")
	}
}
