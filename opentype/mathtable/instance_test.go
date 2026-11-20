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
)

func TestMapValues(t *testing.T) {
	info := sampleInfo()
	before, _ := sampleInfo().Encode()
	got := info.MapValues(func(v Value) Value {
		v.V++
		return v
	})

	want := sampleInfo()
	for _, v := range want.Constants.values() {
		v.V++
	}
	for gid, v := range want.ItalicsCorrection {
		v.V++
		want.ItalicsCorrection[gid] = v
	}
	for gid, v := range want.TopAccent {
		v.V++
		want.TopAccent[gid] = v
	}
	seen := map[*Kern]bool{}
	for _, ki := range want.Kerns {
		for _, k := range ki.corners() {
			if k == nil || seen[k] {
				continue
			}
			seen[k] = true
			for i := range k.Heights {
				k.Heights[i].V++
			}
			for i := range k.Kerns {
				k.Kerns[i].V++
			}
		}
	}
	doneAsm := map[*Assembly]bool{}
	for _, m := range []map[glyph.ID]*Construction{want.Vertical, want.Horizontal} {
		for _, c := range m {
			if c.Assembly != nil && !doneAsm[c.Assembly] {
				doneAsm[c.Assembly] = true
				c.Assembly.ItalicsCorrection.V++
			}
		}
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unexpected result (-want +got):\n%s", diff)
	}

	after, err := info.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("receiver modified")
	}
	if got.Kerns[3].TopRight != got.Kerns[4].TopLeft {
		t.Error("kern table not shared")
	}
	if got.Vertical[10] != got.Horizontal[10] {
		t.Error("construction not shared")
	}
}

// TestValues checks that Values yields exactly the values which MapValues
// visits.
func TestValues(t *testing.T) {
	visited := map[funit.Int16]bool{}
	var next funit.Int16
	info := sampleInfo().MapValues(func(v Value) Value {
		next++
		v.V = next
		visited[next] = true
		return v
	})

	yielded := map[funit.Int16]bool{}
	for v := range info.Values() {
		yielded[v.V] = true
	}
	if diff := cmp.Diff(visited, yielded); diff != "" {
		t.Errorf("unexpected values (-want +got):\n%s", diff)
	}
}

func TestMapGlyphsRenumber(t *testing.T) {
	info := sampleInfo()
	shift := func(g glyph.ID) (glyph.ID, bool) { return g + 1000, true }
	got := info.MapGlyphs(shift)

	want := &Info{
		Constants:           info.Constants,
		ItalicsCorrection:   map[glyph.ID]Value{},
		TopAccent:           map[glyph.ID]Value{},
		ExtendedShapes:      coverage.Set{},
		Kerns:               map[glyph.ID]*KernInfo{},
		MinConnectorOverlap: info.MinConnectorOverlap,
		Vertical:            map[glyph.ID]*Construction{},
		Horizontal:          map[glyph.ID]*Construction{},
	}
	for g, v := range info.ItalicsCorrection {
		want.ItalicsCorrection[g+1000] = v
	}
	for g, v := range info.TopAccent {
		want.TopAccent[g+1000] = v
	}
	for g := range info.ExtendedShapes {
		want.ExtendedShapes[g+1000] = true
	}
	for g, v := range info.Kerns {
		want.Kerns[g+1000] = v
	}
	asm := &Assembly{
		ItalicsCorrection: info.Vertical[10].Assembly.ItalicsCorrection,
		Parts:             append([]Part(nil), info.Vertical[10].Assembly.Parts...),
	}
	for i := range asm.Parts {
		asm.Parts[i].Glyph += 1000
	}
	both := &Construction{
		Variants: []Variant{{Glyph: 1011, Advance: 1000}, {Glyph: 1012, Advance: 1500}},
		Assembly: asm,
	}
	want.Vertical[1010] = both
	want.Vertical[1020] = &Construction{Variants: []Variant{{Glyph: 1021, Advance: 700}}}
	want.Vertical[1030] = &Construction{Assembly: asm}
	want.Horizontal[1010] = both
	want.Horizontal[1040] = &Construction{Assembly: &Assembly{Parts: []Part{{Glyph: 1060, FullAdvance: 1}}}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unexpected result (-want +got):\n%s", diff)
	}

	if got.Vertical[1010] != got.Horizontal[1010] {
		t.Error("construction not shared")
	}
	if got.Vertical[1010].Assembly != got.Vertical[1030].Assembly {
		t.Error("assembly not shared")
	}
	if info.Vertical[10] == nil || info.Vertical[1010] != nil {
		t.Error("receiver modified")
	}
	rt, _ := roundTrip(t, got)
	if diff := cmp.Diff(got, rt); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

func TestMapGlyphsDrop(t *testing.T) {
	info := sampleInfo()
	keep := func(g glyph.ID) (glyph.ID, bool) { return g, g != 4 && g != 11 && g != 51 && g != 60 }
	got := info.MapGlyphs(keep)

	if _, ok := got.ItalicsCorrection[4]; ok || len(got.ItalicsCorrection) != 2 {
		t.Errorf("italics corrections: %v", got.ItalicsCorrection)
	}
	if _, ok := got.Kerns[4]; ok || len(got.Kerns) != 2 {
		t.Errorf("kerns: %v", got.Kerns)
	}
	if len(got.ExtendedShapes) != 3 {
		t.Errorf("extended shapes: %v", got.ExtendedShapes)
	}
	// variant removed, order kept, assembly with an unmapped part removed
	want := &Construction{Variants: []Variant{{Glyph: 12, Advance: 1500}}}
	if diff := cmp.Diff(want, got.Vertical[10]); diff != "" {
		t.Errorf("construction 10 (-want +got):\n%s", diff)
	}
	if got.Vertical[20] == nil {
		t.Error("construction 20 dropped")
	}
	// only an assembly, which lost a part
	if _, ok := got.Vertical[30]; ok {
		t.Error("empty construction kept")
	}
	if _, ok := got.Horizontal[40]; ok {
		t.Error("construction with removed part kept")
	}
	rt, _ := roundTrip(t, got)
	if diff := cmp.Diff(got, rt); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}

	none := info.MapGlyphs(func(glyph.ID) (glyph.ID, bool) { return 0, false })
	if none.ItalicsCorrection != nil || none.Kerns != nil || none.ExtendedShapes != nil ||
		none.Vertical != nil || none.Horizontal != nil {
		t.Error("empty containers not nil")
	}
	roundTrip(t, none)
}

// TestBakeUnsortedKern checks that baking which puts the correction heights
// of a kern table out of order removes that table, and that the result can
// be written.
func TestBakeUnsortedKern(t *testing.T) {
	vi := &device.Table{DeltaFormat: device.VariationIndexFormat}
	sorted := &Kern{
		Heights: []Value{{V: 100}, {V: 300}},
		Kerns:   []Value{{V: 1}, {V: 2}, {V: 3}},
	}
	unsorted := &Kern{
		Heights: []Value{{V: 100}, {V: 200, Device: vi}},
		Kerns:   []Value{{V: 1}, {V: 2}, {V: 3}},
	}
	info := &Info{
		Kerns: map[glyph.ID]*KernInfo{
			1: {TopRight: unsorted, BottomLeft: sorted},
		},
	}

	baked, err := info.BakeVariationDevices(func(outer, inner uint16) int64 { return -150 })
	if err != nil {
		t.Fatal(err)
	}
	ki := baked.Kerns[1]
	if ki.TopRight != nil {
		t.Errorf("kern table with unsorted heights kept: %+v", ki.TopRight)
	}
	if diff := cmp.Diff(sorted, ki.BottomLeft); diff != "" {
		t.Errorf("unaffected kern table changed (-want +got):\n%s", diff)
	}
	if _, err := baked.Encode(); err != nil {
		t.Errorf("baked table cannot be written: %v", err)
	}
}

func TestBakeKeepsDevices(t *testing.T) {
	devs := []*device.Table{
		{StartSize: 10, EndSize: 11, Deltas: []int8{1, -1}, DeltaFormat: 1},
		{StartSize: 10, EndSize: 11, Deltas: []int8{3, -2}, DeltaFormat: 2},
		{StartSize: 10, EndSize: 11, Deltas: []int8{100, -100}, DeltaFormat: 3},
	}
	info := &Info{ItalicsCorrection: map[glyph.ID]Value{}}
	for i, d := range devs {
		info.ItalicsCorrection[glyph.ID(i+1)] = Value{V: 10, Device: d}
	}
	info.ItalicsCorrection[4] = Value{V: 10, Device: variationIndex(1, 2)}

	got, err := info.BakeVariationDevices(func(outer, inner uint16) int64 {
		return int64(100*outer + inner)
	})
	if err != nil {
		t.Fatal(err)
	}

	want := &Info{ItalicsCorrection: map[glyph.ID]Value{4: {V: 112}}}
	for i, d := range devs {
		want.ItalicsCorrection[glyph.ID(i+1)] = Value{V: 10, Device: d}
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unexpected result (-want +got):\n%s", diff)
	}
}

func TestBakeVariationDevices(t *testing.T) {
	info := sampleInfo()
	before, _ := sampleInfo().Encode()

	resolve := func(outer, inner uint16) int64 {
		return 1000*int64(outer) + int64(inner)
	}
	got, err := info.BakeVariationDevices(resolve)
	if err != nil {
		t.Fatal(err)
	}

	// the receiver is unchanged
	after, err := info.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("receiver modified")
	}

	want := sampleInfo()
	want.Constants.MathLeading.V += 7
	want.Constants.MathLeading.Device = nil
	want.ItalicsCorrection[9] = Value{V: 97}
	shared := want.Kerns[3].TopRight
	shared.Heights[1] = Value{V: 207}
	asm := want.Vertical[10].Assembly
	asm.ItalicsCorrection = Value{V: 19}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unexpected result (-want +got):\n%s", diff)
	}

	// ordinary device tables stay, and sharing is preserved
	if got.Constants.AxisHeight.Device == nil {
		t.Error("device table removed")
	}
	if got.Kerns[3].TopRight != got.Kerns[4].TopLeft {
		t.Error("kern table not shared")
	}
	if got.Vertical[10] != got.Horizontal[10] {
		t.Error("construction not shared")
	}
	if _, err := got.Encode(); err != nil {
		t.Error(err)
	}
}

// TestBakeVariationDevicesRange checks that a value which leaves the 16-bit
// range after baking gives an error.
func TestBakeVariationDevicesRange(t *testing.T) {
	info := &Info{
		TopAccent: map[glyph.ID]Value{1: {V: 32000, Device: variationIndex(0, 0)}},
	}
	resolve := func(outer, inner uint16) int64 { return 1000 }
	if _, err := info.BakeVariationDevices(resolve); err == nil {
		t.Error("out-of-range value accepted")
	}
}

// TestValuesShared checks that Values yields the values of a shared kern
// table or assembly once.
func TestValuesShared(t *testing.T) {
	k := &Kern{Kerns: []Value{{V: 7}}}
	a := &Assembly{ItalicsCorrection: Value{V: 9}}
	info := &Info{
		Kerns: map[glyph.ID]*KernInfo{
			1: {TopRight: k, TopLeft: k},
			2: {BottomLeft: k},
		},
		Vertical:   map[glyph.ID]*Construction{1: {Assembly: a}},
		Horizontal: map[glyph.ID]*Construction{2: {Assembly: a}},
	}

	count := map[funit.Int16]int{}
	for v := range info.Values() {
		count[v.V]++
	}
	if count[7] != 1 || count[9] != 1 {
		t.Errorf("shared values yielded %d and %d times, want once", count[7], count[9])
	}
}

// TestMapDevices checks that MapDevices replaces and removes device tables,
// keeps the values and shared kern tables, and leaves the receiver
// unchanged.
func TestMapDevices(t *testing.T) {
	plain := &device.Table{StartSize: 8, EndSize: 8, Deltas: []int8{1}, DeltaFormat: 1}
	vi := &device.Table{DeltaFormat: device.VariationIndexFormat, InnerIndex: 5}
	k := &Kern{Kerns: []Value{{V: 7, Device: vi}}}
	info := &Info{
		ItalicsCorrection: map[glyph.ID]Value{1: {V: 3, Device: plain}},
		Kerns:             map[glyph.ID]*KernInfo{1: {TopRight: k}, 2: {TopLeft: k}},
	}

	got := info.MapDevices(func(d *device.Table) *device.Table {
		if d.IsVariationIndex() {
			return nil
		}
		return d
	})

	if diff := cmp.Diff(Value{V: 3, Device: plain}, got.ItalicsCorrection[1]); diff != "" {
		t.Errorf("italics correction (-want +got):\n%s", diff)
	}
	gotKern := got.Kerns[1].TopRight
	if diff := cmp.Diff(&Kern{Kerns: []Value{{V: 7}}}, gotKern); diff != "" {
		t.Errorf("kern (-want +got):\n%s", diff)
	}
	if got.Kerns[2].TopLeft != gotKern {
		t.Error("shared kern table not shared in the result")
	}
	if k.Kerns[0].Device != vi {
		t.Error("receiver modified")
	}
}

// TestDevices checks that Devices yields the device tables of the values
// and visits a shared kern table once.
func TestDevices(t *testing.T) {
	d1 := &device.Table{DeltaFormat: device.VariationIndexFormat, InnerIndex: 1}
	d2 := &device.Table{DeltaFormat: device.VariationIndexFormat, InnerIndex: 2}
	k := &Kern{Heights: []Value{{V: 10, Device: d2}}, Kerns: []Value{{V: 7}, {V: 8}}}
	info := &Info{
		TopAccent: map[glyph.ID]Value{1: {V: 3, Device: d1}, 2: {V: 4}},
		Kerns:     map[glyph.ID]*KernInfo{1: {TopRight: k}, 2: {BottomLeft: k}},
	}

	count := map[*device.Table]int{}
	for d := range info.Devices() {
		count[d]++
	}
	want := map[*device.Table]int{d1: 1, d2: 1}
	if diff := cmp.Diff(want, count); diff != "" {
		t.Errorf("yielded device tables (-want +got):\n%s", diff)
	}
}
