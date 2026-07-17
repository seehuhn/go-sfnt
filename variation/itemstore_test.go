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

func decodeStore(t *testing.T, data []byte) *ItemVariationStore {
	t.Helper()
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	s, err := ReadItemVariationStore(p, 0)
	if err != nil {
		t.Fatalf("ReadItemVariationStore: %v", err)
	}
	return s
}

// handStore is a byte-for-byte ItemVariationStore (format 1) with two
// single-axis regions and one ItemVariationData with a word+byte split.
var handStore = []byte{
	0x00, 0x01, // format
	0x00, 0x00, 0x00, 0x0C, // variationRegionListOffset = 12
	0x00, 0x01, // itemVariationDataCount = 1
	0x00, 0x00, 0x00, 0x1C, // itemVariationDataOffsets[0] = 28

	// region list at offset 12
	0x00, 0x01, // axisCount = 1
	0x00, 0x02, // regionCount = 2
	0x00, 0x00, 0x20, 0x00, 0x40, 0x00, // region0: start 0, peak 0.5, end 1.0
	0x20, 0x00, 0x40, 0x00, 0x40, 0x00, // region1: start 0.5, peak 1.0, end 1.0

	// ItemVariationData at offset 28
	0x00, 0x02, // itemCount = 2
	0x00, 0x01, // wordDeltaCount = 1 (first column is a word)
	0x00, 0x02, // regionIndexCount = 2
	0x00, 0x01, 0x00, 0x00, // regionIndexes = [1, 0]
	0x00, 0xC8, 0x0A, // row0: 200 (int16), 10 (int8)
	0xFE, 0xD4, 0xEC, // row1: -300 (int16), -20 (int8)
}

func TestReadItemVariationStoreHand(t *testing.T) {
	s := decodeStore(t, handStore)

	want := &ItemVariationStore{
		Regions: []Region{
			{{Start: f2(0), Peak: f2(0.5), End: f2(1.0)}},
			{{Start: f2(0.5), Peak: f2(1.0), End: f2(1.0)}},
		},
		Data: []*ItemVariationData{
			{
				RegionIndexes: []uint16{1, 0},
				Deltas:        [][]int32{{200, 10}, {-300, -20}},
			},
		},
	}
	if diff := cmp.Diff(want, s); diff != "" {
		t.Errorf("store mismatch (-want +got):\n%s", diff)
	}
}

func TestItemVariationStoreEvaluate(t *testing.T) {
	s := decodeStore(t, handStore)

	cases := []struct {
		name         string
		outer, inner uint16
		coords       []F2Dot14
		want         float64
	}{
		{"peak of r0", 0, 0, []F2Dot14{f2(0.5)}, 10},
		{"peak of r1", 0, 0, []F2Dot14{f2(1.0)}, 200},
		{"midway", 0, 0, []F2Dot14{f2(0.75)}, 105},
		{"row1 peak r1", 0, 1, []F2Dot14{f2(1.0)}, -300},
		{"outer out of range", 5, 0, []F2Dot14{f2(1.0)}, 0},
		{"inner out of range", 0, 9, []F2Dot14{f2(1.0)}, 0},
	}
	for _, c := range cases {
		got := s.Evaluate(c.outer, c.inner, c.coords)
		if got != c.want {
			t.Errorf("%s: Evaluate = %v, want %v", c.name, got, c.want)
		}
	}
}

// handStoreLong is a byte-for-byte LONG_WORDS ItemVariationStore.
var handStoreLong = []byte{
	0x00, 0x01, // format
	0x00, 0x00, 0x00, 0x0C, // variationRegionListOffset = 12
	0x00, 0x01, // itemVariationDataCount = 1
	0x00, 0x00, 0x00, 0x1C, // itemVariationDataOffsets[0] = 28

	// region list at offset 12
	0x00, 0x01, // axisCount = 1
	0x00, 0x02, // regionCount = 2
	0x00, 0x00, 0x20, 0x00, 0x40, 0x00, // region0
	0x20, 0x00, 0x40, 0x00, 0x40, 0x00, // region1

	// ItemVariationData at offset 28
	0x00, 0x02, // itemCount = 2
	0x80, 0x01, // wordDeltaCount = 0x8001: LONG_WORDS + 1 long column
	0x00, 0x02, // regionIndexCount = 2
	0x00, 0x00, 0x00, 0x01, // regionIndexes = [0, 1]
	0x00, 0x01, 0x86, 0xA0, 0x00, 0x05, // row0: 100000 (int32), 5 (int16)
	0xFF, 0xFE, 0xEE, 0x90, 0xFF, 0xFB, // row1: -70000 (int32), -5 (int16)
}

func TestReadItemVariationStoreLong(t *testing.T) {
	s := decodeStore(t, handStoreLong)
	want := &ItemVariationStore{
		Regions: []Region{
			{{Start: f2(0), Peak: f2(0.5), End: f2(1.0)}},
			{{Start: f2(0.5), Peak: f2(1.0), End: f2(1.0)}},
		},
		Data: []*ItemVariationData{
			{
				RegionIndexes: []uint16{0, 1},
				Deltas:        [][]int32{{100000, 5}, {-70000, -5}},
			},
		},
	}
	if diff := cmp.Diff(want, s); diff != "" {
		t.Errorf("long store mismatch (-want +got):\n%s", diff)
	}
}

func TestItemVariationStoreRoundTrip(t *testing.T) {
	stores := []*ItemVariationStore{
		{}, // empty
		{
			Regions: []Region{{{f2(0), f2(0.5), f2(1)}}},
			Data: []*ItemVariationData{
				{RegionIndexes: []uint16{0}, Deltas: [][]int32{{1}, {2}, {3}}},
			},
		},
		{
			Regions: []Region{
				{{f2(0), f2(0.5), f2(1)}, {f2(-1), f2(-0.5), f2(0)}},
				{{f2(0.5), f2(1), f2(1)}, {f2(0), f2(0), f2(0)}},
			},
			Data: []*ItemVariationData{
				{RegionIndexes: []uint16{0, 1}, Deltas: [][]int32{{200, -3}, {-300, 4}}},
				{RegionIndexes: []uint16{1, 0}, Deltas: [][]int32{{100000, -5}, {-70000, 6}}},
				{RegionIndexes: []uint16{0}, Deltas: [][]int32{{0}, {0}}},
			},
		},
	}
	for i, s := range stores {
		encoded := s.Encode()
		if len(encoded) != s.EncodeLen() {
			t.Errorf("store %d: EncodeLen = %d, len(Encode) = %d", i, s.EncodeLen(), len(encoded))
		}
		got := decodeStore(t, encoded)
		if diff := cmp.Diff(s, got); diff != "" {
			t.Errorf("store %d: round trip mismatch (-want +got):\n%s", i, diff)
		}
		// encoding is deterministic
		if !bytes.Equal(encoded, s.Encode()) {
			t.Errorf("store %d: Encode not deterministic", i)
		}
	}
}

func FuzzItemVariationStore(f *testing.F) {
	f.Add(handStore)
	f.Add(handStoreLong)
	f.Fuzz(func(t *testing.T, data []byte) {
		p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		s, err := ReadItemVariationStore(p, 0)
		if err != nil {
			return
		}
		encoded := s.Encode()
		if len(encoded) != s.EncodeLen() {
			t.Fatalf("EncodeLen = %d, len(Encode) = %d", s.EncodeLen(), len(encoded))
		}
		p2 := parser.New(bytes.NewReader(encoded), parser.NewBudget(int64(len(encoded))))
		s2, err := ReadItemVariationStore(p2, 0)
		if err != nil {
			t.Fatalf("re-decode failed: %v", err)
		}
		if diff := cmp.Diff(s, s2); diff != "" {
			t.Fatalf("round trip mismatch (-want +got):\n%s", diff)
		}
	})
}
