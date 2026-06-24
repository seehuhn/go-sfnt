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

package device

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/parser"
)

// memReader adapts a byte slice to the parser.ReadSeekSizer interface.
type memReader struct {
	*bytes.Reader
}

func (r memReader) Size() int64 {
	return r.Reader.Size()
}

func newParser(t *testing.T, data []byte) *parser.Parser {
	t.Helper()
	return parser.New(memReader{bytes.NewReader(data)}, parser.NewBudget(int64(len(data))))
}

func TestRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   *Table
	}{
		{
			name: "format-1 small range",
			in: &Table{
				StartSize:   12,
				EndSize:     14,
				Deltas:      []int8{1, -1, 0},
				DeltaFormat: 1,
			},
		},
		{
			name: "format-1 full word",
			in: &Table{
				StartSize:   8,
				EndSize:     15,
				Deltas:      []int8{1, -1, 0, 1, -2, -1, 0, 1},
				DeltaFormat: 1,
			},
		},
		{
			name: "format-2 spec example",
			in: &Table{
				StartSize:   1,
				EndSize:     2,
				Deltas:      []int8{1, -3},
				DeltaFormat: 2,
			},
		},
		{
			name: "format-2 spans two words",
			in: &Table{
				StartSize:   10,
				EndSize:     14,
				Deltas:      []int8{7, -8, 0, 1, -1},
				DeltaFormat: 2,
			},
		},
		{
			name: "format-3 spec example",
			in: &Table{
				StartSize:   1,
				EndSize:     5,
				Deltas:      []int8{-10, 17, -32, 64, -127},
				DeltaFormat: 3,
			},
		},
		{
			name: "variation index",
			in: &Table{
				OuterIndex:  0x1234,
				InnerIndex:  0x5678,
				DeltaFormat: VariationIndexFormat,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			encoded := tc.in.Encode()
			if len(encoded) != tc.in.EncodeLen() {
				t.Errorf("encode length mismatch: Encode=%d EncodeLen=%d", len(encoded), tc.in.EncodeLen())
			}
			p := newParser(t, encoded)
			got, err := Read(p, 0)
			if err != nil {
				t.Fatalf("read failed: %v", err)
			}
			if diff := cmp.Diff(tc.in, got); diff != "" {
				t.Errorf("round trip mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFormat2SpecExample(t *testing.T) {
	// chapter2.md §"Device tables" example 8: a Device table with
	// 4-bit deltas at sizes 1 and 2 (values 1 and -3) is encoded
	// as: 00 01 00 02 00 02 1D 00.
	in := &Table{
		StartSize:   1,
		EndSize:     2,
		Deltas:      []int8{1, -3},
		DeltaFormat: 2,
	}
	want := []byte{0x00, 0x01, 0x00, 0x02, 0x00, 0x02, 0x1D, 0x00}
	got := in.Encode()
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("encode mismatch (-want +got):\n%s", diff)
	}
}

func TestRejectInvalidFormat(t *testing.T) {
	// deltaFormat 0 and reserved values must be rejected.
	for _, format := range []uint16{0x0000, 0x0004, 0x0005, 0x7FFF} {
		buf := []byte{
			0x00, 0x01, // startSize
			0x00, 0x02, // endSize
			byte(format >> 8), byte(format),
		}
		p := newParser(t, buf)
		_, err := Read(p, 0)
		if err == nil {
			t.Errorf("deltaFormat 0x%04x: expected error, got nil", format)
		}
	}
}

func TestRejectInvertedRange(t *testing.T) {
	buf := []byte{
		0x00, 0x05, // startSize=5
		0x00, 0x02, // endSize=2
		0x00, 0x02, // deltaFormat=2
	}
	p := newParser(t, buf)
	_, err := Read(p, 0)
	if err == nil {
		t.Errorf("inverted range: expected error, got nil")
	}
}

func TestRejectTruncated(t *testing.T) {
	// header complete but delta words missing
	buf := []byte{
		0x00, 0x00, // startSize=0
		0x00, 0x03, // endSize=3, count=4
		0x00, 0x03, // deltaFormat=3 (8-bit), needs 2 words = 4 bytes
		0x12, 0x34, // only one word provided
	}
	p := newParser(t, buf)
	_, err := Read(p, 0)
	if err == nil {
		t.Errorf("truncated: expected error, got nil")
	}
}

// TestEncodeInvalidDeltaFormatPanics confirms that Encode and EncodeLen
// agree on rejecting an invalid DeltaFormat — the alternative (silently
// returning a partial header) would corrupt parent subtable offsets.
func TestEncodeInvalidDeltaFormatPanics(t *testing.T) {
	for _, format := range []uint16{0, 4, 5, 0x7FFF, 0x8001, 0xFFFF} {
		t.Run(fmt.Sprintf("format=0x%04x", format), func(t *testing.T) {
			tab := &Table{DeltaFormat: format, Deltas: []int8{1, 2, 3}}
			assertPanics(t, func() { _ = tab.EncodeLen() })
			assertPanics(t, func() { _ = tab.Encode() })
		})
	}
}

func TestEncodeDeltaOutOfRangePanics(t *testing.T) {
	// deltas outside the signed range of the format's bit width must be
	// rejected, not silently masked
	cases := []struct {
		format uint16
		delta  int8
	}{
		{1, 2}, {1, -3}, // format 1: 2-bit, range -2..1
		{2, 8}, {2, -9}, // format 2: 4-bit, range -8..7
	}
	for _, c := range cases {
		tab := &Table{StartSize: 1, EndSize: 1, Deltas: []int8{c.delta}, DeltaFormat: c.format}
		assertPanics(t, func() { _ = tab.Encode() })
	}
}

func assertPanics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("expected panic, got none")
		}
	}()
	fn()
}

func FuzzRead(f *testing.F) {
	for _, seed := range [][]byte{
		{0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
		{0x00, 0x01, 0x00, 0x02, 0x00, 0x02, 0x1D, 0x00},
		{0x12, 0x34, 0x56, 0x78, 0x80, 0x00},
		{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		p := newParser(t, data)
		got, err := Read(p, 0)
		if err != nil {
			return
		}
		// round-trip: re-encode and re-read must yield the same table
		encoded := got.Encode()
		p2 := newParser(t, encoded)
		got2, err := Read(p2, 0)
		if err != nil {
			t.Fatalf("re-read failed: %v\nfirst read: %+v\nencoded: %x", err, got, encoded)
		}
		if diff := cmp.Diff(got, got2); diff != "" {
			t.Errorf("round trip mismatch (-want +got):\n%s", diff)
		}
	})
}
