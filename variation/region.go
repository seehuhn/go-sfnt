// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package variation

// RegionAxis describes the extent of a variation region along one axis.
// A region contributes to an instance only while the axis coordinate lies
// strictly between Start and End, peaking at Peak.
type RegionAxis struct {
	Start F2Dot14
	Peak  F2Dot14
	End   F2Dot14
}

// Region is the support of one variation region, with one [RegionAxis]
// per variation axis.
type Region []RegionAxis

// Scalar returns the interpolation factor of the region at the given
// normalized axis coordinates, a value in the range [0, 1].
//
// coords holds one coordinate per axis; a missing coordinate is treated
// as zero.  Axes with a zero peak, or with an invalid or zero-straddling
// support, do not contribute (factor 1).  The overall factor is the
// product of the per-axis factors, and drops to zero as soon as any axis
// coordinate lies outside the open support (Start, End).
func (r Region) Scalar(coords []F2Dot14) float64 {
	scalar := 1.0
	for i, axis := range r {
		start := axis.Start.Float64()
		peak := axis.Peak.Float64()
		end := axis.End.Float64()

		if peak == 0 {
			continue
		}
		if start > peak || peak > end {
			continue
		}
		if start < 0 && end > 0 {
			continue
		}

		var coord float64
		if i < len(coords) {
			coord = coords[i].Float64()
		}

		if coord == peak {
			continue
		}
		if coord <= start || coord >= end {
			return 0
		}
		if coord < peak {
			scalar *= (coord - start) / (peak - start)
		} else {
			scalar *= (end - coord) / (end - peak)
		}
	}
	return scalar
}
