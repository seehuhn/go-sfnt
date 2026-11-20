// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gdef

import (
	"testing"

	"github.com/google/go-cmp/cmp"
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
	resolve := func(outer, inner uint16) int64 { return int64(inner) }

	got, err := table.BakeVariationDevices(resolve)
	if err != nil {
		t.Fatal(err)
	}

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

// TestBakeVariationDevicesRange checks that a caret coordinate which leaves
// the 16-bit range after baking gives an error.
func TestBakeVariationDevicesRange(t *testing.T) {
	table := &Table{
		LigCaretList: &LigCaretList{
			Cov: coverage.Table{1: 0},
			Carets: [][]CaretValue{{
				{Coordinate: 32000, Device: &device.Table{DeltaFormat: device.VariationIndexFormat}},
			}},
		},
	}
	resolve := func(outer, inner uint16) int64 { return 1000 }
	if _, err := table.BakeVariationDevices(resolve); err == nil {
		t.Error("out-of-range caret coordinate accepted")
	}
}

// TestMapDevices checks that MapDevices replaces and removes device tables,
// keeps the coordinates and shared rows, and leaves the receiver unchanged.
func TestMapDevices(t *testing.T) {
	plain := &device.Table{StartSize: 8, EndSize: 8, Deltas: []int8{1}, DeltaFormat: 1}
	vi := &device.Table{DeltaFormat: device.VariationIndexFormat, InnerIndex: 5}
	shared := []CaretValue{{Coordinate: 10, Device: vi}, {Coordinate: 20, Device: plain}}
	table := &Table{
		LigCaretList: &LigCaretList{
			Cov:    coverage.Table{1: 0, 2: 1},
			Carets: [][]CaretValue{shared, shared},
		},
	}

	got := table.MapDevices(func(d *device.Table) *device.Table {
		if d.IsVariationIndex() {
			return nil
		}
		return d
	})

	want := []CaretValue{{Coordinate: 10}, {Coordinate: 20, Device: plain}}
	carets := got.LigCaretList.Carets
	if diff := cmp.Diff(want, carets[0]); diff != "" {
		t.Errorf("caret values (-want +got):\n%s", diff)
	}
	if &carets[0][0] != &carets[1][0] {
		t.Error("shared row not shared in the result")
	}
	if shared[0].Device != vi {
		t.Error("receiver modified")
	}
}

// TestDevices checks that Devices yields the device tables of the ligature
// caret values and visits a shared row once.
func TestDevices(t *testing.T) {
	d1 := &device.Table{DeltaFormat: device.VariationIndexFormat, InnerIndex: 1}
	d2 := &device.Table{DeltaFormat: device.VariationIndexFormat, InnerIndex: 2}
	shared := []CaretValue{{Coordinate: 10, Device: d1}, {Coordinate: 20}}
	table := &Table{
		LigCaretList: &LigCaretList{
			Cov:    coverage.Table{1: 0, 2: 1, 3: 2},
			Carets: [][]CaretValue{shared, {{Coordinate: 30, Device: d2}}, shared},
		},
	}

	count := map[*device.Table]int{}
	for d := range table.Devices() {
		count[d]++
	}
	want := map[*device.Table]int{d1: 1, d2: 1}
	if diff := cmp.Diff(want, count); diff != "" {
		t.Errorf("yielded device tables (-want +got):\n%s", diff)
	}
}
