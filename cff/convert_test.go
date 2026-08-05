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

package cff

import (
	"slices"
	"testing"

	"seehuhn.de/go/postscript/cid"
	"seehuhn.de/go/postscript/type1"
	"seehuhn.de/go/postscript/type1/names"

	"seehuhn.de/go/sfnt/glyph"
)

// sharedGlyphs returns glyphs of the kind found in a font program taken out of
// a PDF file: some names are missing, invalid, or used twice, so a conversion
// to a simple font has to invent new ones.
func sharedGlyphs() []*Glyph {
	return []*Glyph{
		{Name: ".notdef"},
		{Name: "A"},
		{Name: "A"},        // duplicate, needs a new name
		{Name: "bad name"}, // invalid, needs a new name
		{Name: ""},         // missing, needs a new name
	}
}

func glyphNamesOf(glyphs []*Glyph) []string {
	res := make([]string, len(glyphs))
	for i, g := range glyphs {
		res[i] = g.Name
	}
	return res
}

// checkNamesUnchanged verifies that a conversion left the glyphs it shares
// with another font alone.
func checkNamesUnchanged(t *testing.T, glyphs []*Glyph, want []string) {
	t.Helper()
	for gid, g := range glyphs {
		if g.Name != want[gid] {
			t.Errorf("shared glyph %d is now called %q, want %q", gid, g.Name, want[gid])
		}
	}
}

// MakeSimple gives every glyph a valid name, and no two glyphs share a name.
// Since glyphs are commonly shared between fonts, for example between a font
// and a subset of it, the renamed glyphs must be replaced rather than written
// to: a font sharing a glyph keeps the name it had.
func TestMakeSimpleNames(t *testing.T) {
	shared := sharedGlyphs()
	before := glyphNamesOf(shared)

	o := &Outlines{Glyphs: slices.Clone(shared)}
	o.MakeSimple(map[glyph.ID]string{2: "C"})

	if got := o.Glyphs[0].Name; got != ".notdef" {
		t.Errorf("glyph 0 is called %q, want %q", got, ".notdef")
	}
	seen := make(map[string]int)
	for gid, g := range o.Glyphs {
		if !names.IsValid(g.Name) {
			t.Errorf("glyph %d has invalid name %q", gid, g.Name)
		}
		if prev, ok := seen[g.Name]; ok {
			t.Errorf("glyphs %d and %d are both called %q", prev, gid, g.Name)
		}
		seen[g.Name] = gid
	}

	checkNamesUnchanged(t, shared, before)
}

// MakeCIDKeyed drops all glyph names, without disturbing the glyphs it shares
// with another font.
func TestMakeCIDKeyedNames(t *testing.T) {
	shared := sharedGlyphs()
	before := glyphNamesOf(shared)

	o := &Outlines{
		Glyphs:  slices.Clone(shared),
		Private: []*type1.PrivateDict{{}},
	}
	ros := &cid.SystemInfo{Registry: "Adobe", Ordering: "Identity"}
	o.MakeCIDKeyed(ros, []cid.CID{0, 1, 2, 3, 4})

	for gid, g := range o.Glyphs {
		if g.Name != "" {
			t.Errorf("glyph %d is still called %q", gid, g.Name)
		}
	}

	checkNamesUnchanged(t, shared, before)
}
