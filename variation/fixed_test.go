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

import (
	"bytes"
	"math"
	"testing"

	"seehuhn.de/go/sfnt/parser"
)

// TestF2Dot14RoundTrip exhaustively checks that every possible int16 bit
// pattern survives a Float64/F2Dot14FromFloat round trip.
func TestF2Dot14RoundTrip(t *testing.T) {
	for i := math.MinInt16; i <= math.MaxInt16; i++ {
		orig := F2Dot14(i)
		got := F2Dot14FromFloat(orig.Float64())
		if got != orig {
			t.Fatalf("round trip failed for %d: got %d", orig, got)
		}
	}
}

func TestF2Dot14FromFloatClamp(t *testing.T) {
	cases := []struct {
		in   float64
		want F2Dot14
	}{
		{2.0, math.MaxInt16},              // above range, clamps to max
		{-3.0, math.MinInt16},             // below range, clamps to min
		{1.99993896484375, math.MaxInt16}, // max representable value, not clamped
		{-2.0000610351563, math.MinInt16}, // just below the min representable value
	}
	for _, c := range cases {
		got := F2Dot14FromFloat(c.in)
		if got != c.want {
			t.Errorf("F2Dot14FromFloat(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestF2Dot14FromFloatTie(t *testing.T) {
	// 0.5/16384 rounds away from zero at the tie.
	cases := []struct {
		in   float64
		want F2Dot14
	}{
		{0.5 / 16384, 1},
		{-0.5 / 16384, -1},
		{1.5 / 16384, 2},
		{-1.5 / 16384, -2},
	}
	for _, c := range cases {
		got := F2Dot14FromFloat(c.in)
		if got != c.want {
			t.Errorf("F2Dot14FromFloat(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestF2Dot14FromFloatSpecialValues(t *testing.T) {
	cases := []struct {
		in   float64
		want F2Dot14
	}{
		{math.NaN(), 0},
		{math.Inf(1), math.MaxInt16},
		{math.Inf(-1), math.MinInt16},
	}
	for _, c := range cases {
		got := F2Dot14FromFloat(c.in)
		if got != c.want {
			t.Errorf("F2Dot14FromFloat(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestFixedRoundTrip(t *testing.T) {
	samples := []int32{0, 1, -1, 65536, -65536, math.MaxInt32, math.MinInt32, 12345, -98765}
	for _, s := range samples {
		orig := Fixed(s)
		got := FixedFromFloat(orig.Float64())
		if got != orig {
			t.Errorf("round trip failed for %d: got %d", orig, got)
		}
	}
}

func TestFixedFromFloatClamp(t *testing.T) {
	cases := []struct {
		in   float64
		want Fixed
	}{
		{1e9, math.MaxInt32},
		{-1e9, math.MinInt32},
	}
	for _, c := range cases {
		got := FixedFromFloat(c.in)
		if got != c.want {
			t.Errorf("FixedFromFloat(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestFixedFromFloatTie(t *testing.T) {
	cases := []struct {
		in   float64
		want Fixed
	}{
		{0.5 / 65536, 1},
		{-0.5 / 65536, -1},
	}
	for _, c := range cases {
		got := FixedFromFloat(c.in)
		if got != c.want {
			t.Errorf("FixedFromFloat(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestFixedFromFloatSpecialValues(t *testing.T) {
	cases := []struct {
		in   float64
		want Fixed
	}{
		{math.NaN(), 0},
		{math.Inf(1), math.MaxInt32},
		{math.Inf(-1), math.MinInt32},
	}
	for _, c := range cases {
		got := FixedFromFloat(c.in)
		if got != c.want {
			t.Errorf("FixedFromFloat(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestReadF2Dot14(t *testing.T) {
	data := []byte{0xC0, 0x00} // -1.0 in F2Dot14
	buf := bytes.NewReader(data)
	p := parser.New(buf, parser.NewBudget(int64(len(data))))

	got, err := ReadF2Dot14(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Float64() != -1.0 {
		t.Errorf("wrong value, expected -1.0 but got %v", got.Float64())
	}
}

func TestReadFixed(t *testing.T) {
	data := []byte{0xFF, 0xFF, 0x00, 0x00} // -1.0 in 16.16 fixed
	buf := bytes.NewReader(data)
	p := parser.New(buf, parser.NewBudget(int64(len(data))))

	got, err := ReadFixed(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Float64() != -1.0 {
		t.Errorf("wrong value, expected -1.0 but got %v", got.Float64())
	}
}

func TestMaxAxisCount(t *testing.T) {
	if MaxAxisCount != 1024 {
		t.Errorf("wrong value, expected 1024 but got %d", MaxAxisCount)
	}
}
