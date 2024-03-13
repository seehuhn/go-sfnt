// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2024  Jochen Voss <voss@seehuhn.de>
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

package testcases

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/text/language"
	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/internal/debug"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/gdef"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/opentype/gtab/builder"
)

type FontGen struct {
	CMap cmap.Subtable
	Rev  map[glyph.ID]rune

	info *sfnt.Font
	gdef *gdef.Table
}

func NewFontGen() (*FontGen, error) {
	fontInfo := debug.MakeSimpleFont()

	cmap, err := fontInfo.CMapTable.GetBest()
	if err != nil {
		return nil, err
	}
	gdef := &gdef.Table{
		GlyphClass: classdef.Table{
			cmap.Lookup('B'): gdef.GlyphClassBase,
			cmap.Lookup('K'): gdef.GlyphClassLigature,
			cmap.Lookup('L'): gdef.GlyphClassLigature,
			cmap.Lookup('M'): gdef.GlyphClassMark,
			cmap.Lookup('N'): gdef.GlyphClassMark,
		},
	}

	a, b := cmap.CodeRange()
	rev := make(map[glyph.ID]rune)
	for r := a; r <= b; r++ {
		gid := cmap.Lookup(r)
		if gid != 0 {
			rev[gid] = r
		}
	}

	return &FontGen{
		CMap: cmap,
		Rev:  rev,
		info: fontInfo,
		gdef: gdef,
	}, nil
}

func (g *FontGen) GsubTestFont(idx int) (*sfnt.Font, error) {
	if idx < 0 || idx >= len(Gsub) {
		return nil, errors.New("index out of range")
	}
	c := Gsub[idx]

	info := clone(g.info)

	lookupList, err := builder.Parse(info, c.Desc)
	if err != nil {
		return nil, err
	}
	gsub := &gtab.Info{
		ScriptList: map[language.Tag]*gtab.Features{
			language.MustParse("und-Zzzz"): {Required: 0},
		},
		FeatureList: []*gtab.Feature{
			{Tag: "test", Lookups: []gtab.LookupIndex{0}},
		},
		LookupList: lookupList,
	}

	info.FamilyName = fmt.Sprintf("Test%04d", idx+1)
	info.Description = fixLines(c.Desc)
	info.SampleText = c.In
	info.Gdef = g.gdef
	info.Gsub = gsub

	return info, nil
}

func fixLines(s string) string {
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			lines = append(lines, l+"\n")
		}
	}
	return strings.Join(lines, "")
}

// clone makes a deep copy of *T.
func clone[T any](x *T) *T {
	y := *x
	return &y
}
