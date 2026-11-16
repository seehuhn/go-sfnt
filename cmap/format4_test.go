// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package cmap

import (
	"errors"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"seehuhn.de/go/sfnt/glyph"
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
		{PlatformID: 1, EncodingID: 0}: mustEncode(t, s1),
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

// TestFormat4RangeOffsetDelta checks that idDelta is added to the non-zero
// glyph IDs taken from glyphIdArray.
func TestFormat4RangeOffsetDelta(t *testing.T) {
	words := []uint16{
		4, 38, 0, // format, length, language
		4, 4, 1, 0, // segCountX2, searchRange, entrySelector, rangeShift
		0x0043, 0xFFFF, // endCode
		0,              // reservedPad
		0x0041, 0xFFFF, // startCode
		10, 1, // idDelta
		4, 0, // idRangeOffset
		3, 0, 0xFFFF, // glyphIdArray
	}
	var data []byte
	for _, w := range words {
		data = append(data, byte(w>>8), byte(w))
	}

	got, err := decodeFormat4(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := Format4{
		0x41: 13, // 3 + 10
		0x43: 9,  // 0xFFFF + 10, modulo 65536
	}
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("decode failed (-want +got):\n%s", d)
	}
}

// TestFormat4LookupRange checks that runes outside the BMP are not mapped.
func TestFormat4LookupRange(t *testing.T) {
	cmap := Format4{0x41: 5}
	for _, r := range []rune{0x10041, 0x41 - 0x10000, -1} {
		if g := cmap.Lookup(r); g != 0 {
			t.Errorf("Lookup(%#x) = %d, want 0", r, g)
		}
	}
	if g := cmap.Lookup(0x41); g != 5 {
		t.Errorf("Lookup(0x41) = %d, want 5", g)
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

		data2, err := c1.Encode(0)
		if err != nil {
			t.Fatal(err)
		}
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

// TestFormat4EncodeTooLarge checks that Encode returns ErrTooLarge if the
// mapping does not fit into a format 4 subtable, and that a caller can then
// fall back to format 12.
func TestFormat4EncodeTooLarge(t *testing.T) {
	c4 := Format4{}
	c12 := Format12{}
	// Each code needs at least 8 bytes, whether it gets a segment of its
	// own or is stored in a glyph ID array which also spans the gaps.
	for i := range 8200 {
		code := 5 * i
		g := glyph.ID(1 + (i*i)%65521)
		c4[uint16(code)] = g
		c12[uint32(code)] = g
	}

	var enc Subtable = c4
	_, err := enc.Encode(0)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("got error %v, want ErrTooLarge", err)
	}
	enc = c12
	data := mustEncode(t, enc)
	dec, err := decodeFormat12(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(c12, dec); diff != "" {
		t.Errorf("fallback round trip failed (-want +got):\n%s", diff)
	}
}
