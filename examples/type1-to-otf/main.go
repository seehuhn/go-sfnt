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
	"io"
	"log"
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
		fmt.Fprintf(os.Stderr, "usage: %s font.pf? [font.afm]\n", os.Args[0])
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

	afm, err := readAfm(afmName)
	if err != nil {
		log.Fatal(err)
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

func readType1(fname string, afm *afm.Info) (*sfnt.Info, error) {
	fd, err := os.Open(fname)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	// read the first byte of fd to determine the file type
	var buf [1]byte
	_, err = io.ReadFull(fd, buf[:])
	if err != nil {
		return nil, err
	}
	fd.Seek(0, 0)

	var r io.Reader = fd
	if buf[0] == 0x80 {
		r = type1.DecodePFB(r)
	}
	t1Info, err := type1.Read(r)
	if err != nil {
		return nil, err
	}

	glyphNames := maps.Keys(t1Info.Glyphs)
	order := make(map[string]int, len(glyphNames))
	for i := range order {
		order[i] = 256
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
	version, err := head.VersionFromString(t1Info.Info.Version)
	if err != nil {
		return nil, err // TODO(voss)
	}
	modificationTime := time.Now()
	creationTime := modificationTime // TODO(voss)

	cmap := cmap.Format4{}
	for gid, name := range glyphNames {
		rr := names.ToUnicode(name, false)
		if len(rr) != 1 {
			continue
		}
		if rr[0] > 65535 {
			return nil, fmt.Errorf("need format 12 cmap for glyph %q", name)
		}
		r := uint16(rr[0])
		if _, exists := cmap[r]; exists {
			continue
		}
		cmap[r] = glyph.ID(gid)
	}

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
		// TODO(voss): take a guess if no afm given
	}

	gsub := makeLigatures(afm)
	gpos := makeKerningTable(afm)

	otfInfo := sfnt.Info{
		FamilyName:         t1Info.Info.FamilyName,
		Width:              width,
		Weight:             weight,
		IsItalic:           t1Info.Info.ItalicAngle != 0,
		IsBold:             weight >= os2.WeightBold,
		IsRegular:          false, // TODO(voss)
		IsOblique:          false, // TODO(voss)
		IsSerif:            false, // TODO(voss)
		IsScript:           false, // TODO(voss)
		CodePageRange:      0,     // TODO(voss)
		Version:            version,
		CreationTime:       creationTime,
		ModificationTime:   modificationTime,
		Copyright:          t1Info.Info.Copyright,
		Trademark:          t1Info.Info.Notice,
		UnitsPerEm:         1000, // TODO(voss): get from font matrix
		Ascent:             ascent,
		Descent:            descent,
		LineGap:            (ascent + descent) / 5, // TODO(voss)
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

	return &otfInfo, nil
}

func makeLigatures(afm *afm.Info) *gtab.Info {
	if afm == nil || len(afm.Ligatures) == 0 {
		return nil
	}

	var ll []gtab.Ligature
	for pair, repl := range afm.Ligatures {
		ll = append(ll, gtab.Ligature{
			In:  []glyph.ID{pair.Left, pair.Right},
			Out: repl,
		})
	}

	// sort lexicographically by `In`
	sort.Slice(ll, func(i, j int) bool {
		a := ll[i].In
		b := ll[j].In
		k := len(a)
		if k > len(b) {
			k = len(b)
		}
		for i := 0; i < k; i++ {
			if a[i] != b[i] {
				return a[i] < b[i]
			}
		}
		return len(a) < len(b)
	})

	cov := coverage.Table{}
	var repl [][]gtab.Ligature
	start := -1
	var cur glyph.ID
	for i, l := range ll {
		if start >= 0 && l.In[0] == cur {
			continue
		}
		if start >= 0 {
			cov[cur] = len(repl)
			repl = append(repl, ll[start:i])
		}
		start = i
		cur = l.In[0]
	}
	if start >= 0 {
		cov[cur] = len(repl)
		repl = append(repl, ll[start:])
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
				Meta:      &gtab.LookupMetaInfo{},
				Subtables: []gtab.Subtable{subst},
			},
		},
	}
	return gsub
}

func makeKerningTable(afm *afm.Info) *gtab.Info {
	if afm == nil || len(afm.Kern) == 0 {
		return nil
	}

	all := make(map[glyph.ID]map[glyph.ID]*gtab.PairAdjust)
	for pair, adj := range afm.Kern {
		if _, exists := all[pair.Left]; !exists {
			all[pair.Left] = make(map[glyph.ID]*gtab.PairAdjust)
		}
		pa := &gtab.PairAdjust{
			First: &gtab.GposValueRecord{XAdvance: adj},
		}
		all[pair.Left][pair.Right] = pa
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
				Meta:      &gtab.LookupMetaInfo{},
				Subtables: []gtab.Subtable{kern},
			},
		},
	}
	return gpos
}

func writeOtf(outname string, info *sfnt.Info) error {
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
