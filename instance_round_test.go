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

package sfnt

import (
	"testing"

	"seehuhn.de/go/sfnt/mvar"
	"seehuhn.de/go/sfnt/variation"
)

// TestApplyMVARNegativeHalfTie checks that an MVAR delta landing on an exact
// negative half-integer tie rounds with OpenType's otRound convention
// (toward +Inf), not Go's math.Round (away from zero): an Ascent of 0 with
// an accumulated delta of -0.5 must become 0, not -1.
func TestApplyMVARNegativeHalfTie(t *testing.T) {
	store := &variation.ItemVariationStore{
		Regions: []variation.Region{
			{{Start: 0, Peak: 0x4000, End: 0x4000}}, // 1-axis region, peak 1.0
		},
		Data: []*variation.ItemVariationData{
			{
				RegionIndexes: []uint16{0},
				Deltas:        [][]int32{{-1}}, // at scalar 1.0, delta -1
			},
		},
	}
	mv := &mvar.Table{
		Store:   store,
		Records: []mvar.Record{{Tag: "hasc", OuterIndex: 0, InnerIndex: 0}},
	}

	out := &Font{Ascent: 0}
	// coord 0.5 against peak 1.0 gives scalar 0.5, so the delta is -0.5
	applyMVAR(out, mv, []variation.F2Dot14{0x2000})

	if out.Ascent != 0 {
		t.Errorf("Ascent = %d, want 0 (otRound(-0.5)=0); math.Round would give -1", out.Ascent)
	}
}
