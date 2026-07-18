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

package cvar

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/variation"
)

func testBudget() *membudget.Budget {
	return membudget.New(1 << 30)
}

// goldenTuples describes a 1-axis cvar table: one tuple with an embedded
// peak and private points, and one tuple covering every CVT entry.
var goldenTuples = []variation.TupleVariation{
	{
		Peak:   []variation.F2Dot14{0x4000},
		Points: []uint16{0, 2},
		Deltas: []int32{10, -5},
	},
	{
		Peak:   []variation.F2Dot14{-0x4000},
		Deltas: []int32{1, 2, 3, 4},
	},
}

const goldenCvtCount = 4

// buildGolden assembles a cvar table by hand, appending the tuple variation
// block (produced via variation.EncodeTupleData) after a 4-byte cvar-specific
// header whose dataOffset is measured from the start of the cvar table.
func buildGolden(t *testing.T) []byte {
	t.Helper()

	block, err := variation.EncodeTupleData(goldenTuples, 1, 1, goldenCvtCount, nil)
	if err != nil {
		t.Fatalf("encode tuple data: %v", err)
	}
	// block already starts with [tupleVariationCount][dataOffset relative to
	// block start]; the cvar dataOffset field is relative to the table start,
	// i.e. 4 bytes further out.
	relOffset := int(block[2])<<8 | int(block[3])
	absOffset := relOffset + 4

	var data []byte
	data = appendU16(data, 1) // majorVersion
	data = appendU16(data, 0) // minorVersion
	data = append(data, block[0], block[1])
	data = appendU16(data, uint16(absOffset))
	data = append(data, block[4:]...)
	return data
}

func TestDecodeGolden(t *testing.T) {
	data := buildGolden(t)

	tab, err := Decode(data, 1, goldenCvtCount, testBudget())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if tab.AxisCount != 1 {
		t.Errorf("axis count: got %d, want 1", tab.AxisCount)
	}
	if diff := cmp.Diff(goldenTuples, tab.Tuples); diff != "" {
		t.Errorf("tuples mismatch (-want +got):\n%s", diff)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	tab := &Table{AxisCount: 1, Tuples: goldenTuples}
	enc, err := tab.Encode(goldenCvtCount)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	dec, err := Decode(enc, 1, goldenCvtCount, testBudget())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if diff := cmp.Diff(tab, dec); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}

	// a second cycle must be a fixed point
	enc2, err := dec.Encode(goldenCvtCount)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	if diff := cmp.Diff(enc, enc2); diff != "" {
		t.Errorf("encode not deterministic (-first +second):\n%s", diff)
	}
}

func TestEncodeRejectsMissingPeak(t *testing.T) {
	tab := &Table{
		AxisCount: 1,
		Tuples: []variation.TupleVariation{
			{SharedPeak: 0, Deltas: []int32{1, 2, 3, 4}},
		},
	}
	if _, err := tab.Encode(goldenCvtCount); err == nil {
		t.Error("expected error for tuple without embedded peak")
	}
}

func TestDecodeDropsMissingPeak(t *testing.T) {
	// hand-build a cvar table whose single tuple omits the embedded-peak
	// flag (tupleIndex = 0); per spec this is malformed for cvar (the flag
	// is mandatory there), so the decoder must drop the tuple rather than
	// fail the whole table.
	body := []byte{0x03, 0x0A, 0x14, 0x1E, 0x28} // packed-delta run of 4 byte values

	var header []byte
	header = appendU16(header, uint16(len(body))) // tuple size
	header = appendU16(header, 0)                 // tupleIndex: no flags set

	blockRelDataOffset := 4 + len(header) // tvc+dataOffset fields, then this header
	absDataOffset := blockRelDataOffset + 4

	var data []byte
	data = appendU16(data, 1) // majorVersion
	data = appendU16(data, 0) // minorVersion
	data = appendU16(data, 1) // tupleVariationCount = 1, no shared points flag
	data = appendU16(data, uint16(absDataOffset))
	data = append(data, header...)
	data = append(data, body...)

	tab, err := Decode(data, 1, goldenCvtCount, testBudget())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(tab.Tuples) != 0 {
		t.Errorf("expected tuple without peak to be dropped, got %d tuples", len(tab.Tuples))
	}
}

func TestDecodeErrors(t *testing.T) {
	// truncated header
	if _, err := Decode([]byte{0, 1, 0, 0}, 1, goldenCvtCount, testBudget()); err == nil {
		t.Error("expected error for short header")
	}

	// unsupported version
	bad := make([]byte, 8)
	bad[1] = 2 // majorVersion 2
	if _, err := Decode(bad, 1, goldenCvtCount, testBudget()); err == nil {
		t.Error("expected error for unsupported version")
	}

	// dataOffset out of range
	oob := make([]byte, 8)
	oob[0], oob[1] = 0, 1 // majorVersion 1
	oob[6], oob[7] = 0xFF, 0xFF
	if _, err := Decode(oob, 1, goldenCvtCount, testBudget()); err == nil {
		t.Error("expected error for out-of-range dataOffset")
	}
}

func TestApplyVectors(t *testing.T) {
	tab := &Table{AxisCount: 1, Tuples: goldenTuples}

	cvt := []byte{
		0x00, 0x64, // cvt[0] = 100
		0x00, 0xC8, // cvt[1] = 200
		0xFF, 0x9C, // cvt[2] = -100
		0x01, 0x2C, // cvt[3] = 300
	}

	// at coords = [1.0] (F2Dot14 0x4000): tuple 0 peaks exactly (scalar 1),
	// tuple 1 (peak -1.0) does not contribute (scalar 0, coord on other side)
	got := tab.Apply(cvt, []variation.F2Dot14{0x4000})
	want := []byte{
		0x00, 0x6E, // 100+10 = 110
		0x00, 0xC8, // unchanged
		0xFF, 0x97, // -100-5 = -105
		0x01, 0x2C, // unchanged
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("apply at coords=[1.0] (-want +got):\n%s", diff)
	}

	// at coords = [-1.0] (F2Dot14 -0x4000): tuple 0 (peak 1.0) is zero,
	// tuple 1 (peak -1.0) peaks exactly, applying deltas [1,2,3,4]
	got2 := tab.Apply(cvt, []variation.F2Dot14{-0x4000})
	want2 := []byte{
		0x00, 0x65, // 100+1
		0x00, 0xCA, // 200+2
		0xFF, 0x9F, // -100+3
		0x01, 0x30, // 300+4
	}
	if diff := cmp.Diff(want2, got2); diff != "" {
		t.Errorf("apply at coords=[-1.0] (-want +got):\n%s", diff)
	}
}

func TestApplyEmptyCvt(t *testing.T) {
	tab := &Table{AxisCount: 1, Tuples: goldenTuples}
	got := tab.Apply(nil, []variation.F2Dot14{0x4000})
	if len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
}

func TestApplyOddLengthCvt(t *testing.T) {
	tab := &Table{AxisCount: 1, Tuples: goldenTuples}
	cvt := []byte{0x00, 0x64, 0xAB} // one entry plus a trailing odd byte
	got := tab.Apply(cvt, []variation.F2Dot14{0x4000})
	if len(got) != 3 {
		t.Fatalf("expected 3 bytes, got %d", len(got))
	}
	if got[2] != 0xAB {
		t.Errorf("trailing odd byte not preserved: got %#x, want 0xab", got[2])
	}
	if want := byte(0x00); got[0] != want {
		t.Errorf("cvt[0] high byte: got %#x, want %#x", got[0], want)
	}
}

func TestApplyDropsOutOfRangeIndices(t *testing.T) {
	// tuple references a CVT index beyond the actual cvt table length
	tab := &Table{
		AxisCount: 1,
		Tuples: []variation.TupleVariation{
			{
				Peak:   []variation.F2Dot14{0x4000},
				Points: []uint16{0, 5}, // index 5 is out of range for a 1-entry cvt
				Deltas: []int32{10, 99},
			},
		},
	}
	cvt := []byte{0x00, 0x64} // single entry
	got := tab.Apply(cvt, []variation.F2Dot14{0x4000})
	want := []byte{0x00, 0x6E} // only index 0 applied
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("out-of-range index handling (-want +got):\n%s", diff)
	}
}
