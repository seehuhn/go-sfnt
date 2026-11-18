// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package cmap

import (
	"encoding/binary"
	"reflect"
	"testing"

	"seehuhn.de/go/sfnt/glyph"
)

func TestFormat12(t *testing.T) {
	in := []byte{
		0, 12, // uint16 format
		0, 0, // uint16 reserved
		0, 0, 0, 16 + 12 + 12, // uint32 length
		0, 0, 0, 0, // uint32 language
		0, 0, 0, 2, // uint32 numGroups

		0, 0, 0, 'A', // uint32 startCharCode
		0, 0, 0, 'Z', // uint32 endCharCode
		0, 0, 0, 2, // uint32 startGlyphID

		0, 0, 0, '~', // uint32 startCharCode
		0, 0, 0, '~', // uint32 endCharCode
		0, 0, 0, 1, // uint32 startGlyphID
	}
	cmap, err := decodeFormat12(in, nil)
	if err != nil {
		t.Fatal(err)
	}

	type testCase struct {
		code uint32
		gid  glyph.ID
	}
	cases := []testCase{
		{'A', 2},
		{'B', 3},
		{'Z', 27},
		{'a', 0},
		{'~', 1},
	}
	for _, c := range cases {
		gid := cmap.Lookup(rune(c.code))
		if gid != c.gid {
			t.Errorf("Lookup(%d)=%d, want %d", c.code, gid, c.gid)
		}
	}
}

// TestFormat12GlyphRange checks that segments whose glyph IDs do not fit in
// the uint16 glyph.ID range are rejected rather than silently wrapped around.
func TestFormat12GlyphRange(t *testing.T) {
	cases := []struct {
		desc       string
		startGID   uint32
		endChar    uint32
		wantReject bool
	}{
		{"max valid glyph", 0xFFFF, 0, false},
		{"range up to max glyph", 0, 0xFFFF, false},
		{"start glyph overflows uint16", 0x1_0000, 0, true},
		{"range overflows uint16", 1, 0xFFFF, true},
	}
	for _, c := range cases {
		_, err := decodeFormat12(makeFormat12([3]uint32{0, c.endChar, c.startGID}), nil)
		if c.wantReject && err == nil {
			t.Errorf("%s: accepted, want rejected", c.desc)
		}
		if !c.wantReject && err != nil {
			t.Errorf("%s: rejected (%v), want accepted", c.desc, err)
		}
	}
}

// TestFormat12BeyondUnicode checks that a subtable with character codes
// beyond the Unicode range can be read, and that a read-write-read cycle is
// stable.
func TestFormat12BeyondUnicode(t *testing.T) {
	data := makeFormat12(
		[3]uint32{'A', 'Z', 2},
		[3]uint32{0x10_FFFE, 0x11_0001, 30},
		[3]uint32{0x8000_0000, 0x8000_0003, 40},
		[3]uint32{0xFFFF_FFFF, 0xFFFF_FFFF, 0x1_0000},
	)
	if err := format12RoundTrip(t, data); err != nil {
		t.Fatal(err)
	}
}

// TestFormat12EncodeBeyondUnicode checks that codes beyond the Unicode range
// cannot be encoded.
func TestFormat12EncodeBeyondUnicode(t *testing.T) {
	if _, err := (Format12{0x10_FFFF: 1}).Encode(0); err != nil {
		t.Errorf("U+10FFFF: %v", err)
	}
	for _, code := range []uint32{0x11_0000, 0xFFFF_FFFF} {
		if _, err := (Format12{'A': 1, code: 2}).Encode(0); err == nil {
			t.Errorf("0x%X: expected an error", code)
		}
	}
}

// makeFormat12 returns a format 12 subtable with the given segments.
// Each segment is given as startCharCode, endCharCode, startGlyphID.
func makeFormat12(segments ...[3]uint32) []byte {
	data := make([]byte, 16+12*len(segments))
	binary.BigEndian.PutUint16(data[0:], 12)
	binary.BigEndian.PutUint32(data[4:], uint32(len(data)))
	binary.BigEndian.PutUint32(data[12:], uint32(len(segments)))
	for i, seg := range segments {
		for j, v := range seg {
			binary.BigEndian.PutUint32(data[16+12*i+4*j:], v)
		}
	}
	return data
}

func FuzzFormat12(f *testing.F) {
	f.Add(mustEncode(f, Format12{}))
	f.Add(mustEncode(f, Format12{
		1: 1,
	}))
	f.Add(mustEncode(f, Format12{
		65:  1,
		66:  2,
		67:  3,
		100: 4,
	}))
	f.Add(mustEncode(f, Format12{
		1: 3,
		2: 2,
		3: 1,
	}))
	f.Add(makeFormat12(
		[3]uint32{'A', 'Z', 2},
		[3]uint32{0x10_FFFE, 0x11_0001, 30},
	))

	f.Fuzz(func(t *testing.T, data []byte) {
		_ = format12RoundTrip(t, data)
	})
}

// format12RoundTrip decodes data, encodes the result and checks that
// decoding this again gives the same subtable.
// The error from the first decode is returned.
func format12RoundTrip(t *testing.T, data []byte) error {
	t.Helper()

	c1, err := decodeFormat12(data, nil)
	if err != nil {
		return err
	}

	data2 := mustEncode(t, c1)
	if len(data2) > len(data) {
		t.Error("too long")
	}

	c2, err := decodeFormat12(data2, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(c1, c2) {
		t.Error("not equal")
	}
	return nil
}

var _ Subtable = Format12(nil)
