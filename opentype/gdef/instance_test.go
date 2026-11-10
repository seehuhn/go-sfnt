// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gdef

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/device"
)

func TestBakeVariationDevices(t *testing.T) {
	plain := &device.Table{StartSize: 8, EndSize: 8, Deltas: []int8{1}, DeltaFormat: 1}
	shared := []CaretValue{
		{Coordinate: 10, Device: &device.Table{DeltaFormat: device.VariationIndexFormat, InnerIndex: 5}},
		{Coordinate: 20, Device: plain},
	}
	table := &Table{
		LigCaretList: &LigCaretList{
			Cov:    coverage.Table{1: 0, 2: 1, 3: 2},
			Carets: [][]CaretValue{shared, nil, shared},
		},
	}
	resolve := func(outer, inner uint16) funit.Int16 { return funit.Int16(inner) }

	got := table.BakeVariationDevices(resolve)

	baked := []CaretValue{{Coordinate: 15}, {Coordinate: 20, Device: plain}}
	want := &Table{
		LigCaretList: &LigCaretList{
			Cov:    table.LigCaretList.Cov,
			Carets: [][]CaretValue{baked, nil, baked},
		},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("unexpected result (-want +got):\n%s", diff)
	}

	// shared rows stay shared, and the input is not modified
	carets := got.LigCaretList.Carets
	if &carets[0][0] != &carets[2][0] {
		t.Error("shared row copied")
	}
	if shared[0].Device == nil {
		t.Error("input modified")
	}
}
