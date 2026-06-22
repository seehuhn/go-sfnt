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
	"github.com/google/go-cmp/cmp/cmpopts"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/gdef"
	"seehuhn.de/go/sfnt/opentype/markarray"
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

// TestGsub4_1LigatureComponentTagging checks that forming a ligature tags the
// ligature glyph and each interleaved (skipped) mark with a shared LigID, and
// records on each mark the component it followed.  This is the information
// mark-to-ligature positioning (Gpos5_1) consumes.
func TestGsub4_1LigatureComponentTagging(t *testing.T) {
	const (
		b1  glyph.ID = 1
		m   glyph.ID = 2
		b2  glyph.ID = 3
		lig glyph.ID = 4
	)
	subst := &Gsub4_1{
		Cov:  coverage.Table{b1: 0},
		Repl: [][]Ligature{{{In: []glyph.ID{b2}, Out: lig}}}, // b1 b2 -> lig
	}
	lookupList := []*LookupTable{
		{Meta: &LookupMetaInfo{LookupType: 4, LookupFlags: IgnoreMarks}, Subtables: []Subtable{subst}},
	}
	gdefTable := &gdef.Table{GlyphClass: classdef.Table{m: gdef.GlyphClassMark}}

	// b1 m b2: the mark sits between the two ligature components, so it is
	// skipped during matching and ends up after the ligature glyph.
	in := []glyph.Info{{GID: b1}, {GID: m}, {GID: b2}}
	out := NewContext(lookupList, gdefTable, []LookupIndex{0}).Apply(in)

	if len(out) != 2 || out[0].GID != lig || out[1].GID != m {
		t.Fatalf("expected [lig, mark], got %v", out)
	}
	if out[0].LigID == 0 {
		t.Error("ligature glyph was not assigned a LigID")
	}
	if out[1].LigID != out[0].LigID {
		t.Errorf("mark LigID = %d, want %d (the ligature's id)", out[1].LigID, out[0].LigID)
	}
	if out[1].LigComp != 0 {
		t.Errorf("mark LigComp = %d, want 0 (it followed the first component)", out[1].LigComp)
	}
}

// TestLigatureMarkToLigature is an end-to-end check that the component
// information assigned during GSUB ligature substitution flows into GPOS
// mark-to-ligature positioning: the interleaved mark must attach to the first
// component's anchor, not the last-component fallback.
func TestLigatureMarkToLigature(t *testing.T) {
	const (
		b1  glyph.ID = 1
		m   glyph.ID = 2
		b2  glyph.ID = 3
		lig glyph.ID = 4
	)
	gsub := &Gsub4_1{
		Cov:  coverage.Table{b1: 0},
		Repl: [][]Ligature{{{In: []glyph.ID{b2}, Out: lig}}},
	}
	gpos := &Gpos5_1{
		MarkCov: coverage.Table{m: 0},
		LigCov:  coverage.Table{lig: 0},
		MarkArray: []markarray.Record{
			{Class: 0, Table: anchor.Table{X: 5, Y: 5}},
		},
		LigArray: [][][]anchor.Table{
			{
				{{X: 200, Y: 300}}, // component 0 (the one the mark follows)
				{{X: 400, Y: 500}}, // component 1 (the fallback)
			},
		},
	}
	lookupList := []*LookupTable{
		{Meta: &LookupMetaInfo{LookupType: 4, LookupFlags: IgnoreMarks}, Subtables: []Subtable{gsub}},
		{Meta: &LookupMetaInfo{LookupType: 5}, Subtables: []Subtable{gpos}},
	}
	gdefTable := &gdef.Table{GlyphClass: classdef.Table{m: gdef.GlyphClassMark}}

	in := []glyph.Info{{GID: b1}, {GID: m}, {GID: b2}}
	out := NewContext(lookupList, gdefTable, []LookupIndex{0, 1}).Apply(in)

	if len(out) != 2 || out[0].GID != lig || out[1].GID != m {
		t.Fatalf("expected [lig, mark], got %v", out)
	}
	// component 0 anchor (200,300) minus the mark anchor (5,5); the ligature's
	// advance is zero here, so there is nothing to subtract.
	if out[1].XOffset != 200-5 || out[1].YOffset != 300-5 {
		t.Errorf("mark offset = (%d, %d), want (%d, %d) — should use component 0, not the fallback",
			out[1].XOffset, out[1].YOffset, 200-5, 300-5)
	}
}

// TestGsub4_1LigLoopAliasing exercises the case where ligLoop iterates
// past j == 0 and the matching attempt contains both a skip and a match
// in its inner loop.  A previous version of Gsub4_1.apply reused the
// matchPos backing array for skipPos via `skipPos = matchPos[:0]`, so
// the two slices aliased on iterations after the first and silently
// corrupted each other's contents.
func TestGsub4_1LigLoopAliasing(t *testing.T) {
	const A glyph.ID = 10
	const M glyph.ID = 20
	const X glyph.ID = 30
	const Y glyph.ID = 40

	// LigatureSet for A with two entries: a longer one that fails to
	// match, followed by a shorter one that succeeds while skipping two
	// marks.  The first attempt populates matchPos with a non-nil
	// backing array, so the second attempt's reslice exposes the alias.
	subst := &Gsub4_1{
		Cov: coverage.Table{A: 0},
		Repl: [][]Ligature{
			{
				{In: []glyph.ID{A, A, A}, Out: X}, // AAAA -> X (fails)
				{In: []glyph.ID{A}, Out: Y},       // AA   -> Y (succeeds)
			},
		},
	}
	lookupList := []*LookupTable{
		{
			Meta: &LookupMetaInfo{
				LookupType:  4,
				LookupFlags: IgnoreMarks,
			},
			Subtables: []Subtable{subst},
		},
	}
	gdefTable := &gdef.Table{
		GlyphClass: classdef.Table{
			A: gdef.GlyphClassBase,
			M: gdef.GlyphClassMark,
		},
	}

	// Input A M M A: the second ligature matches A at position 0 and A
	// at position 3, with M at positions 1 and 2 moved past the
	// ligature glyph in the output.
	in := []glyph.Info{{GID: A}, {GID: M}, {GID: M}, {GID: A}}
	got := NewContext(lookupList, gdefTable, []LookupIndex{0}).Apply(in)
	want := []glyph.Info{{GID: Y}, {GID: M}, {GID: M}}
	// ligature tagging (LigID/LigComp) is exercised separately; this test is
	// about the matchPos/skipPos aliasing bug.
	ignoreLig := cmpopts.IgnoreFields(glyph.Info{}, "LigID", "LigComp")
	if d := cmp.Diff(want, got, ignoreLig); d != "" {
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
