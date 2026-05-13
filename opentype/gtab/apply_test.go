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

package gtab

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/gdef"
)

// TestLigature tests the simple case where a type 4 GSUB lookup is used
// to replace two glyphs with one.
func TestLigature(t *testing.T) {
	cov := coverage.Table{
		1: 0,
	}
	repl := [][]Ligature{
		{{In: []glyph.ID{2}, Out: 4}}, // 1 2 -> 4
	}
	subst := &Gsub4_1{
		Cov:  cov,
		Repl: repl,
	}
	lookupList := []*LookupTable{
		{
			Meta:      &LookupMetaInfo{LookupType: 4},
			Subtables: []Subtable{subst},
		},
	}

	in := []glyph.Info{
		{GID: 1, Text: []rune("a")},
		{GID: 2, Text: []rune("b")},
		{GID: 3, Text: []rune("c")},
	}
	e := NewContext(lookupList, nil, []LookupIndex{0})
	out := e.Apply(in)

	expected := []glyph.Info{
		{GID: 4, Text: []rune("ab")},
		{GID: 3, Text: []rune("c")},
	}

	if d := cmp.Diff(expected, out); d != "" {
		t.Errorf("unexpected result (-want +got):\n%s", d)
	}
}

// TestGsub8ReverseOrder verifies that GSUB type 8 lookups are applied
// right-to-left.  The rule "A -> B with backtrack {A}" applied to the
// sequence [A, A, A] yields [A, B, B] when processed in reverse
// (spec-correct) but [A, B, A] if processed forward.
func TestGsub8ReverseOrder(t *testing.T) {
	const A glyph.ID = 10
	const B glyph.ID = 20

	subst := &Gsub8_1{
		Input:              coverage.Table{A: 0},
		Backtrack:          []coverage.Table{{A: 0}},
		SubstituteGlyphIDs: []glyph.ID{B},
	}
	lookupList := []*LookupTable{
		{
			Meta:      &LookupMetaInfo{LookupType: 8},
			Subtables: []Subtable{subst},
		},
	}

	in := []glyph.Info{{GID: A}, {GID: A}, {GID: A}}
	got := NewContext(lookupList, nil, []LookupIndex{0}).Apply(in)
	want := []glyph.Info{{GID: A}, {GID: B}, {GID: B}}
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("reverse order mismatch (-want +got):\n%s", d)
	}
}

// TestGsub8ReverseDirectionIndependent checks that, when the substitute
// glyph is outside the input coverage, the result is identical regardless
// of iteration direction.  This guards against direction handling
// introducing unrelated regressions.
func TestGsub8ReverseDirectionIndependent(t *testing.T) {
	const A glyph.ID = 10
	const B glyph.ID = 20
	const X glyph.ID = 30

	subst := &Gsub8_1{
		Input:              coverage.Table{A: 0},
		Backtrack:          []coverage.Table{{X: 0}},
		SubstituteGlyphIDs: []glyph.ID{B},
	}
	lookupList := []*LookupTable{
		{
			Meta:      &LookupMetaInfo{LookupType: 8},
			Subtables: []Subtable{subst},
		},
	}

	in := []glyph.Info{{GID: X}, {GID: A}, {GID: X}, {GID: A}}
	got := NewContext(lookupList, nil, []LookupIndex{0}).Apply(in)
	want := []glyph.Info{{GID: X}, {GID: B}, {GID: X}, {GID: B}}
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("unexpected result (-want +got):\n%s", d)
	}
}

// TestGsub8ReverseWithIgnoreMarks exercises the keep-flag interaction in
// the reverse driver.  With IgnoreMarks set, an intervening mark glyph
// must not satisfy a backtrack position.
func TestGsub8ReverseWithIgnoreMarks(t *testing.T) {
	const A glyph.ID = 10
	const B glyph.ID = 20
	const M glyph.ID = 30

	subst := &Gsub8_1{
		Input:              coverage.Table{A: 0},
		Backtrack:          []coverage.Table{{A: 0}},
		SubstituteGlyphIDs: []glyph.ID{B},
	}
	lookupList := []*LookupTable{
		{
			Meta: &LookupMetaInfo{
				LookupType:  8,
				LookupFlags: IgnoreMarks,
			},
			Subtables: []Subtable{subst},
		},
	}
	gdefTable := &gdef.Table{
		GlyphClass: classdef.Table{
			A: gdef.GlyphClassBase,
			B: gdef.GlyphClassBase,
			M: gdef.GlyphClassMark,
		},
	}

	// [A, M, A]: with IgnoreMarks, the second A's backtrack sees the first
	// A (skipping M), so it substitutes.  The mark itself is skipped by
	// keep.Keep in applyReverse and is never offered to Gsub8_1.apply.
	in := []glyph.Info{{GID: A}, {GID: M}, {GID: A}}
	got := NewContext(lookupList, gdefTable, []LookupIndex{0}).Apply(in)
	want := []glyph.Info{{GID: A}, {GID: M}, {GID: B}}
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("unexpected result (-want +got):\n%s", d)
	}
}

// TestGsub8MixedWithForwardLookup runs a forward Gsub1_2 lookup followed
// by a reverse Gsub8_1 lookup over the same sequence, confirming the
// per-lookup direction dispatch in Context.Apply.
func TestGsub8MixedWithForwardLookup(t *testing.T) {
	const X glyph.ID = 1
	const A glyph.ID = 10
	const B glyph.ID = 20

	forward := &Gsub1_2{
		Cov:                coverage.Table{X: 0},
		SubstituteGlyphIDs: []glyph.ID{A},
	}
	reverse := &Gsub8_1{
		Input:              coverage.Table{A: 0},
		Backtrack:          []coverage.Table{{A: 0}},
		SubstituteGlyphIDs: []glyph.ID{B},
	}
	lookupList := []*LookupTable{
		{
			Meta:      &LookupMetaInfo{LookupType: 1},
			Subtables: []Subtable{forward},
		},
		{
			Meta:      &LookupMetaInfo{LookupType: 8},
			Subtables: []Subtable{reverse},
		},
	}

	// [X, X, X] -> forward Gsub1_2 -> [A, A, A] -> reverse Gsub8_1 -> [A, B, B]
	in := []glyph.Info{{GID: X}, {GID: X}, {GID: X}}
	got := NewContext(lookupList, nil, []LookupIndex{0, 1}).Apply(in)
	want := []glyph.Info{{GID: A}, {GID: B}, {GID: B}}
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("unexpected result (-want +got):\n%s", d)
	}
}
