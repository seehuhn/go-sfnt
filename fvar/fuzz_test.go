// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package fvar

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/parser"
)

func FuzzFvar(f *testing.F) {
	f.Add((&Table{
		Axes:      []Axis{{Tag: "wght", Min: 100, Default: 400, Max: 900, NameID: 256}},
		Instances: []Instance{{NameID: 258, PostScriptNameID: 0xFFFF, Coordinates: []float64{400}}},
	}).Encode())
	f.Add((&Table{
		Axes: []Axis{{Tag: "wght", Min: 100, Default: 400, Max: 900, NameID: 256}},
		Instances: []Instance{
			{NameID: 258, PostScriptNameID: 300, Coordinates: []float64{700}},
		},
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
