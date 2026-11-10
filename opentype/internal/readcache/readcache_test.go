// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package readcache

import (
	"errors"
	"testing"
)

func TestGet(t *testing.T) {
	c := New[int]()
	calls := 0
	read := func(pos int64) (int, error) {
		calls++
		if pos < 0 {
			return 0, errors.New("negative")
		}
		return int(pos) * 10, nil
	}
	for range 2 {
		if v, err := c.Get(3, read); err != nil || v != 30 {
			t.Errorf("Get(3) = %d, %v; want 30, nil", v, err)
		}
	}
	if calls != 1 {
		t.Errorf("read called %d times, want 1", calls)
	}
	for range 2 {
		if _, err := c.Get(-1, read); err == nil {
			t.Error("error not returned")
		}
	}
	if calls != 3 {
		t.Errorf("read called %d times, want 3 (errors are not cached)", calls)
	}
}
