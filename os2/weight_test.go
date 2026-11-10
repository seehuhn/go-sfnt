// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

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
