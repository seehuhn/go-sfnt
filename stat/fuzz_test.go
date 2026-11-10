// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package stat

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/parser"
)

func FuzzStat(f *testing.F) {
	f.Add((&Table{}).Encode())
	f.Add(sampleTable().Encode())
	f.Add((&Table{
		DesignAxes: []DesignAxis{{Tag: "wght", NameID: 256}},
		AxisValues: []AxisValue{
			&Format1{AxisIndex: 0, NameID: 258, Value: 400},
		},
	}).Encode())
	f.Add((&Table{
		DesignAxes: []DesignAxis{{Tag: "wght", NameID: 256}},
		AxisValues: []AxisValue{
			&Format2{AxisIndex: 0, NameID: 259, Nominal: 400, Min: 100, Max: 900},
		},
	}).Encode())
	f.Add((&Table{
		DesignAxes: []DesignAxis{{Tag: "wght", NameID: 256}},
		AxisValues: []AxisValue{
			&Format3{AxisIndex: 0, Flags: 2, NameID: 260, Value: 700, LinkedValue: 900},
		},
	}).Encode())
	f.Add((&Table{
		DesignAxes: []DesignAxis{{Tag: "wght", NameID: 256}, {Tag: "wdth", NameID: 257}},
		AxisValues: []AxisValue{
			&Format4{NameID: 261, Values: []AxisValueEntry{
				{AxisIndex: 0, Value: 400},
				{AxisIndex: 1, Value: 100},
			}},
		},
	}).Encode())

	f.Fuzz(func(t *testing.T, data []byte) {
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
