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
	"math"

	"seehuhn.de/go/membudget"
)

// flags in a packed delta run control byte.  Per OpenType 1.9.1,
// DELTAS_ARE_ZERO dominates: if set (including 0xC0), the run is a zero run
// with no data bytes and DELTAS_ARE_WORDS is ignored.  There is no 32-bit
// delta form in tuple variation stores.
const (
	deltasAreZero     = 0x80
	deltasAreWords    = 0x40
	deltaRunCountMask = 0x3F
)

// parsePackedDeltas reads exactly needed delta values from r, decoding the
// packed-delta runs.  Runs that supply more values than needed are truncated
// (excess ignored); running out of data before needed values are produced is
// an error.
func parsePackedDeltas(r *byteReader, needed int, budget *membudget.Budget) ([]int32, error) {
	if needed <= 0 {
		return nil, nil
	}
	deltas, err := membudget.AllocSlice[int32](budget, needed)
	if err != nil {
		return nil, err
	}

	produced := 0
	for produced < needed {
		c, err := r.uint8()
		if err != nil {
			return nil, err
		}
		zero := c&deltasAreZero != 0
		words := c&deltasAreWords != 0
		runLen := int(c&deltaRunCountMask) + 1
		take := min(runLen, needed-produced)
		switch {
		case zero: // run of zeros, no data bytes; dominates DELTAS_ARE_WORDS
			for range take {
				deltas[produced] = 0
				produced++
			}
		case words: // 16-bit deltas
			for range take {
				v, err := r.int16()
				if err != nil {
					return nil, err
				}
				deltas[produced] = int32(v)
				produced++
			}
		default: // 8-bit deltas
			for range take {
				v, err := r.uint8()
				if err != nil {
					return nil, err
				}
				deltas[produced] = int32(int8(v))
				produced++
			}
		}
	}
	return deltas, nil
}

// encodePackedDeltas serializes deltas using the shortest of zero, byte and
// word runs.  The output is deterministic.  Deltas outside the int16 range
// cannot be represented and result in an error.
func encodePackedDeltas(deltas []int32) ([]byte, error) {
	var buf []byte
	i := 0
	n := len(deltas)
	for i < n {
		if deltas[i] == 0 {
			j := i + 1
			for j < n && deltas[j] == 0 && j-i < 64 {
				j++
			}
			buf = append(buf, byte(deltasAreZero|(j-i-1)))
			i = j
			continue
		}
		if deltas[i] < math.MinInt16 || deltas[i] > math.MaxInt16 {
			return nil, errors.New("variation: delta out of int16 range")
		}
		word := deltas[i] < math.MinInt8 || deltas[i] > math.MaxInt8
		j := i + 1
		for j < n && j-i < 64 {
			v := deltas[j]
			if v == 0 {
				break
			}
			if v < math.MinInt16 || v > math.MaxInt16 {
				break
			}
			vword := v < math.MinInt8 || v > math.MaxInt8
			if vword != word {
				break
			}
			j++
		}
		if word {
			buf = append(buf, byte(deltasAreWords|(j-i-1)))
			for k := i; k < j; k++ {
				buf = appendU16(buf, uint16(deltas[k]))
			}
		} else {
			buf = append(buf, byte(j-i-1))
			for k := i; k < j; k++ {
				buf = append(buf, byte(deltas[k]))
			}
		}
		i = j
	}
	return buf, nil
}
