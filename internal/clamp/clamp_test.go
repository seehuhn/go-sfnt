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

package clamp

import (
	"math"
	"testing"

	"seehuhn.de/go/geom/rect"
	"seehuhn.de/go/postscript/funit"
)

func TestInt16(t *testing.T) {
	cases := []struct {
		in   float64
		want int16
	}{
		{math.NaN(), 0},
		{math.Inf(1), math.MaxInt16},
		{math.Inf(-1), math.MinInt16},
		{1e9, math.MaxInt16},
		{-1e9, math.MinInt16},
		{32767, math.MaxInt16},
		{32768, math.MaxInt16},
		{-32768, math.MinInt16},
		{-32769, math.MinInt16},
		{12, 12},
		{-12, -12},
		{0, 0},
		{12.7, 12}, // truncates towards zero
		{-12.7, -12},
	}
	for _, c := range cases {
		if got := Int16[int16](c.in); got != c.want {
			t.Errorf("Int16(%v) = %d, want %d", c.in, got, c.want)
		}
		if got := Int16[funit.Int16](c.in); got != funit.Int16(c.want) {
			t.Errorf("Int16[funit.Int16](%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestUint16(t *testing.T) {
	cases := []struct {
		in   float64
		want uint16
	}{
		{math.NaN(), 0},
		{math.Inf(1), math.MaxUint16},
		{math.Inf(-1), 0},
		{1e9, math.MaxUint16},
		{-1, 0},
		{65535, math.MaxUint16},
		{65536, math.MaxUint16},
		{0, 0},
		{12, 12},
		{12.7, 12}, // truncates towards zero
	}
	for _, c := range cases {
		if got := Uint16[uint16](c.in); got != c.want {
			t.Errorf("Uint16(%v) = %d, want %d", c.in, got, c.want)
		}
		if got := Uint16[funit.Uint16](c.in); got != funit.Uint16(c.want) {
			t.Errorf("Uint16[funit.Uint16](%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestRect16RoundsOutward checks that rounding a bounding box to the integer
// type used in font files never shrinks it.
func TestRect16RoundsOutward(t *testing.T) {
	in := rect.Rect{LLx: 0.2, LLy: -0.7, URx: 1.2, URy: 2.3}
	want := funit.Rect16{LLx: 0, LLy: -1, URx: 2, URy: 3}
	if got := Rect16(in); got != want {
		t.Errorf("rounded bbox = %v, want %v", got, want)
	}
}

// TestRect16Saturates checks that a corner outside the int16 range saturates
// while the corners inside it stay exact.
func TestRect16Saturates(t *testing.T) {
	in := rect.Rect{LLx: -1e6, LLy: -12.5, URx: 1e6, URy: 12.5}
	want := funit.Rect16{LLx: -32768, LLy: -13, URx: 32767, URy: 13}
	if got := Rect16(in); got != want {
		t.Errorf("clamped bbox = %v, want %v", got, want)
	}
}
