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

package sfnt_test

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/image/font/gofont/goregular"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/fvar"
	"seehuhn.de/go/sfnt/parser"
)

// TestCanonicalizeVariationNamesPreservedIDCollision reconstructs a
// malformed font where the "fvar" axis at index 0 stores name ID 256 but
// the "name" table has no entry for that ID (so the resolved Name is
// empty).  Before the fix, the allocator handed out IDs from 256 upward
// without checking preserved numeric IDs, so the axis "Width" collided
// with the preserved ID 256; on the next read the empty entry's Name
// flipped to "Width", breaking the read-write-read invariant.
func TestCanonicalizeVariationNamesPreservedIDCollision(t *testing.T) {
	f, err := sfnt.Read(bytes.NewReader(goregular.TTF), parser.NewBudget(int64(len(goregular.TTF))))
	if err != nil {
		t.Fatal(err)
	}

	f.Fvar = &fvar.Table{
		Axes: []fvar.Axis{
			// axis0: NameID 256 with no corresponding "name" table entry,
			// as decoded from a malformed font; the resolved Name is empty.
			{Tag: "wght", Min: 100, Default: 400, Max: 900, NameID: 256, Name: ""},
			// axis1: a resolved name that must not be allocated ID 256.
			{Tag: "wdth", Min: 75, Default: 100, Max: 125, Name: "Width"},
		},
	}

	buf1 := &bytes.Buffer{}
	if _, err := f.Write(buf1); err != nil {
		t.Fatal(err)
	}
	b1 := buf1.Bytes()

	f2, err := sfnt.Read(bytes.NewReader(b1), parser.NewBudget(int64(len(b1))))
	if err != nil {
		t.Fatal(err)
	}

	if f2.Fvar.Axes[0].NameID != 256 || f2.Fvar.Axes[0].Name != "" {
		t.Errorf("axis0 = %+v, want NameID 256 with empty Name preserved", f2.Fvar.Axes[0])
	}
	if f2.Fvar.Axes[1].Name != "Width" {
		t.Errorf("axis1 name = %q, want %q", f2.Fvar.Axes[1].Name, "Width")
	}
	if f2.Fvar.Axes[1].NameID == 256 {
		t.Error("axis1 was allocated NameID 256, colliding with the preserved axis0 entry")
	}

	// a full read-write-read cycle on the malformed font must be a fixpoint
	buf2 := &bytes.Buffer{}
	if _, err := f2.Write(buf2); err != nil {
		t.Fatal(err)
	}
	b2 := buf2.Bytes()
	if !bytes.Equal(b1, b2) {
		t.Errorf("second write not byte-identical: %d vs %d bytes", len(b1), len(b2))
	}

	f3, err := sfnt.Read(bytes.NewReader(b2), parser.NewBudget(int64(len(b2))))
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(f2.Fvar, f3.Fvar); diff != "" {
		t.Errorf("fvar not stable (-f2 +f3):\n%s", diff)
	}
}
