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

package debug

import (
	"testing"

	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/glyph"
)

// TestMakeVarCFF2FontShape checks that the fixture still has the shape its
// glyph-ID constants describe, and that it reaches the parts of the format it
// exists to cover.  Cross-checking against fontTools proves nothing about a
// path no glyph takes, and each of these is easy to lose by accident while
// adjusting a coordinate.
func TestMakeVarCFF2FontShape(t *testing.T) {
	o := MakeVarCFF2Font().Outlines.(*cff.OutlinesCFF2)

	want := []struct {
		gid      glyph.ID
		fd       int
		vsIndex  int
		explicit bool // the charstring must name its vsindex
	}{
		{VarCFF2GidNotdef, 0, 0, false},
		{VarCFF2GidBox, 0, 0, false},
		{VarCFF2GidCurve, 0, 1, true},
		{VarCFF2GidTwo, 0, 1, true},
		{VarCFF2GidFD1, 1, 0, true},
	}
	if len(o.Glyphs) != len(want) {
		t.Fatalf("%d glyphs, want %d", len(o.Glyphs), len(want))
	}
	for _, w := range want {
		g := o.Glyphs[w.gid]
		fd := o.FDSelect(w.gid)
		if fd != w.fd {
			t.Errorf("gid %d: Font DICT %d, want %d", w.gid, fd, w.fd)
			continue
		}
		if g.VSIndex != w.vsIndex {
			t.Errorf("gid %d: vsindex %d, want %d", w.gid, g.VSIndex, w.vsIndex)
		}
		// the encoder writes a vsindex operator exactly when the glyph
		// disagrees with its Font DICT
		if explicit := g.VSIndex != o.Private[fd].VSIndex; explicit != w.explicit {
			t.Errorf("gid %d: explicit vsindex %v, want %v", w.gid, explicit, w.explicit)
		}
	}

	// The two subtables must differ in width, or a glyph picking the wrong one
	// would still decode.  Every varying blend carries one delta per region of
	// the subtable its glyph names.
	widths := make([]int, len(o.VarStore.Data))
	for i, ivd := range o.VarStore.Data {
		widths[i] = len(ivd.RegionIndexes)
	}
	if len(widths) != 2 || widths[0] == widths[1] {
		t.Fatalf("subtable region counts %v, want two different counts", widths)
	}
	for gid, g := range o.Glyphs {
		n := widths[g.VSIndex]
		check := func(what string, bb []cff.Blend) {
			for i, b := range bb {
				if len(b.Deltas) != 0 && len(b.Deltas) != n {
					t.Errorf("gid %d %s %d: %d deltas, want %d or 0",
						gid, what, i, len(b.Deltas), n)
				}
			}
		}
		check("HStem", g.HStem)
		check("VStem", g.VStem)
		for _, cmd := range g.Cmds {
			check("command", cmd.Args)
		}
	}

	// features which have to be present for the oracle to cover them
	var curves, subpaths, hintmasks, blendedPrivate int
	for _, g := range o.Glyphs {
		moves := 0
		for _, cmd := range g.Cmds {
			switch cmd.Op {
			case cff.OpCurveTo:
				curves++
			case cff.OpMoveTo:
				moves++
			case cff.OpHintMask, cff.OpCntrMask:
				hintmasks++
			}
		}
		if moves > 1 {
			subpaths++
		}
	}
	for _, p := range o.Private {
		if len(p.StdHW.Deltas) > 0 || len(p.StdVW.Deltas) > 0 {
			blendedPrivate++
		}
	}
	for _, c := range []struct {
		name string
		n    int
	}{
		{"cubic curves", curves},
		{"glyphs of several subpaths", subpaths},
		{"hintmasks", hintmasks},
		{"private dicts with a blended value", blendedPrivate},
	} {
		if c.n == 0 {
			t.Errorf("the fixture has no %s", c.name)
		}
	}
}
