// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2023  Jochen Voss <voss@seehuhn.de>
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

package type1

import (
	"errors"
	"io"
)

func DecodePFB(r io.Reader) io.Reader {
	return &pfbReader{r: r}
}

type pfbReader struct {
	r     io.Reader
	state int
	len   int64
	tail  byte
}

func (r *pfbReader) Read(b []byte) (n int, err error) {
	for len(b) > 0 {
		switch r.state {
		case 0: // start of new section
			var buf [6]byte
			k, err := io.ReadFull(r.r, buf[:])
			if k >= 2 && buf[0] == 0x80 && buf[1] == 0x03 && err == io.ErrUnexpectedEOF {
				err = nil
			} else if err != nil {
				return n, err
			}
			if buf[0] != 0x80 || buf[1] == 0 || buf[1] > 3 {
				return n, ErrInvalidPFB
			}
			r.state = int(buf[1])
			r.len = int64(buf[2]) | int64(buf[3])<<8 | int64(buf[4])<<16 | int64(buf[5])<<24
		case -1: // leftover byte from binary section
			b[0] = r.tail
			b = b[1:]
			n++
			if r.len == 0 {
				r.state = 0
			} else {
				r.state = 2
			}
		case 1: // ASCII section
			k := len(b)
			if int64(k) > r.len {
				k = int(r.len)
			}
			k, err = r.r.Read(b[:k])
			r.len -= int64(k)
			n += k
			if err != nil {
				return n, err
			}
			b = b[k:]
			if r.len == 0 {
				r.state = 0
			}
		case 2: // binary section (hex decode data on the fly)
			k := (len(b) + 1) / 2
			if int64(k) > r.len {
				k = int(r.len)
			}
			k, err = io.ReadFull(r.r, b[:k])
			r.len -= int64(k)
			if err != nil {
				return n, err
			}
			l := 2 * k
			if len(b) < l {
				r.tail = hexEncode(b[k-1] & 0x0f)
				l--
				r.state = -1
			}
			for i := l - 1; i >= 0; i-- {
				if i%2 == 0 {
					b[i] = hexEncode(b[i/2] >> 4)
				} else {
					b[i] = hexEncode(b[i/2] & 0x0f)
				}
			}
			b = b[l:]
			n += l
			if r.len == 0 && r.state != -1 {
				r.state = 0
			}
		case 3: // EOF
			return n, io.EOF
		}
	}
	return n, nil
}

func hexEncode(b byte) byte {
	if b < 10 {
		return '0' + b
	}
	return 'a' + b - 10
}

var ErrInvalidPFB = errors.New("invalid PFB file")
