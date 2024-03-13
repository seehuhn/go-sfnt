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
	"fmt"
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
	out := gsub.LookupList.ApplyLookup(in, 0, nil)

	expected := []glyph.Info{
		{GID: 4, Text: []rune("ab")},
		{GID: 3, Text: []rune("c")},
	}

	if d := cmp.Diff(expected, out); d != "" {
		t.Errorf("unexpected result (-want +got):\n%s", d)
	}
}

func TestApplyMatch(t *testing.T) {
	cases := []struct {
		m   *Match
		out []glyph.ID
	}{
		{
			m: &Match{
				InputPos: []int{0},
				Replace: []glyph.Info{
					{GID: 100},
				},
			},
			out: []glyph.ID{100, 1, 2, 3, 4, 5, 6},
		},
		{
			m: &Match{
				InputPos: []int{0, 1},
				Replace: []glyph.Info{
					{GID: 100},
				},
			},
			out: []glyph.ID{100, 2, 3, 4, 5, 6},
		},
		{
			m: &Match{
				InputPos: []int{0, 1, 2},
				Replace: []glyph.Info{
					{GID: 100},
				},
			},
			out: []glyph.ID{100, 3, 4, 5, 6},
		},
		{
			m: &Match{
				InputPos: []int{0, 2, 4},
				Replace: []glyph.Info{
					{GID: 100},
				},
			},
			out: []glyph.ID{100, 1, 3, 5, 6},
		},
		{
			m: &Match{
				InputPos: []int{1},
				Replace: []glyph.Info{
					{GID: 100},
				},
			},
			out: []glyph.ID{100, 0, 2, 3, 4, 5, 6},
		},
		{
			m: &Match{
				InputPos: []int{1, 2},
				Replace: []glyph.Info{
					{GID: 100},
				},
			},
			out: []glyph.ID{100, 0, 3, 4, 5, 6},
		},
		{
			m: &Match{
				InputPos: []int{0},
				Replace: []glyph.Info{
					{GID: 100},
					{GID: 101},
				},
			},
			out: []glyph.ID{100, 101, 1, 2, 3, 4, 5, 6},
		},
		{
			m: &Match{
				InputPos: []int{0},
				Replace: []glyph.Info{
					{GID: 100},
					{GID: 101},
					{GID: 102},
				},
			},
			out: []glyph.ID{100, 101, 102, 1, 2, 3, 4, 5, 6},
		},
		{
			m: &Match{
				InputPos: []int{1, 5},
				Replace: []glyph.Info{
					{GID: 100},
					{GID: 101},
					{GID: 102},
				},
			},
			out: []glyph.ID{100, 101, 102, 0, 2, 3, 4, 6},
		},
	}

	for i, test := range cases {
		t.Run(fmt.Sprintf("%02d", i+1), func(t *testing.T) {
			seq := make([]glyph.Info, 7)
			for i := range seq {
				seq[i].GID = glyph.ID(i)
			}
			seq = applyMatch(seq, test.m, 0)
			out := make([]glyph.ID, len(seq))
			for i, g := range seq {
				out[i] = g.GID
			}
			if d := cmp.Diff(out, test.out); d != "" {
				t.Error(d)
			}
		})
	}
}

func TestFixMatchPos(t *testing.T) {
	cases := []struct {
		in        []int
		remove    []int
		numInsert int
		out       []int
	}{
		{ // common case: replace two glyphs with one
			in:        []int{1, 2},
			remove:    []int{1, 2},
			numInsert: 1,
			out:       []int{1},
		},
		{ // common case: replace one glyph with two
			in:        []int{1},
			remove:    []int{1},
			numInsert: 2,
			out:       []int{1, 2},
		},
		{ // replace two glyphs with one, with extra glyphs present at end
			in:        []int{1, 2, 4},
			remove:    []int{1, 2},
			numInsert: 1,
			out:       []int{1, 3},
		},
		{ // glyph 0 was not in input, so is not included in the output either
			in:        []int{1, 2, 4},
			remove:    []int{0},
			numInsert: 1,
			out:       []int{1, 2, 4},
		},
		{
			in:        []int{1, 2, 4},
			remove:    []int{1},
			numInsert: 1,
			out:       []int{1, 2, 4},
		},
		{
			in:        []int{1, 2, 4},
			remove:    []int{2},
			numInsert: 1,
			out:       []int{1, 2, 4},
		},
		{ // glyph 3 was not in input, so is not included in the output either
			in:        []int{1, 2, 4},
			remove:    []int{3},
			numInsert: 1,
			out:       []int{1, 2, 4},
		},
		{
			in:        []int{1, 2, 4},
			remove:    []int{4},
			numInsert: 1,
			out:       []int{1, 2, 4},
		},
		{ // glyph 5 was not in input, so is not included in the output either
			in:        []int{1, 2, 4},
			remove:    []int{5},
			numInsert: 1,
			out:       []int{1, 2, 4},
		},
	}
	for i, test := range cases {
		for _, endOffs := range []int{1, 10} {
			endPos := test.in[len(test.in)-1] + endOffs
			actions := []*nested{
				{
					InputPos: test.in,
					Actions:  []SeqLookup{},
					EndPos:   endPos,
				},
			}
			fixActionStack(actions, test.remove, test.numInsert)
			if d := cmp.Diff(test.out, actions[0].InputPos); d != "" {
				t.Errorf("%d: %s", i, d)
			}
		}
	}
}
