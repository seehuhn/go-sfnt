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
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/exp/maps"
	"golang.org/x/text/language"
	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/afm"
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/funit"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/head"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/os2"
	"seehuhn.de/go/sfnt/type1"
	"seehuhn.de/go/sfnt/type1/names"
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

	var afm *afm.Info
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

func readAfm(afmName string) (*afm.Info, error) {
	fd, err := os.Open(afmName)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	return afm.Read(fd)
}

func readType1(fname string, afm *afm.Info) (*sfnt.Font, error) {
	fd, err := os.Open(fname)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	t1Info, err := type1.Read(fd)
	if err != nil {
		return nil, err
	}

	glyphNames := maps.Keys(t1Info.Glyphs)
	order := make(map[string]int, len(glyphNames))
	for _, name := range glyphNames {
		order[name] = 256
	}
	order[".notdef"] = -1
	for i, name := range t1Info.Encoding {
		if name != ".notdef" {
			order[name] = i
		}
	}
	sort.Slice(glyphNames, func(i, j int) bool {
		oi := order[glyphNames[i]]
		oj := order[glyphNames[j]]
		if oi != oj {
			return oi < oj
		}
		return glyphNames[i] < glyphNames[j]
	})

	glyphs := make([]*cff.Glyph, 0, len(glyphNames))
	name2gid := make(map[string]glyph.ID, len(glyphNames))
	for _, name := range glyphNames {
		t1 := t1Info.Glyphs[name]
		if t1.WidthY != 0 {
			return nil, fmt.Errorf("unsupported WidthY=%d for glyph %q",
				t1.WidthY, name)
		}
		g := cff.NewGlyph(name, t1.WidthX)
		for _, cmd := range t1.Cmds {
			switch cmd.Op {
			case type1.OpMoveTo:
				g.MoveTo(cmd.Args[0], cmd.Args[1])
			case type1.OpLineTo:
				g.LineTo(cmd.Args[0], cmd.Args[1])
			case type1.OpCurveTo:
				g.CurveTo(cmd.Args[0], cmd.Args[1], cmd.Args[2], cmd.Args[3], cmd.Args[4], cmd.Args[5])
			}
		}
		g.HStem = t1.HStem
		g.VStem = t1.VStem
		name2gid[name] = glyph.ID(len(glyphs))
		glyphs = append(glyphs, g)
	}

	encoding := make([]glyph.ID, 256)
	for i, name := range t1Info.Encoding {
		encoding[i] = name2gid[name]
	}

	outlines := &cff.Outlines{
		Glyphs:   glyphs,
		Private:  []*type1.PrivateDict{t1Info.Private},
		FDSelect: func(glyph.ID) int { return 0 },
		Encoding: encoding,
	}

	width := os2.WidthNormal // TODO(voss)
	weight := os2.WeightFromString(t1Info.Info.Weight)
	if weight == 0 {
		weight = os2.WeightNormal
	}
	version, _ := head.VersionFromString(t1Info.Info.Version)
	modificationTime := time.Now()
	creationTime := modificationTime
	if !t1Info.CreationDate.IsZero() {
		creationTime = t1Info.CreationDate
	}

	// TODO(voss): can this be improved?
	isItalic := t1Info.Info.ItalicAngle != 0
	isBold := weight >= os2.WeightBold
	isRegular := strings.Contains(t1Info.Info.FullName, "Regular")
	isOblique := t1Info.Info.ItalicAngle != 0 && !strings.Contains(t1Info.Info.FullName, "Italic")
	isSerif := false  // TODO(voss)
	isScript := false // TODO(voss)

	cmap := makeCmap(t1Info, glyphNames)

	unitsPerEm := 1000 // TODO(voss): get from the font matrix

	var ascent funit.Int16
	var descent funit.Int16
	var capHeight funit.Int16
	var xHeight funit.Int16
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
				ascent = bb.URy
				break
			}
		}
		for _, name := range []string{"p", "q", "g", "j", "y"} {
			if gid, exists := name2gid[name]; exists {
				g := glyphs[gid]
				bb := g.Extent()
				descent = bb.LLy
				break
			}
		}
		for _, name := range []string{"H", "I", "K", "L", "T"} {
			if gid, exists := name2gid[name]; exists {
				g := glyphs[gid]
				bb := g.Extent()
				capHeight = bb.URy
				break
			}
		}
		for _, name := range []string{"x", "u", "v", "w", "z"} {
			if gid, exists := name2gid[name]; exists {
				g := glyphs[gid]
				bb := g.Extent()
				xHeight = bb.URy
				break
			}
		}
	}

	minBaseLineSkip := funit.Int16(math.Round(1.2 * float64(unitsPerEm)))
	if d := minBaseLineSkip - (ascent - descent); d > 0 {
		d1 := d / 3
		d2 := d - d1
		descent -= d1
		ascent += d2
	}

	gsub := makeLigatures(afm, name2gid)
	gpos := makeKerningTable(afm, name2gid)

	otfInfo := sfnt.Font{
		FamilyName:         t1Info.Info.FamilyName,
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
		Copyright:          t1Info.Info.Copyright,
		Trademark:          t1Info.Info.Notice,
		UnitsPerEm:         uint16(unitsPerEm),
		Ascent:             ascent,
		Descent:            descent,
		CapHeight:          capHeight,
		XHeight:            xHeight,
		ItalicAngle:        t1Info.Info.ItalicAngle,
		UnderlinePosition:  t1Info.Info.UnderlinePosition,
		UnderlineThickness: t1Info.Info.UnderlineThickness,
		CMap:               cmap,
		Outlines:           outlines,
		Gsub:               gsub,
		Gpos:               gpos,
	}

	// TODO(voss): how to choose this?
	otfInfo.CodePageRange.Set(os2.CP1252) // Latin 1

	return &otfInfo, nil
}

func makeCmap(t1Info *type1.Font, glyphNames []string) cmap.Subtable {
	canUseFormat4 := true
	codes := make([]rune, len(glyphNames))
	for gid, name := range glyphNames {
		rr := names.ToUnicode(name, false)
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

func makeLigatures(afm *afm.Info, name2gid map[string]glyph.ID) *gtab.Info {
	if afm == nil || len(afm.Ligatures) == 0 {
		return nil
	}

	afmNames := make(map[glyph.ID]string, len(afm.GlyphName))
	for gid, name := range afm.GlyphName {
		afmNames[glyph.ID(gid)] = name
	}
	t := func(gid glyph.ID) glyph.ID {
		return name2gid[afmNames[gid]]
	}

	ll := map[glyph.ID][]gtab.Ligature{}
	for pair, repl := range afm.Ligatures {
		a := t(pair.Left)
		b := t(pair.Right)

		ll[a] = append(ll[a], gtab.Ligature{
			In:  []glyph.ID{b},
			Out: t(repl),
		})
	}

	keys := maps.Keys(ll)
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

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

func makeKerningTable(afm *afm.Info, name2gid map[string]glyph.ID) *gtab.Info {
	if afm == nil || len(afm.Kern) == 0 {
		return nil
	}

	afmNames := make(map[glyph.ID]string, len(afm.GlyphName))
	for gid, name := range afm.GlyphName {
		afmNames[glyph.ID(gid)] = name
	}
	t := func(gid glyph.ID) glyph.ID {
		return name2gid[afmNames[gid]]
	}

	all := make(map[glyph.ID]map[glyph.ID]*gtab.PairAdjust)
	for pair, adj := range afm.Kern {
		left := t(pair.Left)
		right := t(pair.Right)
		if _, exists := all[left]; !exists {
			all[left] = make(map[glyph.ID]*gtab.PairAdjust)
		}
		pa := &gtab.PairAdjust{
			First: &gtab.GposValueRecord{XAdvance: adj},
		}
		all[left][right] = pa
	}
	keys := maps.Keys(all)
	sort.Slice(keys, func(i, j int) bool {
		return keys[i] < keys[j]
	})

	cov := coverage.Table{}
	adjust := make([]map[glyph.ID]*gtab.PairAdjust, len(keys))
	for idx, key := range keys {
		cov[key] = idx
		adjust[idx] = all[key]
	}
	kern := &gtab.Gpos2_1{
		Cov:    cov,
		Adjust: adjust,
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
