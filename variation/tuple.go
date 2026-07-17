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
	"errors"

	"seehuhn.de/go/membudget"
)

// flags in the tupleVariationCount field
const (
	sharedPointNumbers = 0x8000
	tupleCountMask     = 0x0FFF
)

// flags in a tupleIndex field
const (
	embeddedPeakTuple  = 0x8000
	intermediateRegion = 0x4000
	privatePointNumber = 0x2000
	tupleIndexMask     = 0x0FFF
)

var errShortTupleData = errors.New("variation: truncated tuple variation data")

// TupleVariation is one tuple record of a glyph or cvar variation data
// block.  It associates a variation region with a set of deltas.
type TupleVariation struct {
	// Peak holds the peak coordinates of the region, one per axis.  A nil
	// Peak means the region is a shared tuple selected by SharedPeak.
	Peak []F2Dot14

	// SharedPeak indexes the shared-tuple array; it is used only when Peak
	// is nil.
	SharedPeak int

	// IntermediateStart and IntermediateEnd give the region's start and end
	// coordinates.  They are nil unless the tuple stores an explicit
	// intermediate region; otherwise the region is implied by Peak.
	IntermediateStart, IntermediateEnd []F2Dot14

	// Points lists the point numbers the deltas apply to.  A nil Points
	// means the deltas apply to all points.
	Points []uint16

	// Deltas holds the delta values.  Its length is the number of applicable
	// points times the dimension count (for gvar all x deltas precede all y
	// deltas).
	Deltas []int32
}

// Scalar returns the interpolation weight of this tuple at the normalized
// axis coordinates coords, a value in the range [0, 1].  shared supplies the
// shared peak tuples (nil for cvar).  A nil Peak resolves through shared; an
// out-of-range SharedPeak or a nil shared yields 0.
func (tv *TupleVariation) Scalar(coords []F2Dot14, shared [][]F2Dot14) float64 {
	peak := tv.Peak
	if peak == nil {
		if tv.SharedPeak < 0 || tv.SharedPeak >= len(shared) {
			return 0
		}
		peak = shared[tv.SharedPeak]
	}
	return tv.region(peak).Scalar(coords)
}

// region assembles a [Region] view from the tuple's peak and, where present,
// its explicit intermediate coordinates.  Without an explicit intermediate
// region, each axis uses the implicit region start = min(0, peak), end =
// max(0, peak).
func (tv *TupleVariation) region(peak []F2Dot14) Region {
	r := make(Region, len(peak))
	for i, p := range peak {
		var start, end F2Dot14
		if tv.IntermediateStart != nil && i < len(tv.IntermediateStart) && i < len(tv.IntermediateEnd) {
			start = tv.IntermediateStart[i]
			end = tv.IntermediateEnd[i]
		} else {
			start = min(0, p)
			end = max(0, p)
		}
		r[i] = RegionAxis{Start: start, Peak: p, End: end}
	}
	return r
}

// DecodeTupleData parses one complete tuple-variation data block: the
// tupleVariationCount and dataOffset header, the per-tuple headers, and the
// serialized data region.  dims is 2 for gvar and 1 for cvar; nPoints is the
// number of deltable values.  allowSharedPoints enables the
// SHARED_POINT_NUMBERS mechanism.  Allocations are charged against budget.
func DecodeTupleData(data []byte, axisCount, dims, nPoints int, allowSharedPoints bool, budget *membudget.Budget) ([]TupleVariation, error) {
	if axisCount < 0 || axisCount > MaxAxisCount {
		return nil, errors.New("variation: invalid axis count")
	}
	if dims < 1 {
		return nil, errors.New("variation: invalid dimension count")
	}
	if len(data) < 4 {
		return nil, errShortTupleData
	}
	tvc := uint16(data[0])<<8 | uint16(data[1])
	dataOffset := int(uint16(data[2])<<8 | uint16(data[3]))
	sharedFlag := tvc&sharedPointNumbers != 0
	count := int(tvc & tupleCountMask)
	if count == 0 {
		return nil, nil
	}
	if dataOffset > len(data) {
		return nil, errShortTupleData
	}
	// each header is at least 4 bytes
	if 4*count > len(data) {
		return nil, errShortTupleData
	}

	tuples, err := membudget.AllocSlice[TupleVariation](budget, count)
	if err != nil {
		return nil, err
	}
	sizes, err := membudget.AllocSlice[int](budget, count)
	if err != nil {
		return nil, err
	}
	private, err := membudget.AllocSlice[bool](budget, count)
	if err != nil {
		return nil, err
	}

	// tuple headers occupy the range [4, dataOffset)
	hr := &byteReader{data: data[:dataOffset], pos: 4}
	for i := range tuples {
		size, err := hr.uint16()
		if err != nil {
			return nil, err
		}
		tupleIndex, err := hr.uint16()
		if err != nil {
			return nil, err
		}
		tv := &tuples[i]
		if tupleIndex&embeddedPeakTuple != 0 {
			tv.Peak, err = readF2Dot14Slice(hr, axisCount, budget)
			if err != nil {
				return nil, err
			}
		} else {
			tv.SharedPeak = int(tupleIndex & tupleIndexMask)
		}
		if tupleIndex&intermediateRegion != 0 {
			tv.IntermediateStart, err = readF2Dot14Slice(hr, axisCount, budget)
			if err != nil {
				return nil, err
			}
			tv.IntermediateEnd, err = readF2Dot14Slice(hr, axisCount, budget)
			if err != nil {
				return nil, err
			}
		}
		sizes[i] = int(size)
		private[i] = tupleIndex&privatePointNumber != 0
	}

	// serialized data region starts at dataOffset
	pos := dataOffset
	var shared []uint16
	if sharedFlag {
		if !allowSharedPoints {
			return nil, errors.New("variation: shared point numbers not allowed")
		}
		r := &byteReader{data: data, pos: pos}
		shared, err = parsePackedPoints(r, nPoints, budget)
		if err != nil {
			return nil, err
		}
		pos = r.pos
	}

	for i := range tuples {
		size := sizes[i]
		if pos+size > len(data) {
			return nil, errShortTupleData
		}
		chunkEnd := pos + size
		cr := &byteReader{data: data[:chunkEnd], pos: pos}

		var points []uint16
		if private[i] {
			points, err = parsePackedPoints(cr, nPoints, budget)
			if err != nil {
				return nil, err
			}
		} else {
			points = shared
		}
		tuples[i].Points = points

		pointCount := len(points)
		if points == nil {
			pointCount = nPoints
		}
		tuples[i].Deltas, err = parsePackedDeltas(cr, pointCount*dims, budget)
		if err != nil {
			return nil, err
		}
		pos = chunkEnd
	}
	return tuples, nil
}

// readF2Dot14Slice reads n F2Dot14 values from r.  It returns nil for n == 0.
func readF2Dot14Slice(r *byteReader, n int, budget *membudget.Budget) ([]F2Dot14, error) {
	if n == 0 {
		return nil, nil
	}
	s, err := membudget.AllocSlice[F2Dot14](budget, n)
	if err != nil {
		return nil, err
	}
	for i := range s {
		v, err := r.int16()
		if err != nil {
			return nil, err
		}
		s[i] = F2Dot14(v)
	}
	return s, nil
}

// EncodeTupleData serializes tuples into a tuple-variation data block.  The
// encoding is deterministic and strict: it embeds a peak tuple whenever
// Peak is non-nil (otherwise it writes the SharedPeak index), marks an
// intermediate region whenever IntermediateStart is non-nil, and stores each
// tuple's point numbers privately.  It never emits SHARED_POINT_NUMBERS.
//
// shared is accepted for symmetry with the shared-tuple array; the
// deterministic encoder does not rewrite embedded peaks into shared
// references and so does not consult it.
func EncodeTupleData(tuples []TupleVariation, axisCount, dims, nPoints int, shared [][]F2Dot14) ([]byte, error) {
	if len(tuples) > tupleCountMask {
		return nil, errors.New("variation: too many tuples")
	}

	var headerBuf, dataBuf []byte
	for i := range tuples {
		tv := &tuples[i]

		// serialized data: private points (if any) then packed deltas.
		// An empty point list cannot be distinguished from "all points" in
		// the wire format, so treat it the same as nil.
		hasPoints := len(tv.Points) > 0
		var sd []byte
		if hasPoints {
			pb, err := encodePackedPoints(tv.Points)
			if err != nil {
				return nil, err
			}
			sd = append(sd, pb...)
		}
		pd, err := encodePackedDeltas(tv.Deltas)
		if err != nil {
			return nil, err
		}
		sd = append(sd, pd...)
		if len(sd) > 0xFFFF {
			return nil, errors.New("variation: tuple data too large")
		}

		var flags, tupleIndex uint16
		if tv.Peak != nil {
			flags |= embeddedPeakTuple
		} else {
			if tv.SharedPeak < 0 || tv.SharedPeak > tupleIndexMask {
				return nil, errors.New("variation: shared peak index out of range")
			}
			tupleIndex = uint16(tv.SharedPeak) & tupleIndexMask
		}
		if tv.IntermediateStart != nil {
			flags |= intermediateRegion
		}
		if hasPoints {
			flags |= privatePointNumber
		}

		headerBuf = appendU16(headerBuf, uint16(len(sd)))
		headerBuf = appendU16(headerBuf, flags|tupleIndex)
		if tv.Peak != nil {
			headerBuf = appendF2Dot14Slice(headerBuf, tv.Peak, axisCount)
		}
		if tv.IntermediateStart != nil {
			headerBuf = appendF2Dot14Slice(headerBuf, tv.IntermediateStart, axisCount)
			headerBuf = appendF2Dot14Slice(headerBuf, tv.IntermediateEnd, axisCount)
		}
		dataBuf = append(dataBuf, sd...)
	}

	dataOffset := 4 + len(headerBuf)
	if dataOffset > 0xFFFF {
		return nil, errors.New("variation: tuple headers too large")
	}

	out := make([]byte, 0, dataOffset+len(dataBuf))
	out = appendU16(out, uint16(len(tuples))&tupleCountMask)
	out = appendU16(out, uint16(dataOffset))
	out = append(out, headerBuf...)
	out = append(out, dataBuf...)
	return out, nil
}

// appendF2Dot14Slice appends exactly n F2Dot14 values from s, padding with
// zeros or truncating as needed.
func appendF2Dot14Slice(buf []byte, s []F2Dot14, n int) []byte {
	for i := range n {
		var v F2Dot14
		if i < len(s) {
			v = s[i]
		}
		buf = appendU16(buf, uint16(v))
	}
	return buf
}

// byteReader is a big-endian cursor over a byte slice.  Reads past the end
// return errShortTupleData.
type byteReader struct {
	data []byte
	pos  int
}

func (r *byteReader) remaining() int { return len(r.data) - r.pos }

func (r *byteReader) uint8() (uint8, error) {
	if r.pos >= len(r.data) {
		return 0, errShortTupleData
	}
	b := r.data[r.pos]
	r.pos++
	return b, nil
}

func (r *byteReader) uint16() (uint16, error) {
	if r.pos+2 > len(r.data) {
		return 0, errShortTupleData
	}
	v := uint16(r.data[r.pos])<<8 | uint16(r.data[r.pos+1])
	r.pos += 2
	return v, nil
}

func (r *byteReader) int16() (int16, error) {
	v, err := r.uint16()
	return int16(v), err
}
