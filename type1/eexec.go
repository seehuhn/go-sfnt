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

func obfuscateCharstring(plain []byte, iv []byte) []byte {
	var R uint16 = 4330
	var c1 uint16 = eexecC1
	var c2 uint16 = eexecC2
	cipher := make([]byte, len(iv)+len(plain))
	copy(cipher, iv)
	copy(cipher[len(iv):], plain)
	for i, c := range cipher {
		c = c ^ byte(R>>8)
		R = (uint16(c)+R)*c1 + c2
		cipher[i] = c
	}
	return cipher
}

func deobfuscateCharstring(cipher []byte, n int) []byte {
	if len(cipher) < n {
		return nil
	}

	var R uint16 = 4330
	var c1 uint16 = eexecC1
	var c2 uint16 = eexecC2
	plain := make([]byte, 0, len(cipher)-n)
	for i, cipher := range cipher {
		if i >= n {
			plain = append(plain, cipher^byte(R>>8))
		}
		R = (uint16(cipher)+R)*c1 + c2
	}
	return plain
}

type eexecWriter struct {
	w   io.Writer
	buf []byte
	pos int
	R   uint16
}

func newEExecWriter(w io.Writer) (*eexecWriter, error) {
	res := &eexecWriter{
		w:   w,
		buf: make([]byte, 512),
		R:   eexecR0,
	}
	iv := []byte{'X' ^ byte(eexecR0>>8), 0, 0, 0}
	_, err := res.Write(iv)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func (w *eexecWriter) Write(p []byte) (n int, err error) {
	for len(p) > 0 {
		k := copy(w.buf[w.pos:], p)
		w.pos += k
		n += k
		p = p[k:]

		if w.pos >= len(w.buf) {
			err = w.flush()
			if err != nil {
				return n, err
			}
		}
	}
	return n, nil
}

func (w *eexecWriter) Close() error {
	return w.flush()
}

func (w *eexecWriter) flush() error {
	for i := 0; i < w.pos; i++ {
		w.buf[i] = w.buf[i] ^ byte(w.R>>8)
		w.R = (uint16(w.buf[i])+w.R)*eexecC1 + eexecC2
	}

	_, err := w.w.Write(w.buf[:w.pos])
	if err != nil {
		return err
	}
	w.pos = 0
	return nil
}

const (
	eexecC1 = 52845
	eexecC2 = 22719
	eexecR0 = 55665
)
