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
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/parser"
)

// TestValueRecordApply checks that Apply updates all five glyph-info
// fields it can touch, and never panics on a fully populated record.
func TestValueRecordApply(t *testing.T) {
	vr := &GposValueRecord{
		XPlacement:    1,
		YPlacement:    2,
		XAdvance:      3,
		YAdvance:      4,
		XPlacementDev: &device.Table{OuterIndex: 1, DeltaFormat: device.VariationIndexFormat},
		YPlacementDev: &device.Table{OuterIndex: 2, DeltaFormat: device.VariationIndexFormat},
		XAdvanceDev:   &device.Table{OuterIndex: 3, DeltaFormat: device.VariationIndexFormat},
		YAdvanceDev:   &device.Table{OuterIndex: 4, DeltaFormat: device.VariationIndexFormat},
	}
	g := &glyph.Info{XOffset: 100, YOffset: 200, Advance: 300, YAdvance: 400}
	vr.Apply(g)
	want := &glyph.Info{XOffset: 101, YOffset: 202, Advance: 303, YAdvance: 404}
	if diff := cmp.Diff(want, g); diff != "" {
		t.Errorf("apply mismatch (-want +got):\n%s", diff)
	}
}

// TestValueRecordRoundTripAllBits round-trips a Gpos1_1 carrying every
// supported valueFormat bit, including all four Device pointers.
func TestValueRecordRoundTripAllBits(t *testing.T) {
	l1 := &Gpos1_1{
		Cov: coverage.Table{5: 0},
		Adjust: &GposValueRecord{
			XPlacement: 10,
			YPlacement: 20,
			XAdvance:   30,
			YAdvance:   40,
			XPlacementDev: &device.Table{
				StartSize: 8, EndSize: 10,
				Deltas: []int8{1, -1, 2}, DeltaFormat: 2,
			},
			YPlacementDev: &device.Table{
				StartSize: 6, EndSize: 7,
				Deltas: []int8{0, -1}, DeltaFormat: 1,
			},
			XAdvanceDev: &device.Table{
				OuterIndex:  3,
				InnerIndex:  7,
				DeltaFormat: device.VariationIndexFormat,
			},
			YAdvanceDev: &device.Table{
				StartSize: 12, EndSize: 14,
				Deltas: []int8{5, -5, 0}, DeltaFormat: 3,
			},
		},
	}

	data := l1.encode()
	if len(data) != l1.encodeLen() {
		t.Fatalf("encode length mismatch: encode=%d encodeLen=%d", len(data), l1.encodeLen())
	}

	p := parser.New(bytes.NewReader(data))
	if err := p.Discard(2); err != nil {
		t.Fatal(err)
	}
	got, err := readGpos1_1(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(l1, got); diff != "" {
		t.Errorf("round-trip mismatch (-want +got):\n%s", diff)
	}
}
