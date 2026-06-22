// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2021  Jochen Voss <voss@seehuhn.de>
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

package header

import (
	"bytes"
	"testing"
)

// TestWriteChecksum checks the contract that a font written by Write carries a
// valid OpenType checksum: Write sets the head table's checkSumAdjustment so
// that summing the entire file as big-endian uint32 words yields the magic
// constant 0xB1B0AFBA.  The summation here is an independent reimplementation,
// so the test does not merely echo the package's own checksum routine.
func TestWriteChecksum(t *testing.T) {
	head := make([]byte, 54) // real head tables are 54 bytes long
	for i := range head {
		head[i] = byte(7 * i) // arbitrary, non-trivial content
	}
	tables := map[string][]byte{
		"head": head,
		"glyf": {1, 2, 3, 4, 5}, // odd length: also exercises 4-byte padding
		"cmap": {9, 8, 7, 6},
	}

	buf := &bytes.Buffer{}
	if _, err := Write(buf, ScalerTypeTrueType, tables); err != nil {
		t.Fatal(err)
	}

	if sum := fileChecksum(buf.Bytes()); sum != 0xB1B0AFBA {
		t.Errorf("whole-file checksum = %#08x, want 0xB1B0AFBA", sum)
	}
}

// fileChecksum sums data as a sequence of big-endian uint32 words, zero-padding
// a final partial word.
func fileChecksum(data []byte) uint32 {
	var sum uint32
	for i := 0; i < len(data); i += 4 {
		var w uint32
		for j := range 4 {
			w <<= 8
			if i+j < len(data) {
				w |= uint32(data[i+j])
			}
		}
		sum += w
	}
	return sum
}

func TestChecksum(t *testing.T) {
	cases := []struct {
		Body     []byte
		Expected uint32
	}{
		{[]byte{0, 1, 2, 3}, 0x00010203},
		{[]byte{0, 1, 2, 3, 4, 5, 6, 7}, 0x0406080a},
		{[]byte{1}, 0x01000000},
		{[]byte{1, 2, 3}, 0x01020300},
		{[]byte{1, 0, 0, 0, 1}, 0x02000000},
		{[]byte{255, 255, 255, 255, 0, 0, 0, 1}, 0},
	}

	for i, test := range cases {
		computed := checksum(test.Body)
		if computed != test.Expected {
			t.Errorf("test %d failed: %08x != %08x",
				i+1, computed, test.Expected)
		}
	}
}
