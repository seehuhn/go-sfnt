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

package sfnt_test

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/image/font/gofont/goregular"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/avar"
	"seehuhn.de/go/sfnt/cvar"
	"seehuhn.de/go/sfnt/fvar"
	"seehuhn.de/go/sfnt/gvar"
	"seehuhn.de/go/sfnt/hvar"
	"seehuhn.de/go/sfnt/mvar"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/gdef"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/stat"
	"seehuhn.de/go/sfnt/variation"

	"golang.org/x/text/language"
)

// makeVariableFont builds a synthetic 2-axis variable font on top of a real
// glyf base (Go Regular).  It attaches every variation table wired by Read and
// Write.  The fvar/STAT name strings are set to the values a round trip
// resolves.
func makeVariableFont(t testing.TB) *sfnt.Font {
	t.Helper()

	f, err := sfnt.Read(bytes.NewReader(goregular.TTF), parser.NewBudget(int64(len(goregular.TTF))))
	if err != nil {
		t.Fatal(err)
	}
	if !f.IsGlyf() {
		t.Fatal("base font is not glyf")
	}
	n := f.NumGlyphs()

	f.Fvar = &fvar.Table{
		Axes: []fvar.Axis{
			{Tag: "wght", Min: 100, Default: 400, Max: 900, Name: "Weight"},
			{Tag: "wdth", Min: 75, Default: 100, Max: 125, Name: "Width"},
		},
		Instances: []fvar.Instance{
			{
				Coordinates:      []float64{700, 100},
				PostScriptNameID: 0xFFFF,
				Name:             "Bold",
				PostScriptName:   "Test-Bold",
			},
		},
	}

	f.Avar = &avar.Table{
		SegmentMaps: []avar.SegmentMap{
			{
				{From: -0x4000, To: -0x4000},
				{From: 0, To: 0},
				{From: 0x2000, To: 0x2666},
				{From: 0x4000, To: 0x4000},
			},
			{
				{From: -0x4000, To: -0x4000},
				{From: 0, To: 0},
				{From: 0x4000, To: 0x4000},
			},
		},
	}

	f.Stat = &stat.Table{
		DesignAxes: []stat.DesignAxis{
			{Tag: "wght", Name: "Weight"},
			{Tag: "wdth", Ordering: 1, Name: "Width"},
		},
		AxisValues: []stat.AxisValue{
			&stat.Format1{AxisIndex: 0, Value: 400, Name: "Regular"},
			&stat.Format1{AxisIndex: 0, Value: 700, Name: "Bold"},
		},
		ElidedFallbackName: "Regular",
	}

	// gvar: deltas for two glyphs, one using private point numbers.
	block3, err := variation.EncodeTupleData([]variation.TupleVariation{
		{
			Peak:   []variation.F2Dot14{0x4000, 0},
			Points: []uint16{0, 2},
			Deltas: []int32{10, -5, 3, 7},
		},
		{
			Peak:   []variation.F2Dot14{-0x4000, 0x2000},
			Deltas: []int32{1, 2, 3, 4, 5, -1, -2, -3, -4, -5},
		},
	}, 2, 2, 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	block5, err := variation.EncodeTupleData([]variation.TupleVariation{
		{
			Peak:   []variation.F2Dot14{0x4000, 0x4000},
			Deltas: []int32{2, 4, 6, 8, 10, -2, -4, -6, -8, -10},
		},
	}, 2, 2, 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	perGlyph := make([]gvar.GlyphData, n)
	perGlyph[3] = gvar.GlyphData{Data: block3}
	perGlyph[5] = gvar.GlyphData{Data: block5}
	f.Gvar = &gvar.Table{
		AxisCount: 2,
		PerGlyph:  perGlyph,
	}

	// cvar: one tuple over a subset of CVT entries.
	f.Cvar = &cvar.Table{
		AxisCount: 2,
		Tuples: []variation.TupleVariation{
			{
				Peak:   []variation.F2Dot14{0x4000, 0},
				Points: []uint16{0, 1},
				Deltas: []int32{10, -5},
			},
		},
	}

	// a small item variation store shared by HVAR and MVAR (two axes, two
	// inner rows).
	makeStore := func() *variation.ItemVariationStore {
		return &variation.ItemVariationStore{
			Regions: []variation.Region{
				{
					{Start: 0, Peak: 0x4000, End: 0x4000},
					{Start: 0, Peak: 0, End: 0},
				},
			},
			Data: []*variation.ItemVariationData{
				{
					RegionIndexes: []uint16{0},
					Deltas:        [][]int32{{10}, {20}},
				},
			},
		}
	}

	f.Hvar = &hvar.Table{
		Store:      makeStore(),
		AdvanceMap: &variation.DeltaSetIndexMap{Map: []uint32{0, 1}},
	}

	f.Mvar = &mvar.Table{
		Store: makeStore(),
		Records: []mvar.Record{
			{Tag: "hasc", OuterIndex: 0, InnerIndex: 0},
			{Tag: "hdsc", OuterIndex: 0, InnerIndex: 1},
		},
	}

	f.VariationsPostScriptName = "Test-"

	// GDEF item variation store (GDEF 1.3).
	if f.Gdef == nil {
		f.Gdef = &gdef.Table{}
	}
	f.Gdef.ItemVarStore = makeStore()

	// GSUB with one FeatureVariations record.
	f.Gsub = &gtab.Info{
		ScriptList: gtab.ScriptListInfo{
			language.MustParse("und-Latn"): {Required: 0xFFFF, Optional: []gtab.FeatureIndex{0}},
		},
		FeatureList: gtab.FeatureListInfo{
			{Tag: "rvrn", Lookups: []gtab.LookupIndex{}},
		},
		LookupList: gtab.LookupList{
			{
				Meta:      &gtab.LookupMetaInfo{LookupType: 1},
				Subtables: []gtab.Subtable{&gtab.Gsub1_1{Cov: coverage.Set{3: true}, Delta: 1}},
			},
		},
		Variations: []gtab.FeatureVariationRecord{
			{
				Conditions: []gtab.Condition{
					{Format: 1, AxisIndex: 0, Min: 0x2000, Max: 0x4000},
				},
				Substitutions: []gtab.FeatureSubstitution{
					{FeatureIndex: 0, Lookups: []gtab.LookupIndex{0}},
				},
			},
		},
	}

	return f
}

func TestVariableFontRoundTrip(t *testing.T) {
	src := makeVariableFont(t)

	// the source must not be mutated by Write
	srcClone := src.Clone()
	srcFvar := src.Fvar
	srcStat := src.Stat

	buf1 := &bytes.Buffer{}
	if _, err := src.Write(buf1); err != nil {
		t.Fatal(err)
	}
	b1 := buf1.Bytes()

	if src.Fvar != srcFvar || src.Stat != srcStat {
		t.Error("Write replaced the caller's Fvar/Stat pointers")
	}
	if src.Fvar.Axes[0].NameID != srcClone.Fvar.Axes[0].NameID {
		t.Error("Write mutated the caller's fvar axis NameID")
	}

	f2, err := sfnt.Read(bytes.NewReader(b1), parser.NewBudget(int64(len(b1))))
	if err != nil {
		t.Fatal(err)
	}

	// name resolution
	if got := f2.VariationAxes(); len(got) != 2 ||
		got[0].Name != "Weight" || got[1].Name != "Width" {
		t.Errorf("axis names: %+v", got)
	}
	inst := f2.NamedInstances()
	if len(inst) != 1 || inst[0].Name != "Bold" || inst[0].PostScriptName != "Test-Bold" {
		t.Errorf("named instances: %+v", inst)
	}
	if inst[0].Coordinates["wght"] != 700 || inst[0].Coordinates["wdth"] != 100 {
		t.Errorf("instance coordinates: %+v", inst[0].Coordinates)
	}
	if f2.VariationsPostScriptName != "Test-" {
		t.Errorf("VariationsPostScriptName = %q", f2.VariationsPostScriptName)
	}
	if f2.Stat.DesignAxes[0].Name != "Weight" || f2.Stat.AxisValues[0].(*stat.Format1).Name != "Regular" {
		t.Error("STAT names not resolved")
	}
	if f2.Stat.ElidedFallbackName != "Regular" {
		t.Errorf("elided fallback name = %q", f2.Stat.ElidedFallbackName)
	}
	if !f2.IsVariable() {
		t.Error("f2 is not variable")
	}

	// name-independent tables must survive Read unchanged
	if diff := cmp.Diff(src.Avar, f2.Avar); diff != "" {
		t.Errorf("avar mismatch (-src +f2):\n%s", diff)
	}
	if diff := cmp.Diff(src.Gvar, f2.Gvar); diff != "" {
		t.Errorf("gvar mismatch (-src +f2):\n%s", diff)
	}
	if diff := cmp.Diff(src.Cvar, f2.Cvar); diff != "" {
		t.Errorf("cvar mismatch (-src +f2):\n%s", diff)
	}
	if diff := cmp.Diff(src.Hvar, f2.Hvar); diff != "" {
		t.Errorf("hvar mismatch (-src +f2):\n%s", diff)
	}
	if diff := cmp.Diff(src.Mvar, f2.Mvar); diff != "" {
		t.Errorf("mvar mismatch (-src +f2):\n%s", diff)
	}
	if diff := cmp.Diff(src.Gdef.ItemVarStore, f2.Gdef.ItemVarStore); diff != "" {
		t.Errorf("GDEF item variation store mismatch (-src +f2):\n%s", diff)
	}
	if diff := cmp.Diff(src.Gsub.Variations, f2.Gsub.Variations); diff != "" {
		t.Errorf("GSUB feature variations mismatch (-src +f2):\n%s", diff)
	}

	// second write must be byte-identical (fixpoint)
	buf2 := &bytes.Buffer{}
	if _, err := f2.Write(buf2); err != nil {
		t.Fatal(err)
	}
	b2 := buf2.Bytes()
	if !bytes.Equal(b1, b2) {
		t.Errorf("second write not byte-identical: %d vs %d bytes", len(b1), len(b2))
	}

	// structural fixpoint after name resolution
	f3, err := sfnt.Read(bytes.NewReader(b2), parser.NewBudget(int64(len(b2))))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(f2.Fvar, f3.Fvar); diff != "" {
		t.Errorf("fvar not stable (-f2 +f3):\n%s", diff)
	}
	if diff := cmp.Diff(f2.Stat, f3.Stat); diff != "" {
		t.Errorf("STAT not stable (-f2 +f3):\n%s", diff)
	}
}

// WriteTrueTypePDF must not leak variation tables into the PDF stream.
func TestWriteTrueTypePDFNoVariationTables(t *testing.T) {
	src := makeVariableFont(t)

	buf := &bytes.Buffer{}
	if _, err := src.WriteTrueTypePDF(buf); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	for _, tag := range []string{"fvar", "avar", "STAT", "gvar", "cvar", "HVAR", "MVAR"} {
		if bytes.Contains(data, []byte(tag)) {
			t.Errorf("PDF stream contains variation table tag %q", tag)
		}
	}
}
