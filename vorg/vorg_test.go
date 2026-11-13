// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package vorg

import (
	"encoding/binary"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyph"
)

func TestRoundTrip(t *testing.T) {
	t1 := &Table{
		Default: 880,
		Origins: map[glyph.ID]funit.Int16{
			3:     -20,
			1:     900,
			65535: 0,
		},
	}
	data := t1.Encode()
	if len(data) != 8+3*4 {
		t.Errorf("wrong length %d", len(data))
	}
	t2, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(t1, t2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}

	// records are sorted by glyph ID
	for i, want := range []glyph.ID{1, 3, 65535} {
		got := glyph.ID(binary.BigEndian.Uint16(data[8+4*i:]))
		if got != want {
			t.Errorf("record %d: expected glyph %d, got %d", i, want, got)
		}
	}
}

func TestEmpty(t *testing.T) {
	t1 := &Table{Default: -5}
	data := t1.Encode()
	if len(data) != 8 {
		t.Errorf("wrong length %d", len(data))
	}
	t2, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(t1, t2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

func TestCanonical(t *testing.T) {
	// duplicate glyph 2 (first wins), glyph 4 equal to the default
	data := []byte{
		0, 1, 0, 0, 3, 0x70, 0, 4,
		0, 2, 0, 10,
		0, 2, 0, 20,
		0, 4, 3, 0x70,
		0, 5, 0, 30,
	}
	tab, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	want := &Table{
		Default: 0x370,
		Origins: map[glyph.ID]funit.Int16{2: 10, 5: 30},
	}
	if diff := cmp.Diff(want, tab); diff != "" {
		t.Errorf("wrong table (-want +got):\n%s", diff)
	}

	// an explicit entry equal to the default is not written
	tab = &Table{Default: 7, Origins: map[glyph.ID]funit.Int16{1: 7, 2: 8}}
	tab2, err := Decode(tab.Encode())
	if err != nil {
		t.Fatal(err)
	}
	want = &Table{Default: 7, Origins: map[glyph.ID]funit.Int16{2: 8}}
	if diff := cmp.Diff(want, tab2); diff != "" {
		t.Errorf("wrong table (-want +got):\n%s", diff)
	}
}

func TestDuplicateDefault(t *testing.T) {
	// the first record for glyph 5 equals the default and wins
	data := []byte{0, 1, 0, 0, 0, 3, 0, 2, 0, 5, 0, 3, 0, 5, 0, 7}
	tab, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(tab.Origins) != 0 || tab.Get(5) != 3 {
		t.Errorf("wrong table %+v", tab)
	}
}

func TestGet(t *testing.T) {
	tab := &Table{Default: 880, Origins: map[glyph.ID]funit.Int16{2: -3}}
	if y := tab.Get(2); y != -3 {
		t.Errorf("got %d, want -3", y)
	}
	if y := tab.Get(3); y != 880 {
		t.Errorf("got %d, want 880", y)
	}
	if y := (&Table{Default: 4}).Get(0); y != 4 {
		t.Errorf("got %d, want 4", y)
	}
}

func TestFromOrigins(t *testing.T) {
	tab := FromOrigins([]funit.Int16{880, 900, 880, 880, -5, 900})
	want := &Table{
		Default: 880,
		Origins: map[glyph.ID]funit.Int16{1: 900, 4: -5, 5: 900},
	}
	if diff := cmp.Diff(want, tab); diff != "" {
		t.Errorf("wrong table (-want +got):\n%s", diff)
	}

	// ties are broken by the smallest value
	for range 20 {
		tab = FromOrigins([]funit.Int16{5, 3, 9, 3, 5, 9, -2, -2, 7})
		if tab.Default != -2 {
			t.Fatalf("wrong default %d", tab.Default)
		}
		for gid, y := range []funit.Int16{5, 3, 9, 3, 5, 9, -2, -2, 7} {
			if tab.Get(glyph.ID(gid)) != y {
				t.Fatalf("glyph %d: wrong origin %d", gid, tab.Get(glyph.ID(gid)))
			}
		}
	}

	// empty input
	for _, in := range [][]funit.Int16{nil, {}} {
		tab = FromOrigins(in)
		if tab.Default != 0 || len(tab.Origins) != 0 {
			t.Errorf("wrong table for empty input: %+v", tab)
		}
		if len(tab.Encode()) != 8 {
			t.Error("wrong encoding for empty input")
		}
	}
}

func TestMalformed(t *testing.T) {
	good := (&Table{Default: 1, Origins: map[glyph.ID]funit.Int16{1: 2, 3: 4}}).Encode()
	if _, err := Decode(good); err != nil {
		t.Fatal(err)
	}

	// truncated data
	for n := range len(good) {
		if _, err := Decode(good[:n]); err == nil {
			t.Errorf("length %d: expected error", n)
		}
	}

	// wrong major version
	for _, major := range []uint16{0, 2, 0xffff} {
		data := append([]byte(nil), good...)
		binary.BigEndian.PutUint16(data, major)
		if _, err := Decode(data); err == nil {
			t.Errorf("major version %d: expected error", major)
		}
	}

	// count larger than the data
	data := append([]byte(nil), good...)
	binary.BigEndian.PutUint16(data[6:], 0xffff)
	if _, err := Decode(data); err == nil {
		t.Error("huge count: expected error")
	}

	// trailing data and a different minor version are tolerated
	data = append(append([]byte(nil), good...), 1, 2, 3)
	binary.BigEndian.PutUint16(data[2:], 7)
	if _, err := Decode(data); err != nil {
		t.Error(err)
	}
}

func FuzzVorg(f *testing.F) {
	f.Add((&Table{}).Encode())
	f.Add((&Table{
		Default: 880,
		Origins: map[glyph.ID]funit.Int16{1: 900, 7: -3},
	}).Encode())
	f.Add([]byte{0, 1, 0, 0, 0, 5, 0, 2, 0, 3, 0, 5, 0, 3, 0, 6})

	f.Fuzz(func(t *testing.T, data []byte) {
		t1, err := Decode(data)
		if err != nil {
			return
		}
		t2, err := Decode(t1.Encode())
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(t1, t2); diff != "" {
			t.Errorf("round trip failed (-first +second):\n%s", diff)
		}
	})
}
