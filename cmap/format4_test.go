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

	"github.com/google/go-cmp/cmp"
)

func TestFormat4(t *testing.T) {
	s1 := Format4{
		0x20: 1,
		0x21: 2,
		0x48: 3,
		0x57: 4,
		0x64: 5,
		0x65: 6,
		0x6c: 7,
		0x6f: 8,
		0x72: 9,
		0xaa: 10,
		0xba: 11,
	}
	t1 := Table{
		{PlatformID: 1, EncodingID: 0}: s1.Encode(0),
	}
	data := t1.Encode()
	t2, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(t2) != 1 {
		t.Fatal("wrong number of subtables")
	}
	s2, err := t2.GetNoLang(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if d := cmp.Diff(s1, s2); d != "" {
		t.Error(d)
	}
}

func FuzzFormat4(f *testing.F) {
	f.Add([]byte{
		0x00, 0x04, 0x00, 0x18, 0x00, 0x00, 0x00, 0x02,
		0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0xff, 0xff,
		0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0x00, 0x00,
	})

	f.Add([]byte{
		0x00, 0x04, 0x00, 0x20, 0x00, 0x00, 0x00, 0x04,
		0x00, 0x04, 0x00, 0x01, 0x00, 0x00, 0xe3, 0x3f,
		0xff, 0xff, 0x00, 0x00, 0xe1, 0x00, 0xff, 0xff,
		0x1f, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
	})

	f.Add([]byte{
		0x00, 0x04, 0x00, 0x38, 0x00, 0x00, 0x00, 0x0a,
		0x00, 0x08, 0x00, 0x02, 0x00, 0x02, 0x00, 0x00,
		0x00, 0x0d, 0x00, 0x20, 0x00, 0xa0, 0xff, 0xff,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x0d, 0x00, 0x20,
		0x00, 0xa0, 0xff, 0xff, 0x00, 0x01, 0xff, 0xf5,
		0xff, 0xe3, 0xff, 0x95, 0x00, 0x01, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		c1, err := decodeFormat4(data, nil)
		if err != nil {
			return
		}

		data2 := c1.Encode(0)
		// if len(data2) > len(data) {
		// 	t.Error("too long")
		// }

		c2, err := decodeFormat4(data2, nil)
		if err != nil {
			t.Error(err)
		}

		if !reflect.DeepEqual(c1, c2) {
			// for i := uint32(0); i < 65536; i++ {
			// 	if c1[i] != c2[i] {
			// 		fmt.Printf("%5d | %5d | %5d \n", i, c1[i], c2[i])
			// 	}
			// }
			t.Error("not equal")
		}
	})
}

var _ Subtable = Format4(nil)
