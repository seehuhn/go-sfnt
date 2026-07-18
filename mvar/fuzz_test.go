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

package mvar

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/parser"
)

func FuzzMvar(f *testing.F) {
	if enc, err := (&Table{Store: goldenStore, Records: goldenRecords}).Encode(); err == nil {
		f.Add(enc)
	}
	if enc, err := (&Table{}).Encode(); err == nil {
		f.Add(enc)
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// budget proportional to the input bounds memory use
		t1, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		if err != nil {
			return
		}

		encoded, err := t1.Encode()
		if err != nil {
			t.Fatalf("encode failed: %v", err)
		}
		t2, err := Read(bytes.NewReader(encoded), parser.NewBudget(int64(len(data))))
		if err != nil {
			t.Fatalf("re-read failed: %v", err)
		}
		if diff := cmp.Diff(t1, t2); diff != "" {
			t.Errorf("round trip failed (-first +second):\n%s", diff)
		}
	})
}
