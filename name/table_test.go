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
