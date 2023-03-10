// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2022  Jochen Voss <voss@seehuhn.de>
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

package cmap

import (
	"reflect"
	"testing"
)

func FuzzFormat12(f *testing.F) {
	f.Add(format12{
		{StartCharCode: 10, EndCharCode: 20, StartGlyphID: 30},
		{StartCharCode: 1000, EndCharCode: 2000, StartGlyphID: 41},
		{StartCharCode: 2000, EndCharCode: 3000, StartGlyphID: 1},
	}.Encode(0))

	f.Fuzz(func(t *testing.T, data []byte) {
		c1, err := decodeFormat12(data, nil)
		if err != nil {
			return
		}

		data2 := c1.Encode(0)
		if len(data2) > len(data) {
			t.Error("too long")
		}

		c2, err := decodeFormat12(data2, nil)
		if err != nil {
			t.Error(err)
		}

		if !reflect.DeepEqual(c1, c2) {
			t.Error("not equal")
		}
	})
}

var _ Subtable = format12(nil)
