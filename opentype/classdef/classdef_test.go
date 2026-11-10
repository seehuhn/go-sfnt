// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package classdef

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

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

		data2 := info.Append(nil)

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
