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

package mvar

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/membudget"
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

// goldenRecords is already sorted by tag.
var goldenRecords = []Record{
	{Tag: "hasc", OuterIndex: 0, InnerIndex: 0},
	{Tag: "xhgt", OuterIndex: 0, InnerIndex: 1},
}

func buildGolden(t *testing.T) []byte {
	t.Helper()
	storeBytes := goldenStore.Encode()
	storeOffset := headerSize + len(goldenRecords)*valueRecordSize

	var data []byte
	data = appendU16(data, 1) // majorVersion
	data = appendU16(data, 0) // minorVersion
	data = appendU16(data, 0) // reserved
	data = appendU16(data, valueRecordSize)
	data = appendU16(data, uint16(len(goldenRecords)))
	data = appendU16(data, uint16(storeOffset))
	for _, rec := range goldenRecords {
		data = appendTag(data, rec.Tag)
		data = appendU16(data, rec.OuterIndex)
		data = appendU16(data, rec.InnerIndex)
	}
	data = append(data, storeBytes...)
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
	if diff := cmp.Diff(goldenRecords, tab.Records); diff != "" {
		t.Errorf("records mismatch (-want +got):\n%s", diff)
	}
}

func TestReadUnsortedOversizeRecords(t *testing.T) {
	// records in reverse (unsorted) order, and a valueRecordSize of 10 with
	// 2 trailing padding bytes per record that must be skipped
	const oversizeRecordSize = 10
	unsorted := []Record{
		{Tag: "xhgt", OuterIndex: 0, InnerIndex: 1},
		{Tag: "hasc", OuterIndex: 0, InnerIndex: 0},
	}

	storeBytes := goldenStore.Encode()
	storeOffset := headerSize + len(unsorted)*oversizeRecordSize

	var data []byte
	data = appendU16(data, 1) // majorVersion
	data = appendU16(data, 0) // minorVersion
	data = appendU16(data, 0) // reserved
	data = appendU16(data, oversizeRecordSize)
	data = appendU16(data, uint16(len(unsorted)))
	data = appendU16(data, uint16(storeOffset))
	for _, rec := range unsorted {
		data = appendTag(data, rec.Tag)
		data = appendU16(data, rec.OuterIndex)
		data = appendU16(data, rec.InnerIndex)
		data = append(data, 0xAA, 0xBB) // padding, must be skipped
	}
	data = append(data, storeBytes...)

	tab, err := Read(bytes.NewReader(data), testBudget())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	// Read tolerates unsorted input but normalizes to tag order, matching
	// what Encode would produce
	if diff := cmp.Diff(goldenRecords, tab.Records); diff != "" {
		t.Errorf("records mismatch (-want +got):\n%s", diff)
	}
}

func TestReadDropsDuplicateTags(t *testing.T) {
	// a malformed table repeating a tag; Read must keep only the first
	// occurrence so the result stays writable by Encode
	dup := []Record{
		{Tag: "hasc", OuterIndex: 0, InnerIndex: 0},
		{Tag: "hasc", OuterIndex: 1, InnerIndex: 1},
	}

	var data []byte
	data = appendU16(data, 1) // majorVersion
	data = appendU16(data, 0) // minorVersion
	data = appendU16(data, 0) // reserved
	data = appendU16(data, valueRecordSize)
	data = appendU16(data, uint16(len(dup)))
	data = appendU16(data, 0) // no store
	for _, rec := range dup {
		data = appendTag(data, rec.Tag)
		data = appendU16(data, rec.OuterIndex)
		data = appendU16(data, rec.InnerIndex)
	}

	tab, err := Read(bytes.NewReader(data), testBudget())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := []Record{{Tag: "hasc", OuterIndex: 0, InnerIndex: 0}}
	if diff := cmp.Diff(want, tab.Records); diff != "" {
		t.Errorf("records mismatch (-want +got):\n%s", diff)
	}

	// the result must be writable
	if _, err := tab.Encode(); err != nil {
		t.Errorf("encode after dedup: %v", err)
	}
}

func TestReadNoRecordsNoStore(t *testing.T) {
	data := make([]byte, headerSize)
	data[1] = 1 // majorVersion = 1
	// reserved, valueRecordSize, valueRecordCount, storeOffset all zero

	tab, err := Read(bytes.NewReader(data), testBudget())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if tab.Store != nil {
		t.Errorf("expected nil store, got %v", tab.Store)
	}
	if len(tab.Records) != 0 {
		t.Errorf("expected no records, got %v", tab.Records)
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
	tab := &Table{Store: goldenStore, Records: goldenRecords}
	enc, err := tab.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	dec, err := Read(bytes.NewReader(enc), testBudget())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if diff := cmp.Diff(tab, dec); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}

	enc2, err := dec.Encode()
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	if diff := cmp.Diff(enc, enc2); diff != "" {
		t.Errorf("encode not deterministic (-first +second):\n%s", diff)
	}
}

func TestEncodeSortsRecords(t *testing.T) {
	tab := &Table{
		Store: goldenStore,
		Records: []Record{
			{Tag: "xhgt", OuterIndex: 0, InnerIndex: 1},
			{Tag: "hasc", OuterIndex: 0, InnerIndex: 0},
		},
	}
	enc, err := tab.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	dec, err := Read(bytes.NewReader(enc), testBudget())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if diff := cmp.Diff(goldenRecords, dec.Records); diff != "" {
		t.Errorf("expected sorted records (-want +got):\n%s", diff)
	}
}

func TestEncodeDuplicateTagErrors(t *testing.T) {
	tab := &Table{
		Store: goldenStore,
		Records: []Record{
			{Tag: "hasc", OuterIndex: 0, InnerIndex: 0},
			{Tag: "hasc", OuterIndex: 0, InnerIndex: 1},
		},
	}
	if _, err := tab.Encode(); err == nil {
		t.Error("expected error for duplicate tag")
	}
}

func TestEncodeBadTagLengthErrors(t *testing.T) {
	tab := &Table{
		Records: []Record{
			{Tag: "abc", OuterIndex: 0, InnerIndex: 0},
		},
	}
	if _, err := tab.Encode(); err == nil {
		t.Error("expected error for tag of wrong length")
	}
}

func TestEncodeTooManyRecordsErrors(t *testing.T) {
	// more records than fit in the uint16 record-count field; construction
	// is cheap since no store is involved
	records := make([]Record, math.MaxUint16+1)
	for i := range records {
		records[i] = Record{Tag: fmt.Sprintf("%04x", i)}
	}
	tab := &Table{Records: records}
	if _, err := tab.Encode(); err == nil {
		t.Error("expected error for too many records")
	}
}

func TestEncodeStoreOffsetOverflowErrors(t *testing.T) {
	// with a store present, the store offset equals headerSize plus the
	// records; more than 8191 records pushes that offset past uint16 range
	const n = 8200
	records := make([]Record, n)
	for i := range records {
		records[i] = Record{Tag: fmt.Sprintf("%04x", i)}
	}
	tab := &Table{Store: goldenStore, Records: records}
	if _, err := tab.Encode(); err == nil {
		t.Error("expected error for store offset overflow")
	}
}

func TestEncodeEmptyTable(t *testing.T) {
	tab := &Table{}
	enc, err := tab.Encode()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if off := u16(enc, 10); off != 0 {
		t.Errorf("store offset: got %d, want 0", off)
	}
	if count := u16(enc, 8); count != 0 {
		t.Errorf("record count: got %d, want 0", count)
	}
	dec, err := Read(bytes.NewReader(enc), testBudget())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if dec.Store != nil {
		t.Errorf("expected nil store, got %v", dec.Store)
	}
	if len(dec.Records) != 0 {
		t.Errorf("expected no records, got %v", dec.Records)
	}
}

func TestDeltaHit(t *testing.T) {
	tab := &Table{Store: goldenStore, Records: goldenRecords}
	coords := []variation.F2Dot14{0x4000} // 1.0, peaks the only region exactly

	got, ok := tab.Delta("hasc", coords)
	if !ok || got != 100 {
		t.Errorf("hasc: got (%v, %v), want (100, true)", got, ok)
	}
	got, ok = tab.Delta("xhgt", coords)
	if !ok || got != 200 {
		t.Errorf("xhgt: got (%v, %v), want (200, true)", got, ok)
	}
}

func TestDeltaMiss(t *testing.T) {
	tab := &Table{Store: goldenStore, Records: goldenRecords}
	coords := []variation.F2Dot14{0x4000}

	if got, ok := tab.Delta("zzzz", coords); ok || got != 0 {
		t.Errorf("unknown tag: got (%v, %v), want (0, false)", got, ok)
	}

	empty := &Table{}
	if got, ok := empty.Delta("hasc", coords); ok || got != 0 {
		t.Errorf("nil store: got (%v, %v), want (0, false)", got, ok)
	}
}
