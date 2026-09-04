// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2023  Jochen Voss <voss@seehuhn.de>
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

package main

import (
	"flag"
	"fmt"
	"log"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/text/language"

	"seehuhn.de/go/postscript/afm"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"
	"seehuhn.de/go/postscript/type1/names"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/head"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/os2"
)

func main() {
	outNameFlag := flag.String("o", "", "output file name")
	flag.Parse()

	var fname string
	var afmName string
	switch flag.NArg() {
	case 2:
		afmName = flag.Arg(1)
		fallthrough
	case 1:
		fname = flag.Arg(0)
	default:
		fmt.Fprintf(os.Stderr, "usage: %s font.pf{a,b} [font.afm]\n", os.Args[0])
		os.Exit(1)
	}

	outName := *outNameFlag
	if outName == "" {
		basename := filepath.Base(fname)
		outName = strings.TrimSuffix(basename, filepath.Ext(basename)) + ".otf"
	}

	if afmName != "" {
		fmt.Println(fname, afmName, "->", outName)
	} else {
		fmt.Println(fname, "->", outName)
		fmt.Fprintln(os.Stderr, "warning: no AFM file specified")
	}

	var afm *afm.Metrics
	if afmName != "" {
		var err error
		afm, err = readAfm(afmName)
		if err != nil {
			log.Fatal(err)
		}
	}

	info, err := readType1(fname, afm)
	if err != nil {
		log.Fatal(err)
	}

	err = writeOtf(outName, info)
	if err != nil {
		log.Fatal(err)
	}
}

func readAfm(afmName string) (*afm.Metrics, error) {
	fd, err := os.Open(afmName)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	return afm.Read(fd)
}

func readType1(fname string, afm *afm.Metrics) (*sfnt.Font, error) {
	fd, err := os.Open(fname)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	t1Info, err := type1.Read(fd)
	if err != nil {
		return nil, err
	}

	cffFont, err := cff.FromType1(t1Info)
	if err != nil {
		return nil, err
	}
	outlines := cffFont.Outlines

	glyphs := outlines.Glyphs
	glyphNames := make([]string, len(glyphs))
	name2gid := make(map[string]glyph.ID, len(glyphs))
	for gid, g := range glyphs {
		glyphNames[gid] = g.Name
		name2gid[g.Name] = glyph.ID(gid)
	}

	// CFF charstrings cannot express a vertical advance width, and
	// cff.FromType1 discards it silently.  Refuse such fonts here rather than
	// writing an OpenType font with wrong metrics.
	for _, name := range glyphNames {
		if g := t1Info.Glyphs[name]; g != nil && g.WidthY != 0 {
			return nil, fmt.Errorf("unsupported WidthY=%g for glyph %q",
				g.WidthY, name)
		}
	}

	width := os2.WidthNormal // TODO(voss)
	weight := os2.WeightFromString(t1Info.FontInfo.Weight)
	if weight == 0 {
		weight = os2.WeightNormal
	}
	version, _ := head.VersionFromString(t1Info.FontInfo.Version)
	modificationTime := time.Now()
	creationTime := modificationTime
	if !t1Info.CreationDate.IsZero() {
		creationTime = t1Info.CreationDate
	}

	// TODO(voss): can this be improved?
	isItalic := t1Info.FontInfo.ItalicAngle != 0
	isBold := weight >= os2.WeightBold
	isRegular := strings.Contains(t1Info.FontInfo.FullName, "Regular")
	isOblique := t1Info.FontInfo.ItalicAngle != 0 && !strings.Contains(t1Info.FontInfo.FullName, "Italic")
	isSerif := false  // TODO(voss)
	isScript := false // TODO(voss)

	// A Type 1 font has no subfamily name, so one is made up from the style.
	var subfamily string
	switch {
	case isBold && isItalic:
		subfamily = "Bold Italic"
	case isBold:
		subfamily = "Bold"
	case isItalic:
		subfamily = "Italic"
	default:
		subfamily = "Regular"
	}

	cmap := makeCmap(glyphNames)

	unitsPerEm := math.Round(1 / t1Info.FontMatrix[0])

	var ascent float64
	var descent float64
	var capHeight float64
	var xHeight float64
	if afm != nil {
		ascent = afm.Ascent
		descent = afm.Descent
		capHeight = afm.CapHeight
		xHeight = afm.XHeight
	} else {
		for _, name := range []string{"b", "d", "h", "l", "f"} {
			if gid, exists := name2gid[name]; exists {
				g := glyphs[gid]
				bb := g.Extent()
				ascent = float64(bb.URy)
				break
			}
		}
		for _, name := range []string{"p", "q", "g", "j", "y"} {
			if gid, exists := name2gid[name]; exists {
				g := glyphs[gid]
				bb := g.Extent()
				descent = float64(bb.LLy)
				break
			}
		}
		// convert from PDF glyph space units to font design units
		q := unitsPerEm / 1000
		capHeight = t1Info.CapHeightPDF() * q
		xHeight = t1Info.XHeightPDF() * q
	}

	minBaseLineSkip := math.Ceil(1.2 * unitsPerEm)
	if d := minBaseLineSkip - (ascent - descent); d > 0 {
		d1 := d / 3
		d2 := d - d1
		descent -= d1
		ascent += d2
	}

	gsub := makeLigatures(afm, name2gid)
	gpos := makeKerningTable(afm, name2gid)

	otfInfo := sfnt.Font{
		FamilyName:         t1Info.FontInfo.FamilyName,
		Subfamily:          subfamily,
		FullName:           t1Info.FontInfo.FullName,
		Width:              width,
		Weight:             weight,
		IsItalic:           isItalic,
		IsBold:             isBold,
		IsRegular:          isRegular,
		IsOblique:          isOblique,
		IsSerif:            isSerif,
		IsScript:           isScript,
		Version:            version,
		CreationTime:       creationTime,
		ModificationTime:   modificationTime,
		Copyright:          t1Info.FontInfo.Copyright,
		Trademark:          t1Info.FontInfo.Notice,
		UnitsPerEm:         uint16(unitsPerEm),
		Ascent:             funit.Int16(math.Round(ascent)),
		Descent:            funit.Int16(math.Round(descent)),
		CapHeight:          funit.Int16(math.Round(capHeight)),
		XHeight:            funit.Int16(math.Round(xHeight)),
		ItalicAngle:        t1Info.FontInfo.ItalicAngle,
		UnderlinePosition:  t1Info.FontInfo.UnderlinePosition,
		UnderlineThickness: t1Info.FontInfo.UnderlineThickness,
		Outlines:           outlines,
		Gsub:               gsub,
		Gpos:               gpos,
	}
	otfInfo.InstallCMap(cmap)

	// TODO(voss): how to choose this?
	otfInfo.CodePageRange.Set(os2.CP1252) // Latin 1

	return &otfInfo, nil
}

func makeCmap(glyphNames []string) cmap.Subtable {
	canUseFormat4 := true
	codes := make([]rune, len(glyphNames))
	for gid, name := range glyphNames {
		rr := []rune(names.ToUnicode(name, ""))
		if len(rr) != 1 {
			continue
		}
		r := rr[0]
		if r > 65535 {
			canUseFormat4 = false
		}
		codes[gid] = r
	}

	if canUseFormat4 {
		cmap := cmap.Format4{}
		for gid, r := range codes {
			r16 := uint16(r)
			if _, exists := cmap[r16]; exists {
				continue
			}
			cmap[r16] = glyph.ID(gid)
		}
		return cmap
	}

	cmap := cmap.Format12{}
	for gid, r := range codes {
		r32 := uint32(r)
		if _, exists := cmap[r32]; exists {
			continue
		}
		cmap[r32] = glyph.ID(gid)
	}
	return cmap
}

func makeLigatures(afm *afm.Metrics, name2gid map[string]glyph.ID) *gtab.Info {
	if afm == nil {
		return nil
	}

	ll := map[glyph.ID][]gtab.Ligature{}
	for left, g := range afm.Glyphs {
		a := name2gid[left]
		for right, repl := range g.Ligatures {
			b := name2gid[right]
			ll[a] = append(ll[a], gtab.Ligature{
				In:  []glyph.ID{b},
				Out: name2gid[repl],
			})
		}
	}

	// TODO(voss): merge this with the code in go-sfnt/ligatures.go

	keys := slices.Sorted(maps.Keys(ll))

	cov := coverage.Table{}
	var repl [][]gtab.Ligature
	for i, gid := range keys {
		cov[gid] = i
		repl = append(repl, ll[gid])
	}
	subst := &gtab.Gsub4_1{
		Cov:  cov,
		Repl: repl,
	}
	gsub := &gtab.Info{
		ScriptList: map[language.Tag]*gtab.Features{
			language.Und: {Optional: []gtab.FeatureIndex{0}},
		},
		FeatureList: []*gtab.Feature{
			{Tag: "liga", Lookups: []gtab.LookupIndex{0}},
		},
		LookupList: []*gtab.LookupTable{
			{
				Meta:      &gtab.LookupMetaInfo{LookupType: 4},
				Subtables: []gtab.Subtable{subst},
			},
		},
	}
	return gsub
}

func makeKerningTable(afm *afm.Metrics, name2gid map[string]glyph.ID) *gtab.Info {
	if afm == nil || len(afm.Kern) == 0 {
		return nil
	}

	kern := gtab.Gpos2_1{}
	for _, pair := range afm.Kern {
		left := name2gid[pair.Left]
		right := name2gid[pair.Right]
		kern[glyph.Pair{
			Left:  left,
			Right: right,
		}] = &gtab.PairAdjust{
			First: &gtab.GposValueRecord{XAdvance: pair.Adjust},
		}
	}

	gpos := &gtab.Info{
		ScriptList: map[language.Tag]*gtab.Features{
			language.Und: {Optional: []gtab.FeatureIndex{0}},
		},
		FeatureList: []*gtab.Feature{
			{Tag: "kern", Lookups: []gtab.LookupIndex{0}},
		},
		LookupList: []*gtab.LookupTable{
			{
				Meta:      &gtab.LookupMetaInfo{LookupType: 2},
				Subtables: []gtab.Subtable{kern},
			},
		},
	}
	return gpos
}

func writeOtf(outname string, info *sfnt.Font) error {
	out, err := os.Create(outname)
	if err != nil {
		return err
	}
	_, err = info.Write(out)
	if err != nil {
		return err
	}
	err = out.Close()
	if err != nil {
		return err
	}
	return nil
}
