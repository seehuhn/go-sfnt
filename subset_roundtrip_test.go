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
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/text/language"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/opentype/markarray"
)

// subsetRoundTripCase describes one round-trip scenario.  The same table
// drives the unit test and seeds the fuzzer.
//
// info is the input *gtab.Info (one lookup, one subtable in the standard
// case).  keep is the list of old glyph IDs to retain (must start with
// gid 0 to match the public Font.Subset contract).  tp says whether the
// info is a GSUB or GPOS table; subset is the matching subsetter entry
// point.
type subsetRoundTripCase struct {
	name   string
	keep   []glyph.ID
	info   *gtab.Info
	tp     gtab.Type
	subset func(s *subsetter, info *gtab.Info) *gtab.Info
}

// wrapLookup builds a *gtab.Info containing exactly one lookup of the
// given lookup type, with the supplied subtables.  Tests must use the
// correct lookup type so round-tripped data parses as the same Go type.
func wrapLookup(lookupType uint16, subs ...gtab.Subtable) *gtab.Info {
	return &gtab.Info{
		LookupList: gtab.LookupList{
			&gtab.LookupTable{
				Meta:      &gtab.LookupMetaInfo{LookupType: lookupType},
				Subtables: subs,
			},
		},
	}
}

// subsetGsub adapts (*subsetter).SubsetGsub to the case struct signature.
func subsetGsub(s *subsetter, info *gtab.Info) *gtab.Info {
	return s.SubsetGsub(info)
}

// subsetGpos adapts (*subsetter).SubsetGpos to the case struct signature.
func subsetGpos(s *subsetter, info *gtab.Info) *gtab.Info {
	return s.SubsetGpos(info)
}

// encodeAndReadGtabLookups runs an encode → read round trip on info and
// returns the decoded LookupList.  The Encoder discards LookupList when
// no Script/Feature references it, so this helper synthesises a minimal
// ScriptList + FeatureList pointing at every lookup.
func encodeAndReadGtabLookups(t *testing.T, info *gtab.Info, tp gtab.Type) gtab.LookupList {
	t.Helper()
	if len(info.LookupList) == 0 {
		return nil
	}
	idxs := make([]gtab.LookupIndex, len(info.LookupList))
	for i := range info.LookupList {
		idxs[i] = gtab.LookupIndex(i)
	}
	wrapped := &gtab.Info{
		ScriptList: gtab.ScriptListInfo{
			language.MustParse("und"): &gtab.Features{
				Required: 0xFFFF,
				Optional: []gtab.FeatureIndex{0},
			},
		},
		FeatureList: gtab.FeatureListInfo{
			&gtab.Feature{Tag: "kern", Lookups: idxs},
		},
		LookupList: info.LookupList,
	}
	data := wrapped.Encode()
	got, err := gtab.Read(bytes.NewReader(data), tp)
	if err != nil {
		t.Fatalf("re-read failed: %v", err)
	}
	return got.LookupList
}

// subsetAndRoundTrip runs the subsetter on tc.info, encodes the result,
// reads it back, and asserts that the round-tripped LookupList matches
// the subset output.  Both guarantees from CLAUDE.md are checked here:
// (1) the subset output is encodable; (2) the encode → read cycle
// preserves the Go representation.
func subsetAndRoundTrip(t *testing.T, tc subsetRoundTripCase) {
	t.Helper()

	s := newSubsetter(tc.keep...)
	got := tc.subset(s, tc.info)

	roundTrip := encodeAndReadGtabLookups(t, got, tc.tp)

	// The round-tripped LookupList is empty exactly when the subset
	// produced no lookups.
	if len(got.LookupList) == 0 && len(roundTrip) == 0 {
		return
	}

	// Compare only the Subtables — Meta carries LookupType, which the
	// reader recomputes from the LookupList header, plus other fields
	// that are preserved across the round trip.  Subtables are the
	// interesting bit and where the iteration-order bug would surface.
	if len(got.LookupList) != len(roundTrip) {
		t.Fatalf("lookup count mismatch: subset=%d roundtrip=%d",
			len(got.LookupList), len(roundTrip))
	}
	// cmpopts.EquateEmpty treats nil/empty slices and maps as equal —
	// the encoder writes empty Actions/Backtrack/etc. as zero counts,
	// and the reader builds nil slices for zero counts.
	opts := cmp.Options{cmpopts.EquateEmpty()}
	for i := range got.LookupList {
		want := got.LookupList[i].Subtables
		gotSubs := roundTrip[i].Subtables
		if diff := cmp.Diff(want, gotSubs, opts); diff != "" {
			t.Errorf("lookup %d round trip failed (-want +got):\n%s", i, diff)
		}
	}
}

// subsetRoundTripCases covers every subset path that builds a parallel
// coverage table or coverage set.  Each case uses enough glyphs that
// random map iteration is overwhelmingly likely to produce a non-
// monotonic coverage table before the fix in s.glyphs-ordered
// iteration.  keep deliberately maps surviving glyphs to non-contiguous
// new gids to broaden the test coverage.
var subsetRoundTripCases = []subsetRoundTripCase{
	{
		name: "Gpos1_1",
		keep: []glyph.ID{0, 1, 3, 5, 7, 9},
		info: wrapLookup(1, &gtab.Gpos1_1{
			Cov:    coverage.Table{1: 0, 3: 1, 5: 2, 7: 3, 9: 4},
			Adjust: &gtab.GposValueRecord{XAdvance: 100},
		}),
		tp:     gtab.TypeGpos,
		subset: subsetGpos,
	},
	{
		name: "Gpos1_2",
		keep: []glyph.ID{0, 1, 3, 5, 7, 9},
		info: wrapLookup(1, &gtab.Gpos1_2{
			Cov: coverage.Table{1: 0, 3: 1, 5: 2, 7: 3, 9: 4},
			Adjust: []*gtab.GposValueRecord{
				{XAdvance: 10},
				{XAdvance: 20},
				{XAdvance: 30},
				{XAdvance: 40},
				{XAdvance: 50},
			},
		}),
		tp:     gtab.TypeGpos,
		subset: subsetGpos,
	},
	{
		name: "Gpos2_1",
		keep: []glyph.ID{0, 1, 3, 5, 7},
		info: wrapLookup(2, gtab.Gpos2_1{
			{Left: 1, Right: 3}: &gtab.PairAdjust{
				First: &gtab.GposValueRecord{XAdvance: -10},
			},
			{Left: 3, Right: 5}: &gtab.PairAdjust{
				First: &gtab.GposValueRecord{XAdvance: -20},
			},
			{Left: 5, Right: 7}: &gtab.PairAdjust{
				First: &gtab.GposValueRecord{XAdvance: -30},
			},
		}),
		tp:     gtab.TypeGpos,
		subset: subsetGpos,
	},
	{
		name: "Gpos2_2",
		keep: []glyph.ID{0, 1, 3, 5, 7},
		info: wrapLookup(2, &gtab.Gpos2_2{
			Cov:    coverage.Set{1: true, 3: true, 5: true, 7: true},
			Class1: classdef.Table{1: 1, 3: 1, 5: 2, 7: 2},
			Class2: classdef.Table{1: 1, 3: 2, 5: 1, 7: 2},
			Adjust: [][]*gtab.PairAdjust{
				{
					{First: &gtab.GposValueRecord{XAdvance: 1}},
					{First: &gtab.GposValueRecord{XAdvance: 2}},
					{First: &gtab.GposValueRecord{XAdvance: 3}},
				},
				{
					{First: &gtab.GposValueRecord{XAdvance: 4}},
					{First: &gtab.GposValueRecord{XAdvance: 5}},
					{First: &gtab.GposValueRecord{XAdvance: 6}},
				},
				{
					{First: &gtab.GposValueRecord{XAdvance: 7}},
					{First: &gtab.GposValueRecord{XAdvance: 8}},
					{First: &gtab.GposValueRecord{XAdvance: 9}},
				},
			},
		}),
		tp:     gtab.TypeGpos,
		subset: subsetGpos,
	},
	{
		name: "Gpos3_1",
		keep: []glyph.ID{0, 1, 3, 5, 7, 9},
		info: wrapLookup(3, &gtab.Gpos3_1{
			Cov: coverage.Table{1: 0, 3: 1, 5: 2, 7: 3, 9: 4},
			Records: []gtab.EntryExitRecord{
				{Entry: anchor.Table{X: 1, Y: 2}, Exit: anchor.Table{X: 3, Y: 4}},
				{Entry: anchor.Table{X: 5, Y: 6}, Exit: anchor.Table{X: 7, Y: 8}},
				{Entry: anchor.Table{X: 9, Y: 10}},
				{Exit: anchor.Table{X: 11, Y: 12}},
				{Entry: anchor.Table{X: 13, Y: 14}, Exit: anchor.Table{X: 15, Y: 16}},
			},
		}),
		tp:     gtab.TypeGpos,
		subset: subsetGpos,
	},
	{
		name: "Gpos4_1",
		keep: []glyph.ID{0, 10, 11, 12, 20, 21, 22},
		info: wrapLookup(4, &gtab.Gpos4_1{
			MarkCov: coverage.Table{10: 0, 11: 1, 12: 2},
			BaseCov: coverage.Table{20: 0, 21: 1, 22: 2},
			MarkArray: []markarray.Record{
				{Class: 0, Table: anchor.Table{X: 1, Y: 2}},
				{Class: 1, Table: anchor.Table{X: 3, Y: 4}},
				{Class: 0, Table: anchor.Table{X: 5, Y: 6}},
			},
			BaseArray: [][]anchor.Table{
				{{X: 10, Y: 11}, {X: 12, Y: 13}},
				{{X: 14, Y: 15}, {X: 16, Y: 17}},
				{{X: 18, Y: 19}, {X: 20, Y: 21}},
			},
		}),
		tp:     gtab.TypeGpos,
		subset: subsetGpos,
	},
	{
		name: "Gpos5_1",
		keep: []glyph.ID{0, 10, 11, 12, 20, 21},
		info: wrapLookup(5, &gtab.Gpos5_1{
			MarkCov: coverage.Table{10: 0, 11: 1, 12: 2},
			LigCov:  coverage.Table{20: 0, 21: 1},
			MarkArray: []markarray.Record{
				{Class: 0, Table: anchor.Table{X: 1, Y: 2}},
				{Class: 1, Table: anchor.Table{X: 3, Y: 4}},
				{Class: 0, Table: anchor.Table{X: 5, Y: 6}},
			},
			LigArray: [][][]anchor.Table{
				{
					{{X: 10, Y: 11}, {X: 12, Y: 13}},
					{{X: 14, Y: 15}, {}},
				},
				{
					{{X: 30, Y: 31}, {X: 32, Y: 33}},
				},
			},
		}),
		tp:     gtab.TypeGpos,
		subset: subsetGpos,
	},
	{
		name: "Gpos6_1",
		keep: []glyph.ID{0, 10, 11, 12, 20, 21, 22},
		info: wrapLookup(6, &gtab.Gpos6_1{
			Mark1Cov: coverage.Table{10: 0, 11: 1, 12: 2},
			Mark2Cov: coverage.Table{20: 0, 21: 1, 22: 2},
			Mark1Array: []markarray.Record{
				{Class: 0, Table: anchor.Table{X: 1, Y: 2}},
				{Class: 1, Table: anchor.Table{X: 3, Y: 4}},
				{Class: 0, Table: anchor.Table{X: 5, Y: 6}},
			},
			Mark2Array: [][]anchor.Table{
				{{X: 10, Y: 11}, {X: 12, Y: 13}},
				{{X: 14, Y: 15}, {X: 16, Y: 17}},
				{{X: 18, Y: 19}, {X: 20, Y: 21}},
			},
		}),
		tp:     gtab.TypeGpos,
		subset: subsetGpos,
	},
	{
		name: "Gsub1_1",
		keep: []glyph.ID{0, 1, 3, 5, 7, 9, 100, 103, 105, 107, 109},
		// Gsub1_1 substitutes each covered glyph with gid + Delta.
		// Keep all "target" glyphs (oldGid + 99) so getNewGid is a lookup.
		// The subsetter rewrites Gsub1_1 to Gsub1_2 (a constant offset
		// would no longer hold after gid remapping).
		info: wrapLookup(1, &gtab.Gsub1_1{
			Cov:   coverage.Set{1: true, 3: true, 5: true, 7: true, 9: true},
			Delta: 99,
		}),
		tp:     gtab.TypeGsub,
		subset: subsetGsub,
	},
	{
		name: "Gsub1_2",
		keep: []glyph.ID{0, 1, 3, 5, 7, 9, 50, 60, 70, 80, 90},
		info: wrapLookup(1, &gtab.Gsub1_2{
			Cov:                coverage.Table{1: 0, 3: 1, 5: 2, 7: 3, 9: 4},
			SubstituteGlyphIDs: []glyph.ID{50, 60, 70, 80, 90},
		}),
		tp:     gtab.TypeGsub,
		subset: subsetGsub,
	},
	{
		name: "Gsub2_1",
		keep: []glyph.ID{0, 1, 3, 5, 50, 60, 70, 80},
		info: wrapLookup(2, &gtab.Gsub2_1{
			Cov: coverage.Table{1: 0, 3: 1, 5: 2},
			Repl: [][]glyph.ID{
				{50, 60},
				{70, 80},
				{50, 80},
			},
		}),
		tp:     gtab.TypeGsub,
		subset: subsetGsub,
	},
	{
		name: "Gsub3_1",
		keep: []glyph.ID{0, 1, 3, 5, 50, 60, 70, 80, 90},
		info: wrapLookup(3, &gtab.Gsub3_1{
			Cov: coverage.Table{1: 0, 3: 1, 5: 2},
			Alternates: [][]glyph.ID{
				{50, 60, 70},
				{80, 90},
				{60, 90},
			},
		}),
		tp:     gtab.TypeGsub,
		subset: subsetGsub,
	},
	{
		name: "Gsub4_1",
		keep: []glyph.ID{0, 1, 3, 5, 7, 50, 60},
		info: wrapLookup(4, &gtab.Gsub4_1{
			Cov: coverage.Table{1: 0, 3: 1, 5: 2},
			Repl: [][]gtab.Ligature{
				{{In: []glyph.ID{3, 5}, Out: 50}},
				{{In: []glyph.ID{5, 7}, Out: 60}},
				{{In: []glyph.ID{7}, Out: 50}},
			},
		}),
		tp:     gtab.TypeGsub,
		subset: subsetGsub,
	},
	{
		name: "Gsub8_1",
		keep: []glyph.ID{0, 1, 3, 5, 7, 9, 50, 60, 70},
		info: wrapLookup(8, &gtab.Gsub8_1{
			Input:              coverage.Table{1: 0, 3: 1, 5: 2},
			Backtrack:          []coverage.Table{{7: 0, 9: 1}},
			Lookahead:          []coverage.Table{{7: 0, 9: 1}},
			SubstituteGlyphIDs: []glyph.ID{50, 60, 70},
		}),
		tp:     gtab.TypeGsub,
		subset: subsetGsub,
	},
	{
		name: "SeqContext1-Gsub",
		keep: []glyph.ID{0, 1, 3, 5, 7, 9},
		info: wrapLookup(5, &gtab.SeqContext1{
			Cov: coverage.Table{1: 0, 3: 1, 5: 2},
			Rules: [][]*gtab.SeqRule{
				{{Input: []glyph.ID{3}, Actions: []gtab.SeqLookup{}}},
				{{Input: []glyph.ID{5}, Actions: []gtab.SeqLookup{}}},
				{{Input: []glyph.ID{7}, Actions: []gtab.SeqLookup{}}},
			},
		}),
		tp:     gtab.TypeGsub,
		subset: subsetGsub,
	},
	{
		name: "SeqContext2-Gsub",
		keep: []glyph.ID{0, 1, 3, 5, 7, 9},
		info: wrapLookup(5, &gtab.SeqContext2{
			Cov:   coverage.Table{1: 0, 3: 1, 5: 2, 7: 3, 9: 4},
			Input: classdef.Table{1: 1, 3: 1, 5: 2, 7: 2, 9: 1},
			Rules: [][]*gtab.ClassSeqRule{
				nil,
				{{Input: []uint16{2}, Actions: []gtab.SeqLookup{}}},
				{{Input: []uint16{1}, Actions: []gtab.SeqLookup{}}},
			},
		}),
		tp:     gtab.TypeGsub,
		subset: subsetGsub,
	},
	{
		name: "ChainedSeqContext1-Gsub",
		keep: []glyph.ID{0, 1, 3, 5, 7, 9, 11},
		info: wrapLookup(6, &gtab.ChainedSeqContext1{
			Cov: coverage.Table{1: 0, 3: 1, 5: 2},
			Rules: [][]*gtab.ChainedSeqRule{
				{{Backtrack: []glyph.ID{11}, Input: []glyph.ID{3}, Lookahead: []glyph.ID{7}, Actions: []gtab.SeqLookup{}}},
				{{Backtrack: []glyph.ID{9}, Input: []glyph.ID{5}, Lookahead: []glyph.ID{}, Actions: []gtab.SeqLookup{}}},
				{{Backtrack: []glyph.ID{}, Input: []glyph.ID{7, 9}, Lookahead: []glyph.ID{11}, Actions: []gtab.SeqLookup{}}},
			},
		}),
		tp:     gtab.TypeGsub,
		subset: subsetGsub,
	},
	{
		name: "SeqContext3-Gsub",
		keep: []glyph.ID{0, 1, 3, 5, 7, 9},
		info: wrapLookup(5, &gtab.SeqContext3{
			Input: []coverage.Set{
				{1: true, 3: true, 5: true},
				{3: true, 5: true, 7: true},
				{5: true, 7: true, 9: true},
			},
			Actions: []gtab.SeqLookup{},
		}),
		tp:     gtab.TypeGsub,
		subset: subsetGsub,
	},
	{
		name: "ChainedSeqContext2-Gsub",
		keep: []glyph.ID{0, 1, 3, 5, 7, 9, 11},
		info: wrapLookup(6, &gtab.ChainedSeqContext2{
			Cov:       coverage.Table{1: 0, 3: 1, 5: 2},
			Backtrack: classdef.Table{1: 1, 3: 1, 11: 2},
			Input:     classdef.Table{1: 1, 3: 1, 5: 2, 7: 2},
			Lookahead: classdef.Table{7: 1, 9: 1, 11: 2},
			Rules: [][]*gtab.ChainedClassSeqRule{
				nil,
				{
					{Backtrack: []uint16{1}, Input: []uint16{2}, Lookahead: []uint16{1}, Actions: []gtab.SeqLookup{}},
					{Backtrack: []uint16{2}, Input: []uint16{1}, Lookahead: []uint16{2}, Actions: []gtab.SeqLookup{}},
				},
				{{Backtrack: []uint16{1}, Input: []uint16{1}, Lookahead: []uint16{1}, Actions: []gtab.SeqLookup{}}},
			},
		}),
		tp:     gtab.TypeGsub,
		subset: subsetGsub,
	},
	{
		name: "ChainedSeqContext3-Gsub",
		keep: []glyph.ID{0, 1, 3, 5, 7, 9, 11},
		info: wrapLookup(6, &gtab.ChainedSeqContext3{
			Backtrack: []coverage.Set{{9: true, 11: true}},
			Input:     []coverage.Set{{1: true, 3: true}, {3: true, 5: true}},
			Lookahead: []coverage.Set{{7: true, 9: true}},
			Actions:   []gtab.SeqLookup{},
		}),
		tp:     gtab.TypeGsub,
		subset: subsetGsub,
	},
	{
		name: "SeqContext1-Gpos",
		keep: []glyph.ID{0, 1, 3, 5, 7, 9},
		info: wrapLookup(7, &gtab.SeqContext1{
			Cov: coverage.Table{1: 0, 3: 1, 5: 2},
			Rules: [][]*gtab.SeqRule{
				{{Input: []glyph.ID{3}, Actions: []gtab.SeqLookup{}}},
				{{Input: []glyph.ID{5}, Actions: []gtab.SeqLookup{}}},
				{{Input: []glyph.ID{7}, Actions: []gtab.SeqLookup{}}},
			},
		}),
		tp:     gtab.TypeGpos,
		subset: subsetGpos,
	},
	{
		name: "SeqContext2-Gpos",
		keep: []glyph.ID{0, 1, 3, 5, 7, 9},
		info: wrapLookup(7, &gtab.SeqContext2{
			Cov:   coverage.Table{1: 0, 3: 1, 5: 2, 7: 3, 9: 4},
			Input: classdef.Table{1: 1, 3: 1, 5: 2, 7: 2, 9: 1},
			Rules: [][]*gtab.ClassSeqRule{
				nil,
				{{Input: []uint16{2}, Actions: []gtab.SeqLookup{}}},
				{{Input: []uint16{1}, Actions: []gtab.SeqLookup{}}},
			},
		}),
		tp:     gtab.TypeGpos,
		subset: subsetGpos,
	},
	{
		name: "SeqContext3-Gpos",
		keep: []glyph.ID{0, 1, 3, 5, 7, 9},
		info: wrapLookup(7, &gtab.SeqContext3{
			Input: []coverage.Set{
				{1: true, 3: true, 5: true},
				{3: true, 5: true, 7: true},
				{5: true, 7: true, 9: true},
			},
			Actions: []gtab.SeqLookup{},
		}),
		tp:     gtab.TypeGpos,
		subset: subsetGpos,
	},
	{
		name: "ChainedSeqContext1-Gpos",
		keep: []glyph.ID{0, 1, 3, 5, 7, 9, 11},
		info: wrapLookup(8, &gtab.ChainedSeqContext1{
			Cov: coverage.Table{1: 0, 3: 1, 5: 2},
			Rules: [][]*gtab.ChainedSeqRule{
				{{Backtrack: []glyph.ID{11}, Input: []glyph.ID{3}, Lookahead: []glyph.ID{7}, Actions: []gtab.SeqLookup{}}},
				{{Backtrack: []glyph.ID{9}, Input: []glyph.ID{5}, Lookahead: []glyph.ID{}, Actions: []gtab.SeqLookup{}}},
				{{Backtrack: []glyph.ID{}, Input: []glyph.ID{7, 9}, Lookahead: []glyph.ID{11}, Actions: []gtab.SeqLookup{}}},
			},
		}),
		tp:     gtab.TypeGpos,
		subset: subsetGpos,
	},
	{
		name: "ChainedSeqContext2-Gpos",
		keep: []glyph.ID{0, 1, 3, 5, 7, 9, 11},
		info: wrapLookup(8, &gtab.ChainedSeqContext2{
			Cov:       coverage.Table{1: 0, 3: 1, 5: 2},
			Backtrack: classdef.Table{1: 1, 3: 1, 11: 2},
			Input:     classdef.Table{1: 1, 3: 1, 5: 2, 7: 2},
			Lookahead: classdef.Table{7: 1, 9: 1, 11: 2},
			Rules: [][]*gtab.ChainedClassSeqRule{
				nil,
				{
					{Backtrack: []uint16{1}, Input: []uint16{2}, Lookahead: []uint16{1}, Actions: []gtab.SeqLookup{}},
					{Backtrack: []uint16{2}, Input: []uint16{1}, Lookahead: []uint16{2}, Actions: []gtab.SeqLookup{}},
				},
				{{Backtrack: []uint16{1}, Input: []uint16{1}, Lookahead: []uint16{1}, Actions: []gtab.SeqLookup{}}},
			},
		}),
		tp:     gtab.TypeGpos,
		subset: subsetGpos,
	},
	{
		name: "ChainedSeqContext3-Gpos",
		keep: []glyph.ID{0, 1, 3, 5, 7, 9, 11},
		info: wrapLookup(8, &gtab.ChainedSeqContext3{
			Backtrack: []coverage.Set{{9: true, 11: true}},
			Input:     []coverage.Set{{1: true, 3: true}, {3: true, 5: true}},
			Lookahead: []coverage.Set{{7: true, 9: true}},
			Actions:   []gtab.SeqLookup{},
		}),
		tp:     gtab.TypeGpos,
		subset: subsetGpos,
	},
}

func TestSubsetRoundTrip(t *testing.T) {
	for _, tc := range subsetRoundTripCases {
		t.Run(tc.name, func(t *testing.T) {
			subsetAndRoundTrip(t, tc)
		})
	}
}

// FuzzSubsetRoundTrip mutates the "keep" list while iterating every
// case from subsetRoundTripCases, asserting that the subset →
// encode → read cycle is correct for every combination of surviving
// glyph IDs the fuzzer explores.
//
// The fuzz input is a byte sequence; each byte is read as a glyph ID,
// and the result is deduplicated.  gid 0 is always prepended to honour
// the public Font.Subset contract.
func FuzzSubsetRoundTrip(f *testing.F) {
	// Seed corpus: one entry per case, encoding the case's own keep
	// list.  Map glyph IDs that fit in a byte are kept; the others
	// are skipped by the seed (still preserved by the test cases).
	for _, tc := range subsetRoundTripCases {
		seed := make([]byte, 0, len(tc.keep))
		for _, g := range tc.keep {
			if g <= 0xFF {
				seed = append(seed, byte(g))
			}
		}
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		keep := []glyph.ID{0}
		seen := map[glyph.ID]bool{0: true}
		for _, b := range data {
			gid := glyph.ID(b)
			if seen[gid] {
				continue
			}
			seen[gid] = true
			keep = append(keep, gid)
		}
		for _, tc := range subsetRoundTripCases {
			tc.keep = keep
			subsetAndRoundTrip(t, tc)
		}
	})
}
