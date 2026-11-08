// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2022  Jochen Voss <voss@seehuhn.de>
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

package name

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestTable(t *testing.T) {
	table := &Table{}
	for i := range ID(300) {
		val := fmt.Sprint(i)
		table.set(i, val)
		val2 := table.get(i)
		if val != val2 {
			t.Errorf("table[%d]: %q != %q", i, val, val2)
		}
	}
}

func TestTableString(t *testing.T) {
	// A field is printed under its own guard: present iff non-empty, with its
	// own label and value.  Designer and Description are independent.
	t.Run("designer-without-description", func(t *testing.T) {
		got := (&Table{Designer: "Jane"}).String()
		if !strings.Contains(got, `Designer: "Jane"`) {
			t.Errorf("Designer dropped from output:\n%s", got)
		}
	})
	t.Run("description-without-designer", func(t *testing.T) {
		got := (&Table{Description: "a font"}).String()
		if strings.Contains(got, "Designer:") {
			t.Errorf("spurious Designer line for empty Designer:\n%s", got)
		}
		if !strings.Contains(got, `Description: "a font"`) {
			t.Errorf("Description missing from output:\n%s", got)
		}
	})
}

func TestMerged(t *testing.T) {
	win := &Table{
		Subfamily: "Regular",
		Version:   "Version 1.0",
		Extra:     map[ID]string{256: "Weight"},
	}
	mac := &Table{
		Family:         "Quire Sans",
		Subfamily:      "Roman",
		FullName:       "Quire Sans",
		PostScriptName: "QuireSans",
		Extra:          map[ID]string{256: "Gewicht", 257: "Width"},
	}

	got := Merged(win, mac)
	want := &Table{
		Family:         "Quire Sans",
		Subfamily:      "Regular",
		FullName:       "Quire Sans",
		Version:        "Version 1.0",
		PostScriptName: "QuireSans",
		Extra:          map[ID]string{256: "Weight", 257: "Width"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("merge failed (-want +got):\n%s", diff)
	}

	// the arguments are left alone
	if win.Family != "" || mac.Subfamily != "Roman" || len(win.Extra) != 1 {
		t.Error("arguments modified")
	}

	if Merged(nil, nil) != nil {
		t.Error("merging two nil tables gave a non-nil table")
	}
	if diff := cmp.Diff(mac, Merged(nil, mac)); diff != "" {
		t.Errorf("merge with nil table failed (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(win, Merged(win, nil)); diff != "" {
		t.Errorf("merge with nil fallback failed (-want +got):\n%s", diff)
	}
}
