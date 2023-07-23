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
	"io"
)

type hexWriter struct {
	w   io.Writer
	buf []byte
}

func (w *hexWriter) Write(p []byte) (n int, err error) {
	for _, c := range p {
		w.buf = append(w.buf, hex[c>>4], hex[c&0x0f])
		n++
		if len(w.buf) >= 78 {
			if err = w.flush(); err != nil {
				return n, err
			}
		}
	}
	return len(p), nil
}

func (w *hexWriter) Close() error {
	return w.flush()
}

func (w *hexWriter) flush() error {
	if len(w.buf) == 0 {
		return nil
	}
	w.buf = append(w.buf, '\n')
	_, err := w.w.Write(w.buf)
	w.buf = w.buf[:0]
	return err
}

const hex = "0123456789abcdef"
