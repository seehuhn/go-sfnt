// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2026  Jochen Voss <voss@seehuhn.de>
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

package variation

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"seehuhn.de/go/sfnt/parser"
)

func decodeMap(t *testing.T, data []byte) *DeltaSetIndexMap {
	t.Helper()
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	m, err := ReadDeltaSetIndexMap(p, 0)
	if err != nil {
		t.Fatalf("ReadDeltaSetIndexMap: %v", err)
	}
	return m
}

func TestReadDeltaSetIndexMapFormat0(t *testing.T) {
	// format 0, entryFormat 0x01: innerBitCount = 2, entry size = 1.
	data := []byte{
		0x00,       // format 0
		0x01,       // entryFormat: innerBits=2, size=1
		0x00, 0x03, // mapCount = 3
		0x01, // (0,1) = 0<<2|1
		0x02, // (0,2)
		0x04, // (1,0) = 1<<2|0
	}
	m := decodeMap(t, data)
	want := &DeltaSetIndexMap{Map: []uint32{1, 2, 1 << 16}}
	if diff := cmp.Diff(want, m); diff != "" {
		t.Errorf("map mismatch (-want +got):\n%s", diff)
	}

	// Lookup, including clamping past the end.
	lc := []struct {
		i            uint32
		outer, inner uint16
	}{
		{0, 0, 1}, {1, 0, 2}, {2, 1, 0}, {5, 1, 0},
	}
	for _, c := range lc {
		o, in := m.Lookup(c.i)
		if o != c.outer || in != c.inner {
			t.Errorf("Lookup(%d) = (%d,%d), want (%d,%d)", c.i, o, in, c.outer, c.inner)
		}
	}
}

func TestReadDeltaSetIndexMapFormat1(t *testing.T) {
	// format 1, entryFormat 0x13: innerBits=4, entry size=2.
	data := []byte{
		0x01,                   // format 1
		0x13,                   // entryFormat: innerBits=4, size=2
		0x00, 0x00, 0x00, 0x03, // mapCount = 3
		0x00, 0x05, // (0,5)
		0x00, 0x30, // (3,0) = 3<<4
		0x00, 0x0F, // (0,15)
	}
	m := decodeMap(t, data)
	want := &DeltaSetIndexMap{Map: []uint32{5, 3 << 16, 15}}
	if diff := cmp.Diff(want, m); diff != "" {
		t.Errorf("map mismatch (-want +got):\n%s", diff)
	}
}

// The pre-OT-1.8.4 HVAR index map has a uint16 entryFormat followed by a
// uint16 mapCount.  For entryFormat <= 0x3F the leading byte is zero, so
// the bytes are identical to a format-0 DeltaSetIndexMap.
func TestReadDeltaSetIndexMapLegacyHVAR(t *testing.T) {
	legacy := []byte{
		0x00, 0x01, // uint16 entryFormat = 0x0001
		0x00, 0x03, // uint16 mapCount = 3
		0x01, 0x02, 0x04,
	}
	format0 := []byte{
		0x00,       // format 0
		0x01,       // entryFormat
		0x00, 0x03, // mapCount
		0x01, 0x02, 0x04,
	}
	if !bytes.Equal(legacy, format0) {
		t.Fatal("legacy HVAR layout is not byte-identical to format 0")
	}
	m := decodeMap(t, legacy)
	want := &DeltaSetIndexMap{Map: []uint32{1, 2, 1 << 16}}
	if diff := cmp.Diff(want, m); diff != "" {
		t.Errorf("legacy map mismatch (-want +got):\n%s", diff)
	}
}

func TestDeltaSetIndexMapLookupEmpty(t *testing.T) {
	m := &DeltaSetIndexMap{}
	o, in := m.Lookup(7)
	if o != 0 || in != 0 {
		t.Errorf("empty Lookup = (%d,%d), want (0,0)", o, in)
	}
}

func TestDeltaSetIndexMapRoundTrip(t *testing.T) {
	maps := []*DeltaSetIndexMap{
		{},
		{Map: []uint32{0}},
		{Map: []uint32{1, 2, 1 << 16}},
		{Map: []uint32{5, 3 << 16, 15, 0xFFFF<<16 | 0xFFFF}},
	}
	for i, m := range maps {
		encoded := m.Encode()
		got := decodeMap(t, encoded)
		if diff := cmp.Diff(m, got); diff != "" {
			t.Errorf("map %d: round trip mismatch (-want +got):\n%s", i, diff)
		}
		if !bytes.Equal(encoded, m.Encode()) {
			t.Errorf("map %d: Encode not deterministic", i)
		}
	}
}

func FuzzDeltaSetIndexMap(f *testing.F) {
	f.Add([]byte{0x00, 0x01, 0x00, 0x03, 0x01, 0x02, 0x04})
	f.Add([]byte{0x01, 0x13, 0x00, 0x00, 0x00, 0x03, 0x00, 0x05, 0x00, 0x30, 0x00, 0x0F})
	f.Fuzz(func(t *testing.T, data []byte) {
		p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		m, err := ReadDeltaSetIndexMap(p, 0)
		if err != nil {
			return
		}
		encoded := m.Encode()
		p2 := parser.New(bytes.NewReader(encoded), parser.NewBudget(int64(len(encoded))))
		m2, err := ReadDeltaSetIndexMap(p2, 0)
		if err != nil {
			t.Fatalf("re-decode failed: %v", err)
		}
		if diff := cmp.Diff(m, m2); diff != "" {
			t.Fatalf("round trip mismatch (-want +got):\n%s", diff)
		}
	})
}
