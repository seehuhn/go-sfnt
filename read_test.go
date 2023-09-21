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

package sfnt

import (
	"bytes"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
	"seehuhn.de/go/sfnt/header"
)

// TestEmptyGDEF tests that we can read a font with an empty GDEF table.
// This is a regression test for https://github.com/seehuhn/go-sfnt/issues/1 .
func TestEmptyGDEF(t *testing.T) {
	// create a TrueType font with an empty GDEF table
	r := bytes.NewReader(goregular.TTF)
	info, err := header.Read(r)
	if err != nil {
		t.Fatal(err)
	}
	tables := make(map[string][]byte)
	for name := range info.Toc {
		data, err := info.ReadTableBytes(r, name)
		if err != nil {
			t.Fatal(err)
		}
		tables[name] = data
	}
	tables["GDEF"] = []byte{}
	w := &bytes.Buffer{}
	_, err = header.Write(w, info.ScalerType, tables)
	if err != nil {
		t.Fatal(err)
	}

	// read the font again
	r = bytes.NewReader(w.Bytes())
	_, err = Read(r)
	if err != nil {
		t.Fatal(err)
	}
}
