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
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/text/language"
	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/internal/debug"
	"seehuhn.de/go/sfnt/opentype/classdef"
	"seehuhn.de/go/sfnt/opentype/gdef"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/opentype/gtab/builder"
	"seehuhn.de/go/sfnt/opentype/gtab/testcases"
)

var exportFonts = flag.Bool("export-fonts", false, "export fonts used in tests")

// TestGsub tests corner cases for GSUB rules.
//
// To test the behaviour of other implementations, use the "-export-fonts"
// flag.  This will create a font file for each test case.
//
// I have used the following procedure on MacOS:
//
// 1. Run one of the tests, using commands like the folowing:
//
//	go test -run TestGsub/63 -export-fonts
//
// This shows the expected ligature substituions, e.g. "LALALAL -> LXXAL", and
// saves the font file "test0063.otf".
//
// 2. Install the font in the system using the "Font Book" app, for example by
// double clicking on the generated .otf file.
//
// 3. The MacOS "Font Book" app can be used to show the ligature substitutions
// in MacOS.  The sample text in the font file is set to the input string, so
// so substituions can be seen directly.  There is also the possibility to edit
// the sample text to see the effect of the substitutions.
//
// 4. Use "hb-view" (part of the Harfbuzz suite) to show the ligature
// substitutions in Harfbuzz.  For example, the following command shows the
// ligature substitutions for the input string "LALALAL":
//
//	hb-view test0063.otf LALALAL
//
// Alternatively, "hb-shape" can be used:
//
//	hb-shape --no-clusters --no-positions test0063.otf LALALAL
//
// 5. To see the ligature substitutions in Microsoft Word: (a) open word, quit
// word, open word, open a new, blank document, (b) Click on the font name, and
// replace with the name of the newly installed font (e.g. "Test0064"), (c)
// Open the "Text Effects" menu (the icon is a white "A" with a blue outline),
// and choose "Ligatures > All Ligatures", and (d) type the input string into
// the document.
func TestGsub(t *testing.T) {
	fontGen, err := testcases.NewFontGen()
	if err != nil {
		t.Fatal(err)
	}

	for testIdx, test := range testcases.Gsub {
		t.Run(fmt.Sprintf("%02d", testIdx+1), func(t *testing.T) {
			info, err := fontGen.GsubTestFont(testIdx)
			if err != nil {
				t.Fatal(err)
			}

			if *exportFonts {
				fontName := fmt.Sprintf("test%04d.otf", testIdx+1)
				fmt.Printf("%s %s -> %s\n", fontName, test.In, test.Out)
				exportFont(info, testIdx+1, test.In)
			}

			seq := make([]glyph.Info, len(test.In))
			for i, r := range test.In {
				seq[i].GID = fontGen.CMap.Lookup(r)
				seq[i].Text = []rune{r}
			}
			lookups := info.Gsub.FindLookups(language.AmericanEnglish, nil)
			for _, lookupIndex := range lookups {
				seq = info.Gsub.LookupList.ApplyLookup(seq, lookupIndex, info.Gdef)
			}

			var textRunes []rune
			var outRunes []rune
			for _, g := range seq {
				textRunes = append(textRunes, g.Text...)
				outRunes = append(outRunes, fontGen.Rev[g.GID])
			}
			text := string(textRunes)
			out := string(outRunes)

			expectedText := test.Text
			if expectedText == "" {
				expectedText = test.In
			}
			fmt.Printf("%s (test%04d.otf) %s -> %s\n",
				t.Name(), testIdx+1, test.In, test.Out)
			if out != test.Out {
				t.Errorf("expected output %q, got %q", test.Out, out)
			} else if text != expectedText {
				t.Errorf("expected text %q, got %q", expectedText, text)
			}
		})
	}
}

func FuzzGsub(f *testing.F) {
	for _, test := range testcases.Gsub {
		f.Add(test.Desc, test.In)
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
		lookupList, err := builder.Parse(fontInfo, desc)
		if err != nil {
			return
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

		seq := make([]glyph.Info, len(in))
		for i, r := range in {
			seq[i].GID = cmap.Lookup(r)
			seq[i].Text = []rune{r}
		}
		lookups := gsub.FindLookups(language.AmericanEnglish, nil)
		for _, lookupIndex := range lookups {
			seq = gsub.LookupList.ApplyLookup(seq, lookupIndex, gdefTable)
		}

		runeCountIn := len([]rune(in))
		runeCountOut := 0
		for _, g := range seq {
			runeCountOut += len(g.Text)
		}
		if runeCountOut != runeCountIn {
			fmt.Printf("desc = %q\n", desc)
			fmt.Printf("in = %q\n", in)
			for i, g := range seq {
				fmt.Printf("out[%d] = %d %q\n", i, g.GID, string(g.Text))
			}
			t.Errorf("expected %d runes, got %d", runeCountIn, runeCountOut)
		}
	})
}

func exportFont(fontInfo *sfnt.Font, idx int, in string) {
	if !*exportFonts {
		return
	}

	fontInfo.FamilyName = fmt.Sprintf("Test%04d", idx)
	now := time.Now()
	fontInfo.CreationTime = now
	fontInfo.ModificationTime = now
	fontInfo.SampleText = in

	fname := fmt.Sprintf("test%04d.otf", idx)
	fd, err := os.Create(fname)
	if err != nil {
		panic(err)
	}
	_, err = fontInfo.Write(fd)
	if err != nil {
		panic(err)
	}
	err = fd.Close()
	if err != nil {
		panic(err)
	}
}
