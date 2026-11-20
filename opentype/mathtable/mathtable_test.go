// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package mathtable

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/parser"
)

func readBytes(data []byte) (*Info, error) {
	return Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
}

func roundTrip(t *testing.T, info *Info) (*Info, []byte) {
	t.Helper()
	data, err := info.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := readBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	return got, data
}

func variationIndex(outer, inner uint16) *device.Table {
	return &device.Table{
		DeltaFormat: device.VariationIndexFormat,
		OuterIndex:  outer,
		InnerIndex:  inner,
	}
}

// sampleInfo returns a table which uses every kind of subtable.
func sampleInfo() *Info {
	dev1 := &device.Table{StartSize: 10, EndSize: 12, Deltas: []int8{1, -1, 0}, DeltaFormat: 1}
	dev3 := &device.Table{StartSize: 9, EndSize: 10, Deltas: []int8{100, -100}, DeltaFormat: 3}
	vi := variationIndex(0, 7)

	var c Constants
	c.ScriptPercentScaleDown = 80
	c.ScriptScriptPercentScaleDown = 60
	c.DelimitedSubFormulaMinHeight = 1500
	c.DisplayOperatorMinHeight = 1800
	c.RadicalDegreeBottomRaisePercent = 60
	for i, v := range c.values() {
		*v = Value{V: funit.Int16(10*i - 100)}
	}
	c.AxisHeight = Value{V: 250, Device: dev1}
	c.MathLeading.Device = vi
	c.SpaceAfterScript = Value{V: 40, Device: dev1}
	c.RadicalKernAfterDegree = Value{V: -555, Device: dev3}

	shared := &Kern{
		Heights: []Value{{V: -100}, {V: 200, Device: vi}},
		Kerns:   []Value{{V: 10}, {V: 20, Device: dev1}, {V: -30}},
	}
	flat := &Kern{Kerns: []Value{{V: 5}}}

	asm := &Assembly{
		ItalicsCorrection: Value{V: 12, Device: vi},
		Parts: []Part{
			{Glyph: 50, EndConnector: 100, FullAdvance: 500},
			{Glyph: 51, StartConnector: 100, EndConnector: 100, FullAdvance: 300, Extender: true},
			{Glyph: 52, StartConnector: 100, FullAdvance: 500},
		},
	}
	both := &Construction{
		Variants: []Variant{{Glyph: 11, Advance: 1000}, {Glyph: 12, Advance: 1500}},
		Assembly: asm,
	}

	return &Info{
		Constants: c,
		ItalicsCorrection: map[glyph.ID]Value{
			3: {V: 30},
			4: {V: 40, Device: dev1},
			9: {V: 90, Device: vi},
		},
		TopAccent: map[glyph.ID]Value{
			3: {V: 300},
			5: {V: 500, Device: dev3},
		},
		ExtendedShapes: coverage.Set{7: true, 8: true, 100: true},
		Kerns: map[glyph.ID]*KernInfo{
			3: {TopRight: shared, BottomLeft: flat},
			4: {TopRight: shared, TopLeft: shared, BottomRight: shared},
			5: {},
		},
		MinConnectorOverlap: 20,
		Vertical: map[glyph.ID]*Construction{
			10: both,
			20: {Variants: []Variant{{Glyph: 21, Advance: 700}}},
			30: {Assembly: asm},
		},
		Horizontal: map[glyph.ID]*Construction{
			10: both,
			40: {Assembly: &Assembly{Parts: []Part{{Glyph: 60, FullAdvance: 1}}}},
		},
	}
}

func TestRoundTrip(t *testing.T) {
	info := sampleInfo()
	got, _ := roundTrip(t, info)
	if diff := cmp.Diff(info, got); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

func TestEmpty(t *testing.T) {
	got, data := roundTrip(t, &Info{})
	if diff := cmp.Diff(&Info{}, got); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}

	// nil and empty maps encode alike
	empty := &Info{
		ItalicsCorrection: map[glyph.ID]Value{},
		TopAccent:         map[glyph.ID]Value{},
		ExtendedShapes:    coverage.Set{},
		Kerns:             map[glyph.ID]*KernInfo{},
		Vertical:          map[glyph.ID]*Construction{},
		Horizontal:        map[glyph.ID]*Construction{},
	}
	data2, err := empty.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, data2) {
		t.Error("nil and empty maps encode differently")
	}
}

func TestSharing(t *testing.T) {
	dev := &device.Table{StartSize: 1, EndSize: 40, Deltas: make([]int8, 40), DeltaFormat: 3}
	info := sampleInfo()
	got, data := roundTrip(t, info)

	k := got.Kerns
	if k[3].TopRight != k[4].TopRight || k[4].TopRight != k[4].TopLeft {
		t.Error("kern table not shared")
	}
	if got.Vertical[10] != got.Horizontal[10] {
		t.Error("construction not shared")
	}
	c := got.Constants
	if c.AxisHeight.Device != c.SpaceAfterScript.Device {
		t.Error("device table not shared")
	}

	// a second encoding keeps the sharing
	data2, err := got.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, data2) {
		t.Error("encoding not stable")
	}

	// a table shared between several glyphs is written only once
	kern := func() *Kern {
		return &Kern{Kerns: []Value{{V: 1, Device: dev}}}
	}
	sizes := make([]int, 3)
	for i, n := range []int{1, 2, 3} {
		info := &Info{Kerns: map[glyph.ID]*KernInfo{}}
		for j := range n {
			info.Kerns[glyph.ID(j+1)] = &KernInfo{TopRight: kern()}
		}
		d, err := info.Encode()
		if err != nil {
			t.Fatal(err)
		}
		sizes[i] = len(d)
	}
	for i := 1; i < len(sizes); i++ {
		if perSharer := sizes[i] - sizes[i-1]; perSharer >= len(dev.Encode()) {
			t.Errorf("shared table duplicated: %d bytes per sharer", perSharer)
		}
	}
}
