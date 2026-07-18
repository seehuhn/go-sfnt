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

package gvar

// iupContour fills in the inferred deltas for the untouched points of a single
// contour along one axis ("Inferred deltas for un-referenced point numbers" in
// the OpenType gvar specification).  coords holds the original coordinate of
// each point on this axis; touched[i] reports whether point i carries an
// explicit delta.  On entry deltas[i] holds the explicit delta for touched
// points; on return every untouched entry is overwritten with its interpolated
// delta.  Interpolation is cyclic within the contour.
//
// The three slices must have equal length.
func iupContour(deltas, coords []float64, touched []bool) {
	// indices of the touched points, in contour order
	var idx []int
	for i, t := range touched {
		if t {
			idx = append(idx, i)
		}
	}

	if len(idx) == 0 {
		// no reference points: every inferred delta is zero
		for i := range deltas {
			deltas[i] = 0
		}
		return
	}
	if len(idx) == 1 {
		// single reference point: every untouched point copies its delta
		d := deltas[idx[0]]
		for i := range deltas {
			if !touched[i] {
				deltas[i] = d
			}
		}
		return
	}

	// fill each run of untouched points between two consecutive touched
	// points, treating the point sequence as cyclic
	for k := range idx {
		a := idx[k]
		b := idx[(k+1)%len(idx)]
		iupSegment(deltas, coords, touched, a, b)
	}
}

// iupSegment fills the untouched points strictly between touched points a and
// b (walking cyclically forward from a to b) on a single axis.
func iupSegment(deltas, coords []float64, touched []bool, a, b int) {
	n := len(touched)

	c1, c2 := coords[a], coords[b]
	d1, d2 := deltas[a], deltas[b]

	if c1 == c2 {
		// degenerate span: a constant delta only if both ends agree
		d := 0.0
		if d1 == d2 {
			d = d1
		}
		for i := next(a, n); i != b; i = next(i, n) {
			deltas[i] = d
		}
		return
	}

	if c1 > c2 {
		c1, c2 = c2, c1
		d1, d2 = d2, d1
	}
	scale := (d2 - d1) / (c2 - c1)
	for i := next(a, n); i != b; i = next(i, n) {
		x := coords[i]
		var d float64
		switch {
		case x <= c1:
			d = d1
		case x >= c2:
			d = d2
		default:
			d = d1 + (x-c1)*scale
		}
		deltas[i] = d
	}
}

func next(i, n int) int {
	i++
	if i >= n {
		i = 0
	}
	return i
}
