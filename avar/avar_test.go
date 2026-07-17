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

package avar

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/variation"
)

func f2(v float64) variation.F2Dot14 { return variation.F2Dot14FromFloat(v) }

func TestMap(t *testing.T) {
	// non-linear segment map with an intermediate control point
	seg := SegmentMap{
		{From: f2(-1), To: f2(-1)},
		{From: f2(0), To: f2(0)},
		{From: f2(0.5), To: f2(0.25)},
		{From: f2(1), To: f2(1)},
	}
	tab := &Table{SegmentMaps: []SegmentMap{seg}}

	cases := []struct {
		in   variation.F2Dot14
		want variation.F2Dot14
	}{
		{f2(0), f2(0)},        // exact point
		{f2(0.5), f2(0.25)},   // exact intermediate point
		{f2(1), f2(1)},        // exact endpoint
		{f2(-1), f2(-1)},      // exact endpoint
		{f2(0.25), f2(0.125)}, // between (0,0) and (0.5,0.25)
		{f2(0.75), f2(0.625)}, // between (0.5,0.25) and (1,1)
		{f2(-0.5), f2(-0.5)},  // between (-1,-1) and (0,0)
	}
	for _, c := range cases {
		got := tab.Map([]variation.F2Dot14{c.in})
		if len(got) != 1 {
			t.Fatalf("in %v: got %d coords", c.in, len(got))
		}
		if got[0] != c.want {
			t.Errorf("in %v: got %d, want %d", c.in.Float64(), got[0], c.want)
		}
	}
}

func TestMapClampOutOfRange(t *testing.T) {
	// partial map covering only [0, 0.5]
	seg := SegmentMap{
		{From: f2(0), To: f2(0)},
		{From: f2(0.5), To: f2(0.25)},
	}
	tab := &Table{SegmentMaps: []SegmentMap{seg}}

	if got := tab.Map([]variation.F2Dot14{f2(-0.5)}); got[0] != f2(0) {
		t.Errorf("below range: got %d, want %d", got[0], f2(0))
	}
	if got := tab.Map([]variation.F2Dot14{f2(1)}); got[0] != f2(0.25) {
		t.Errorf("above range: got %d, want %d", got[0], f2(0.25))
	}
}

func TestMapEmptyIsIdentity(t *testing.T) {
	tab := &Table{SegmentMaps: []SegmentMap{nil}}
	in := f2(0.5)
	if got := tab.Map([]variation.F2Dot14{in}); got[0] != in {
		t.Errorf("empty map not identity: got %d, want %d", got[0], in)
	}
}

func TestMapMissingSegmentIsIdentity(t *testing.T) {
	// more coords than segment maps
	tab := &Table{SegmentMaps: nil}
	in := []variation.F2Dot14{f2(0.5), f2(-0.25)}
	got := tab.Map(in)
	if diff := cmp.Diff(in, got); diff != "" {
		t.Errorf("missing segment maps not identity (-want +got):\n%s", diff)
	}
}

func newReader(data []byte) parser.ReadSeekSizer {
	return bytes.NewReader(data)
}

func TestRoundTripV1(t *testing.T) {
	orig := &Table{
		SegmentMaps: []SegmentMap{
			{{From: -16384, To: -16384}, {From: 0, To: 0}, {From: 8192, To: 4096}, {From: 16384, To: 16384}},
			{{From: -16384, To: -16384}, {From: 0, To: 0}, {From: 16384, To: 16384}},
		},
	}
	data := orig.Encode()
	got, err := Read(newReader(data), parser.NewBudget(int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsSupported() {
		t.Error("v1 table reported unsupported")
	}
	if diff := cmp.Diff(orig, got); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

func TestVersion2Passthrough(t *testing.T) {
	raw := []byte{0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xDE, 0xAD, 0xBE, 0xEF}
	got, err := Read(newReader(raw), parser.NewBudget(int64(len(raw))))
	if err != nil {
		t.Fatal(err)
	}
	if got.IsSupported() {
		t.Error("v2 table reported supported")
	}
	if diff := cmp.Diff(raw, got.Encode()); diff != "" {
		t.Errorf("v2 passthrough not byte-identical (-want +got):\n%s", diff)
	}
}

func TestEmptyReader(t *testing.T) {
	if _, err := Read(newReader(nil), membudget.New(1024)); err == nil {
		t.Error("expected error for empty input")
	}
}
