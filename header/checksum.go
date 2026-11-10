// Copyright 2021 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package header

import (
	"encoding/binary"
)

type check struct {
	sum  uint32
	buf  [4]byte
	used int
}

func (s *check) Write(p []byte) (int, error) {
	n := 0
	for len(p) > 0 {
		k := copy(s.buf[s.used:], p)
		p = p[k:]
		n += k
		s.used += k

		if s.used == 4 {
			s.sum += binary.BigEndian.Uint32(s.buf[:])
			s.used = 0
		}
	}
	return n, nil
}

func (s *check) Sum() uint32 {
	if s.used != 0 {
		_, _ = s.Write([]byte{0, 0, 0}[:4-s.used])
	}
	return s.sum
}

func (s *check) Reset() {
	s.sum = 0
	s.used = 0
}

// checksum implements the sfnt checksum algorithm.
// https://docs.microsoft.com/en-us/typography/opentype/spec/otff#calculating-checksums
func checksum(data []byte) uint32 {
	cc := &check{}
	_, _ = cc.Write(data)
	return cc.Sum()
}
