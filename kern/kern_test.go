// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package kern

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
)

func FuzzKern(f *testing.F) {
	kern := Info{}
	f.Add(kern.Encode())
	kern[glyph.Pair{Left: 0, Right: 0}] = 0
	f.Add(kern.Encode())
	kern[glyph.Pair{Left: 1, Right: 2}] = -10
	kern[glyph.Pair{Left: 2, Right: 2}] = 10
	kern[glyph.Pair{Left: 3, Right: 2}] = 100
	f.Add(kern.Encode())

	f.Fuzz(func(t *testing.T, data1 []byte) {
		info1, err := Read(bytes.NewReader(data1), parser.NewBudget(int64(len(data1))))
		if err != nil {
			return
		}

		data2 := info1.Encode()
		info2, err := Read(bytes.NewReader(data2), parser.NewBudget(int64(len(data2))))
		if err != nil {
			t.Fatal(err)
		}

		if d := cmp.Diff(info1, info2); d != "" {
			t.Errorf("kern mismatch (-want +got):\n%s", d)
		}
	})
}
