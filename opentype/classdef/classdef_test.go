// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package classdef

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
)

func FuzzClassDef(f *testing.F) {
	f.Add([]byte{0, 1, 0, 1, 0, 3, 0, 1, 0, 2, 0, 1})
	f.Add([]byte{0, 1, 0, 0, 0, 0})
	f.Add([]byte{0, 2, 0, 0})
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

		if !reflect.DeepEqual(info, info2) {
			fmt.Printf("A % x\n", data)
			fmt.Printf("B % x\n", data2)
			fmt.Println(info)
			fmt.Println(info2)
			t.Error("different")
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
	if !reflect.DeepEqual(info, info2) {
		t.Error("round trip failed")
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
