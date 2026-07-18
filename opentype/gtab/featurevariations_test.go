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

package gtab

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/text/language"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/variation"
)

func f2(x float64) variation.F2Dot14 { return variation.F2Dot14FromFloat(x) }

// baseVariationInfo builds an Info with scripts, features and lookups but no
// variations.  The dummy lookups let readGtab round-trip the table.
func baseVariationInfo() *Info {
	return &Info{
		ScriptList: ScriptListInfo{
			language.MustParse("und-Latn"): {
				Required: 0,
				Optional: []FeatureIndex{1, 2},
			},
		},
		FeatureList: FeatureListInfo{
			{Tag: "kern", Lookups: []LookupIndex{0, 1}},
			{Tag: "liga", Lookups: []LookupIndex{1}},
			{Tag: "frac", Lookups: []LookupIndex{0}},
		},
		LookupList: LookupList{
			{Meta: &LookupMetaInfo{LookupType: 1}, Subtables: []Subtable{dummySubtable{0, 1}}},
			{Meta: &LookupMetaInfo{LookupType: 1}, Subtables: []Subtable{dummySubtable{2, 3}}},
		},
	}
}

func roundTripGtab(t *testing.T, info *Info) *Info {
	t.Helper()
	data := info.Encode()
	out, err := readGtab(bytes.NewReader(data), parser.NewBudget(int64(len(data))), 0, readDummySubtable)
	if err != nil {
		t.Fatalf("readGtab: %v", err)
	}
	return out
}

func TestFeatureVariationsRoundTrip(t *testing.T) {
	info := baseVariationInfo()
	info.Variations = []FeatureVariationRecord{
		{
			Conditions: []Condition{
				{Format: 1, AxisIndex: 0, Min: f2(0.5), Max: f2(1.0)},
			},
			Substitutions: []FeatureSubstitution{
				{FeatureIndex: 0, Lookups: []LookupIndex{1}},
			},
		},
		{
			Conditions: []Condition{
				{Format: 1, AxisIndex: 0, Min: f2(-1.0), Max: f2(0.0)},
				{Format: 1, AxisIndex: 1, Min: f2(0.25), Max: f2(0.75)},
			},
			Substitutions: []FeatureSubstitution{
				{FeatureIndex: 1, Lookups: []LookupIndex{0, 1}},
				{FeatureIndex: 2, Lookups: nil},
			},
		},
	}

	out := roundTripGtab(t, info)
	if diff := cmp.Diff(info.Variations, out.Variations); diff != "" {
		t.Errorf("variations round trip failed (-want +got):\n%s", diff)
	}
	if out.VariationsRaw != nil {
		t.Errorf("unexpected VariationsRaw")
	}
}

func TestFeatureVariationsEmptyConditions(t *testing.T) {
	info := baseVariationInfo()
	info.Variations = []FeatureVariationRecord{
		{
			// empty conditions: always applies
			Substitutions: []FeatureSubstitution{
				{FeatureIndex: 0, Lookups: []LookupIndex{1}},
			},
		},
	}

	out := roundTripGtab(t, info)
	if diff := cmp.Diff(info.Variations, out.Variations); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// TestFeatureVariationsByteIdentity guards the promise that a variations-free
// Info encodes byte-for-byte as it did before FeatureVariations support was
// added.  The golden was produced by the pre-change Encode.
func TestFeatureVariationsByteIdentity(t *testing.T) {
	const golden = "00010000000a0020004800016c61746e0008000400000000000000020001000200036b65726e00146c696761001c66726163002200000002000000010000000100010000000100000002000600100001000000010008000100010000000100080203"

	info := baseVariationInfo()
	got := hex.EncodeToString(info.Encode())
	if got != golden {
		t.Errorf("byte-identity broken:\n got %s\nwant %s", got, golden)
	}

	// sanity: header is 10 bytes with minor version 0
	raw := info.Encode()
	if raw[2] != 0 || raw[3] != 0 {
		t.Errorf("expected minor version 0, got %d", raw[3])
	}
}

func TestFeatureVariationsRawFallback(t *testing.T) {
	// build a GSUB table with a FeatureVariations table whose single condition
	// uses an unmodeled format (format 2); it must round-trip as raw bytes.
	info := baseVariationInfo()
	data := info.Encode() // version 1.0 so far

	// FeatureVariations table with one record, one condition (format 2)
	fv := buildRawFeatureVariations()

	// splice: rewrite as version 1.1 with FeatureVariationsOffset
	table := spliceFeatureVariations(data, fv)

	out, err := readGtab(bytes.NewReader(table), parser.NewBudget(int64(len(table))), 0, readDummySubtable)
	if err != nil {
		t.Fatalf("readGtab: %v", err)
	}
	if out.Variations != nil {
		t.Errorf("expected nil Variations for unknown format")
	}
	if out.VariationsRaw == nil {
		t.Fatalf("expected VariationsRaw for unknown format")
	}

	// the raw bytes should re-encode and re-read byte-identically
	data2 := out.Encode()
	out2, err := readGtab(bytes.NewReader(data2), parser.NewBudget(int64(len(data2))), 0, readDummySubtable)
	if err != nil {
		t.Fatalf("re-read: %v", err)
	}
	if !bytes.Equal(out.VariationsRaw, out2.VariationsRaw) {
		t.Errorf("VariationsRaw did not round-trip byte-exactly")
	}
	if !bytes.Equal(fv, out.VariationsRaw) {
		t.Errorf("VariationsRaw does not match the original table bytes")
	}
}

// buildRawFeatureVariations builds a FeatureVariations table with one record
// whose condition set holds a single format-2 (unmodeled) condition.
func buildRawFeatureVariations() []byte {
	// FeatureVariations header: major 1, minor 0, count 1
	// one record: conditionSetOffset, featureTableSubstitutionOffset
	// layout: [0:8] header, [8:16] record
	// ConditionSet at 16, condition table right after.
	fv := []byte{
		0, 1, 0, 0, // version 1.0
		0, 0, 0, 1, // record count = 1
		0, 0, 0, 16, // conditionSetOffset
		0, 0, 0, 0, // featureTableSubstitutionOffset = 0
	}
	// ConditionSet at offset 16: count 1, one offset (from CS start)
	cs := []byte{
		0, 1, // condition count
		0, 0, 0, 6, // offset to condition (6 = 2 + 4)
		0, 2, 0, 0, // condition: format 2, plus a stray body byte pair
	}
	return append(fv, cs...)
}

// spliceFeatureVariations rewrites a version-1.0 table as version 1.1 with fv
// appended and referenced.
func spliceFeatureVariations(table, fv []byte) []byte {
	out := make([]byte, 0, len(table)+4+len(fv))
	// header: keep major, set minor 1, keep the three list offsets shifted by 4
	scriptListOffset := int(table[4])<<8 | int(table[5])
	featureListOffset := int(table[6])<<8 | int(table[7])
	lookupListOffset := int(table[8])<<8 | int(table[9])
	scriptListOffset += 4
	featureListOffset += 4
	lookupListOffset += 4
	body := table[10:]
	fvOffset := 14 + len(body)
	out = append(out,
		0, 1, // major
		0, 1, // minor
		byte(scriptListOffset>>8), byte(scriptListOffset),
		byte(featureListOffset>>8), byte(featureListOffset),
		byte(lookupListOffset>>8), byte(lookupListOffset),
		byte(fvOffset>>24), byte(fvOffset>>16), byte(fvOffset>>8), byte(fvOffset),
	)
	out = append(out, body...)
	out = append(out, fv...)
	return out
}

func TestConditionMatches(t *testing.T) {
	coords := []variation.F2Dot14{f2(0.5), f2(-0.5)}

	cases := []struct {
		name string
		rec  FeatureVariationRecord
		want bool
	}{
		{"empty always matches", FeatureVariationRecord{}, true},
		{"in range", FeatureVariationRecord{Conditions: []Condition{
			{Format: 1, AxisIndex: 0, Min: f2(0.0), Max: f2(1.0)},
		}}, true},
		{"lower boundary inclusive", FeatureVariationRecord{Conditions: []Condition{
			{Format: 1, AxisIndex: 0, Min: f2(0.5), Max: f2(1.0)},
		}}, true},
		{"upper boundary inclusive", FeatureVariationRecord{Conditions: []Condition{
			{Format: 1, AxisIndex: 0, Min: f2(0.0), Max: f2(0.5)},
		}}, true},
		{"below range", FeatureVariationRecord{Conditions: []Condition{
			{Format: 1, AxisIndex: 0, Min: f2(0.6), Max: f2(1.0)},
		}}, false},
		{"above range", FeatureVariationRecord{Conditions: []Condition{
			{Format: 1, AxisIndex: 0, Min: f2(0.0), Max: f2(0.4)},
		}}, false},
		{"conjunction both hold", FeatureVariationRecord{Conditions: []Condition{
			{Format: 1, AxisIndex: 0, Min: f2(0.0), Max: f2(1.0)},
			{Format: 1, AxisIndex: 1, Min: f2(-1.0), Max: f2(0.0)},
		}}, true},
		{"conjunction one fails", FeatureVariationRecord{Conditions: []Condition{
			{Format: 1, AxisIndex: 0, Min: f2(0.0), Max: f2(1.0)},
			{Format: 1, AxisIndex: 1, Min: f2(0.0), Max: f2(1.0)},
		}}, false},
		{"axis out of range", FeatureVariationRecord{Conditions: []Condition{
			{Format: 1, AxisIndex: 5, Min: f2(-1.0), Max: f2(1.0)},
		}}, false},
		{"unknown format never matches", FeatureVariationRecord{Conditions: []Condition{
			{Format: 2, AxisIndex: 0},
		}}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.rec.Matches(coords); got != tc.want {
				t.Errorf("Matches = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFeatureVariationsDropInvalid(t *testing.T) {
	// a substitution naming an out-of-range feature index and one naming an
	// out-of-range lookup index must be dropped; the valid one survives.
	// The base table has 3 features and 2 lookups.
	fv := encodeFeatureVariations([]FeatureVariationRecord{
		{
			Substitutions: []FeatureSubstitution{
				{FeatureIndex: 99, Lookups: []LookupIndex{0}}, // bad feature
				{FeatureIndex: 0, Lookups: []LookupIndex{99}}, // bad lookup
				{FeatureIndex: 1, Lookups: []LookupIndex{0}},  // valid
			},
		},
	})
	base := baseVariationInfo()
	table := spliceFeatureVariations(base.Encode(), fv)

	out, err := readGtab(bytes.NewReader(table), parser.NewBudget(int64(len(table))), 0, readDummySubtable)
	if err != nil {
		t.Fatalf("readGtab: %v", err)
	}
	want := []FeatureVariationRecord{
		{
			Substitutions: []FeatureSubstitution{
				{FeatureIndex: 1, Lookups: []LookupIndex{0}},
			},
		},
	}
	if diff := cmp.Diff(want, out.Variations); diff != "" {
		t.Errorf("invalid substitutions not dropped (-want +got):\n%s", diff)
	}
}

func TestFeatureParamsIgnored(t *testing.T) {
	// an alternate Feature table with a non-zero featureParams offset must read
	// the same as one with a zero offset.
	base := baseVariationInfo()

	// hand-build a FeatureVariations table with a non-zero featureParams offset
	// in the alternate feature table.
	fv := []byte{
		0, 1, 0, 0, // version
		0, 0, 0, 1, // count 1
		0, 0, 0, 0, // conditionSetOffset 0
		0, 0, 0, 16, // featureTableSubstitutionOffset
	}
	fts := []byte{
		0, 1, 0, 0, // version
		0, 1, // substitution count
		0, 0, // featureIndex 0
		0, 0, 0, 12, // alternateFeatureOffset (6 + 6)
	}
	altFeature := []byte{
		0, 42, // non-zero featureParams offset (ignored)
		0, 1, // lookupIndexCount
		0, 1, // lookup index 1
	}
	fv = append(fv, fts...)
	fv = append(fv, altFeature...)

	table := spliceFeatureVariations(base.Encode(), fv)
	out, err := readGtab(bytes.NewReader(table), parser.NewBudget(int64(len(table))), 0, readDummySubtable)
	if err != nil {
		t.Fatalf("readGtab: %v", err)
	}
	want := []FeatureVariationRecord{
		{
			Substitutions: []FeatureSubstitution{
				{FeatureIndex: 0, Lookups: []LookupIndex{1}},
			},
		},
	}
	if diff := cmp.Diff(want, out.Variations); diff != "" {
		t.Errorf("featureParams not ignored (-want +got):\n%s", diff)
	}
}
