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
	"testing"

	"seehuhn.de/go/sfnt/os2"
)

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

	var rr []rune
	for i := 0; i < 255; i++ {
		rr = append(rr, rune(i))
	}
	info.FamilyName = string(rr)
	psName = info.PostScriptName()
	if len(psName) != 127-33-10+len("-BoldItalic") {
		t.Errorf("wrong postscript name: %q", psName)
	}
}
