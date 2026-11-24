// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package classdef

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
)

func FuzzClassDef(f *testing.F) {
	f.Add([]byte{0, 1, 0, 1, 0, 3, 0, 1, 0, 2, 0, 1})
	f.Add([]byte{0, 1, 0, 0, 0, 0})
	f.Add([]byte{0, 2, 0, 0})
	f.Add([]byte{0, 2, 0, 2, 0, 10, 0, 5, 0, 1, 0, 0, 0, 3, 0, 2})
	f.Add([]byte{0, 2, 0, 2, 0, 8, 0, 9, 0, 1, 0, 0, 0, 3, 0, 2})
	f.Add([]byte{0, 2, 0, 2, 0, 0, 0, 5, 0, 1, 0, 3, 0, 7, 0, 3})
	f.Fuzz(func(t *testing.T, data []byte) {
		info, err := Read(parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data)))), 0)
		if err != nil {
			return
		}

		data2, err := info.Append(nil)
		if err != nil {
			t.Fatal(err)
		}

		// The compact re-encoding may be smaller than data, giving it a
		// smaller input-proportional budget; reuse data's allowance so a
		// wide class range that fit the first read does not trip the budget.
		p := parser.New(bytes.NewReader(data2), parser.NewBudget(int64(len(data))))
		info2, err := Read(p, 0)
		if err != nil {
			fmt.Printf("A % x\n", data)
			fmt.Printf("B % x\n", data2)
			fmt.Println(info)
			t.Fatal(err)
		}

		if len(data2) > len(data) {
			fmt.Printf("A % x\n", data)
			fmt.Printf("B % x\n", data2)
			fmt.Println(info)
			fmt.Println(info2)
			t.Error("encode is longer than original")
		}

		if len(data2) != info.AppendLen() {
			t.Errorf("wrong length, expected %d, got %d", info.AppendLen(), len(data2))
		}

		if diff := cmp.Diff(info, info2); diff != "" {
			t.Errorf("round trip failed (-want +got):\n%s", diff)
		}
	})
}

// TestAppendAllGlyphs checks that a table which covers all glyph IDs is
// written in a form which reads back unchanged.
func TestAppendAllGlyphs(t *testing.T) {
	info := Table{}
	for i := range 0x10000 {
		info[glyph.ID(i)] = uint16(i/1000%2 + 1)
	}
	data, err := info.Append(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != info.AppendLen() {
		t.Errorf("wrong length, expected %d, got %d", info.AppendLen(), len(data))
	}
	p := parser.New(bytes.NewReader(data), parser.NewBudget(1<<20))
	info2, err := Read(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(info, info2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// TestAppendTooLarge checks that a table which neither format can represent
// is refused.
func TestAppendTooLarge(t *testing.T) {
	info := Table{}
	for i := range 0x10000 {
		info[glyph.ID(i)] = uint16(i%2 + 1)
	}
	if _, err := info.Append(nil); err == nil {
		t.Error("encoding succeeded")
	}
}

// TestReadMalformed checks that malformed tables can be read, and that the
// result survives a read-write-read cycle.
func TestReadMalformed(t *testing.T) {
	cases := []struct {
		name string
		data []byte
	}{
		{"inverted range", []byte{
			0, 2, // format
			0, 1, // classRangeCount
			0, 10, 0, 5, 0, 1,
		}},
		{"inverted and valid ranges", []byte{
			0, 2, // format
			0, 3, // classRangeCount
			0, 10, 0, 5, 0, 1,
			0, 0, 0, 3, 0, 2,
			0, 7, 0, 6, 0, 3,
		}},
		{"unsorted ranges", []byte{
			0, 2, // format
			0, 3, // classRangeCount
			0, 8, 0, 9, 0, 1,
			0, 0, 0, 3, 0, 2,
			0, 4, 0, 7, 0, 0,
		}},
		{"format 1 past last glyph", []byte{
			0, 1, // format
			0xFF, 0xFE, // startGlyphID
			0, 3, // glyphCount
			0, 1, 0, 2, 0, 3,
		}},
		{"format 1 truncated past last glyph", []byte{
			0, 1, // format
			0xFF, 0xFE, // startGlyphID
			0, 3, // glyphCount
			0, 1, 0, 2,
		}},
		{"overlap with equal classes", []byte{
			0, 2, // format
			0, 2, // classRangeCount
			0, 0, 0, 5, 0, 1,
			0, 3, 0, 7, 0, 1,
		}},
		{"overlap with equal class 0", []byte{
			0, 2, // format
			0, 3, // classRangeCount
			0, 0, 0, 5, 0, 0,
			0, 3, 0, 7, 0, 0,
			0, 8, 0, 9, 0, 1,
		}},
		{"conflicting overlap", []byte{
			0, 2, // format
			0, 2, // classRangeCount
			0, 0, 0, 5, 0, 1,
			0, 3, 0, 7, 0, 3,
		}},
		{"conflicting overlap, unsorted", []byte{
			0, 2, // format
			0, 2, // classRangeCount
			0, 3, 0, 7, 0, 3,
			0, 0, 0, 5, 0, 1,
		}},
		{"conflicting overlap across inverted range", []byte{
			0, 2, // format
			0, 3, // classRangeCount
			0, 0, 0, 5, 0, 1,
			0, 9, 0, 8, 0, 2, // inverted range
			0, 3, 0, 7, 0, 3,
		}},
		{"conflict, class 0 after non-zero", []byte{
			0, 2, // format
			0, 2, // classRangeCount
			0, 0, 0, 5, 0, 1,
			0, 3, 0, 7, 0, 0,
		}},
		{"conflict, non-zero after class 0", []byte{
			0, 2, // format
			0, 2, // classRangeCount
			0, 3, 0, 7, 0, 0,
			0, 0, 0, 5, 0, 1,
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info, err := Read(parser.New(bytes.NewReader(c.data), parser.NewBudget(1<<20)), 0)
			if err != nil {
				t.Fatal(err)
			}

			data2, err := info.Append(nil)
			if err != nil {
				t.Fatal(err)
			}
			info2, err := Read(parser.New(bytes.NewReader(data2), parser.NewBudget(1<<20)), 0)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(info, info2); diff != "" {
				t.Errorf("round trip failed (-want +got):\n%s", diff)
			}
		})
	}
}

// TestReadClassZeroBudget checks that an explicit class 0 range covering all
// glyphs reads with a budget much smaller than the map it would need.
func TestReadClassZeroBudget(t *testing.T) {
	data := []byte{
		0, 2, // format
		0, 1, // classRangeCount
		0, 0, 0xFF, 0xFF, 0, 0,
	}
	p := parser.New(bytes.NewReader(data), membudget.New(64<<10))
	info, err := Read(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(info) != 0 {
		t.Errorf("expected empty table, got %d entries", len(info))
	}
}

func TestTableLimit(t *testing.T) {
	table := Table{1: 1, 5: 2, 10: 3}

	same, changed := table.Limit(11)
	if changed || len(same) != 3 {
		t.Errorf("unexpected change: %v", same)
	}

	got, changed := table.Limit(6)
	if !changed {
		t.Error("change not reported")
	}
	if diff := cmp.Diff(Table{1: 1, 5: 2}, got); diff != "" {
		t.Errorf("wrong result (-want +got):\n%s", diff)
	}
	if len(table) != 3 {
		t.Error("receiver modified")
	}
}
