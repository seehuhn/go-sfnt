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
	"fmt"
	"testing"

	"seehuhn.de/go/postscript/cid"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/glyph"
)

// TestFDSelect tests that the FDSelect function in subsetted fonts is correct.
func TestFDSelect(t *testing.T) {
	// Construct a CID-keyed CFF font with several FDs.
	o1 := &cff.Outlines{}
	for i := 0; i < 10; i++ {
		g := cff.NewGlyph(fmt.Sprintf("orig%d", i), funit.Int16(100*i))
		g.MoveTo(0, 0)
		g.LineTo(100*float64(i), 0)
		g.LineTo(100*float64(i), 500)
		g.LineTo(0, 500)
		o1.Glyphs = append(o1.Glyphs, g)
		o1.Private = append(o1.Private, &type1.PrivateDict{
			StdHW: float64(10*i + 1),
		})
	}
	o1.FDSelect = func(gid glyph.ID) int {
		return int(gid) % 10
	}
	o1.ROS = &cff.CIDSystemInfo{
		Registry:   "Seehuhn",
		Ordering:   "Sonderbar",
		Supplement: 0,
	}
	o1.GIDToCID = make([]cid.CID, 10)
	for i := 0; i < 10; i++ {
		o1.GIDToCID[i] = cid.CID(i)
	}
	i1 := &Font{
		FamilyName: "Test",
		UnitsPerEm: 1000,
		Outlines:   o1,
	}

	ss := []glyph.ID{0, 3, 5, 4}
	i2, err := i1.Subset(ss)
	if err != nil {
		t.Fatal(err)
	}
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
			if w != funit.Int16(100*int(cid)) {
				t.Errorf("%d: wrong glyph %s@%d, expected width %d, got %d",
					i, o.Glyphs[gid].Name, gid, 100*int(cid), w)
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
