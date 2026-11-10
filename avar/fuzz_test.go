// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package avar

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/parser"
)

func FuzzAvar(f *testing.F) {
	f.Add((&Table{SegmentMaps: []SegmentMap{
		{{From: -16384, To: -16384}, {From: 0, To: 0}, {From: 16384, To: 16384}},
	}}).Encode())
	// version 2 raw seed
	f.Add([]byte{0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xDE, 0xAD})

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
