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

// Package variation holds shared types for OpenType font variations.
package variation

import (
	"math"

	"seehuhn.de/go/sfnt/parser"
)

// MaxAxisCount caps the number of variation axes read from untrusted
// fonts.
const MaxAxisCount = 1024

// F2Dot14 is a signed 2.14 fixed-point number, used for normalized
// variation axis coordinates.  The represented value is the stored
// int16 divided by 16384.
type F2Dot14 int16

// Float64 returns f as a floating-point number.
func (f F2Dot14) Float64() float64 {
	return float64(f) / 16384
}

// F2Dot14FromFloat converts f to the nearest F2Dot14 value, clamping to
// the representable range if f is out of bounds.
func F2Dot14FromFloat(f float64) F2Dot14 {
	v := math.Round(f * 16384)
	switch {
	case v > math.MaxInt16:
		v = math.MaxInt16
	case v < math.MinInt16:
		v = math.MinInt16
	}
	return F2Dot14(v)
}

// ReadF2Dot14 reads a single F2Dot14 value from p.
func ReadF2Dot14(p *parser.Parser) (F2Dot14, error) {
	val, err := p.ReadInt16()
	return F2Dot14(val), err
}

// Fixed is a signed 16.16 fixed-point number.  The represented value is
// the stored int32 divided by 65536.
type Fixed int32

// Float64 returns f as a floating-point number.
func (f Fixed) Float64() float64 {
	return float64(f) / 65536
}

// FixedFromFloat converts f to the nearest Fixed value, clamping to the
// representable range if f is out of bounds.
func FixedFromFloat(f float64) Fixed {
	v := math.Round(f * 65536)
	switch {
	case v > math.MaxInt32:
		v = math.MaxInt32
	case v < math.MinInt32:
		v = math.MinInt32
	}
	return Fixed(v)
}

// ReadFixed reads a single Fixed value from p.
func ReadFixed(p *parser.Parser) (Fixed, error) {
	val, err := p.ReadInt32()
	return Fixed(val), err
}
