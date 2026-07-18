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

package hvar

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/variation"
)

func testBudget() *membudget.Budget {
	return membudget.New(1 << 30)
}

// goldenStore is a 1-axis item variation store with one region peaking at
// 1.0 and one data subtable holding two rows: inner index 0 -> delta 100,
// inner index 1 -> delta 200.
var goldenStore = &variation.ItemVariationStore{
	Regions: []variation.Region{
		{{Start: 0, Peak: 0x4000, End: 0x4000}},
	},
	Data: []*variation.ItemVariationData{
		{
			RegionIndexes: []uint16{0},
			Deltas:        [][]int32{{100}, {200}},
		},
	},
}

// goldenAdvanceMap sends glyph 0 to inner index 1 (delta 200) and glyph 1 to
// inner index 0 (delta 100); a lookup beyond its length clamps to the last
// entry.
var goldenAdvanceMap = &variation.DeltaSetIndexMap{
	Map: []uint32{1, 0},
}

// buildGolden hand-wraps goldenStore's own wire bytes in an HVAR header,
// exercising HVAR's header field layout independent of Table.Encode.
func buildGolden(t *testing.T) []byte {
	t.Helper()
	storeBytes := goldenStore.Encode()
	mapBytes := goldenAdvanceMap.Encode()

	storeOffset := headerSize
	advanceOffset := storeOffset + len(storeBytes)

	var data []byte
	data = appendU16(data, 1) // majorVersion
	data = appendU16(data, 0) // minorVersion
	data = appendU32(data, uint32(storeOffset))
	data = appendU32(data, uint32(advanceOffset))
	data = appendU32(data, 0) // lsbMappingOffset (absent)
	data = appendU32(data, 0) // rsbMappingOffset (absent)
	data = append(data, storeBytes...)
	data = append(data, mapBytes...)
	return data
}

func TestReadGolden(t *testing.T) {
	data := buildGolden(t)

	tab, err := Read(bytes.NewReader(data), testBudget())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if diff := cmp.Diff(goldenStore, tab.Store); diff != "" {
		t.Errorf("store mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(goldenAdvanceMap, tab.AdvanceMap); diff != "" {
		t.Errorf("advance map mismatch (-want +got):\n%s", diff)
	}
	if tab.LsbMap != nil {
		t.Errorf("expected nil LsbMap, got %v", tab.LsbMap)
	}
	if tab.RsbMap != nil {
		t.Errorf("expected nil RsbMap, got %v", tab.RsbMap)
	}
}

func TestReadMissingStoreErrors(t *testing.T) {
	data := make([]byte, headerSize)
	data[1] = 1 // majorVersion = 1
	// all offsets left at zero
	if _, err := Read(bytes.NewReader(data), testBudget()); err == nil {
		t.Error("expected error for zero store offset")
	}
}

func TestReadUnsupportedVersion(t *testing.T) {
	data := make([]byte, headerSize)
	data[1] = 2 // majorVersion = 2
	if _, err := Read(bytes.NewReader(data), testBudget()); err == nil {
		t.Error("expected error for unsupported version")
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	tab := &Table{
		Store:      goldenStore,
		AdvanceMap: goldenAdvanceMap,
		LsbMap:     &variation.DeltaSetIndexMap{Map: []uint32{0, 0}},
		RsbMap:     &variation.DeltaSetIndexMap{Map: []uint32{1, 1}},
	}
	enc := tab.Encode()
	dec, err := Read(bytes.NewReader(enc), testBudget())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if diff := cmp.Diff(tab, dec); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}

	enc2 := dec.Encode()
	if diff := cmp.Diff(enc, enc2); diff != "" {
		t.Errorf("encode not deterministic (-first +second):\n%s", diff)
	}
}

func TestAdvanceDeltaWithMap(t *testing.T) {
	tab := &Table{Store: goldenStore, AdvanceMap: goldenAdvanceMap}
	coords := []variation.F2Dot14{0x4000} // 1.0, peaks the only region exactly

	if got := tab.AdvanceDelta(0, coords); got != 200 {
		t.Errorf("gid 0: got %v, want 200", got)
	}
	if got := tab.AdvanceDelta(1, coords); got != 100 {
		t.Errorf("gid 1: got %v, want 100", got)
	}
}

func TestAdvanceDeltaWithoutMap(t *testing.T) {
	// implicit map: outer 0, inner = gid
	tab := &Table{Store: goldenStore}
	coords := []variation.F2Dot14{0x4000}

	if got := tab.AdvanceDelta(0, coords); got != 100 {
		t.Errorf("gid 0: got %v, want 100", got)
	}
	if got := tab.AdvanceDelta(1, coords); got != 200 {
		t.Errorf("gid 1: got %v, want 200", got)
	}
}

func TestAdvanceDeltaGidBeyondMapClamps(t *testing.T) {
	tab := &Table{Store: goldenStore, AdvanceMap: goldenAdvanceMap}
	coords := []variation.F2Dot14{0x4000}

	// index 1 is the last map entry (-> inner 0, delta 100); indices at or
	// beyond len(Map) must clamp to it
	want := tab.AdvanceDelta(1, coords)
	if got := tab.AdvanceDelta(glyph.ID(500), coords); got != want {
		t.Errorf("gid beyond map: got %v, want %v (clamped)", got, want)
	}
}

func TestAdvanceDeltaNilStore(t *testing.T) {
	tab := &Table{}
	if got := tab.AdvanceDelta(0, []variation.F2Dot14{0x4000}); got != 0 {
		t.Errorf("nil store: got %v, want 0", got)
	}
}

func TestEncodeAbsentMapsUseZeroOffset(t *testing.T) {
	tab := &Table{Store: goldenStore}
	enc := tab.Encode()
	if off := u32(enc, 8); off != 0 {
		t.Errorf("advance offset: got %d, want 0", off)
	}
	if off := u32(enc, 12); off != 0 {
		t.Errorf("lsb offset: got %d, want 0", off)
	}
	if off := u32(enc, 16); off != 0 {
		t.Errorf("rsb offset: got %d, want 0", off)
	}
}

func TestEncodeNilStoreWritesEmptyStore(t *testing.T) {
	tab := &Table{}
	enc := tab.Encode()
	if off := u32(enc, 4); off == 0 {
		t.Error("expected non-zero store offset even for a nil Store")
	}
	dec, err := Read(bytes.NewReader(enc), testBudget())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if dec.Store == nil {
		t.Error("expected a non-nil (empty) store after round trip")
	}
}
