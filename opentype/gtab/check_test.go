// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/text/language"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/markarray"
	"seehuhn.de/go/sfnt/parser"
)

// TestEncodeLookupTypeMismatch checks that a lookup whose type does not
// match its subtables cannot be encoded.
func TestEncodeLookupTypeMismatch(t *testing.T) {
	gsub := &Gsub1_1{Cov: coverage.Set{1: true}, Delta: 1}
	gpos := &Gpos1_1{Cov: coverage.Table{1: 0}, Adjust: &GposValueRecord{XAdvance: 10}}
	cases := []struct {
		name       string
		tp         Type
		lookupType uint16
		subtable   Subtable
	}{
		{"no type", TypeGsub, 0, gsub},
		{"wrong type", TypeGsub, 2, gsub},
		{"GPOS subtable in GSUB table", TypeGsub, 1, gpos},
		{"GSUB subtable in GPOS table", TypeGpos, 1, gsub},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info := &Info{LookupList: LookupList{{
				Meta:      &LookupMetaInfo{LookupType: c.lookupType},
				Subtables: []Subtable{c.subtable},
			}}}
			if _, err := info.Encode(c.tp); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

// TestEncodeInvalidCoverage checks that a subtable with a coverage table
// whose indices are not in order of glyph ID cannot be encoded.
func TestEncodeInvalidCoverage(t *testing.T) {
	good := coverage.Table{1: 0, 2: 1}
	bad := coverage.Table{1: 1, 2: 0}
	marks := []markarray.Record{{}, {}}
	anchors := [][]*anchor.Table{{nil}, {nil}}
	cases := []struct {
		name       string
		tp         Type
		lookupType uint16
		subtable   Subtable
	}{
		{"Gsub8_1 backtrack", TypeGsub, 8, &Gsub8_1{
			Input:              good,
			Backtrack:          []coverage.Table{bad},
			SubstituteGlyphIDs: []glyph.ID{3, 4},
		}},
		{"Gsub8_1 lookahead", TypeGsub, 8, &Gsub8_1{
			Input:              good,
			Lookahead:          []coverage.Table{good, bad},
			SubstituteGlyphIDs: []glyph.ID{3, 4},
		}},
		{"Gpos1_1", TypeGpos, 1, &Gpos1_1{Cov: bad, Adjust: &GposValueRecord{XAdvance: 10}}},
		{"Gpos4_1 mark", TypeGpos, 4, &Gpos4_1{
			MarkCov: bad, MarkArray: marks,
			BaseCov: good, BaseArray: anchors,
		}},
		{"Gpos4_1 base", TypeGpos, 4, &Gpos4_1{
			MarkCov: good, MarkArray: marks,
			BaseCov: bad, BaseArray: anchors,
		}},
		{"Gpos5_1 mark", TypeGpos, 5, &Gpos5_1{
			MarkCov: bad, MarkArray: marks,
			LigCov: good, LigArray: [][][]*anchor.Table{{{nil}}, {{nil}}},
		}},
		{"Gpos5_1 ligature", TypeGpos, 5, &Gpos5_1{
			MarkCov: good, MarkArray: marks,
			LigCov: bad, LigArray: [][][]*anchor.Table{{{nil}}, {{nil}}},
		}},
		{"Gpos6_1 mark1", TypeGpos, 6, &Gpos6_1{
			Mark1Cov: bad, Mark1Array: marks,
			Mark2Cov: good, Mark2Array: anchors,
		}},
		{"Gpos6_1 mark2", TypeGpos, 6, &Gpos6_1{
			Mark1Cov: good, Mark1Array: marks,
			Mark2Cov: bad, Mark2Array: anchors,
		}},
		{"SeqContext2", TypeGsub, 5, &SeqContext2{Cov: bad}},
		{"ChainedSeqContext2", TypeGsub, 6, &ChainedSeqContext2{Cov: bad}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info := &Info{LookupList: LookupList{{
				Meta:      &LookupMetaInfo{LookupType: c.lookupType},
				Subtables: []Subtable{c.subtable},
			}}}
			if _, err := info.Encode(c.tp); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

// TestEncodeEmptyContextInput checks that a coverage-based contextual
// subtable without an input sequence cannot be encoded.
func TestEncodeEmptyContextInput(t *testing.T) {
	cases := []struct {
		name       string
		lookupType uint16
		subtable   Subtable
	}{
		{"SeqContext3", 5, &SeqContext3{}},
		{"ChainedSeqContext3", 6, &ChainedSeqContext3{
			Backtrack: []coverage.Set{{1: true}},
			Lookahead: []coverage.Set{{2: true}},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info := &Info{LookupList: LookupList{{
				Meta:      &LookupMetaInfo{LookupType: c.lookupType},
				Subtables: []Subtable{c.subtable},
			}}}
			if _, err := info.Encode(TypeGsub); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

// TestEncodeInvalidFeatureVariations checks that feature variations which
// refer to a missing feature or lookup cannot be encoded.
func TestEncodeInvalidFeatureVariations(t *testing.T) {
	cases := map[string]FeatureSubstitution{
		"feature": {FeatureIndex: 1, Lookups: []LookupIndex{0}},
		"lookup":  {FeatureIndex: 0, Lookups: []LookupIndex{1}},
	}
	for name, sub := range cases {
		t.Run(name, func(t *testing.T) {
			info := &Info{
				FeatureList: FeatureListInfo{{Tag: "test", Lookups: []LookupIndex{0}}},
				LookupList: LookupList{{
					Meta:      &LookupMetaInfo{LookupType: 1},
					Subtables: []Subtable{&Gsub1_1{Cov: coverage.Set{1: true}, Delta: 1}},
				}},
				Variations: []FeatureVariationRecord{{
					Substitutions: []FeatureSubstitution{sub},
				}},
			}
			if _, err := info.Encode(TypeGsub); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

// markFilteringTestInfo returns a table with four lookups.  Lookups 0 and 1
// select no mark filtering set, lookup 2 selects set 2 and lookup 3 selects
// set 5.
func markFilteringTestInfo() *Info {
	lookup := func(flags LookupFlags, set uint16) *LookupTable {
		return &LookupTable{
			Meta: &LookupMetaInfo{
				LookupType:       1,
				LookupFlags:      flags,
				MarkFilteringSet: set,
			},
			Subtables: []Subtable{&Gsub1_1{Cov: coverage.Set{1: true}, Delta: 1}},
		}
	}
	return &Info{
		ScriptList: ScriptListInfo{
			language.MustParse("und-Latn-x-latn"): {Optional: []FeatureIndex{0}},
		},
		FeatureList: FeatureListInfo{{Tag: "test", Lookups: []LookupIndex{0, 1, 2, 3}}},
		LookupList: LookupList{
			lookup(0, 0),
			lookup(IgnoreLigatures, 9), // the set index is unused without the flag
			lookup(UseMarkFilteringSet, 2),
			lookup(UseMarkFilteringSet|IgnoreBaseGlyphs, 5),
		},
	}
}

func TestCheckMarkFilteringSets(t *testing.T) {
	info := markFilteringTestInfo()
	for _, numSets := range []int{6, 7, 1000} {
		if err := info.CheckMarkFilteringSets(numSets); err != nil {
			t.Errorf("%d sets: %v", numSets, err)
		}
	}
	for _, numSets := range []int{0, 2, 3, 5} {
		if err := info.CheckMarkFilteringSets(numSets); err == nil {
			t.Errorf("%d sets: expected an error", numSets)
		}
	}

	// without a lookup which selects a set, no sets are needed
	info.LookupList = info.LookupList[:2]
	if err := info.CheckMarkFilteringSets(0); err != nil {
		t.Error(err)
	}
}

func TestLimitMarkFilteringSets(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		info := markFilteringTestInfo()
		if got := info.LimitMarkFilteringSets(6); got != info {
			t.Error("table without undefined sets was not returned as is")
		}
	})

	// The lookups before firstInvalid select no undefined set.
	cases := []struct {
		numSets      int
		firstInvalid int
	}{
		{5, 3},
		{3, 3},
		{2, 2},
		{0, 2},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%d sets", c.numSets), func(t *testing.T) {
			info := markFilteringTestInfo()
			if err := info.CheckMarkFilteringSets(c.numSets); err == nil {
				t.Fatal("test table has no undefined set")
			}

			got := info.LimitMarkFilteringSets(c.numSets)
			if err := got.CheckMarkFilteringSets(c.numSets); err != nil {
				t.Error(err)
			}
			if len(got.LookupList) != len(info.LookupList) {
				t.Fatalf("got %d lookups, want %d", len(got.LookupList), len(info.LookupList))
			}
			if diff := cmp.Diff(info.LookupList[:c.firstInvalid], got.LookupList[:c.firstInvalid]); diff != "" {
				t.Errorf("valid lookups changed (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(info.ScriptList, got.ScriptList); diff != "" {
				t.Errorf("script list changed (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(info.FeatureList, got.FeatureList); diff != "" {
				t.Errorf("feature list changed (-want +got):\n%s", diff)
			}

			if diff := cmp.Diff(markFilteringTestInfo(), info); diff != "" {
				t.Errorf("receiver was modified (-want +got):\n%s", diff)
			}
		})
	}
}

// readGsub reads a GSUB table.
func readGsub(t *testing.T, data []byte) *Info {
	t.Helper()
	info, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))), TypeGsub)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

// checkReadRoundTrip reads a GSUB table, writes it, and checks that reading
// the result gives the same table.
func checkReadRoundTrip(t *testing.T, data []byte) {
	t.Helper()
	info1 := readGsub(t, data)
	data2, err := info1.Encode(TypeGsub)
	if err != nil {
		t.Fatal(err)
	}
	info2 := readGsub(t, data2)
	if diff := cmp.Diff(info1, info2, cmpopts.EquateEmpty()); diff != "" {
		t.Errorf("round trip failed (-first +second):\n%s", diff)
	}
}

// TestReadNestedLookupOutOfRange checks that a table in which a contextual
// rule refers to a missing lookup can be read, and that the result survives
// a write/read cycle.
func TestReadNestedLookupOutOfRange(t *testing.T) {
	info := &Info{
		ScriptList: ScriptListInfo{
			language.MustParse("und-Latn-x-latn"): {Optional: []FeatureIndex{0}},
		},
		FeatureList: FeatureListInfo{{Tag: "test", Lookups: []LookupIndex{0}}},
		LookupList: LookupList{
			{
				Meta: &LookupMetaInfo{LookupType: 5},
				Subtables: []Subtable{&SeqContext3{
					Input: []coverage.Set{{1: true}},
					Actions: []SeqLookup{
						{SequenceIndex: 0, LookupListIndex: 1},
						{SequenceIndex: 0, LookupListIndex: 2},
					},
				}},
			},
			{
				Meta:      &LookupMetaInfo{LookupType: 1},
				Subtables: []Subtable{&Gsub1_1{Cov: coverage.Set{1: true}, Delta: 1}},
			},
			{
				Meta:      &LookupMetaInfo{LookupType: 1},
				Subtables: []Subtable{&Gsub1_1{Cov: coverage.Set{2: true}, Delta: 1}},
			},
		},
	}
	data, err := info.Encode(TypeGsub)
	if err != nil {
		t.Fatal(err)
	}

	// drop the last lookup from the lookup list
	lookupListOffset := int(data[8])<<8 | int(data[9])
	if n := int(data[lookupListOffset])<<8 | int(data[lookupListOffset+1]); n != 3 {
		t.Fatalf("unexpected lookup count %d", n)
	}
	data[lookupListOffset+1] = 2

	checkReadRoundTrip(t, data)
}

// TestReadSharedFeatureLookupOutOfRange checks that a table in which many
// features share a Feature table which refers to a missing lookup can be
// read, and that the result survives a write/read cycle.
func TestReadSharedFeatureLookupOutOfRange(t *testing.T) {
	const numFeatures = 100
	lookups := make([]LookupIndex, 1000)
	for i := range lookups {
		lookups[i] = LookupIndex(i % 2)
	}
	info := &Info{
		ScriptList: ScriptListInfo{
			language.MustParse("und-Latn-x-latn"): {Optional: []FeatureIndex{0}},
		},
		LookupList: LookupList{
			{
				Meta:      &LookupMetaInfo{LookupType: 1},
				Subtables: []Subtable{&Gsub1_1{Cov: coverage.Set{1: true}, Delta: 1}},
			},
			{
				Meta:      &LookupMetaInfo{LookupType: 1},
				Subtables: []Subtable{&Gsub1_1{Cov: coverage.Set{2: true}, Delta: 1}},
			},
		},
	}
	for i := range numFeatures {
		info.FeatureList = append(info.FeatureList,
			&Feature{Tag: fmt.Sprintf("f%03d", i), Lookups: lookups})
	}
	data, err := info.Encode(TypeGsub)
	if err != nil {
		t.Fatal(err)
	}

	// drop the last lookup from the lookup list
	lookupListOffset := int(data[8])<<8 | int(data[9])
	if n := int(data[lookupListOffset])<<8 | int(data[lookupListOffset+1]); n != 2 {
		t.Fatalf("unexpected lookup count %d", n)
	}
	data[lookupListOffset+1] = 1

	checkReadRoundTrip(t, data)
}

// TestSharedValuesRoundTrip checks that tables which reach one value through
// many offsets can be read and written, and that the result survives a
// write/read cycle.  The work must be in proportion to the size of the
// values, not to the number of references.
func TestSharedValuesRoundTrip(t *testing.T) {
	const n = 7000

	actions := make([]SeqLookup, n)
	for i := range actions {
		actions[i] = SeqLookup{LookupListIndex: 1}
	}
	rule := &SeqRule{Actions: actions}
	ruleSet := make([]*SeqRule, n)
	for i := range ruleSet {
		ruleSet[i] = rule
	}
	cov := coverage.Table{}
	rules := make([][]*SeqRule, n)
	for i := range n {
		cov[glyph.ID(i)] = i
		rules[i] = ruleSet
	}

	big := coverage.Table{}
	for i := range 0xFFFF {
		big[glyph.ID(i)] = i
	}
	backtrack := make([]coverage.Table, 20000)
	for i := range backtrack {
		backtrack[i] = big
	}

	cases := map[string]Subtable{
		"rules": &SeqContext1{Cov: cov, Rules: rules},
		"coverage": &Gsub8_1{
			Input:              coverage.Table{1: 0},
			Backtrack:          backtrack,
			SubstituteGlyphIDs: []glyph.ID{2},
		},
	}
	for name, st := range cases {
		t.Run(name, func(t *testing.T) {
			lookupType := uint16(5)
			if _, ok := st.(*Gsub8_1); ok {
				lookupType = 8
			}
			info := &Info{
				ScriptList: ScriptListInfo{
					language.MustParse("und-Latn-x-latn"): {Optional: []FeatureIndex{0}},
				},
				FeatureList: FeatureListInfo{{Tag: "test", Lookups: []LookupIndex{0}}},
				LookupList: LookupList{
					{Meta: &LookupMetaInfo{LookupType: lookupType}, Subtables: []Subtable{st}},
					{
						Meta:      &LookupMetaInfo{LookupType: 1},
						Subtables: []Subtable{&Gsub1_1{Cov: coverage.Set{1: true}, Delta: 1}},
					},
				},
			}
			data1, err := info.Encode(TypeGsub)
			if err != nil {
				t.Fatal(err)
			}
			info1 := readGsub(t, data1)
			data2, err := info1.Encode(TypeGsub)
			if err != nil {
				t.Fatal(err)
			}
			info2 := readGsub(t, data2)
			// comparing the encodings avoids visiting every reference
			data3, err := info2.Encode(TypeGsub)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data2, data3) {
				t.Error("round trip failed")
			}
		})
	}
}
