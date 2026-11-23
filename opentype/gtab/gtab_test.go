// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/text/language"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/markarray"
	"seehuhn.de/go/sfnt/parser"
)

func TestGetLookups(t *testing.T) {
	gtabInfo := Info{
		ScriptList: map[language.Tag]*Features{
			language.MustParse("und-Latn"): {
				Required: 0,
				Optional: []FeatureIndex{
					1, 2, 3,
				},
			},
		},
		FeatureList: []*Feature{
			{Tag: "dflt", Lookups: []LookupIndex{0, 3}},
			{Tag: "kern", Lookups: []LookupIndex{1, 3}},
			{Tag: "mkmk", Lookups: []LookupIndex{4, 3, 2}},
			{Tag: "test", Lookups: []LookupIndex{4, 5}},
		},
		LookupList: LookupList{
			nil, nil, nil, nil, nil, nil,
		},
	}

	cases := []struct {
		tags     []string
		expected []LookupIndex
	}{
		{nil, []LookupIndex{0, 3}},
		{[]string{"kern"}, []LookupIndex{0, 1, 3}},
		{[]string{"kern", "test"}, []LookupIndex{0, 1, 3, 4, 5}},
	}

	for _, test := range cases {
		includeFeature := map[string]bool{}
		for _, tag := range test.tags {
			includeFeature[tag] = true
		}
		ll := gtabInfo.FindLookups(language.BritishEnglish, includeFeature)
		if len(ll) != len(test.expected) {
			t.Errorf("GetLookups(%v) = %v, expected %v", test.tags, ll, test.expected)
		}
	}
}

func FuzzGtab(f *testing.F) {
	info := &Info{}
	f.Add(encodeInfo(f, info))

	info.ScriptList = ScriptListInfo{
		language.MustParse("und-Zzzz"): {
			Required: 0xFFFF,
			Optional: []FeatureIndex{1, 2, 3, 4},
		},
		language.MustParse("und-Latn"): {
			Required: 0,
			Optional: []FeatureIndex{2, 4, 5},
		},
		language.German: {
			Required: 6,
		},
	}
	info.FeatureList = FeatureListInfo{
		{Tag: "kern", Lookups: []LookupIndex{0, 1}},
		{Tag: "liga", Lookups: []LookupIndex{2, 3, 4}},
		{Tag: "frac", Lookups: []LookupIndex{1, 5}},
		{Tag: "locl", Lookups: []LookupIndex{2, 6}},
		{Tag: "onum", Lookups: []LookupIndex{3, 7}},
		{Tag: "sups", Lookups: []LookupIndex{9}},
		{Tag: "numr", Lookups: []LookupIndex{1, 9, 10}},
	}
	info.LookupList = LookupList{
		&LookupTable{
			Meta: &LookupMetaInfo{LookupType: 1},
			Subtables: []Subtable{
				dummySubtable{0},
				dummySubtable{1},
				dummySubtable{2},
			},
		},
		&LookupTable{
			Meta: &LookupMetaInfo{LookupType: 2, LookupFlags: UseMarkFilteringSet, MarkFilteringSet: 7},
			Subtables: []Subtable{
				dummySubtable{3, 4},
				dummySubtable{5, 6},
			},
		},
		&LookupTable{
			Meta: &LookupMetaInfo{LookupType: 3},
			Subtables: []Subtable{
				dummySubtable{7, 8, 9},
			},
		},
		&LookupTable{
			Meta: &LookupMetaInfo{LookupType: 1},
			Subtables: []Subtable{
				dummySubtable{0},
				dummySubtable{1},
				dummySubtable{2},
			},
		},
		&LookupTable{
			Meta: &LookupMetaInfo{LookupType: 2, LookupFlags: UseMarkFilteringSet, MarkFilteringSet: 7},
			Subtables: []Subtable{
				dummySubtable{3, 4},
				dummySubtable{5, 6},
			},
		},
		&LookupTable{
			Meta: &LookupMetaInfo{LookupType: 3},
			Subtables: []Subtable{
				dummySubtable{7, 8, 9},
			},
		},
		&LookupTable{
			Meta: &LookupMetaInfo{LookupType: 1},
			Subtables: []Subtable{
				dummySubtable{0},
				dummySubtable{1},
				dummySubtable{2},
			},
		},
		&LookupTable{
			Meta: &LookupMetaInfo{LookupType: 2, LookupFlags: UseMarkFilteringSet, MarkFilteringSet: 7},
			Subtables: []Subtable{
				dummySubtable{3, 4},
				dummySubtable{5, 6},
			},
		},
		&LookupTable{
			Meta: &LookupMetaInfo{LookupType: 3},
			Subtables: []Subtable{
				dummySubtable{7, 8, 9},
			},
		},
		&LookupTable{
			Meta: &LookupMetaInfo{LookupType: 5},
			Subtables: []Subtable{
				dummySubtable{10, 11, 12, 13, 14},
			},
		},
	}
	f.Add(encodeInfo(f, info))

	// version 1.1 seed with a modeled FeatureVariations table
	info.Variations = []FeatureVariationRecord{
		{
			Conditions: []Condition{
				{Format: 1, AxisIndex: 0, Min: -8192, Max: 16384},
			},
			Substitutions: []FeatureSubstitution{
				{FeatureIndex: 0, Lookups: []LookupIndex{1, 2}},
			},
		},
		{
			Conditions: []Condition{
				{Format: 1, AxisIndex: 0, Min: -16384, Max: 0},
				{Format: 1, AxisIndex: 1, Min: 4096, Max: 12288},
			},
			Substitutions: []FeatureSubstitution{
				{FeatureIndex: 1, Lookups: []LookupIndex{3}},
				{FeatureIndex: 2, Lookups: nil},
			},
		},
	}
	f.Add(encodeInfo(f, info))
	info.Variations = nil

	f.Fuzz(func(t *testing.T, data1 []byte) {
		info1, err := readGtab(bytes.NewReader(data1), parser.NewBudget(int64(len(data1))), 0, readDummySubtable)
		if err != nil {
			return
		}

		data2 := encodeInfo(t, info1)

		info2, err := readGtab(bytes.NewReader(data2), parser.NewBudget(int64(len(data2))), 0, readDummySubtable)
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(info1, info2) {
			t.Error("different")
		}
	})
}

func doFuzz(t *testing.T, lookupType, lookupFormat uint16,
	readFn func(p *parser.Parser, subtablePos int64) (Subtable, error),
	data1 []byte) {

	// t.Helper()

	p := parser.New(bytes.NewReader(data1), parser.NewBudget(int64(len(data1))))
	format, err := p.ReadUint16()
	if err != nil || format != lookupFormat {
		return
	}

	l1, err := readFn(p, 0)
	if err != nil {
		return
	}
	lay, err := l1.layout()
	if err != nil {
		// readLookupList drops such subtables
		return
	}

	data2 := lay.encode()
	if len(data2) != lay.encodeLen() {
		t.Errorf("encodeLen mismatch: %d != %d", len(data2), lay.encodeLen())
	}

	p = parser.New(bytes.NewReader(data2), parser.NewBudget(int64(len(data2))))
	// The re-encoded form may be slightly smaller than data1, which would give
	// it a smaller input-proportional budget; since it decodes to the same
	// structure that already fit in data1's budget, reuse that allowance so a
	// budget boundary does not turn a faithful round trip into a failure.
	p.Budget = parser.NewBudget(int64(len(data1)))
	format, err = p.ReadUint16()
	if err != nil {
		t.Fatal(err)
	} else if format != lookupFormat {
		t.Fatalf("unexpected format: %d.%d", lookupType, format)
	}
	l2, err := readFn(p, 0)
	if err != nil {
		t.Fatal(err)
	}

	if diff := cmp.Diff(l1, l2); diff != "" {
		t.Errorf("different (-old +new):\n%s", diff)
	}
}

func TestInfoEncodeHeaderOffsets(t *testing.T) {
	smallFeats := FeatureListInfo{{Tag: "liga", Lookups: []LookupIndex{0}}}
	smallScripts := ScriptListInfo{
		language.MustParse("und-Latn-x-latn"): {Required: 0xFFFF, Optional: []FeatureIndex{0}},
	}
	bigScripts := func(n int) ScriptListInfo {
		return ScriptListInfo{language.MustParse("und-Latn-x-latn"): bigFeatures(n)}
	}
	bigFeats := func(n int) FeatureListInfo {
		return FeatureListInfo{{Tag: "liga", Lookups: make([]LookupIndex, n)}}
	}

	// FeatureVariations data makes the table use the version 1.1 header
	for _, variations := range [][]FeatureVariationRecord{nil, {}} {
		name := "v1.0"
		if variations != nil {
			name = "v1.1"
		}
		t.Run(name, func(t *testing.T) {
			// In each case, the standard order puts a list out of reach,
			// but storing the largest list last works.
			fitting := []struct {
				name     string
				scripts  ScriptListInfo
				features FeatureListInfo
			}{
				{"big script list", bigScripts(40000), smallFeats},
				{"big feature list", smallScripts, bigFeats(40000)},
				{"two medium lists", bigScripts(20000), bigFeats(20000)},
				{"within limit", bigScripts(10000), bigFeats(10000)},
			}
			for _, c := range fitting {
				t.Run(c.name, func(t *testing.T) {
					info := &Info{
						ScriptList:  c.scripts,
						FeatureList: c.features,
						LookupList:  LookupList{},
						Variations:  variations,
					}
					data := encodeInfo(t, info)
					got, err := readGtab(bytes.NewReader(data),
						parser.NewBudget(int64(len(data))), 0, readDummySubtable)
					if err != nil {
						t.Fatal(err)
					}
					if diff := cmp.Diff(info, got); diff != "" {
						t.Errorf("round trip failed (-want +got):\n%s", diff)
					}
				})
			}

			t.Run("too large", func(t *testing.T) {
				info := &Info{
					ScriptList:  bigScripts(40000),
					FeatureList: bigFeats(40000),
					LookupList:  LookupList{},
					Variations:  variations,
				}
				if _, err := info.Encode(); err == nil {
					t.Error("Encode succeeded")
				}
			})
		})
	}
}

// TestReadGtabListsTooLarge checks that a table is rejected if its script
// and feature lists are each valid, but cannot be reached together from the
// header of the encoded table.  Two Script tables share one LangSys table
// of about 40 kB, which is copied into both on write.
func TestReadGtabListsTooLarge(t *testing.T) {
	n := 20000

	var langSys []byte
	langSys = append(langSys, 0, 0, 0xFF, 0xFF, byte(n>>8), byte(n))
	langSys = append(langSys, make([]byte, 2*n)...)

	var scriptList []byte
	scriptList = append(scriptList, 0, 2)
	scriptList = append(scriptList, "cyrl"...)
	scriptList = append(scriptList, 0, 14)
	scriptList = append(scriptList, "latn"...)
	scriptList = append(scriptList, 0, 24)
	// Script tables at 14 and 24, the LangSys table at 34
	scriptList = append(scriptList, 0, 0, 0, 1)
	scriptList = append(scriptList, "RUS "...)
	scriptList = append(scriptList, 0, 20)
	scriptList = append(scriptList, 0, 0, 0, 1)
	scriptList = append(scriptList, "DEU "...)
	scriptList = append(scriptList, 0, 10)
	scriptList = append(scriptList, langSys...)

	featureList := encodeList(t, FeatureListInfo{{Tag: "liga", Lookups: make([]LookupIndex, 40000)}})
	lookupList := []byte{0, 0}

	// the lookup list comes first, so that every offset fits
	lookupOffset := 10
	scriptOffset := lookupOffset + len(lookupList)
	featureOffset := scriptOffset + len(scriptList)
	if featureOffset > 0xFFFF {
		t.Fatal("test data does not fit")
	}
	data := []byte{0, 1, 0, 0,
		byte(scriptOffset >> 8), byte(scriptOffset),
		byte(featureOffset >> 8), byte(featureOffset),
		byte(lookupOffset >> 8), byte(lookupOffset),
	}
	data = append(data, lookupList...)
	data = append(data, scriptList...)
	data = append(data, featureList...)

	// the script list on its own can be encoded
	p := parser.New(bytes.NewReader(scriptList), parser.NewBudget(int64(len(scriptList))))
	scripts, err := readScriptList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(scripts) != 2 {
		t.Fatalf("expected 2 language systems, got %d", len(scripts))
	}

	_, err = readGtab(bytes.NewReader(data), parser.NewBudget(int64(len(data))), 0, readDummySubtable)
	if err == nil {
		t.Error("expected error, got none")
	}
}

func BenchmarkReadGtab(b *testing.B) {
	info := &Info{
		ScriptList: ScriptListInfo{
			language.MustParse("und-Latn-x-latn-dflt"): {Required: 0xFFFF, Optional: []FeatureIndex{0, 1, 2}},
			language.MustParse("und-Grek-x-grek-dflt"): {Required: 0xFFFF, Optional: []FeatureIndex{1, 2}},
		},
		LookupList: largeLookupList(20, 4, 500),
	}
	for i := range 40 {
		info.FeatureList = append(info.FeatureList, &Feature{
			Tag:     "liga",
			Lookups: []LookupIndex{LookupIndex(i % len(info.LookupList))},
		})
	}
	data := encodeInfo(b, info)

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		_, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))), TypeGsub)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// duplicateGlyphCases lists subtables where a coverage table covers three
// consecutive glyphs, starting with the glyph first, and where the data for
// the first two of these glyphs is equal.  The subtable want is expected
// when the coverage table of a file lists the first glyph twice, in place
// of the first and the second glyph.
var duplicateGlyphCases = []struct {
	name     string
	read     func(*parser.Parser, int64) (Subtable, error)
	first    glyph.ID
	in, want Subtable
}{
	{
		name: "Gsub1_2", read: readGsub1_2, first: 5,
		in: &Gsub1_2{
			Cov:                coverage.Table{5: 0, 6: 1, 7: 2},
			SubstituteGlyphIDs: []glyph.ID{20, 20, 22},
		},
		want: &Gsub1_2{
			Cov:                coverage.Table{5: 0, 7: 1},
			SubstituteGlyphIDs: []glyph.ID{20, 22},
		},
	},
	{
		name: "Gsub2_1", read: readGsub2_1, first: 5,
		in: &Gsub2_1{
			Cov:  coverage.Table{5: 0, 6: 1, 7: 2},
			Repl: [][]glyph.ID{{20, 21}, {20, 21}, {22}},
		},
		want: &Gsub2_1{
			Cov:  coverage.Table{5: 0, 7: 1},
			Repl: [][]glyph.ID{{20, 21}, {22}},
		},
	},
	{
		name: "Gsub3_1", read: readGsub3_1, first: 5,
		in: &Gsub3_1{
			Cov:        coverage.Table{5: 0, 6: 1, 7: 2},
			Alternates: [][]glyph.ID{{20, 21}, {20, 21}, {22}},
		},
		want: &Gsub3_1{
			Cov:        coverage.Table{5: 0, 7: 1},
			Alternates: [][]glyph.ID{{20, 21}, {22}},
		},
	},
	{
		name: "Gsub4_1", read: readGsub4_1, first: 5,
		in: &Gsub4_1{
			Cov: coverage.Table{5: 0, 6: 1, 7: 2},
			Repl: [][]Ligature{
				{{In: []glyph.ID{1}, Out: 20}},
				{{In: []glyph.ID{1}, Out: 20}},
				{{In: []glyph.ID{2}, Out: 22}},
			},
		},
		want: &Gsub4_1{
			Cov: coverage.Table{5: 0, 7: 1},
			Repl: [][]Ligature{
				{{In: []glyph.ID{1}, Out: 20}},
				{{In: []glyph.ID{2}, Out: 22}},
			},
		},
	},
	{
		name: "Gsub8_1", read: readGsub8_1, first: 5,
		in: &Gsub8_1{
			Input:              coverage.Table{5: 0, 6: 1, 7: 2},
			Backtrack:          []coverage.Table{{1: 0}},
			SubstituteGlyphIDs: []glyph.ID{20, 20, 22},
		},
		want: &Gsub8_1{
			Input:              coverage.Table{5: 0, 7: 1},
			Backtrack:          []coverage.Table{{1: 0}},
			SubstituteGlyphIDs: []glyph.ID{20, 22},
		},
	},
	{
		name: "Gpos1_2", read: readGpos1_2, first: 5,
		in: &Gpos1_2{
			Cov:    coverage.Table{5: 0, 6: 1, 7: 2},
			Adjust: []*GposValueRecord{{XAdvance: 10}, {XAdvance: 10}, {XAdvance: 30}},
		},
		want: &Gpos1_2{
			Cov:    coverage.Table{5: 0, 7: 1},
			Adjust: []*GposValueRecord{{XAdvance: 10}, {XAdvance: 30}},
		},
	},
	{
		name: "Gpos2_1", read: readGpos2_1, first: 5,
		in: &Gpos2_1{Adjust: map[glyph.Pair]*PairAdjust{
			{Left: 5, Right: 1}: {First: &GposValueRecord{XAdvance: 10}},
			{Left: 6, Right: 1}: {First: &GposValueRecord{XAdvance: 10}},
			{Left: 7, Right: 1}: {First: &GposValueRecord{XAdvance: 30}},
		}},
		want: &Gpos2_1{Adjust: map[glyph.Pair]*PairAdjust{
			{Left: 5, Right: 1}: {First: &GposValueRecord{XAdvance: 10}},
			{Left: 7, Right: 1}: {First: &GposValueRecord{XAdvance: 30}},
		}},
	},
	{
		name: "Gpos3_1", read: readGpos3_1, first: 5,
		in: &Gpos3_1{
			Cov: coverage.Table{5: 0, 6: 1, 7: 2},
			Records: []EntryExitRecord{
				{Entry: &anchor.Table{X: 10}},
				{Entry: &anchor.Table{X: 10}},
				{Entry: &anchor.Table{X: 30}},
			},
		},
		want: &Gpos3_1{
			Cov: coverage.Table{5: 0, 7: 1},
			Records: []EntryExitRecord{
				{Entry: &anchor.Table{X: 10}},
				{Entry: &anchor.Table{X: 30}},
			},
		},
	},
	{
		name: "Gpos4_1 marks", read: readGpos4_1, first: 5,
		in: &Gpos4_1{
			MarkCov: coverage.Table{5: 0, 6: 1, 7: 2},
			BaseCov: coverage.Table{15: 0},
			MarkArray: []markarray.Record{
				{X: 10},
				{X: 10},
				{X: 30},
			},
			BaseArray: [][]*anchor.Table{{{X: 1}}},
		},
		want: &Gpos4_1{
			MarkCov: coverage.Table{5: 0, 7: 1},
			BaseCov: coverage.Table{15: 0},
			MarkArray: []markarray.Record{
				{X: 10},
				{X: 30},
			},
			BaseArray: [][]*anchor.Table{{{X: 1}}},
		},
	},
	{
		name: "Gpos4_1 bases", read: readGpos4_1, first: 15,
		in: &Gpos4_1{
			MarkCov:   coverage.Table{5: 0},
			BaseCov:   coverage.Table{15: 0, 16: 1, 17: 2},
			MarkArray: []markarray.Record{{X: 1}},
			BaseArray: [][]*anchor.Table{{{X: 10}}, {{X: 10}}, {{X: 30}}},
		},
		want: &Gpos4_1{
			MarkCov:   coverage.Table{5: 0},
			BaseCov:   coverage.Table{15: 0, 17: 1},
			MarkArray: []markarray.Record{{X: 1}},
			BaseArray: [][]*anchor.Table{{{X: 10}}, {{X: 30}}},
		},
	},
	{
		name: "Gpos5_1 marks", read: readGpos5_1, first: 5,
		in: &Gpos5_1{
			MarkCov: coverage.Table{5: 0, 6: 1, 7: 2},
			LigCov:  coverage.Table{15: 0},
			MarkArray: []markarray.Record{
				{X: 10},
				{X: 10},
				{X: 30},
			},
			LigArray: [][][]*anchor.Table{{{{X: 1}}}},
		},
		want: &Gpos5_1{
			MarkCov: coverage.Table{5: 0, 7: 1},
			LigCov:  coverage.Table{15: 0},
			MarkArray: []markarray.Record{
				{X: 10},
				{X: 30},
			},
			LigArray: [][][]*anchor.Table{{{{X: 1}}}},
		},
	},
	{
		name: "Gpos5_1 ligatures", read: readGpos5_1, first: 15,
		in: &Gpos5_1{
			MarkCov:   coverage.Table{5: 0},
			LigCov:    coverage.Table{15: 0, 16: 1, 17: 2},
			MarkArray: []markarray.Record{{X: 1}},
			LigArray:  [][][]*anchor.Table{{{{X: 10}}}, {{{X: 10}}}, {{{X: 30}}}},
		},
		want: &Gpos5_1{
			MarkCov:   coverage.Table{5: 0},
			LigCov:    coverage.Table{15: 0, 17: 1},
			MarkArray: []markarray.Record{{X: 1}},
			LigArray:  [][][]*anchor.Table{{{{X: 10}}}, {{{X: 30}}}},
		},
	},
	{
		name: "Gpos6_1 mark1", read: readGpos6_1, first: 5,
		in: &Gpos6_1{
			Mark1Cov: coverage.Table{5: 0, 6: 1, 7: 2},
			Mark2Cov: coverage.Table{15: 0},
			Mark1Array: []markarray.Record{
				{X: 10},
				{X: 10},
				{X: 30},
			},
			Mark2Array: [][]*anchor.Table{{{X: 1}}},
		},
		want: &Gpos6_1{
			Mark1Cov: coverage.Table{5: 0, 7: 1},
			Mark2Cov: coverage.Table{15: 0},
			Mark1Array: []markarray.Record{
				{X: 10},
				{X: 30},
			},
			Mark2Array: [][]*anchor.Table{{{X: 1}}},
		},
	},
	{
		name: "Gpos6_1 mark2", read: readGpos6_1, first: 15,
		in: &Gpos6_1{
			Mark1Cov:   coverage.Table{5: 0},
			Mark2Cov:   coverage.Table{15: 0, 16: 1, 17: 2},
			Mark1Array: []markarray.Record{{X: 1}},
			Mark2Array: [][]*anchor.Table{{{X: 10}}, {{X: 10}}, {{X: 30}}},
		},
		want: &Gpos6_1{
			Mark1Cov:   coverage.Table{5: 0},
			Mark2Cov:   coverage.Table{15: 0, 17: 1},
			Mark1Array: []markarray.Record{{X: 1}},
			Mark2Array: [][]*anchor.Table{{{X: 10}}, {{X: 30}}},
		},
	},
	{
		name: "SeqContext1", read: readSeqContext1, first: 5,
		in: &SeqContext1{
			Cov: coverage.Table{5: 0, 6: 1, 7: 2},
			Rules: [][]*SeqRule{
				{{Input: []glyph.ID{1}}},
				{{Input: []glyph.ID{1}}},
				{{Input: []glyph.ID{2}}},
			},
		},
		want: &SeqContext1{
			Cov: coverage.Table{5: 0, 7: 1},
			Rules: [][]*SeqRule{
				{{Input: []glyph.ID{1}}},
				{{Input: []glyph.ID{2}}},
			},
		},
	},
	{
		name: "ChainedSeqContext1", read: readChainedSeqContext1, first: 5,
		in: &ChainedSeqContext1{
			Cov: coverage.Table{5: 0, 6: 1, 7: 2},
			Rules: [][]*ChainedSeqRule{
				{{Input: []glyph.ID{1}}},
				{{Input: []glyph.ID{1}}},
				{{Input: []glyph.ID{2}}},
			},
		},
		want: &ChainedSeqContext1{
			Cov: coverage.Table{5: 0, 7: 1},
			Rules: [][]*ChainedSeqRule{
				{{Input: []glyph.ID{1}}},
				{{Input: []glyph.ID{2}}},
			},
		},
	},
}

// duplicateGlyph returns the encoded subtable st, changed so that the
// coverage table for the glyphs first, first+1 and first+2 lists the glyph
// first twice, in place of the first two glyphs.
func duplicateGlyph(tb testing.TB, st Subtable, first glyph.ID) []byte {
	tb.Helper()
	data := encodeSubtable(tb, st)
	var cov []byte
	for _, x := range []glyph.ID{1, 3, first, first + 1, first + 2} {
		cov = append(cov, byte(x>>8), byte(x))
	}
	if bytes.Count(data, cov) != 1 {
		tb.Fatal("coverage table not found")
	}
	pos := bytes.Index(data, cov)
	data[pos+6], data[pos+7] = byte(first>>8), byte(first)
	return data
}

// TestDuplicateCoverageGlyph checks that a coverage table which lists a
// glyph twice leaves the other glyphs with their own data, for the subtables
// which hold data for each covered glyph.
func TestDuplicateCoverageGlyph(t *testing.T) {
	for _, tc := range duplicateGlyphCases {
		t.Run(tc.name, func(t *testing.T) {
			got := readSubtable(t, tc.read, duplicateGlyph(t, tc.in, tc.first))
			want := readSubtable(t, tc.read, encodeSubtable(t, tc.want))
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("wrong subtable (-want +got):\n%s", diff)
			}

			again := readSubtable(t, tc.read, encodeSubtable(t, got))
			if diff := cmp.Diff(got, again); diff != "" {
				t.Errorf("round trip failed (-first +second):\n%s", diff)
			}
		})
	}
}

// TestOverlappingCoverageRanges checks that a format 2 coverage table whose
// ranges overlap leaves each glyph with the data at the coverage index which
// its range record gives.
func TestOverlappingCoverageRanges(t *testing.T) {
	data := []byte{
		0, 2, // format
		0, 18, // coverage offset
		0, 6, // glyph count
		0, 20, 0, 21, 0, 22, 0, 23, 0, 24, 0, 25, // substitutes
		// coverage, format 2: glyphs 5-7 from index 0, glyphs 7-9 from index 3
		0, 2, 0, 2,
		0, 5, 0, 7, 0, 0,
		0, 7, 0, 9, 0, 3,
	}
	got := readSubtable(t, readGsub1_2, data).(*Gsub1_2)
	for gid, subst := range map[glyph.ID]glyph.ID{5: 20, 6: 21, 8: 24, 9: 25} {
		idx, ok := got.Cov[gid]
		if !ok {
			t.Errorf("glyph %d is not covered", gid)
		} else if got.SubstituteGlyphIDs[idx] != subst {
			t.Errorf("glyph %d: got substitute %d, want %d",
				gid, got.SubstituteGlyphIDs[idx], subst)
		}
	}

	again := readSubtable(t, readGsub1_2, encodeSubtable(t, got))
	if diff := cmp.Diff(got, again); diff != "" {
		t.Errorf("round trip failed (-first +second):\n%s", diff)
	}
}
