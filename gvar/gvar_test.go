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

package gvar

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/variation"
)

func testBudget() *membudget.Budget {
	return membudget.New(1 << 30)
}

// sharedGolden is the shared-tuple array used by the golden table:
// one tuple peaking on the first axis.
var sharedGolden = [][]variation.F2Dot14{
	{0x4000, 0},
}

// glyph0Tuples describes glyph 0 of the golden table: one tuple referencing
// shared tuple 0 with private points, and one tuple with an embedded peak
// covering all points.
var glyph0Tuples = []variation.TupleVariation{
	{
		SharedPeak: 0,
		Points:     []uint16{0, 2},
		Deltas:     []int32{10, -5, 3, 7},
	},
	{
		Peak:   []variation.F2Dot14{-0x4000, 0x2000},
		Deltas: []int32{1, 2, 3, 4, 5, -1, -2, -3, -4, -5},
	},
}

const glyph0Points = 5 // 1 real + 4 phantom, arbitrary but fixed for the test

// buildGolden assembles a 2-axis, 3-glyph gvar table by hand.  Glyph 0 holds
// glyph0Tuples, glyph 1 is empty, and glyph 2 is an opaque odd-length block.
func buildGolden(t *testing.T) (data []byte, block0 []byte) {
	t.Helper()

	block0, err := variation.EncodeTupleData(glyph0Tuples, 2, 2, glyph0Points, sharedGolden)
	if err != nil {
		t.Fatalf("encode tuple data: %v", err)
	}
	block2 := []byte{0xAB, 0xCD, 0xEF} // odd length, opaque

	blocks := [][]byte{block0, nil, block2}

	// cumulative offsets, exact block lengths (matches the codec layout)
	offsets := []int{0}
	for _, b := range blocks {
		offsets = append(offsets, offsets[len(offsets)-1]+len(b))
	}
	// block2 has odd length, so at least one offset is odd -> long offsets
	long := false
	for _, off := range offsets {
		if off%2 != 0 {
			long = true
		}
	}
	if !long {
		t.Fatal("expected golden table to need long offsets")
	}

	sharedLen := len(sharedGolden) * 2 * 2
	sharedTuplesOffset := headerSize + len(offsets)*4
	dataArrayOffset := sharedTuplesOffset + sharedLen

	data = nil
	data = appendU16(data, 1) // majorVersion
	data = appendU16(data, 0) // minorVersion
	data = appendU16(data, 2) // axisCount
	data = appendU16(data, uint16(len(sharedGolden)))
	data = appendU32(data, uint32(sharedTuplesOffset))
	data = appendU16(data, uint16(len(blocks)))
	data = appendU16(data, longOffsetsFlag)
	data = appendU32(data, uint32(dataArrayOffset))
	for _, off := range offsets {
		data = appendU32(data, uint32(off))
	}
	for _, tup := range sharedGolden {
		for _, v := range tup {
			data = appendU16(data, uint16(v))
		}
	}
	for _, b := range blocks {
		data = append(data, b...)
	}
	return data, block0
}

func TestDecodeGolden(t *testing.T) {
	data, block0 := buildGolden(t)

	tab, err := Decode(data, testBudget())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	if tab.AxisCount != 2 {
		t.Errorf("axis count: got %d, want 2", tab.AxisCount)
	}
	if diff := cmp.Diff(sharedGolden, tab.SharedTuples); diff != "" {
		t.Errorf("shared tuples mismatch (-want +got):\n%s", diff)
	}
	if len(tab.PerGlyph) != 3 {
		t.Fatalf("glyph count: got %d, want 3", len(tab.PerGlyph))
	}
	if diff := cmp.Diff(block0, tab.PerGlyph[0].Data); diff != "" {
		t.Errorf("glyph 0 block mismatch (-want +got):\n%s", diff)
	}
	if tab.PerGlyph[1].Data != nil {
		t.Errorf("glyph 1 should be empty, got %v", tab.PerGlyph[1].Data)
	}
	if diff := cmp.Diff([]byte{0xAB, 0xCD, 0xEF}, tab.PerGlyph[2].Data); diff != "" {
		t.Errorf("glyph 2 block mismatch (-want +got):\n%s", diff)
	}
}

func TestUnpackGolden(t *testing.T) {
	data, _ := buildGolden(t)
	tab, err := Decode(data, testBudget())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	got, err := tab.Unpack(0, glyph0Points, testBudget())
	if err != nil {
		t.Fatalf("unpack glyph 0: %v", err)
	}
	if diff := cmp.Diff(glyph0Tuples, got); diff != "" {
		t.Errorf("glyph 0 tuples mismatch (-want +got):\n%s", diff)
	}

	// empty glyph yields nil, no error
	empty, err := tab.Unpack(1, glyph0Points, testBudget())
	if err != nil {
		t.Fatalf("unpack glyph 1: %v", err)
	}
	if empty != nil {
		t.Errorf("glyph 1 should unpack to nil, got %v", empty)
	}

	// out-of-range gid errors
	if _, err := tab.Unpack(3, glyph0Points, testBudget()); err == nil {
		t.Error("expected error for out-of-range gid")
	}
}

func TestEncodeShortOffsets(t *testing.T) {
	// all blocks even length -> short offsets
	tab := &Table{
		AxisCount:    2,
		SharedTuples: sharedGolden,
		PerGlyph: []GlyphData{
			{Data: []byte{0x00, 0x00, 0x00, 0x02}},
			{},
			{Data: []byte{0x00, 0x00}},
		},
	}

	enc, err := tab.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if flags := u16(enc, 14); flags&longOffsetsFlag != 0 {
		t.Errorf("expected short offsets, flags = %#x", flags)
	}

	dec, err := Decode(enc, testBudget())
	if err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	if diff := cmp.Diff(tab, dec); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

func TestEncodeLongOffsets(t *testing.T) {
	// an odd-length block forces long offsets
	tab := &Table{
		AxisCount: 1,
		PerGlyph: []GlyphData{
			{Data: []byte{0x01, 0x02, 0x03}}, // odd
			{Data: []byte{0x04, 0x05}},
		},
	}

	enc, err := tab.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if flags := u16(enc, 14); flags&longOffsetsFlag == 0 {
		t.Errorf("expected long offsets, flags = %#x", flags)
	}

	dec, err := Decode(enc, testBudget())
	if err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	if diff := cmp.Diff(tab, dec); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// TestOddBlockAlignment checks that an odd-length block followed by another
// block survives a decode/encode/decode cycle with its exact bytes and
// length, i.e. no padding leaks into an offset range.
func TestOddBlockAlignment(t *testing.T) {
	tab := &Table{
		AxisCount: 0,
		PerGlyph: []GlyphData{
			{Data: []byte{0xDE, 0xAD, 0xBE}}, // 3 bytes, odd
			{Data: []byte{0xEF, 0x01}},       // follows the odd block
		},
	}

	enc, err := tab.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	dec, err := Decode(enc, testBudget())
	if err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	if len(dec.PerGlyph[0].Data) != 3 {
		t.Errorf("odd block gained padding: got %d bytes, want 3", len(dec.PerGlyph[0].Data))
	}
	if diff := cmp.Diff(tab, dec); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}

	// a second full cycle must be a fixed point
	enc2, err := dec.Encode()
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	if diff := cmp.Diff(enc, enc2); diff != "" {
		t.Errorf("encode not deterministic (-first +second):\n%s", diff)
	}
}

func TestDecodeErrors(t *testing.T) {
	// truncated header
	if _, err := Decode([]byte{0, 1, 0, 0}, testBudget()); err == nil {
		t.Error("expected error for short header")
	}

	// unsupported version
	bad := make([]byte, headerSize)
	bad[0] = 0x00
	bad[1] = 0x02 // majorVersion 2
	if _, err := Decode(bad, testBudget()); err == nil {
		t.Error("expected error for unsupported version")
	}
}

func TestEncodeWrongTupleSize(t *testing.T) {
	tab := &Table{
		AxisCount: 2,
		SharedTuples: [][]variation.F2Dot14{
			{0x4000}, // wrong: only 1 value, should be 2
		},
		PerGlyph: []GlyphData{
			{Data: []byte{0x00, 0x00}},
		},
	}

	_, err := tab.Encode()
	if err == nil {
		t.Error("expected error for wrong tuple size")
	}
}
