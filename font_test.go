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

package sfnt

import (
	"strings"
	"testing"

	"seehuhn.de/go/sfnt/os2"
)

// legalPostScriptChar reports whether r may appear in a PostScript name.
// This is an independent statement of the rule (printable ASCII excluding
// the ten delimiters and whitespace), so the test verifies the spec rather
// than echoing the implementation's own character class.
func legalPostScriptChar(r rune) bool {
	if r < '!' || r > '~' { // whitespace, control codes, non-ASCII
		return false
	}
	return !strings.ContainsRune("()<>[]{}/%", r) // PostScript delimiters
}

func TestPostScriptName(t *testing.T) {
	info := &Font{
		FamilyName: `A(n)d[r]o{m}e/d<a> N%ebula`,
		Weight:     os2.WeightBold,
		IsItalic:   true,
	}
	psName := info.PostScriptName()
	if psName != "AndromedaNebula-BoldItalic" {
		t.Errorf("wrong postscript name: %q", psName)
	}

	// Drive every byte value through the name and check the contract: the
	// result must contain only legal characters, and it must keep exactly
	// the legal characters of "family-subfamily" (illegal ones removed, the
	// rest preserved in order).
	var rr []rune
	for i := range 255 {
		rr = append(rr, rune(i))
	}
	info.FamilyName = string(rr)
	psName = info.PostScriptName()

	for _, r := range psName {
		if !legalPostScriptChar(r) {
			t.Errorf("PostScript name contains illegal character %q: %q", r, psName)
		}
	}

	var want strings.Builder
	for _, r := range info.FamilyName + "-BoldItalic" {
		if legalPostScriptChar(r) {
			want.WriteRune(r)
		}
	}
	if psName != want.String() {
		t.Errorf("PostScript name = %q, want %q", psName, want.String())
	}
}
