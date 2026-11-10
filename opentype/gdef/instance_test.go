// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2022  Jochen Voss <voss@seehuhn.de>
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
