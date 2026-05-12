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

package sfnt

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/gdef"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/opentype/markarray"
)

// newSubsetter constructs a subsetter that keeps the given old glyph IDs,
// in the same order, starting at new GID 0.
func newSubsetter(keep ...glyph.ID) *subsetter {
	s := &subsetter{
		glyphs: append([]glyph.ID(nil), keep...),
		newGid: map[glyph.ID]glyph.ID{},
	}
	for i, oldGid := range keep {
		s.newGid[oldGid] = glyph.ID(i)
	}
	return s
}

func TestSubsetGsub1_2(t *testing.T) {
	s := newSubsetter(0, 5, 7)
	old := wrapLookup(1, &gtab.Gsub1_2{
		Cov:                coverage.Table{5: 0, 6: 1, 7: 2},
		SubstituteGlyphIDs: []glyph.ID{50, 60, 70},
	})
	got := s.SubsetGsub(old)
	sub := got.LookupList[0].Subtables[0].(*gtab.Gsub1_2)
	// 5 and 7 survive; 6 is filtered out. Substitute targets 50 and 70 are
	// added to the subset via getNewGid.
	if len(sub.Cov) != 2 {
		t.Errorf("expected 2 surviving entries, got %d", len(sub.Cov))
	}
	if _, ok := sub.Cov[1]; !ok { // gid 5 remapped to 1
		t.Errorf("missing remapped gid 5 → 1")
	}
	if _, ok := sub.Cov[2]; !ok { // gid 7 remapped to 2
		t.Errorf("missing remapped gid 7 → 2")
	}
}

func TestSubsetGsub2_1(t *testing.T) {
	s := newSubsetter(0, 5)
	old := wrapLookup(2, &gtab.Gsub2_1{
		Cov:  coverage.Table{5: 0},
		Repl: [][]glyph.ID{{50, 51}},
	})
	got := s.SubsetGsub(old)
	sub := got.LookupList[0].Subtables[0].(*gtab.Gsub2_1)
	if len(sub.Cov) != 1 || len(sub.Repl) != 1 {
		t.Fatalf("expected 1 entry, got cov=%d repl=%d", len(sub.Cov), len(sub.Repl))
	}
	if len(sub.Repl[0]) != 2 {
		t.Errorf("expected 2 replacement glyphs, got %d", len(sub.Repl[0]))
	}
}

func TestSubsetGsub3_1(t *testing.T) {
	s := newSubsetter(0, 5)
	old := wrapLookup(3, &gtab.Gsub3_1{
		Cov:        coverage.Table{5: 0},
		Alternates: [][]glyph.ID{{60, 61, 62}},
	})
	got := s.SubsetGsub(old)
	sub := got.LookupList[0].Subtables[0].(*gtab.Gsub3_1)
	if len(sub.Cov) != 1 || len(sub.Alternates[0]) != 3 {
		t.Errorf("unexpected shape: cov=%d alts=%d", len(sub.Cov), len(sub.Alternates[0]))
	}
}

// TestSubsetGsub4_1Append confirms the previous bug where surviving
// ligature rules were silently dropped is fixed.
func TestSubsetGsub4_1Append(t *testing.T) {
	s := newSubsetter(0, 5, 6, 7)
	old := wrapLookup(4, &gtab.Gsub4_1{
		Cov: coverage.Table{5: 0},
		Repl: [][]gtab.Ligature{
			{
				{In: []glyph.ID{6}, Out: 7},
			},
		},
	})
	got := s.SubsetGsub(old)
	if len(got.LookupList[0].Subtables) != 1 {
		t.Fatalf("expected 1 surviving subtable, got %d", len(got.LookupList[0].Subtables))
	}
	sub := got.LookupList[0].Subtables[0].(*gtab.Gsub4_1)
	if len(sub.Cov) != 1 || len(sub.Repl) != 1 || len(sub.Repl[0]) != 1 {
		t.Errorf("ligature was dropped: %+v", sub)
	}
}

func TestSubsetGpos1_1(t *testing.T) {
	s := newSubsetter(0, 5, 7)
	old := wrapLookup(1, &gtab.Gpos1_1{
		Cov:    coverage.Table{5: 0, 6: 1, 7: 2},
		Adjust: &gtab.GposValueRecord{XAdvance: 10},
	})
	got := s.SubsetGpos(old)
	sub := got.LookupList[0].Subtables[0].(*gtab.Gpos1_1)
	if len(sub.Cov) != 2 {
		t.Errorf("expected 2 surviving entries, got %d", len(sub.Cov))
	}
}

func TestSubsetGpos1_2(t *testing.T) {
	s := newSubsetter(0, 5, 7)
	old := wrapLookup(1, &gtab.Gpos1_2{
		Cov: coverage.Table{5: 0, 6: 1, 7: 2},
		Adjust: []*gtab.GposValueRecord{
			{XAdvance: 10},
			{XAdvance: 20},
			{XAdvance: 30},
		},
	})
	got := s.SubsetGpos(old)
	sub := got.LookupList[0].Subtables[0].(*gtab.Gpos1_2)
	if len(sub.Cov) != 2 || len(sub.Adjust) != 2 {
		t.Fatalf("expected 2 entries: cov=%d adj=%d", len(sub.Cov), len(sub.Adjust))
	}
	// gid 6 was filtered → its XAdvance=20 must not be present.
	for _, a := range sub.Adjust {
		if a.XAdvance == 20 {
			t.Errorf("filtered-out adjust kept")
		}
	}
}

func TestSubsetGpos2_2(t *testing.T) {
	s := newSubsetter(0, 5, 7)
	adj := &gtab.PairAdjust{First: &gtab.GposValueRecord{XAdvance: 1}}
	old := wrapLookup(2, &gtab.Gpos2_2{
		Cov:    coverage.Set{5: true, 6: true, 7: true},
		Class1: classdef.Table{5: 1, 6: 1, 7: 2},
		Class2: classdef.Table{5: 1, 6: 2, 7: 1},
		Adjust: [][]*gtab.PairAdjust{
			{adj, adj, adj},
			{adj, adj, adj},
			{adj, adj, adj},
		},
	})
	got := s.SubsetGpos(old)
	sub := got.LookupList[0].Subtables[0].(*gtab.Gpos2_2)
	if len(sub.Cov) != 2 || len(sub.Class1) != 2 || len(sub.Class2) != 2 {
		t.Errorf("unexpected sizes: cov=%d class1=%d class2=%d",
			len(sub.Cov), len(sub.Class1), len(sub.Class2))
	}
	if !sub.Cov[1] || !sub.Cov[2] {
		t.Errorf("missing remapped gids in Cov: %+v", sub.Cov)
	}
}

func TestSubsetGpos3_1(t *testing.T) {
	s := newSubsetter(0, 5, 7)
	old := wrapLookup(3, &gtab.Gpos3_1{
		Cov: coverage.Table{5: 0, 6: 1, 7: 2},
		Records: []gtab.EntryExitRecord{
			{Entry: anchor.Table{X: 1, Y: 2}},
			{Entry: anchor.Table{X: 3, Y: 4}},
			{Entry: anchor.Table{X: 5, Y: 6}},
		},
	})
	got := s.SubsetGpos(old)
	sub := got.LookupList[0].Subtables[0].(*gtab.Gpos3_1)
	if len(sub.Cov) != 2 || len(sub.Records) != 2 {
		t.Errorf("expected 2 surviving entries: %+v", sub)
	}
}

func TestSubsetGpos4_1(t *testing.T) {
	s := newSubsetter(0, 10, 11, 20)
	old := wrapLookup(4, &gtab.Gpos4_1{
		MarkCov:   coverage.Table{10: 0, 11: 1, 12: 2},
		BaseCov:   coverage.Table{20: 0, 21: 1},
		MarkArray: []markarray.Record{{Class: 0}, {Class: 1}, {Class: 0}},
		BaseArray: [][]anchor.Table{
			{{X: 1, Y: 2}, {X: 3, Y: 4}},
			{{X: 5, Y: 6}, {X: 7, Y: 8}},
		},
	})
	got := s.SubsetGpos(old)
	sub := got.LookupList[0].Subtables[0].(*gtab.Gpos4_1)
	if len(sub.MarkCov) != 2 || len(sub.MarkArray) != 2 {
		t.Errorf("expected 2 marks: %+v", sub)
	}
	if len(sub.BaseCov) != 1 || len(sub.BaseArray) != 1 {
		t.Errorf("expected 1 base: %+v", sub)
	}
}

func TestSubsetGpos5_1(t *testing.T) {
	// keep mark glyphs 10, 11 (drop 12); keep lig glyph 20 (drop 21).
	s := newSubsetter(0, 10, 11, 20)
	old := wrapLookup(5, &gtab.Gpos5_1{
		MarkCov: coverage.Table{10: 0, 11: 1, 12: 2},
		LigCov:  coverage.Table{20: 0, 21: 1},
		MarkArray: []markarray.Record{
			{Class: 0, Table: anchor.Table{X: 1, Y: 2}},
			{Class: 1, Table: anchor.Table{X: 3, Y: 4}},
			{Class: 0, Table: anchor.Table{X: 5, Y: 6}},
		},
		LigArray: [][][]anchor.Table{
			// ligature 20: 2 components × 2 mark classes
			{
				{{X: 10, Y: 11}, {X: 12, Y: 13}},
				{{X: 14, Y: 15}, {}},
			},
			// ligature 21: 1 component × 2 mark classes
			{
				{{X: 30, Y: 31}, {X: 32, Y: 33}},
			},
		},
	})
	got := s.SubsetGpos(old)
	if len(got.LookupList) != 1 {
		t.Fatalf("expected 1 lookup, got %d", len(got.LookupList))
	}
	sub := got.LookupList[0].Subtables[0].(*gtab.Gpos5_1)
	if len(sub.MarkCov) != 2 || len(sub.MarkArray) != 2 {
		t.Errorf("expected 2 surviving marks: cov=%d arr=%d",
			len(sub.MarkCov), len(sub.MarkArray))
	}
	if len(sub.LigCov) != 1 || len(sub.LigArray) != 1 {
		t.Errorf("expected 1 surviving ligature: cov=%d arr=%d",
			len(sub.LigCov), len(sub.LigArray))
	}
	// gid 10 → new 1, gid 11 → new 2
	if _, ok := sub.MarkCov[1]; !ok {
		t.Errorf("missing remapped mark gid 10 → 1")
	}
	if _, ok := sub.MarkCov[2]; !ok {
		t.Errorf("missing remapped mark gid 11 → 2")
	}
	// gid 20 → new 3
	if _, ok := sub.LigCov[3]; !ok {
		t.Errorf("missing remapped ligature gid 20 → 3")
	}
	// the surviving ligature should carry the original 2×2 anchor matrix
	if len(sub.LigArray[0]) != 2 || len(sub.LigArray[0][0]) != 2 {
		t.Errorf("ligature shape lost: %+v", sub.LigArray[0])
	}
	if sub.LigArray[0][0][0].X != 10 {
		t.Errorf("ligature anchors mangled: got %+v", sub.LigArray[0][0][0])
	}
	// round-trip via Info.Encode + Read.
	roundTrip := encodeAndReadGtabLookups(t, got, gtab.TypeGpos)
	if len(roundTrip) != 1 || len(roundTrip[0].Subtables) != 1 {
		t.Fatalf("round trip: expected 1 lookup with 1 subtable, got %+v", roundTrip)
	}
	if _, ok := roundTrip[0].Subtables[0].(*gtab.Gpos5_1); !ok {
		t.Errorf("round trip: expected *Gpos5_1, got %T", roundTrip[0].Subtables[0])
	}
}

func TestSubsetGpos5_1AllMarksDropped(t *testing.T) {
	s := newSubsetter(0, 20)
	old := wrapLookup(5, &gtab.Gpos5_1{
		MarkCov:   coverage.Table{10: 0},
		LigCov:    coverage.Table{20: 0},
		MarkArray: []markarray.Record{{Class: 0, Table: anchor.Table{X: 1, Y: 2}}},
		LigArray:  [][][]anchor.Table{{{{X: 3, Y: 4}}}},
	})
	got := s.SubsetGpos(old)
	if len(got.LookupList) != 0 {
		t.Errorf("expected lookup dropped (no marks), got %d", len(got.LookupList))
	}
}

func TestSubsetGpos5_1AllLigsDropped(t *testing.T) {
	s := newSubsetter(0, 10)
	old := wrapLookup(5, &gtab.Gpos5_1{
		MarkCov:   coverage.Table{10: 0},
		LigCov:    coverage.Table{20: 0},
		MarkArray: []markarray.Record{{Class: 0, Table: anchor.Table{X: 1, Y: 2}}},
		LigArray:  [][][]anchor.Table{{{{X: 3, Y: 4}}}},
	})
	got := s.SubsetGpos(old)
	if len(got.LookupList) != 0 {
		t.Errorf("expected lookup dropped (no ligatures), got %d", len(got.LookupList))
	}
}

func TestSubsetGpos6_1(t *testing.T) {
	s := newSubsetter(0, 10, 20)
	old := wrapLookup(6, &gtab.Gpos6_1{
		Mark1Cov:   coverage.Table{10: 0, 11: 1},
		Mark2Cov:   coverage.Table{20: 0, 21: 1},
		Mark1Array: []markarray.Record{{Class: 0}, {Class: 1}},
		Mark2Array: [][]anchor.Table{
			{{X: 1, Y: 2}, {X: 3, Y: 4}},
			{{X: 5, Y: 6}, {X: 7, Y: 8}},
		},
	})
	got := s.SubsetGpos(old)
	sub := got.LookupList[0].Subtables[0].(*gtab.Gpos6_1)
	if len(sub.Mark1Cov) != 1 || len(sub.Mark2Cov) != 1 {
		t.Errorf("expected 1 mark1 and 1 mark2: %+v", sub)
	}
}

func TestSubsetSeqContext1(t *testing.T) {
	s := newSubsetter(0, 5, 6)
	old := &gtab.Info{
		LookupList: gtab.LookupList{
			&gtab.LookupTable{
				Meta: &gtab.LookupMetaInfo{LookupType: 5},
				Subtables: []gtab.Subtable{
					&gtab.SeqContext1{
						Cov: coverage.Table{5: 0},
						Rules: [][]*gtab.SeqRule{
							{
								// rule 0: input gid 6 — kept
								{
									Input:   []glyph.ID{6},
									Actions: []gtab.SeqLookup{{SequenceIndex: 0, LookupListIndex: 0}},
								},
								// rule 1: input gid 99 — dropped (99 not in subset)
								{
									Input:   []glyph.ID{99},
									Actions: []gtab.SeqLookup{{SequenceIndex: 0, LookupListIndex: 0}},
								},
							},
						},
					},
				},
			},
		},
	}
	got := s.SubsetGsub(old)
	if len(got.LookupList) != 1 {
		t.Fatalf("expected 1 surviving lookup, got %d", len(got.LookupList))
	}
	sub := got.LookupList[0].Subtables[0].(*gtab.SeqContext1)
	if len(sub.Cov) != 1 || len(sub.Rules) != 1 || len(sub.Rules[0]) != 1 {
		t.Errorf("rule filter wrong: cov=%d rules=%d rule[0]=%d",
			len(sub.Cov), len(sub.Rules), len(sub.Rules[0]))
	}
	if got := sub.Rules[0][0].Input[0]; got != glyph.ID(2) {
		t.Errorf("expected remapped input 6→2, got %d", got)
	}
}

func TestSubsetSeqContext3(t *testing.T) {
	s := newSubsetter(0, 5, 6)
	// First rule requires both glyphs present → keep.
	// Second subtable's coverage drops to empty for one position → drop.
	old := &gtab.Info{
		LookupList: gtab.LookupList{
			&gtab.LookupTable{
				Meta: &gtab.LookupMetaInfo{LookupType: 5},
				Subtables: []gtab.Subtable{
					&gtab.SeqContext3{
						Input: []coverage.Set{
							{5: true, 6: true},
							{5: true, 6: true},
						},
						Actions: []gtab.SeqLookup{{LookupListIndex: 0}},
					},
					&gtab.SeqContext3{
						Input: []coverage.Set{
							{5: true},
							{99: true}, // gid 99 not in subset → second set empty
						},
						Actions: []gtab.SeqLookup{{LookupListIndex: 0}},
					},
				},
			},
		},
	}
	got := s.SubsetGsub(old)
	if len(got.LookupList) != 1 {
		t.Fatalf("expected 1 lookup, got %d", len(got.LookupList))
	}
	if len(got.LookupList[0].Subtables) != 1 {
		t.Errorf("expected 1 surviving subtable, got %d", len(got.LookupList[0].Subtables))
	}
}

func TestSubsetGsub8_1(t *testing.T) {
	s := newSubsetter(0, 5)
	old := &gtab.Info{
		LookupList: gtab.LookupList{
			&gtab.LookupTable{
				Meta: &gtab.LookupMetaInfo{LookupType: 8},
				Subtables: []gtab.Subtable{
					&gtab.Gsub8_1{
						Input:              coverage.Table{5: 0},
						Backtrack:          []coverage.Table{{5: 0}},
						Lookahead:          []coverage.Table{{5: 0}},
						SubstituteGlyphIDs: []glyph.ID{50},
					},
				},
			},
		},
	}
	got := s.SubsetGsub(old)
	if len(got.LookupList) != 1 {
		t.Fatalf("expected 1 lookup, got %d", len(got.LookupList))
	}
	sub := got.LookupList[0].Subtables[0].(*gtab.Gsub8_1)
	// gid 5 stays (remapped to 1); substitute glyph 50 added via getNewGid
	if len(sub.Input) != 1 || len(sub.SubstituteGlyphIDs) != 1 {
		t.Errorf("expected 1 input entry, got input=%d subs=%d",
			len(sub.Input), len(sub.SubstituteGlyphIDs))
	}
}

// TestSubsetGsub8_1ContextFiltered confirms that Gsub8_1's
// backtrack/lookahead are treated as required inputs during the
// step-2 propagation: when a backtrack glyph isn't in the subset,
// the rule can never fire, so the substitution target (and its
// transitive substitutions) must NOT be pulled into the subset.
func TestSubsetGsub8_1ContextFiltered(t *testing.T) {
	// keep A and B only.  The Gsub8_1 rule needs backtrack X, which is
	// dropped — so the rule never fires.  T must not appear in the
	// output, and the downstream T→U substitution must not pull U in
	// either.
	const (
		A glyph.ID = 1
		B glyph.ID = 2
		X glyph.ID = 99
		T glyph.ID = 50
		U glyph.ID = 60
	)
	s := newSubsetter(0, A, B)
	old := &gtab.Info{
		LookupList: gtab.LookupList{
			// lookup 0: Gsub8_1 A → T with backtrack X
			&gtab.LookupTable{
				Meta: &gtab.LookupMetaInfo{LookupType: 8},
				Subtables: []gtab.Subtable{
					&gtab.Gsub8_1{
						Input:              coverage.Table{A: 0},
						Backtrack:          []coverage.Table{{X: 0}},
						Lookahead:          nil,
						SubstituteGlyphIDs: []glyph.ID{T},
					},
				},
			},
			// lookup 1: Gsub1_2 T → U
			&gtab.LookupTable{
				Meta: &gtab.LookupMetaInfo{LookupType: 1},
				Subtables: []gtab.Subtable{
					&gtab.Gsub1_2{
						Cov:                coverage.Table{T: 0},
						SubstituteGlyphIDs: []glyph.ID{U},
					},
				},
			},
		},
	}
	_ = s.SubsetGsub(old)
	if _, ok := s.newGid[T]; ok {
		t.Errorf("Gsub8_1 cannot fire (backtrack X dropped); target T should not be in subset")
	}
	if _, ok := s.newGid[U]; ok {
		t.Errorf("downstream U was pulled into subset via a rule that cannot fire")
	}
}

// TestSubsetGsub8_1ContextLiveTransitive confirms that when a Gsub8_1
// rule CAN fire (some glyph in every context position survives), its
// substitution target AND any transitive substitutions on it propagate
// — regardless of whether the downstream lookup sits before or after
// the Gsub8_1 in LookupList.  The "before" case is the interesting one:
// step 2's propagation must add T before step 3 walks the LookupList,
// otherwise the T → U lookup (processed first by step 3) loses U.
func TestSubsetGsub8_1ContextLiveTransitive(t *testing.T) {
	// keep A and X1 (one of two backtrack alternatives) — the
	// Gsub8_1 rule fires because X1 satisfies the backtrack position.
	const (
		A  glyph.ID = 1
		X1 glyph.ID = 2
		X2 glyph.ID = 99
		T  glyph.ID = 50
		U  glyph.ID = 60
	)
	// lookup 0 is T → U (downstream), lookup 1 is A → T with
	// backtrack {X1, X2}.  Reversed lookup order vs. the previous
	// test — the bug this guards against was order-dependent.
	old := &gtab.Info{
		LookupList: gtab.LookupList{
			&gtab.LookupTable{
				Meta: &gtab.LookupMetaInfo{LookupType: 1},
				Subtables: []gtab.Subtable{
					&gtab.Gsub1_2{
						Cov:                coverage.Table{T: 0},
						SubstituteGlyphIDs: []glyph.ID{U},
					},
				},
			},
			&gtab.LookupTable{
				Meta: &gtab.LookupMetaInfo{LookupType: 8},
				Subtables: []gtab.Subtable{
					&gtab.Gsub8_1{
						Input:              coverage.Table{A: 0},
						Backtrack:          []coverage.Table{{X1: 0, X2: 1}},
						Lookahead:          nil,
						SubstituteGlyphIDs: []glyph.ID{T},
					},
				},
			},
		},
	}
	s := newSubsetter(0, A, X1)
	got := s.SubsetGsub(old)
	if _, ok := s.newGid[T]; !ok {
		t.Errorf("T should have been added to subset (Gsub8_1 can fire via X1)")
	}
	if _, ok := s.newGid[U]; !ok {
		t.Errorf("U should have been added transitively (T → U)")
	}
	// the T → U lookup must survive — its coverage references T,
	// which is in the subset.
	if len(got.LookupList) != 2 {
		t.Fatalf("expected 2 surviving lookups, got %d", len(got.LookupList))
	}
}

// TestLookupIndexRemap: original LookupList has [contextual, dead, target].
// "dead" has no glyphs in subset and is dropped.  The contextual's action
// originally points at index 2 (target); after remap it should point at
// index 1 (the new position of target).
func TestLookupIndexRemap(t *testing.T) {
	s := newSubsetter(0, 5, 50)
	old := &gtab.Info{
		LookupList: gtab.LookupList{
			// lookup 0: contextual, fires lookup 2 on the matched glyph
			&gtab.LookupTable{
				Meta: &gtab.LookupMetaInfo{LookupType: 5},
				Subtables: []gtab.Subtable{
					&gtab.SeqContext1{
						Cov: coverage.Table{5: 0},
						Rules: [][]*gtab.SeqRule{
							{{Actions: []gtab.SeqLookup{{LookupListIndex: 2}}}},
						},
					},
				},
			},
			// lookup 1: dead — references only out-of-subset glyph 999
			&gtab.LookupTable{
				Meta: &gtab.LookupMetaInfo{LookupType: 1},
				Subtables: []gtab.Subtable{
					&gtab.Gsub1_2{
						Cov:                coverage.Table{999: 0},
						SubstituteGlyphIDs: []glyph.ID{888},
					},
				},
			},
			// lookup 2: target — gid 5 → gid 50
			&gtab.LookupTable{
				Meta: &gtab.LookupMetaInfo{LookupType: 1},
				Subtables: []gtab.Subtable{
					&gtab.Gsub1_2{
						Cov:                coverage.Table{5: 0},
						SubstituteGlyphIDs: []glyph.ID{50},
					},
				},
			},
		},
	}
	got := s.SubsetGsub(old)
	if len(got.LookupList) != 2 {
		t.Fatalf("expected 2 surviving lookups (contextual + target), got %d", len(got.LookupList))
	}
	ctx := got.LookupList[0].Subtables[0].(*gtab.SeqContext1)
	action := ctx.Rules[0][0].Actions[0]
	if action.LookupListIndex != 1 {
		t.Errorf("expected remapped LookupListIndex 1, got %d", action.LookupListIndex)
	}
}

// TestLookupIndexRemapDeadAction: SeqLookup actions pointing to dropped
// lookups must be filtered out.
func TestLookupIndexRemapDeadAction(t *testing.T) {
	s := newSubsetter(0, 5)
	old := &gtab.Info{
		LookupList: gtab.LookupList{
			// lookup 0: contextual; action points to lookup 1 (dead)
			&gtab.LookupTable{
				Meta: &gtab.LookupMetaInfo{LookupType: 5},
				Subtables: []gtab.Subtable{
					&gtab.SeqContext1{
						Cov: coverage.Table{5: 0},
						Rules: [][]*gtab.SeqRule{
							{{Actions: []gtab.SeqLookup{{LookupListIndex: 1}}}},
						},
					},
				},
			},
			// lookup 1: dead — substitutes 999→888, neither in subset
			&gtab.LookupTable{
				Meta: &gtab.LookupMetaInfo{LookupType: 1},
				Subtables: []gtab.Subtable{
					&gtab.Gsub1_2{
						Cov:                coverage.Table{999: 0},
						SubstituteGlyphIDs: []glyph.ID{888},
					},
				},
			},
		},
	}
	got := s.SubsetGsub(old)
	if len(got.LookupList) != 1 {
		t.Fatalf("expected 1 surviving lookup, got %d", len(got.LookupList))
	}
	ctx := got.LookupList[0].Subtables[0].(*gtab.SeqContext1)
	if len(ctx.Rules[0][0].Actions) != 0 {
		t.Errorf("expected dead action pruned, got %d actions", len(ctx.Rules[0][0].Actions))
	}
}

// TestSubsetSeqContext2DeadClassPrune verifies that format-2 contextual
// subsetting drops rules whose first-glyph class or input-class sequence
// references a class with no surviving glyph.  Class 0 (the implicit class
// for unclassed glyphs) is treated alive iff any subset glyph is unclassed.
func TestSubsetSeqContext2DeadClassPrune(t *testing.T) {
	// gid 0 deliberately omitted so all subset glyphs are classed and
	// class 0 (the implicit unclassed class) is dead.
	s := newSubsetter(5, 6)
	old := wrapLookup(5, &gtab.SeqContext2{
		Cov: coverage.Table{5: 0, 6: 1},
		Input: classdef.Table{
			5:  1, // class 1: alive (5 in subset)
			6:  2, // class 2: alive (6 in subset)
			99: 3, // class 3: dead (99 not in subset)
		},
		Rules: [][]*gtab.ClassSeqRule{
			// firstClass 0: class 0 dead (both subset glyphs classed) → drop
			{{Input: []uint16{1}, Actions: []gtab.SeqLookup{}}},
			// firstClass 1: alive
			{
				{Input: []uint16{2}, Actions: []gtab.SeqLookup{}}, // alive → keep
				{Input: []uint16{3}, Actions: []gtab.SeqLookup{}}, // dead class 3 → drop
			},
			// firstClass 2: alive
			{{Input: []uint16{1}, Actions: []gtab.SeqLookup{}}},
			// firstClass 3: dead → drop entirely
			{{Input: []uint16{1}, Actions: []gtab.SeqLookup{}}},
		},
	})
	got := s.SubsetGsub(old)
	if len(got.LookupList) != 1 {
		t.Fatalf("expected 1 surviving lookup, got %d", len(got.LookupList))
	}
	sub := got.LookupList[0].Subtables[0].(*gtab.SeqContext2)

	// After dead-class pruning, input class def has classes {0, 1, 2}
	// only (class 3 was dropped), so NumClasses == 3 and Rules is
	// trimmed to length 3 — class 3's nil entry is gone.
	if len(sub.Rules) != 3 {
		t.Fatalf("expected Rules len 3 (trimmed to NumClasses), got %d", len(sub.Rules))
	}
	if sub.Rules[0] != nil {
		t.Errorf("firstClass 0 dead but kept %d rules", len(sub.Rules[0]))
	}
	if len(sub.Rules[1]) != 1 {
		t.Errorf("firstClass 1: expected 1 surviving rule, got %d", len(sub.Rules[1]))
	} else if sub.Rules[1][0].Input[0] != 2 {
		t.Errorf("wrong rule survived in firstClass 1: %+v", sub.Rules[1][0])
	}
	if len(sub.Rules[2]) != 1 {
		t.Errorf("firstClass 2: expected 1 surviving rule, got %d", len(sub.Rules[2]))
	}
}

// TestSubsetSeqContext2Class0Alive checks that class 0 is alive when at
// least one surviving glyph is unclassed by the Input classdef.
func TestSubsetSeqContext2Class0Alive(t *testing.T) {
	s := newSubsetter(0, 5, 7)
	old := wrapLookup(5, &gtab.SeqContext2{
		Cov: coverage.Table{5: 0},
		Input: classdef.Table{
			5: 1, // 7 is unclassed → class 0 alive
		},
		Rules: [][]*gtab.ClassSeqRule{
			// firstClass 0: alive thanks to gid 7
			{{Input: []uint16{0}, Actions: []gtab.SeqLookup{}}},
			// firstClass 1: alive
			{{Input: []uint16{0}, Actions: []gtab.SeqLookup{}}},
		},
	})
	got := s.SubsetGsub(old)
	sub := got.LookupList[0].Subtables[0].(*gtab.SeqContext2)
	if len(sub.Rules[0]) != 1 {
		t.Errorf("class 0 should be alive but Rules[0] has %d entries", len(sub.Rules[0]))
	}
	if len(sub.Rules[1]) != 1 {
		t.Errorf("Rules[1] should survive, got %d entries", len(sub.Rules[1]))
	}
}

// TestSubsetChainedSeqContext2DeadClassPrune verifies dead-class pruning
// across the three independent classdefs (Backtrack, Input, Lookahead).
func TestSubsetChainedSeqContext2DeadClassPrune(t *testing.T) {
	// gid 0 deliberately omitted so all subset glyphs are classed in
	// every classdef and class 0 is dead across the board.
	s := newSubsetter(5, 6)
	old := wrapLookup(6, &gtab.ChainedSeqContext2{
		Cov: coverage.Table{5: 0, 6: 1},
		// each classdef gives the surviving glyphs class 1, plus a dead class 2
		// fed by glyphs not in the subset
		Backtrack: classdef.Table{5: 1, 6: 1, 99: 2},
		Input:     classdef.Table{5: 1, 6: 1, 99: 2},
		Lookahead: classdef.Table{5: 1, 6: 1, 99: 2},
		Rules: [][]*gtab.ChainedClassSeqRule{
			nil, // firstClass 0: class 0 dead (both subset glyphs classed)
			{
				// alive across all three classdefs
				{Backtrack: []uint16{1}, Input: []uint16{1}, Lookahead: []uint16{1}},
				// dead in Backtrack
				{Backtrack: []uint16{2}, Input: []uint16{1}, Lookahead: []uint16{1}},
				// dead in Input
				{Backtrack: []uint16{1}, Input: []uint16{2}, Lookahead: []uint16{1}},
				// dead in Lookahead
				{Backtrack: []uint16{1}, Input: []uint16{1}, Lookahead: []uint16{2}},
			},
			nil, // firstClass 2: dead → drop entirely
		},
	})
	got := s.SubsetGsub(old)
	sub := got.LookupList[0].Subtables[0].(*gtab.ChainedSeqContext2)

	// After dead-class pruning, Input class def has classes {0, 1}
	// only (class 2 was dropped), so NumClasses == 2 and Rules is
	// trimmed to length 2 — class 2's nil entry is gone.
	if len(sub.Rules) != 2 {
		t.Fatalf("expected Rules len 2 (trimmed to NumClasses), got %d", len(sub.Rules))
	}
	if len(sub.Rules[1]) != 1 {
		t.Errorf("expected 1 surviving rule in firstClass 1, got %d", len(sub.Rules[1]))
	}
}

// TestSubsetSeqContext2AllRulesDropped: if pruning leaves no rules, the
// whole subtable is dropped.
func TestSubsetSeqContext2AllRulesDropped(t *testing.T) {
	// gid 0 omitted so class 0 is dead and the only surviving rule
	// (which references dead class 2) is pruned.
	s := newSubsetter(5)
	old := wrapLookup(5, &gtab.SeqContext2{
		Cov:   coverage.Table{5: 0},
		Input: classdef.Table{5: 1, 99: 2},
		Rules: [][]*gtab.ClassSeqRule{
			nil,
			// the one alive first-class has rules referencing dead class 2
			{{Input: []uint16{2}}},
		},
	})
	got := s.SubsetGsub(old)
	// The lookup should be dropped entirely (no subtables survive).
	if len(got.LookupList) != 0 {
		t.Errorf("expected lookup dropped, got %d", len(got.LookupList))
	}
}

// TestSubsetGposContextDroppedDoesNotPanic: a GPOS lookup whose only
// subtable is a contextual one that filters away entirely must not leave a
// nil slot in tNew.Subtables — encoding the result would otherwise panic
// with a nil-pointer dereference inside LookupList.encode.
func TestSubsetGposContextDroppedDoesNotPanic(t *testing.T) {
	s := newSubsetter(0, 5)
	old := wrapLookup(7, &gtab.SeqContext1{
		Cov:   coverage.Table{99: 0}, // gid 99 not in subset
		Rules: [][]*gtab.SeqRule{{{Input: []glyph.ID{99}}}},
	})
	got := s.SubsetGpos(old)
	for li, l := range got.LookupList {
		for j, st := range l.Subtables {
			if st == nil {
				t.Fatalf("lookup %d subtable %d is nil", li, j)
			}
		}
	}
	// the empty lookup itself should be dropped
	if len(got.LookupList) != 0 {
		t.Errorf("expected lookup dropped, got %d", len(got.LookupList))
	}
	// re-encoding must not panic
	_ = got.Encode()
}

// TestSubsetGposEmptySubtableDropped: a non-contextual subtable whose
// coverage filters to empty must be dropped rather than emitted as an
// empty subtable.
func TestSubsetGposEmptySubtableDropped(t *testing.T) {
	s := newSubsetter(0, 5)
	old := wrapLookup(1, &gtab.Gpos1_1{
		Cov:    coverage.Table{99: 0}, // gid 99 not in subset
		Adjust: &gtab.GposValueRecord{XAdvance: 10},
	})
	got := s.SubsetGpos(old)
	if len(got.LookupList) != 0 {
		t.Errorf("expected empty lookup dropped, got %d lookups", len(got.LookupList))
	}
}

// TestSubsetGposMixedSubtablesPartiallyDropped: in a lookup that mixes
// a surviving Gpos1_1 with a contextual subtable that filters away,
// only the surviving subtable should remain.
func TestSubsetGposMixedSubtablesPartiallyDropped(t *testing.T) {
	s := newSubsetter(0, 5)
	old := &gtab.Info{
		LookupList: gtab.LookupList{
			&gtab.LookupTable{
				Meta: &gtab.LookupMetaInfo{LookupType: 1},
				Subtables: []gtab.Subtable{
					&gtab.SeqContext1{
						Cov:   coverage.Table{99: 0},
						Rules: [][]*gtab.SeqRule{{{Input: []glyph.ID{99}}}},
					},
					&gtab.Gpos1_1{
						Cov:    coverage.Table{5: 0},
						Adjust: &gtab.GposValueRecord{XAdvance: 10},
					},
				},
			},
		},
	}
	got := s.SubsetGpos(old)
	if len(got.LookupList) != 1 {
		t.Fatalf("expected 1 lookup, got %d", len(got.LookupList))
	}
	if len(got.LookupList[0].Subtables) != 1 {
		t.Fatalf("expected 1 surviving subtable, got %d", len(got.LookupList[0].Subtables))
	}
	if _, ok := got.LookupList[0].Subtables[0].(*gtab.Gpos1_1); !ok {
		t.Errorf("expected *Gpos1_1 to survive, got %T", got.LookupList[0].Subtables[0])
	}
}

// TestSubsetGposLookupIndexRemap mirrors TestLookupIndexRemap for the
// GPOS path: when an intermediate lookup is dropped because no glyph
// survives, contextual action indices in remaining lookups must be
// remapped to the new positions.
func TestSubsetGposLookupIndexRemap(t *testing.T) {
	s := newSubsetter(0, 5)
	old := &gtab.Info{
		LookupList: gtab.LookupList{
			// lookup 0: contextual, fires lookup 2
			&gtab.LookupTable{
				Meta: &gtab.LookupMetaInfo{LookupType: 7},
				Subtables: []gtab.Subtable{
					&gtab.SeqContext1{
						Cov: coverage.Table{5: 0},
						Rules: [][]*gtab.SeqRule{
							{{Actions: []gtab.SeqLookup{{LookupListIndex: 2}}}},
						},
					},
				},
			},
			// lookup 1: dead — references only gid 99
			&gtab.LookupTable{
				Meta: &gtab.LookupMetaInfo{LookupType: 1},
				Subtables: []gtab.Subtable{
					&gtab.Gpos1_1{
						Cov:    coverage.Table{99: 0},
						Adjust: &gtab.GposValueRecord{XAdvance: 1},
					},
				},
			},
			// lookup 2: target — adjusts gid 5
			&gtab.LookupTable{
				Meta: &gtab.LookupMetaInfo{LookupType: 1},
				Subtables: []gtab.Subtable{
					&gtab.Gpos1_1{
						Cov:    coverage.Table{5: 0},
						Adjust: &gtab.GposValueRecord{XAdvance: 1},
					},
				},
			},
		},
	}
	got := s.SubsetGpos(old)
	if len(got.LookupList) != 2 {
		t.Fatalf("expected 2 surviving lookups, got %d", len(got.LookupList))
	}
	ctx := got.LookupList[0].Subtables[0].(*gtab.SeqContext1)
	if ctx.Rules[0][0].Actions[0].LookupListIndex != 1 {
		t.Errorf("expected remapped LookupListIndex 1, got %d",
			ctx.Rules[0][0].Actions[0].LookupListIndex)
	}
}

func TestSubsetGdef(t *testing.T) {
	s := newSubsetter(0, 5, 7)
	old := &gdef.Table{
		GlyphClass: classdef.Table{
			5: gdef.GlyphClassBase,
			6: gdef.GlyphClassMark,
			7: gdef.GlyphClassLigature,
		},
		MarkAttachClass: classdef.Table{
			6: 1,
		},
		MarkGlyphSets: []coverage.Set{
			{5: true, 6: true},
			{7: true},
		},
	}
	got := s.SubsetGdef(old)
	want := &gdef.Table{
		GlyphClass: classdef.Table{
			1: gdef.GlyphClassBase,     // 5 → 1
			2: gdef.GlyphClassLigature, // 7 → 2
		},
		MarkAttachClass: classdef.Table{}, // 6 filtered out
		MarkGlyphSets: []coverage.Set{
			{1: true}, // 5 → 1; 6 dropped
			{2: true}, // 7 → 2
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("SubsetGdef mismatch (-want +got):\n%s", diff)
	}
}
