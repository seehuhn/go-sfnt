// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2025  Jochen Voss <voss@seehuhn.de>
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

package glyf

import (
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"
	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/sfnt/glyph"
)

const f2dot14FactorTest = 1 << 14

func floatToF2dot14Test(f float64) int16 {
	return int16(math.Round(f * f2dot14FactorTest))
}

func f2dot14ToFloatTest(i int16) float64 {
	return float64(i) / f2dot14FactorTest
}

func TestComponentUnpacked_Roundtrip(t *testing.T) {
	tests := []struct {
		name     string
		original ComponentUnpacked
	}{
		{
			name: "identity transform, no flags",
			original: ComponentUnpacked{
				Child: glyph.ID(1),
				Trfm:  matrix.Matrix{1, 0, 0, 1, 0, 0},
			},
		},
		{
			name: "translation only, byte args",
			original: ComponentUnpacked{
				Child: glyph.ID(2),
				Trfm:  matrix.Matrix{1, 0, 0, 1, 10, -20},
			},
		},
		{
			name: "translation only, word args",
			original: ComponentUnpacked{
				Child: glyph.ID(22),
				Trfm:  matrix.Matrix{1, 0, 0, 1, 130, -150},
			},
		},
		{
			name: "uniform scale",
			original: ComponentUnpacked{
				Child: glyph.ID(3),
				Trfm:  matrix.Matrix{0.5, 0, 0, 0.5, 10, 20},
			},
		},
		{
			name: "non-uniform scale",
			original: ComponentUnpacked{
				Child: glyph.ID(4),
				Trfm:  matrix.Matrix{0.5, 0, 0, 0.75, 10, 20},
			},
		},
		{
			name: "full matrix",
			original: ComponentUnpacked{
				Child: glyph.ID(5),
				Trfm:  matrix.Matrix{1, 0.125, 0.25, 0.875, -10, -20},
			},
		},
		{
			name: "with RoundXYToGrid",
			original: ComponentUnpacked{
				Child:         glyph.ID(6),
				Trfm:          matrix.Matrix{1, 0, 0, 1, 10.2, 20.8}, // dx, dy will be rounded by Pack
				RoundXYToGrid: true,
			},
		},
		{
			name: "with UseMyMetrics",
			original: ComponentUnpacked{
				Child:        glyph.ID(7),
				Trfm:         matrix.Matrix{1, 0, 0, 1, 0, 0},
				UseMyMetrics: true,
			},
		},
		{
			name: "with OverlapCompound",
			original: ComponentUnpacked{
				Child:           glyph.ID(8),
				Trfm:            matrix.Matrix{1, 0, 0, 1, 0, 0},
				OverlapCompound: true,
			},
		},
		{
			name: "with ScaledComponentOffset",
			original: ComponentUnpacked{
				Child:                 glyph.ID(9),
				Trfm:                  matrix.Matrix{0.5, 0, 0, 0.5, 10, 20},
				ScaledComponentOffset: true,
			},
		},
		{
			name: "scaled component offset false",
			original: ComponentUnpacked{
				Child:                 glyph.ID(10),
				Trfm:                  matrix.Matrix{0.5, 0, 0, 0.5, 10, 20},
				ScaledComponentOffset: false,
			},
		},
		{
			name: "translation with non-integer values (dx, dy will be rounded by Pack)",
			original: ComponentUnpacked{
				Child: glyph.ID(11),
				Trfm:  matrix.Matrix{1, 0, 0, 1, 10.2, -20.8},
			},
		},
		{
			name: "full matrix with values requiring F2DOT14 rounding",
			original: ComponentUnpacked{
				Child: glyph.ID(12),
				Trfm:  matrix.Matrix{0.1, 0.2, 0.3, 0.4, 5.5, 15.4},
			},
		},
		{
			name: "zero scale (edge case, becomes identity in F2.14 if not careful)",
			original: ComponentUnpacked{
				Child: glyph.ID(13),
				Trfm:  matrix.Matrix{0, 0, 0, 0, 1, 2},
			},
		},
		{
			name: "negative scale",
			original: ComponentUnpacked{
				Child: glyph.ID(14),
				Trfm:  matrix.Matrix{-0.5, 0, 0, -0.5, 10, 20},
			},
		},
		{
			name: "point matching (FlagArgsAreXYValues unset)",
			original: ComponentUnpacked{
				Child:       glyph.ID(15),
				Trfm:        matrix.Matrix{1, 0, 0, 1, 0, 0}, // Offset ignored in point matching
				AlignPoints: true,
				OurPoint:    5,
				TheirPoint:  3,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packed := tt.original.Pack()
			unpacked, err := packed.Unpack()
			if err != nil {
				t.Fatalf("Unpack() error = %v", err)
			}

			expected := tt.original
			// Adjust expected Trfm to account for conversions:
			// - Trfm[0-3] (scale/matrix) go float64 -> F2DOT14 (int16) -> float64
			// - Trfm[4-5] (dx,dy) go float64 -> int16/int8 (rounded) -> float64
			expected.Trfm[0] = f2dot14ToFloatTest(floatToF2dot14Test(tt.original.Trfm[0]))
			expected.Trfm[1] = f2dot14ToFloatTest(floatToF2dot14Test(tt.original.Trfm[1]))
			expected.Trfm[2] = f2dot14ToFloatTest(floatToF2dot14Test(tt.original.Trfm[2]))
			expected.Trfm[3] = f2dot14ToFloatTest(floatToF2dot14Test(tt.original.Trfm[3]))
			expected.Trfm[4] = math.Round(tt.original.Trfm[4])
			expected.Trfm[5] = math.Round(tt.original.Trfm[5])

			// If original was 0 scale, and it became identity through F2.14, reflect that.
			// This specific check might be too nuanced if floatToF2dot14(0) is 0.
			// The general f2dot14ToFloat(floatToF2dot14(x)) should handle it.

			if diff := cmp.Diff(expected, *unpacked); diff != "" {
				t.Errorf("Roundtrip failed (-expected +got):\n%s", diff)
				t.Logf("Original: %+v\n", tt.original)
				t.Logf("Packed Flags: %s (0x%04X)\nPacked Data: %x\n", packed.Flags.String(), uint16(packed.Flags), packed.Data)
				t.Logf("Unpacked: %+v\n", *unpacked)
				t.Logf("Adjusted Expected for comparison: %+v\n", expected)
			}
		})
	}
}
