// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package mapfilter

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestKeys(t *testing.T) {
	even := func(k int) bool { return k%2 == 0 }

	m := map[int]string{0: "a", 1: "b", 2: "c", 3: "d"}
	got, changed := Keys(m, even)
	if !changed {
		t.Error("removal not reported")
	}
	want := map[int]string{0: "a", 2: "c"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("wrong result (-want +got):\n%s", diff)
	}
	if len(m) != 4 {
		t.Error("argument modified")
	}

	all := map[int]string{0: "a", 2: "c"}
	got, changed = Keys(all, even)
	if changed {
		t.Error("removal reported for unchanged map")
	}
	got[4] = "e"
	if len(all) != 3 {
		t.Error("unchanged map not returned as is")
	}

	none := map[int]string{1: "b"}
	got, changed = Keys(none, even)
	if !changed || len(got) != 0 {
		t.Errorf("got %v, %t, want empty map, true", got, changed)
	}
}
