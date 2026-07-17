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

// flags in a packed point-number run control byte
const (
	pointsAreWords    = 0x80
	pointRunCountMask = 0x7F
)

// parsePackedPoints reads a packed point-number sequence from r.  A leading
// count of zero selects "all points" and returns a nil slice.  Otherwise the
// declared count of point numbers is decoded from delta-coded runs; every
// accumulated point number must be strictly below nPoints, and the runs must
// supply exactly the declared count.
func parsePackedPoints(r *byteReader, nPoints int, budget *membudget.Budget) ([]uint16, error) {
	b0, err := r.uint8()
	if err != nil {
		return nil, err
	}
	count := int(b0)
	if b0&0x80 != 0 {
		b1, err := r.uint8()
		if err != nil {
			return nil, err
		}
		count = int(b0&0x7F)<<8 | int(b1)
	}
	if count == 0 {
		return nil, nil // all points
	}
	// each point number needs at least one byte in its run
	if count > r.remaining() {
		return nil, errShortTupleData
	}
	points, err := membudget.AllocSlice[uint16](budget, count)
	if err != nil {
		return nil, err
	}

	produced := 0
	acc := 0
	for produced < count {
		c, err := r.uint8()
		if err != nil {
			return nil, err
		}
		words := c&pointsAreWords != 0
		runLen := int(c&pointRunCountMask) + 1
		if produced+runLen > count {
			return nil, errors.New("variation: point run exceeds declared count")
		}
		for range runLen {
			var d int
			if words {
				v, err := r.uint16()
				if err != nil {
					return nil, err
				}
				d = int(v)
			} else {
				v, err := r.uint8()
				if err != nil {
					return nil, err
				}
				d = int(v)
			}
			acc += d
			if acc >= nPoints || acc > 0xFFFF {
				return nil, errors.New("variation: point number out of range")
			}
			points[produced] = uint16(acc)
			produced++
		}
	}
	return points, nil
}

// encodePackedPoints serializes points as a packed point-number sequence.
// The point numbers must be non-decreasing.  A nil or empty slice encodes as
// the single "all points" byte.
func encodePackedPoints(points []uint16) ([]byte, error) {
	count := len(points)
	if count == 0 {
		return []byte{0}, nil
	}
	if count > 0x7FFF {
		return nil, errors.New("variation: too many point numbers")
	}

	// delta-code the point numbers
	deltas := make([]int, count)
	prev := 0
	for i, p := range points {
		d := int(p) - prev
		if d < 0 {
			return nil, errors.New("variation: point numbers not ascending")
		}
		deltas[i] = d
		prev = int(p)
	}

	var buf []byte
	if count < 0x80 {
		buf = append(buf, byte(count))
	} else {
		buf = append(buf, byte(0x80|count>>8), byte(count))
	}

	// greedy runs of equal width, split at 128 values
	i := 0
	for i < count {
		words := deltas[i] > 0xFF
		j := i + 1
		for j < count && (deltas[j] > 0xFF) == words && j-i < 128 {
			j++
		}
		control := byte(j - i - 1)
		if words {
			control |= pointsAreWords
		}
		buf = append(buf, control)
		for k := i; k < j; k++ {
			if words {
				buf = append(buf, byte(deltas[k]>>8), byte(deltas[k]))
			} else {
				buf = append(buf, byte(deltas[k]))
			}
		}
		i = j
	}
	return buf, nil
}
