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
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestPeekReader(t *testing.T) {
	msg := "Hello World!"
	for i := 0; i < 2; i++ {
		var r io.Reader
		switch i {
		case 0:
			r = strings.NewReader(msg)
		case 1:
			buf := &bytes.Buffer{}
			buf.WriteString(msg)
			r = buf
		}
		head, r2, err := peek(r, 1)
		if err != nil {
			t.Fatal(err)
		}
		if string(head) != msg[:1] {
			t.Errorf("%d: got %q, want %q", i, head, msg[:1])
		}
		all, err := io.ReadAll(r2)
		if err != nil {
			t.Fatal(err)
		}
		if string(all) != msg {
			t.Errorf("%d: got %q, want %q", i, all, msg)
		}
	}
}
