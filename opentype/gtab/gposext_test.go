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

package gtab_test

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"golang.org/x/text/language"

	"seehuhn.de/go/postscript/funit"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/internal/debug"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/gdef"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/opentype/gtab/builder"
)

func TestGpos(t *testing.T) {
	fontInfo := debug.MakeSimpleFont()
	cmap, err := fontInfo.CMapTable.GetBest()
	if err != nil {
		t.Fatal(err)
	}

	gdefTable := &gdef.Table{
		GlyphClass: classdef.Table{
			cmap.Lookup('B'): gdef.GlyphClassBase,
			cmap.Lookup('K'): gdef.GlyphClassLigature,
			cmap.Lookup('L'): gdef.GlyphClassLigature,
			cmap.Lookup('M'): gdef.GlyphClassMark,
			cmap.Lookup('N'): gdef.GlyphClassMark,
		},
	}

	for testIdx, test := range gposTestCases {
		t.Run(fmt.Sprintf("%02d", testIdx+501), func(t *testing.T) {
			desc := test.desc
			if strings.Contains(desc, "Δ") {
				ax := 0
				bx := 0
				pos := 0
				for _, r := range test.in {
					gid := cmap.Lookup(r)
					if r == '>' {
						ax = pos
					} else if r == '<' {
						bx = pos
					}
					pos += int(fontInfo.GlyphWidth(gid))
				}
				delta := fmt.Sprintf("x%+d", ax-bx)
				desc = strings.Replace(desc, "Δ", delta, 1)
			}

			lookupList, err := builder.Parse(fontInfo, desc)
			if err != nil {
				t.Fatal(err)
			}

			gpos := &gtab.Info{
				ScriptList: map[language.Tag]*gtab.Features{
					language.MustParse("und-Zzzz"): {Required: 0},
				},
				FeatureList: []*gtab.Feature{
					{Tag: "kern", Lookups: []gtab.LookupIndex{0}},
				},
				LookupList: lookupList,
			}

			testName := fmt.Sprintf("%04d", testIdx+501)
			fontName := "test" + testName + ".otf"
			if *exportFonts {
				fmt.Printf("%s %s\n", fontName, test.in)

				fontInfo.Gdef = gdefTable
				fontInfo.Gpos = gpos
				exportFont(fontInfo, testName, test.in)
			}

			seq := make([]glyph.Info, len(test.in))
			for i, r := range test.in {
				gid := cmap.Lookup(r)
				seq[i].GID = gid
				seq[i].Text = []rune{r}
				if gdefTable.GlyphClass[gid] != gdef.GlyphClassMark {
					seq[i].Advance = funit.Int16(fontInfo.GlyphWidth(gid)) // TODO(voss)
				}
			}
			lookups := gpos.FindLookups(language.AmericanEnglish, nil)
			e := gtab.NewContext(gpos.LookupList, gdefTable, lookups)
			seq = e.Apply(seq)

			for _, check := range test.check {
				switch check.which {
				case checkX:
					if seq[check.idx].XOffset != check.val {
						t.Errorf("%s[%d]: expected XOffset == %d, got %d",
							fontName, check.idx, check.val, seq[check.idx].XOffset)
					}
				case checkY:
					if seq[check.idx].YOffset != check.val {
						t.Errorf("%s[%d]: expected YOffset == %d, got %d",
							fontName, check.idx, check.val, seq[check.idx].YOffset)
					}
				case checkDX:
					if seq[check.idx].Advance != check.val {
						t.Errorf("%s[%d]: expected XAdvance == %d, got %d",
							fontName, check.idx, check.val, seq[check.idx].Advance)
					}
				case checkDXRel:
					w := fontInfo.GlyphWidth(seq[check.idx].GID)
					expected := check.val + funit.Int16(math.Round(w))
					if seq[check.idx].Advance != expected {
						t.Errorf("%s[%d]: expected XAdvance == %v, got %v",
							fontName, check.idx, expected, seq[check.idx].Advance)
					}
				default:
					panic("unknown check")
				}
			}
		})
	}
}

func FuzzGpos(f *testing.F) {
	for _, test := range gposTestCases {
		f.Add(test.desc, test.in)
	}

	fontInfo := debug.MakeSimpleFont()

	cmap, err := fontInfo.CMapTable.GetBest()
	if err != nil {
		f.Fatal(err)
	}
	gdefTable := &gdef.Table{
		GlyphClass: classdef.Table{
			cmap.Lookup('B'): gdef.GlyphClassBase,
			cmap.Lookup('K'): gdef.GlyphClassLigature,
			cmap.Lookup('L'): gdef.GlyphClassLigature,
			cmap.Lookup('M'): gdef.GlyphClassMark,
			cmap.Lookup('N'): gdef.GlyphClassMark,
		},
	}

	f.Fuzz(func(t *testing.T, desc string, in string) {
		if strings.Contains(desc, "Δ") {
			ax := 0
			bx := 0
			pos := 0
			for _, r := range in {
				gid := cmap.Lookup(r)
				if r == '>' {
					ax = pos
				} else if r == '<' {
					bx = pos
				}
				pos += int(fontInfo.GlyphWidth(gid))
			}
			delta := fmt.Sprintf("x%+d", ax-bx)
			desc = strings.Replace(desc, "Δ", delta, 1)
		}
		lookupList, err := builder.Parse(fontInfo, desc)
		if err != nil {
			return
		}

		gpos := &gtab.Info{
			ScriptList: map[language.Tag]*gtab.Features{
				language.MustParse("und-Zzzz"): {Required: 0},
			},
			FeatureList: []*gtab.Feature{
				{Tag: "kern", Lookups: []gtab.LookupIndex{0}},
			},
			LookupList: lookupList,
		}

		seq := make([]glyph.Info, len(in))
		for i, r := range in {
			gid := cmap.Lookup(r)
			seq[i].GID = gid
			seq[i].Text = []rune{r}
			if gdefTable.GlyphClass[gid] != gdef.GlyphClassMark {
				seq[i].Advance = funit.Int16(math.Round(fontInfo.GlyphWidth(gid)))
			}
		}
		lookups := gpos.FindLookups(language.AmericanEnglish, nil)
		e := gtab.NewContext(gpos.LookupList, gdefTable, lookups)
		seq = e.Apply(seq)

		// TODO(voss): put some plausibility checks here.
	})
}

type gposCheck struct {
	idx   int
	which gposCheckType
	val   funit.Int16
}

type gposCheckType uint16

const (
	checkX gposCheckType = iota
	checkY
	checkDX
	checkDXRel
)

type gposTestCase struct {
	desc  string
	in    string
	check []gposCheck
}

var gposTestCases = []gposTestCase{
	{ // test0501.odf
		desc: "GPOS1: [A] -> y+500",
		in:   "ABC",
		check: []gposCheck{
			{0, checkX, 0},
			{0, checkY, 500},
		},
	},
	{
		desc: "GPOS1: B -> x+10 y-20 dx+30",
		in:   "ABC",
		check: []gposCheck{
			{1, checkX, 10},
			{1, checkY, -20},
			{1, checkDXRel, 30},
		},
	},
	{
		desc: "GPOS1: [A D] -> y+100 || B -> y+200, E -> y+300",
		in:   "ABCDE",
		check: []gposCheck{
			{0, checkY, 100},
			{1, checkY, 200},
			{2, checkY, 0},
			{3, checkY, 100},
			{4, checkY, 300},
		},
	},
	{
		desc: `GPOS1: "<" -> Δ`,
		in:   ">ABC<", // visual test only
	},
	{
		desc: "GPOS1: [M] -> y+500",
		in:   "AMA",
		check: []gposCheck{
			{1, checkDX, 0},
			{1, checkY, 500},
		},
	},
	{
		desc: "GPOS1: -marks [M] -> y+500",
		in:   "AMA",
		check: []gposCheck{
			{1, checkDX, 0},
			{1, checkY, 0},
		},
	},
	{
		desc: "GPOS1: M -> y+500",
		in:   "AMA",
		check: []gposCheck{
			{1, checkDX, 0},
			{1, checkY, 500},
		},
	},
	{
		desc: "GPOS1: -marks M -> y+500",
		in:   "AMA",
		check: []gposCheck{
			{1, checkDX, 0},
			{1, checkY, 0},
		},
	},
	{
		desc: "GPOS1: [] -> x+0",
		in:   "AMA",
		check: []gposCheck{
			{1, checkDX, 0},
			{1, checkY, 0},
		},
	},

	{
		desc: "GPOS2: A V -> dx-200",
		in:   "AV",
		check: []gposCheck{
			{0, checkDXRel, -200},
		},
	},
	{
		desc: "GPOS2: A V -> dx-300 & y+200",
		in:   "AV",
		check: []gposCheck{
			{0, checkDXRel, -300},
			{1, checkY, 200},
		},
	},
	{
		desc: "GPOS2: A A -> y+200",
		in:   "AAAAAA",
		check: []gposCheck{
			{0, checkY, 200},
			{1, checkY, 200},
			{2, checkY, 200},
			{3, checkY, 200},
			{4, checkY, 200},
		},
	},
	{
		desc: "GPOS2: A A -> & y+200",
		in:   "AAAAAA",
		check: []gposCheck{
			{1, checkY, 200},
			{3, checkY, 200},
			{5, checkY, 200},
		},
	},
	{
		desc: `GPOS2:
			/A/
			first A;
			second A;
			_, _;
			_, y+500`,
		in: "AAAAAA",
		check: []gposCheck{
			{0, checkY, 500},
			{1, checkY, 500},
			{2, checkY, 500},
			{3, checkY, 500},
			{4, checkY, 500},
		},
	},

	{
		desc: `GPOS3:
			A: 0,0 to 100,100;
			B: 10,10 to 100,-100`,
		in: "AB",
		check: []gposCheck{
			{0, checkX, 0},
			{0, checkY, 0},
			{0, checkDX, 90},
			{1, checkY, 90},
		},
	},

	{
		desc: `GPOS4:
			mark M: 0@400,0
			base A: @400,1000`,
		in: "AM",
		check: []gposCheck{
			{0, checkX, 0},
			{0, checkY, 0},
			{1, checkX, -1366},
			{1, checkY, 1000},
		},
	},
}
