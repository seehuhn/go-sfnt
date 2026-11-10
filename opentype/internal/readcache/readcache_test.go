// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2026  Jochen Voss <voss@seehuhn.de>
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

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
