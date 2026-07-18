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

package gvar

import (
	"math"
	"testing"
)

// iupCase runs iupContour on both axes and returns the resulting per-point
// (x,y) deltas.  Untouched input deltas are passed as (0,0).
func iupCase(coords [][2]float64, touched []bool, deltas [][2]float64) [][2]float64 {
	n := len(coords)
	dx := make([]float64, n)
	dy := make([]float64, n)
	cx := make([]float64, n)
	cy := make([]float64, n)
	for i := range coords {
		cx[i], cy[i] = coords[i][0], coords[i][1]
		dx[i], dy[i] = deltas[i][0], deltas[i][1]
	}
	iupContour(dx, cx, touched)
	iupContour(dy, cy, touched)
	out := make([][2]float64, n)
	for i := range out {
		out[i] = [2]float64{dx[i], dy[i]}
	}
	return out
}

func closeEnough(a, b [][2]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.Abs(a[i][0]-b[i][0]) > 1e-9 || math.Abs(a[i][1]-b[i][1]) > 1e-9 {
			return false
		}
	}
	return true
}

func TestIUPContour(t *testing.T) {
	tests := []struct {
		name    string
		coords  [][2]float64
		touched []bool
		deltas  [][2]float64
		want    [][2]float64
	}{
		{
			// Cross-checked against fontTools varLib.iup.iup_contour
			// (fontTools 4.53.1):
			//   coords = [(0,0),(10,100),(20,50),(30,200),(40,300)]
			//   deltas = [None,(100,10),None,None,(500,90)]
			//   -> [(100,10),(100,10),(233.333..,10),(366.666..,50),(500,90)]
			// Exercises wrap-around (point 0 between touched 4 and 1),
			// below-range clamping and interior interpolation on both axes.
			name:    "fonttools-wraparound",
			coords:  [][2]float64{{0, 0}, {10, 100}, {20, 50}, {30, 200}, {40, 300}},
			touched: []bool{false, true, false, false, true},
			deltas:  [][2]float64{{0, 0}, {100, 10}, {0, 0}, {0, 0}, {500, 90}},
			want: [][2]float64{
				{100, 10},
				{100, 10},
				{100 + 10.0*400/30, 10},
				{100 + 20.0*400/30, 50},
				{500, 90},
			},
		},
		{
			// fontTools cross-check: outside-range on both sides, boundary,
			// and a descending y delta.
			//   coords = [(5,5),(10,10),(20,20),(30,30),(50,7)]
			//   deltas = [None,(40,-40),None,(80,-80),None]
			//   -> [(40,-40),(40,-40),(60,-60),(80,-80),(80,-40)]
			name:    "fonttools-outside-range",
			coords:  [][2]float64{{5, 5}, {10, 10}, {20, 20}, {30, 30}, {50, 7}},
			touched: []bool{false, true, false, true, false},
			deltas:  [][2]float64{{0, 0}, {40, -40}, {0, 0}, {80, -80}, {0, 0}},
			want:    [][2]float64{{40, -40}, {40, -40}, {60, -60}, {80, -80}, {80, -40}},
		},
		{
			// equal reference coordinates: unequal deltas infer 0, equal
			// deltas infer that shared value.  fontTools cross-check:
			//   coords = [(10,0)]*4, deltas = [(7,3),None,(9,3),None]
			//   -> [(7,3),(0,3),(9,3),(0,3)]
			name:    "equal-ref-coords",
			coords:  [][2]float64{{10, 0}, {10, 0}, {10, 0}, {10, 0}},
			touched: []bool{true, false, true, false},
			deltas:  [][2]float64{{7, 3}, {0, 0}, {9, 3}, {0, 0}},
			want:    [][2]float64{{7, 3}, {0, 3}, {9, 3}, {0, 3}},
		},
		{
			// exactly one touched point: every untouched point copies it.
			name:    "single-touched",
			coords:  [][2]float64{{0, 0}, {10, 10}, {20, 20}, {30, 30}},
			touched: []bool{false, false, true, false},
			deltas:  [][2]float64{{0, 0}, {0, 0}, {7, -3}, {0, 0}},
			want:    [][2]float64{{7, -3}, {7, -3}, {7, -3}, {7, -3}},
		},
		{
			// no touched points: all inferred deltas are zero.
			name:    "none-touched",
			coords:  [][2]float64{{0, 0}, {10, 10}, {20, 20}},
			touched: []bool{false, false, false},
			deltas:  [][2]float64{{0, 0}, {0, 0}, {0, 0}},
			want:    [][2]float64{{0, 0}, {0, 0}, {0, 0}},
		},
		{
			// exact boundary c(t) == c1 yields the endpoint delta; midpoint
			// yields the average.
			name:    "boundary-and-midpoint",
			coords:  [][2]float64{{0, 0}, {10, 0}, {20, 0}},
			touched: []bool{true, false, true},
			deltas:  [][2]float64{{100, 0}, {0, 0}, {300, 0}},
			want:    [][2]float64{{100, 0}, {200, 0}, {300, 0}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := iupCase(tt.coords, tt.touched, tt.deltas)
			if !closeEnough(got, tt.want) {
				t.Errorf("iup mismatch\n got %v\nwant %v", got, tt.want)
			}
		})
	}
}
