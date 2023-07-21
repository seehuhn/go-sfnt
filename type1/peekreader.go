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

import "io"

func peek(r io.Reader, n int) ([]byte, io.Reader, error) {
	buf := make([]byte, n)

	if r, ok := r.(io.ReadSeeker); ok {
		pos, err := r.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, nil, err
		}
		k, err := io.ReadFull(r, buf)
		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			return nil, nil, err
		}
		_, err = r.Seek(pos, io.SeekStart)
		if err != nil {
			return nil, nil, err
		}
		return buf[:k], r, nil
	}

	k, err := io.ReadFull(r, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, nil, err
	}

	return buf[:k], &peekReader{r: r, buf: buf[:k]}, nil
}

type peekReader struct {
	r   io.Reader
	buf []byte
}

func (r *peekReader) Read(b []byte) (n int, err error) {
	if len(r.buf) == 0 {
		return r.r.Read(b)
	}
	k := len(b)
	if k > len(r.buf) {
		k = len(r.buf)
	}
	copy(b, r.buf[:k])
	r.buf = r.buf[k:]
	return k, nil
}
