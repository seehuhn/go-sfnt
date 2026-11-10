// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

// Package clamp converts floating-point values and rectangles to the
// fixed-width integer types stored in font files.  Values outside a type's
// range saturate at its limits, instead of wrapping around or leaving a
// value which depends on the processor.
//
// The conversion truncates towards zero, so a caller which needs a
// particular rounding rounds the value first.
package clamp

import (
	"math"

	"seehuhn.de/go/geom/rect"
	"seehuhn.de/go/postscript/funit"
)

// Int16 converts v to a 16-bit signed integer type.  Values outside the
// range saturate, and NaN converts to zero.
func Int16[T ~int16](v float64) T {
	switch {
	case math.IsNaN(v):
		return 0
	case v > math.MaxInt16:
		return math.MaxInt16
	case v < math.MinInt16:
		return math.MinInt16
	}
	return T(v)
}

// Uint16 converts v to a 16-bit unsigned integer type.  Values outside the
// range saturate, and NaN converts to zero.
func Uint16[T ~uint16](v float64) T {
	switch {
	case math.IsNaN(v):
		return 0
	case v > math.MaxUint16:
		return math.MaxUint16
	case v < 0:
		return 0
	}
	return T(v)
}

// Rect16 rounds r outwards and converts it to the integer type used in the
// "glyf" and "head" tables, so that the stored box still contains r.  A
// corner outside the range saturates, and the stored box then no longer
// contains r on that side.
func Rect16(r rect.Rect) funit.Rect16 {
	r = r.Rounded()
	return funit.Rect16{
		LLx: Int16[funit.Int16](r.LLx),
		LLy: Int16[funit.Int16](r.LLy),
		URx: Int16[funit.Int16](r.URx),
		URy: Int16[funit.Int16](r.URy),
	}
}
