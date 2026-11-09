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

package os2

import "testing"

func TestWeightFromString(t *testing.T) {
	cases := []struct {
		in   string
		want Weight
	}{
		{"Thin", WeightThin},
		{"Hairline", WeightThin},
		{"Extra Light", WeightExtraLight},
		{"ExtraLight", WeightExtraLight},
		{"Ultra Light", WeightExtraLight},
		{"UltraLight", WeightExtraLight},
		{"Light", WeightLight},
		{"Normal", WeightNormal},
		{"Regular", WeightNormal},
		{"Roman", WeightNormal},
		{"Book", WeightNormal},
		{"Medium", WeightMedium},
		{"Semi Bold", WeightSemiBold},
		{"Semibold", WeightSemiBold},
		{"DemiBold", WeightSemiBold},
		{"Demi", WeightSemiBold},
		{"Bold", WeightBold},
		{"Extra Bold", WeightExtraBold},
		{"ExtraBold", WeightExtraBold},
		{"Ultra Bold", WeightExtraBold},
		{"Black", WeightBlack},
		{"Heavy", WeightBlack},
		{"Ultra", WeightBlack},
		{"Extra Black", WeightBlack},
		{"UltraBlack", WeightBlack},
		{"roman", WeightNormal},
		{"650", 650},
		{"1001", 0},
		{"-5", 0},
		{"", 0},
		{"unknown", 0},
	}
	for _, c := range cases {
		if got := WeightFromString(c.in); got != c.want {
			t.Errorf("WeightFromString(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestWeightStringRoundTrip(t *testing.T) {
	for w := WeightThin; w <= WeightBlack; w += 100 {
		if got := WeightFromString(w.String()); got != w {
			t.Errorf("round trip of %d gave %d", w, got)
		}
	}
}
