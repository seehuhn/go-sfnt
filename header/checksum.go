// Copyright 2021 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package header

import (
	"encoding/binary"
)

// checksum implements the sfnt checksum algorithm: the sum of data as
// big-endian uint32 words, with the final partial word zero-padded.
// https://learn.microsoft.com/en-us/typography/opentype/spec/otff#calculating-checksums
func checksum(data []byte) uint32 {
	var sum uint32
	for len(data) >= 4 {
		sum += binary.BigEndian.Uint32(data)
		data = data[4:]
	}
	if len(data) > 0 {
		var last [4]byte
		copy(last[:], data)
		sum += binary.BigEndian.Uint32(last[:])
	}
	return sum
}
