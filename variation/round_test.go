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

package variation

import "testing"

func TestOTRound(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{3.5, 4},   // positive tie rounds up, same as math.Round
		{-3.5, -3}, // negative tie rounds up, unlike math.Round (-4)
		{-0.5, 0},  // negative tie rounds up to zero
		{0.5, 1},   // positive tie rounds up
		{-1.5, -1}, // negative tie rounds up
		{2.4, 2},   // non-tie, rounds to nearest as usual
		{2.6, 3},   // non-tie, rounds to nearest as usual
		{-2.4, -2}, // non-tie, rounds to nearest as usual
		{-2.6, -3}, // non-tie, rounds to nearest as usual
		{0, 0},
	}
	for _, c := range cases {
		got := OTRound(c.in)
		if got != c.want {
			t.Errorf("OTRound(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
