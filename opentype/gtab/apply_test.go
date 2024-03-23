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

package gtab

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/text/language"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/coverage"
)

// TestLigature tests the simple case where a type 4 GSUB lookup is used
// to replace two glyphs with one.
func TestLigature(t *testing.T) {
	cov := coverage.Table{
		1: 0,
	}
	repl := [][]Ligature{
		{{In: []glyph.ID{2}, Out: 4}}, // 1 2 -> 4
	}
	subst := &Gsub4_1{
		Cov:  cov,
		Repl: repl,
	}
	gsub := &Info{
		ScriptList: map[language.Tag]*Features{
			language.MustParse("und-Latn-x-latn"): {Optional: []FeatureIndex{0}},
		},
		FeatureList: []*Feature{
			{Tag: "liga", Lookups: []LookupIndex{0}},
		},
		LookupList: []*LookupTable{
			{
				Meta:      &LookupMetaInfo{LookupType: 4},
				Subtables: []Subtable{subst},
			},
		},
	}

	in := []glyph.Info{
		{GID: 1, Text: []rune("a")},
		{GID: 2, Text: []rune("b")},
		{GID: 3, Text: []rune("c")},
	}
	e := gsub.LookupList.NewContext([]LookupIndex{0}, nil)
	out := e.ApplyAll(in)

	expected := []glyph.Info{
		{GID: 4, Text: []rune("ab")},
		{GID: 3, Text: []rune("c")},
	}

	if d := cmp.Diff(expected, out); d != "" {
		t.Errorf("unexpected result (-want +got):\n%s", d)
	}
}
