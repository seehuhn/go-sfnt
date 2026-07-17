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
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParsePackedDeltas(t *testing.T) {
	cases := []struct {
		name   string
		data   []byte
		needed int
		want   []int32
	}{
		{"zero run", []byte{0x84}, 5, []int32{0, 0, 0, 0, 0}},
		{"byte run", []byte{0x02, 0x0A, 0xFB, 0x03}, 3, []int32{10, -5, 3}},
		{"word run", []byte{0x41, 0x01, 0x00, 0xFF, 0xFF}, 2, []int32{256, -1}},
		// 0xC0 = DELTAS_ARE_LONGS per OT 1.9.1 (32-bit deltas).
		{"long run OT 1.9.1", []byte{0xC0, 0x00, 0x01, 0x00, 0x00}, 1, []int32{65536}},
		// over-supply: run declares 3 values, only 2 needed.
		{"over-supply tolerated", []byte{0x02, 0x01, 0x02, 0x03}, 2, []int32{1, 2}},
		// mixed runs: zero run, then byte run.
		{"mixed", []byte{0x82, 0x01, 0x07}, 4, []int32{0, 0, 0, 7}},
	}
	for _, c := range cases {
		r := &byteReader{data: c.data, pos: 0}
		got, err := parsePackedDeltas(r, c.needed, testBudget())
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if diff := cmp.Diff(c.want, got); diff != "" {
			t.Errorf("%s: mismatch (-want +got):\n%s", c.name, diff)
		}
	}
}

// TestParsePackedDeltasLongHistorical documents that a 0xC0 control byte is
// decoded as DELTAS_ARE_LONGS (OT 1.9.1), not as a zero-run-of-words (the
// older interpretation where DELTAS_ARE_ZERO dominated).
func TestParsePackedDeltasLongHistorical(t *testing.T) {
	data := []byte{0xC0, 0x00, 0x00, 0x00, 0x2A} // one 32-bit delta = 42
	r := &byteReader{data: data, pos: 0}
	got, err := parsePackedDeltas(r, 1, testBudget())
	if err != nil {
		t.Fatal(err)
	}
	// OT 1.9.1 behavior: value 42.
	if len(got) != 1 || got[0] != 42 {
		t.Errorf("OT 1.9.1: got %v, want [42]", got)
	}
	// old interpretation would have yielded a single zero and consumed no
	// data bytes; assert we did not do that.
	if r.pos != 5 {
		t.Errorf("expected 5 bytes consumed, got %d", r.pos)
	}
}

func TestParsePackedDeltasUnderSupply(t *testing.T) {
	// run declares 2 values but 3 are needed.
	r := &byteReader{data: []byte{0x01, 0x01, 0x02}, pos: 0}
	if _, err := parsePackedDeltas(r, 3, testBudget()); err == nil {
		t.Error("expected under-supply error")
	}
}

func TestPackedDeltasRoundTrip(t *testing.T) {
	cases := [][]int32{
		nil,
		{0, 0, 0, 0},
		{1, -1, 2, -2},
		{300, -300},
		{100000, -100000},
		{0, 5, 0, 0, 300, 100000, 0, 1},
	}
	for i, deltas := range cases {
		enc := encodePackedDeltas(deltas)
		r := &byteReader{data: enc, pos: 0}
		got, err := parsePackedDeltas(r, len(deltas), testBudget())
		if err != nil {
			t.Errorf("case %d: %v", i, err)
			continue
		}
		want := deltas
		if len(want) == 0 {
			want = nil
		}
		if diff := cmp.Diff(want, got); diff != "" {
			t.Errorf("case %d: mismatch (-want +got):\n%s", i, diff)
		}
	}
}
