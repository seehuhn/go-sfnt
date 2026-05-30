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

package header

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// A table whose offset+length overflows uint32 must be rejected. The real span
// lies far beyond the end of the file, but a naive uint32 sum wraps to a small
// value that spuriously passes the "extends beyond EOF" check.
func TestReadTableOffsetOverflow(t *testing.T) {
	data := make([]byte, 256)
	binary.BigEndian.PutUint32(data[0:], ScalerTypeTrueType)
	binary.BigEndian.PutUint16(data[4:], 1) // numTables

	rec := data[12:28]
	copy(rec[0:], "test")                            // tag
	binary.BigEndian.PutUint32(rec[8:], 0xFFFFFF00)  // offset
	binary.BigEndian.PutUint32(rec[12:], 0x00000200) // length; sum wraps to 0x100

	if _, err := Read(bytes.NewReader(data)); err == nil {
		t.Error("accepted table with offset+length overflowing uint32")
	}
}

func FuzzTables(f *testing.F) {
	buf := &bytes.Buffer{}
	_, _ = Write(buf, ScalerTypeTrueType, map[string][]byte{
		"OS/2": {},
		"hhea": {1},
		"maxp": {2, 3},
		"hmtx": {4, 5, 6},
		"LTSH": {7, 8, 9, 10},
		"VDMX": {11, 12, 13, 14, 15},
	})
	f.Add(buf.Bytes())
	buf.Reset()
	_, _ = Write(buf, ScalerTypeCFF, map[string][]byte{
		"head": {0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}, // must be at least 12 bytes, to hold the checksum
		"hdmx": {12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26},
	})
	f.Add(buf.Bytes())

	f.Fuzz(func(t *testing.T, data1 []byte) {
		r1 := bytes.NewReader(data1)
		info1, err := Read(r1)
		if err != nil {
			return
		}
		tables1 := make(map[string][]byte, len(info1.Toc))
		for name := range info1.Toc {
			body, err := info1.ReadTableBytes(r1, name)
			if err != nil {
				t.Fatal(err)
			}
			tables1[name] = body
		}

		buf := &bytes.Buffer{}
		_, err = Write(buf, info1.ScalerType, tables1)
		if err != nil {
			t.Fatal(err)
		}

		data2 := buf.Bytes()
		r2 := bytes.NewReader(data2)
		info2, err := Read(r2)
		if err != nil {
			t.Fatal(err)
		}
		tables2 := make(map[string][]byte, len(info2.Toc))
		for name := range info2.Toc {
			body, err := info2.ReadTableBytes(r2, name)
			if err != nil {
				t.Fatal(err)
			}
			tables2[name] = body
		}

		if d := cmp.Diff(tables1, tables2); d != "" {
			t.Errorf("tables differ: %s", d)
		}
	})
}
