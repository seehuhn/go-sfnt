// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package hvar

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/variation"
)

func FuzzHvar(f *testing.F) {
	f.Add((&Table{Store: goldenStore}).Encode())
	f.Add((&Table{
		Store:      goldenStore,
		AdvanceMap: goldenAdvanceMap,
		LsbMap:     &variation.DeltaSetIndexMap{Map: []uint32{0, 0}},
		RsbMap:     &variation.DeltaSetIndexMap{Map: []uint32{1, 1}},
	}).Encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		// budget proportional to the input bounds memory use
		t1, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		if err != nil {
			return
		}

		encoded := t1.Encode()
		t2, err := Read(bytes.NewReader(encoded), parser.NewBudget(int64(len(data))))
		if err != nil {
			t.Fatalf("re-read failed: %v", err)
		}
		if diff := cmp.Diff(t1, t2); diff != "" {
			t.Errorf("round trip failed (-first +second):\n%s", diff)
		}
	})
}
