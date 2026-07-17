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

package fvar

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/variation"
)

// wghtTable is a single-axis fvar table with the classic weight axis.
func wghtTable() *Table {
	return &Table{
		Axes: []Axis{
			{Tag: "wght", Min: 100, Default: 400, Max: 900},
		},
	}
}

func TestNormalizeWght(t *testing.T) {
	tab := wghtTable()

	cases := []struct {
		value float64
		want  variation.F2Dot14
	}{
		{400, 0},      // default
		{100, -16384}, // min -> -1
		{900, 16384},  // max -> +1
		{250, -8192},  // -0.5
		{700, 9830},   // 0.6 rounded
		{550, 4915},   // 0.3 rounded
		{50, -16384},  // clamp below min
		{1000, 16384}, // clamp above max
	}
	for _, c := range cases {
		got, err := tab.Normalize(map[string]float64{"wght": c.value})
		if err != nil {
			t.Fatalf("value %v: %v", c.value, err)
		}
		if len(got) != 1 {
			t.Fatalf("value %v: got %d coords", c.value, len(got))
		}
		if got[0] != c.want {
			t.Errorf("value %v: got %d, want %d", c.value, got[0], c.want)
		}
	}
}

func TestNormalizeDegenerate(t *testing.T) {
	// min == default == max
	flat := &Table{Axes: []Axis{{Tag: "wght", Min: 400, Default: 400, Max: 400}}}
	for _, v := range []float64{100, 400, 900} {
		got, err := flat.Normalize(map[string]float64{"wght": v})
		if err != nil {
			t.Fatal(err)
		}
		if got[0] != 0 {
			t.Errorf("flat axis value %v: got %d, want 0", v, got[0])
		}
	}

	// min == default
	lo := &Table{Axes: []Axis{{Tag: "wght", Min: 400, Default: 400, Max: 900}}}
	if got, _ := lo.Normalize(map[string]float64{"wght": 300}); got[0] != 0 {
		t.Errorf("min==default below: got %d, want 0", got[0])
	}
	if got, _ := lo.Normalize(map[string]float64{"wght": 650}); got[0] != 8192 {
		t.Errorf("min==default 650: got %d, want 8192", got[0])
	}

	// default == max
	hi := &Table{Axes: []Axis{{Tag: "wght", Min: 100, Default: 900, Max: 900}}}
	if got, _ := hi.Normalize(map[string]float64{"wght": 1000}); got[0] != 0 {
		t.Errorf("default==max above: got %d, want 0", got[0])
	}
	if got, _ := hi.Normalize(map[string]float64{"wght": 500}); got[0] != -8192 {
		t.Errorf("default==max 500: got %d, want -8192", got[0])
	}
}

func TestNormalizeMissingAxisUsesDefault(t *testing.T) {
	tab := &Table{Axes: []Axis{
		{Tag: "wght", Min: 100, Default: 400, Max: 900},
		{Tag: "wdth", Min: 50, Default: 100, Max: 200},
	}}
	got, err := tab.Normalize(map[string]float64{"wght": 900})
	if err != nil {
		t.Fatal(err)
	}
	want := []variation.F2Dot14{16384, 0}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("normalize with missing axis (-want +got):\n%s", diff)
	}
}

func TestNormalizeUnknownTag(t *testing.T) {
	tab := wghtTable()
	if _, err := tab.Normalize(map[string]float64{"xxxx": 1}); err == nil {
		t.Error("expected error for unknown axis tag")
	}
}

func TestNormalizeInRange(t *testing.T) {
	tab := &Table{Axes: []Axis{{Tag: "wght", Min: 100, Default: 400, Max: 900}}}
	for v := 0.0; v <= 1200; v += 7 {
		got, err := tab.Normalize(map[string]float64{"wght": v})
		if err != nil {
			t.Fatal(err)
		}
		if got[0] < -16384 || got[0] > 16384 {
			t.Errorf("value %v: out of range: %d", v, got[0])
		}
	}
	// default always 0
	if got, _ := tab.Normalize(map[string]float64{"wght": 400}); got[0] != 0 {
		t.Errorf("default not zero: %d", got[0])
	}
}

func newReader(data []byte) parser.ReadSeekSizer {
	return bytes.NewReader(data)
}

func TestRoundTrip(t *testing.T) {
	orig := &Table{
		Axes: []Axis{
			{Tag: "wght", Min: 100, Default: 400, Max: 900, NameID: 256},
			{Tag: "wdth", Min: 50, Default: 100, Max: 200, Hidden: true, NameID: 257},
		},
		Instances: []Instance{
			{NameID: 258, PostScriptNameID: 0xFFFF, Flags: 0, Coordinates: []float64{400, 100}},
			{NameID: 259, PostScriptNameID: 300, Flags: 1, Coordinates: []float64{700, 75},
				PostScriptName: "ignored"},
		},
	}
	data := orig.Encode()

	got, err := Read(newReader(data), parser.NewBudget(int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}
	// informational fields are not encoded
	orig.Instances[1].PostScriptName = ""
	if diff := cmp.Diff(orig, got); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

func TestRoundTripNoPostScript(t *testing.T) {
	orig := &Table{
		Axes: []Axis{{Tag: "wght", Min: 100, Default: 400, Max: 900, NameID: 256}},
		Instances: []Instance{
			{NameID: 258, PostScriptNameID: 0xFFFF, Coordinates: []float64{400}},
			{NameID: 259, PostScriptNameID: 0xFFFF, Coordinates: []float64{900}},
		},
	}
	data := orig.Encode()
	got, err := Read(newReader(data), parser.NewBudget(int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(orig, got); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}
