// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package coverage

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/google/go-cmp/cmp"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
)

func FuzzCoverageTable(f *testing.F) {
	f.Add([]byte{0, 1, 0, 0})
	f.Add([]byte{0, 1, 0, 3, 1, 0, 1, 1, 1, 2})
	f.Add([]byte{0, 1, 0, 3, 0, 5, 0, 5, 0, 7}) // duplicate gid (format 1)
	f.Add([]byte{0, 2, 0, 0})
	f.Add([]byte{0, 2, 0, 1, 1, 0, 1, 2, 0, 0})
	f.Add([]byte{0, 2, 0, 2, 1, 0, 1, 2, 0, 0, 2, 0, 2, 5, 0, 3})
	f.Fuzz(func(t *testing.T, data1 []byte) {
		c1, err := Read(parser.New(bytes.NewReader(data1), parser.NewBudget(int64(len(data1)))), 0)
		if err != nil {
			return
		}

		data2 := c1.Encode()

		// The compact re-encoding may be smaller than data1, giving it a
		// smaller input-proportional budget; reuse data1's allowance so a
		// wide coverage range that fit the first read does not trip the budget.
		p := parser.New(bytes.NewReader(data2), parser.NewBudget(int64(len(data1))))
		c2, err := Read(p, 0)
		if err != nil {
			t.Fatal(err)
		}

		if len(data2) > len(data1) {
			t.Error("inefficient encoding")
		}

		if !reflect.DeepEqual(c1, c2) {
			fmt.Printf("A % x\n", data1)
			fmt.Printf("B % x\n", data2)
			fmt.Println(c1)
			fmt.Println(c2)
			t.Fatal("different")
		}
	})
}

func TestTableValid(t *testing.T) {
	cases := []struct {
		name  string
		table Table
		want  bool
	}{
		{"empty", Table{}, true},
		{"sequential", Table{3: 0, 7: 1, 8: 2}, true},
		{"glyph zero", Table{0: 0, 1: 1}, true},
		{"gap", Table{1: 0, 2: 5}, false},
		{"repeated index", Table{5: 1, 6: 1}, false},
		{"repeated first index", Table{5: 0, 6: 0}, false},
		{"wrong order", Table{1: 1, 2: 0}, false},
		{"negative index", Table{1: -1}, false},
	}
	for _, c := range cases {
		if got := c.table.Valid(); got != c.want {
			t.Errorf("%s: got %t, want %t", c.name, got, c.want)
		}
	}
}

func TestFilter(t *testing.T) {
	cov := Table{3: 0, 7: 1, 8: 2, 12: 3}
	data := []string{"a", "", "c", ""}
	calls := make(map[glyph.ID]int)
	newCov, newData, changed := Filter(cov, data, func(gid glyph.ID, item string) bool {
		calls[gid]++
		return item != ""
	})
	if !changed {
		t.Error("removal not reported")
	}
	if diff := cmp.Diff(Table{3: 0, 8: 1}, newCov); diff != "" {
		t.Errorf("wrong coverage (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"a", "c"}, newData); diff != "" {
		t.Errorf("wrong data (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(map[glyph.ID]int{3: 1, 7: 1, 8: 1, 12: 1}, calls); diff != "" {
		t.Errorf("wrong calls of keep (-want +got):\n%s", diff)
	}

	// the arguments are not modified
	if diff := cmp.Diff(Table{3: 0, 7: 1, 8: 2, 12: 3}, cov); diff != "" {
		t.Errorf("coverage modified (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"a", "", "c", ""}, data); diff != "" {
		t.Errorf("data modified (-want +got):\n%s", diff)
	}
}

func TestFilterNothingRemoved(t *testing.T) {
	cov := Table{3: 0, 7: 1}
	data := []string{"a", "b"}
	newCov, newData, changed := Filter(cov, data, func(glyph.ID, string) bool {
		return true
	})
	if changed {
		t.Error("removal reported")
	}
	if !sameMap(cov, newCov) {
		t.Error("coverage table not returned")
	}
	if len(newData) != len(data) || &newData[0] != &data[0] {
		t.Error("data not returned")
	}
}

func TestFilterAllRemoved(t *testing.T) {
	newCov, newData, changed := Filter(Table{3: 0, 7: 1}, []int{1, 2},
		func(glyph.ID, int) bool { return false })
	if !changed {
		t.Error("removal not reported")
	}
	if len(newCov) != 0 || len(newData) != 0 {
		t.Errorf("got %v and %v, want nothing", newCov, newData)
	}
}

func TestSetLimit(t *testing.T) {
	set := Set{1: true, 5: true, 10: true}

	same, changed := set.Limit(11)
	if changed || len(same) != 3 {
		t.Errorf("unexpected change: %v", same)
	}

	got, changed := set.Limit(6)
	if !changed {
		t.Error("change not reported")
	}
	if !reflect.DeepEqual(Set{1: true, 5: true}, got) {
		t.Errorf("wrong result: %v", got)
	}
	if len(set) != 3 {
		t.Error("receiver modified")
	}
}

func TestTableLimit(t *testing.T) {
	table := Table{1: 0, 5: 1, 10: 2}

	same, changed := table.Limit(11)
	if changed || len(same) != 3 {
		t.Errorf("unexpected change: %v", same)
	}

	got, changed := table.Limit(6)
	if !changed {
		t.Error("change not reported")
	}
	if diff := cmp.Diff(Table{1: 0, 5: 1}, got); diff != "" {
		t.Errorf("wrong result (-want +got):\n%s", diff)
	}
	if !got.Valid() {
		t.Error("result not valid")
	}
	if len(table) != 3 {
		t.Error("receiver modified")
	}
}
