// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

// Package vardelta stores values computed from the variation deltas of a
// variable font in the 16-bit fields of font files.  A value which does not
// fit gives an error, instead of wrapping around or saturating.
package vardelta

import (
	"errors"
	"math"
)

// ErrRange is returned when a value with its variation delta added does not
// fit the 16-bit field which stores it.
var ErrRange = errors.New("sfnt: value out of range after applying variation deltas")

// AddInt16 returns v + delta.  If the sum is outside the range of int16,
// AddInt16 returns [ErrRange].
func AddInt16[T ~int16](v T, delta int64) (T, error) {
	sum := int64(v) + delta
	if sum < math.MinInt16 || sum > math.MaxInt16 {
		return 0, ErrRange
	}
	return T(sum), nil
}

// Int16 converts x, an integer computed by applying variation deltas, to a
// 16-bit signed integer type.  If x is outside the range of int16, Int16
// returns [ErrRange].
func Int16[T ~int16](x float64) (T, error) {
	if !(x >= math.MinInt16 && x <= math.MaxInt16) {
		return 0, ErrRange
	}
	return T(x), nil
}

// Uint16 converts x, an integer computed by applying variation deltas, to a
// 16-bit unsigned integer type.  If x is outside the range of uint16, Uint16
// returns [ErrRange].
func Uint16[T ~uint16](x float64) (T, error) {
	if !(x >= 0 && x <= math.MaxUint16) {
		return 0, ErrRange
	}
	return T(x), nil
}
