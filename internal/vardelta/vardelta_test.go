// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package vardelta

import (
	"errors"
	"math"
	"testing"
)

func TestAddInt16(t *testing.T) {
	cases := []struct {
		v     int16
		delta int64
		want  int16
		ok    bool
	}{
		{100, 50, 150, true},
		{100, -150, -50, true},
		{math.MaxInt16 - 1, 1, math.MaxInt16, true},
		{math.MinInt16 + 1, -1, math.MinInt16, true},
		{math.MaxInt16, 1, 0, false},
		{math.MinInt16, -1, 0, false},
		{0, 1 << 40, 0, false},
		{0, -1 << 40, 0, false},
	}
	for _, c := range cases {
		got, err := AddInt16(c.v, c.delta)
		if !c.ok {
			if !errors.Is(err, ErrRange) {
				t.Errorf("%d%+d: got %d, %v; want ErrRange", c.v, c.delta, got, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("%d%+d: got %d, %v; want %d", c.v, c.delta, got, err, c.want)
		}
	}
}

func TestInt16(t *testing.T) {
	for _, x := range []float64{0, -5, math.MinInt16, math.MaxInt16} {
		got, err := Int16[int16](x)
		if err != nil || float64(got) != x {
			t.Errorf("Int16(%g) = %d, %v", x, got, err)
		}
	}
	for _, x := range []float64{math.MinInt16 - 1, math.MaxInt16 + 1, math.Inf(1), math.NaN()} {
		if _, err := Int16[int16](x); !errors.Is(err, ErrRange) {
			t.Errorf("Int16(%g): got %v, want ErrRange", x, err)
		}
	}
}

func TestUint16(t *testing.T) {
	for _, x := range []float64{0, 500, math.MaxUint16} {
		got, err := Uint16[uint16](x)
		if err != nil || float64(got) != x {
			t.Errorf("Uint16(%g) = %d, %v", x, got, err)
		}
	}
	for _, x := range []float64{-1, math.MaxUint16 + 1, math.Inf(-1), math.NaN()} {
		if _, err := Uint16[uint16](x); !errors.Is(err, ErrRange) {
			t.Errorf("Uint16(%g): got %v, want ErrRange", x, err)
		}
	}
}
