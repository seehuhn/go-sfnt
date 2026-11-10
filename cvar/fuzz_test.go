// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package cvar

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/parser"
)

func FuzzCvar(f *testing.F) {
	if enc, err := (&Table{AxisCount: 1, Tuples: goldenTuples}).Encode(goldenCvtCount); err == nil {
		f.Add(enc)
	}
	if enc, err := (&Table{AxisCount: 0}).Encode(0); err == nil {
		f.Add(enc)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// budget proportional to the input bounds memory use
		t1, err := Decode(data, 3, 8, parser.NewBudget(int64(len(data))))
		if err != nil {
			return
		}

		encoded, err := t1.Encode(8)
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}
		t2, err := Decode(encoded, 3, 8, parser.NewBudget(int64(len(data))))
		if err != nil {
			t.Fatalf("re-decode failed: %v", err)
		}
		if diff := cmp.Diff(t1, t2); diff != "" {
			t.Errorf("round trip failed (-first +second):\n%s", diff)
		}
	})
}
