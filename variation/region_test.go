// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package variation

import "testing"

func f2(x float64) F2Dot14 { return F2Dot14FromFloat(x) }

func TestRegionScalar(t *testing.T) {
	tent := Region{{Start: f2(0), Peak: f2(0.5), End: f2(1.0)}}

	cases := []struct {
		name   string
		region Region
		coords []F2Dot14
		want   float64
	}{
		{"peak", tent, []F2Dot14{f2(0.5)}, 1.0},
		{"left ramp", tent, []F2Dot14{f2(0.25)}, 0.5},
		{"right ramp", tent, []F2Dot14{f2(0.75)}, 0.5},
		{"at start", tent, []F2Dot14{f2(0)}, 0.0},
		{"at end", tent, []F2Dot14{f2(1.0)}, 0.0},
		{"below start", tent, []F2Dot14{f2(-0.5)}, 0.0},

		// peak == 0 makes the axis a no-op (factor 1).
		{"peak zero", Region{{Start: f2(-1), Peak: f2(0), End: f2(1)}}, []F2Dot14{f2(0.7)}, 1.0},
		// start > peak is invalid, axis ignored.
		{"start gt peak", Region{{Start: f2(0.5), Peak: f2(0.25), End: f2(1)}}, []F2Dot14{f2(0.3)}, 1.0},
		// peak > end is invalid, axis ignored.
		{"peak gt end", Region{{Start: f2(0), Peak: f2(0.75), End: f2(0.5)}}, []F2Dot14{f2(0.3)}, 1.0},
		// start < 0 < end with non-zero peak is invalid, axis ignored.
		{"straddles zero", Region{{Start: f2(-1), Peak: f2(0.5), End: f2(1)}}, []F2Dot14{f2(0.4)}, 1.0},

		// two axes multiply.
		{"two axes both partial",
			Region{{f2(0), f2(0.5), f2(1)}, {f2(0), f2(1), f2(1)}},
			[]F2Dot14{f2(0.25), f2(0.5)}, 0.25},
		{"two axes one at peak",
			Region{{f2(0), f2(0.5), f2(1)}, {f2(0), f2(1), f2(1)}},
			[]F2Dot14{f2(0.25), f2(1.0)}, 0.5},

		// missing coordinate defaults to 0.
		{"missing coord defaults zero", tent, []F2Dot14{}, 0.0},

		// empty region ⇒ scalar 1.
		{"empty region", Region{}, []F2Dot14{f2(0.5)}, 1.0},
	}

	for _, c := range cases {
		got := c.region.Scalar(c.coords)
		if got != c.want {
			t.Errorf("%s: Scalar = %v, want %v", c.name, got, c.want)
		}
	}
}
