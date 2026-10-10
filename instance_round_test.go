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

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/hvar"
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

// TestApplyMVARClippingMetrics checks the MVAR tags for OS/2 Windows metrics.
func TestApplyMVARClippingMetrics(t *testing.T) {
	mv := &mvar.Table{
		Store: &variation.ItemVariationStore{
			Regions: []variation.Region{
				{{Start: 0, Peak: 0x4000, End: 0x4000}},
			},
			Data: []*variation.ItemVariationData{
				{
					RegionIndexes: []uint16{0},
					Deltas:        [][]int32{{20}, {-10}},
				},
			},
		},
		Records: []mvar.Record{
			{Tag: "hcla", OuterIndex: 0, InnerIndex: 0},
			{Tag: "hcld", OuterIndex: 0, InnerIndex: 1},
		},
	}
	out := &Font{WinAscent: 1000, WinDescent: 300}

	applyMVAR(out, mv, []variation.F2Dot14{0x4000})

	if out.WinAscent != 1020 || out.WinDescent != 290 {
		t.Errorf("Windows metrics = (%d, %d), want (1020, 290)",
			out.WinAscent, out.WinDescent)
	}
}

// TestApplyMVARHheaSync checks that the hhea metrics follow the typographic
// metrics when both were equal before instancing, and stay put otherwise.
func TestApplyMVARHheaSync(t *testing.T) {
	mv := &mvar.Table{
		Store: &variation.ItemVariationStore{
			Regions: []variation.Region{
				{{Start: 0, Peak: 0x4000, End: 0x4000}},
			},
			Data: []*variation.ItemVariationData{
				{
					RegionIndexes: []uint16{0},
					Deltas:        [][]int32{{20}, {-10}, {5}},
				},
			},
		},
		Records: []mvar.Record{
			{Tag: "hasc", OuterIndex: 0, InnerIndex: 0},
			{Tag: "hdsc", OuterIndex: 0, InnerIndex: 1},
			{Tag: "hlgp", OuterIndex: 0, InnerIndex: 2},
		},
	}
	norm := []variation.F2Dot14{0x4000}

	synced := &Font{
		Ascent: 1000, Descent: -300, LineGap: 100,
		HheaAscent: 1000, HheaDescent: -300, HheaLineGap: 100,
	}
	applyMVAR(synced, mv, norm)
	if synced.HheaAscent != 1020 || synced.HheaDescent != -310 || synced.HheaLineGap != 105 {
		t.Errorf("synced hhea metrics = (%d, %d, %d), want (1020, -310, 105)",
			synced.HheaAscent, synced.HheaDescent, synced.HheaLineGap)
	}

	separate := &Font{
		Ascent: 1000, Descent: -300, LineGap: 100,
		HheaAscent: 900, HheaDescent: -250, HheaLineGap: 20,
	}
	applyMVAR(separate, mv, norm)
	if separate.Ascent != 1020 || separate.Descent != -310 || separate.LineGap != 105 {
		t.Errorf("typographic metrics = (%d, %d, %d), want (1020, -310, 105)",
			separate.Ascent, separate.Descent, separate.LineGap)
	}
	if separate.HheaAscent != 900 || separate.HheaDescent != -250 || separate.HheaLineGap != 20 {
		t.Errorf("separate hhea metrics = (%d, %d, %d), want (900, -250, 20)",
			separate.HheaAscent, separate.HheaDescent, separate.HheaLineGap)
	}
}

// TestInstanceCFF2HVARHalfTie checks that an HVAR delta of +0.5 is added to
// the stored "hmtx" width, rather than to a value reconstructed from the
// design width which may fall just short of it.  At a scale of 0.001 and
// 1000 units per em, 4007 design units convert back to 4006.9999999999995.
func TestInstanceCFF2HVARHalfTie(t *testing.T) {
	f := &Font{
		UnitsPerEm: 1000,
		FontMatrix: matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
		Hvar: &hvar.Table{
			Store: &variation.ItemVariationStore{
				Regions: []variation.Region{
					{{Start: 0, Peak: 0x4000, End: 0x4000}},
				},
				Data: []*variation.ItemVariationData{
					{RegionIndexes: []uint16{0}, Deltas: [][]int32{{1}}},
				},
			},
		},
	}
	outlines := &cff.OutlinesCFF2{
		Glyphs:   []*cff.GlyphCFF2{{}},
		Widths:   []float64{4007},
		Private:  []*cff.PrivateCFF2{{}},
		FDSelect: func(glyph.ID) int { return 0 },
	}

	// coord 0.5 against peak 1.0 gives scalar 0.5, so the delta is +0.5
	out := &Font{}
	err := instanceCFF2(f, out, outlines, []variation.F2Dot14{0x2000})
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Outlines.(*cff.Outlines).Glyphs[0].Width; got != 4008 {
		t.Errorf("advance = %v, want 4008 (otRound(4007.5))", got)
	}
}
