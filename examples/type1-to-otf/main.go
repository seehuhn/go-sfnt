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
	"cmp"
	"flag"
	"fmt"
	"log"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
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
	// The output is dated by the Type 1 font, so that converting the same font
	// twice gives the same file.  Fonts without a date fall back to
	// SOURCE_DATE_EPOCH, and finally to the current time.
	fontDate := t1Info.CreationDate
	if fontDate.IsZero() {
		fontDate, err = sourceDateEpoch()
		if err != nil {
			return nil, err
		}
	}
	if fontDate.IsZero() {
		fontDate = time.Now()
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

	// conversion factor from units of 1/1000 em to font design units
	q := unitsPerEm / 1000

	// measurements taken from the font program itself; the glyph extents are
	// already in font design units
	var ascent float64
	var descent float64
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
	capHeight := t1Info.CapHeightPDF() * q
	xHeight := t1Info.XHeightPDF() * q

	// where the metrics give a usable value, it wins over the measurement
	if afm != nil {
		ascent = afmValue(afm.Ascent, q, ascent)
		descent = afmValue(afm.Descent, q, descent)
		capHeight = afmValue(afm.CapHeight, q, capHeight)
		xHeight = afmValue(afm.XHeight, q, xHeight)
	}

	minBaseLineSkip := math.Ceil(1.2 * unitsPerEm)
	if d := minBaseLineSkip - (ascent - descent); d > 0 {
		d1 := d / 3
		d2 := d - d1
		descent -= d1
		ascent += d2
	}

	gsub, ligSkipped := makeLigatures(afm, name2gid)
	gpos, kernSkipped := makeKerningTable(afm, name2gid, q)
	if ligSkipped > 0 {
		fmt.Fprintf(os.Stderr,
			"warning: skipped %d ligatures naming glyphs the font lacks\n", ligSkipped)
	}
	if kernSkipped > 0 {
		fmt.Fprintf(os.Stderr,
			"warning: skipped %d kerning pairs naming glyphs the font lacks\n", kernSkipped)
	}

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
		CreationTime:       fontDate,
		ModificationTime:   fontDate,
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

// afmValue converts x, a value from an AFM file in units of 1/1000 em, to font
// design units.  AFM files leave unknown values unset, and scaling can carry a
// value beyond what the font tables hold; both cases yield fallback instead.
func afmValue(x, q, fallback float64) float64 {
	if x == 0 {
		return fallback
	}
	y := x * q
	if r := math.Round(y); math.IsNaN(r) || r < math.MinInt16 || r > math.MaxInt16 {
		return fallback
	}
	return y
}

// sourceDateEpoch returns the date given by the SOURCE_DATE_EPOCH environment
// variable, or the zero time if the variable is unset.
func sourceDateEpoch() (time.Time, error) {
	s := os.Getenv("SOURCE_DATE_EPOCH")
	if s == "" {
		return time.Time{}, nil
	}
	secs, err := strconv.ParseInt(s, 10, 64)
	if err != nil || secs < 0 {
		return time.Time{}, fmt.Errorf("invalid SOURCE_DATE_EPOCH %q", s)
	}
	return time.Unix(secs, 0).UTC(), nil
}

func makeCmap(glyphNames []string) cmap.Subtable {
	// Glyph names which stand for no single character, including ".notdef",
	// get no entry.  Where several glyphs share a character, the first wins.
	canUseFormat4 := true
	codes := make(map[rune]glyph.ID)
	for gid, name := range glyphNames {
		rr := []rune(names.ToUnicode(name, ""))
		if len(rr) != 1 {
			continue
		}
		r := rr[0]
		if _, exists := codes[r]; exists {
			continue
		}
		if r > 0xFFFF {
			canUseFormat4 = false
		}
		codes[r] = glyph.ID(gid)
	}

	if canUseFormat4 {
		cmap := cmap.Format4{}
		for r, gid := range codes {
			cmap[uint16(r)] = gid
		}
		return cmap
	}

	cmap := cmap.Format12{}
	for r, gid := range codes {
		cmap[uint32(r)] = gid
	}
	return cmap
}

// makeLigatures builds the "liga" feature from the metrics.  The second return
// value counts the ligatures skipped because the font program lacks a glyph.
func makeLigatures(afm *afm.Metrics, name2gid map[string]glyph.ID) (*gtab.Info, int) {
	if afm == nil {
		return nil, 0
	}

	// The metrics may name glyphs the font program does not have: the two
	// come from separate files and need not agree.  Such entries are skipped,
	// since an unknown name would otherwise resolve to glyph ID 0 and quietly
	// attach the ligature to ".notdef".
	var skipped int
	ll := map[glyph.ID][]gtab.Ligature{}
	for left, g := range afm.Glyphs {
		a, leftOK := name2gid[left]
		for right, repl := range g.Ligatures {
			b, rightOK := name2gid[right]
			out, outOK := name2gid[repl]
			if !leftOK || !rightOK || !outOK {
				skipped++
				continue
			}
			ll[a] = append(ll[a], gtab.Ligature{
				In:  []glyph.ID{b},
				Out: out,
			})
		}
	}
	if len(ll) == 0 {
		return nil, skipped
	}

	// TODO(voss): merge this with the code in go-sfnt/ligatures.go

	keys := slices.Sorted(maps.Keys(ll))

	// the metrics are held in maps, so one glyph's ligatures arrive in no
	// particular order; sorting them makes the output file reproducible
	byInput := func(a, b gtab.Ligature) int {
		return cmp.Or(cmp.Compare(a.In[0], b.In[0]), cmp.Compare(a.Out, b.Out))
	}

	cov := coverage.Table{}
	var repl [][]gtab.Ligature
	for i, gid := range keys {
		cov[gid] = i
		entries := ll[gid]
		slices.SortFunc(entries, byInput)
		repl = append(repl, entries)
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
	return gsub, skipped
}

// makeKerningTable builds the "kern" feature from the metrics.  The adjustments
// are scaled from PDF glyph space units to font design units by q.  The second
// return value counts the pairs skipped because the font program lacks a glyph.
func makeKerningTable(afm *afm.Metrics, name2gid map[string]glyph.ID, q float64) (*gtab.Info, int) {
	if afm == nil || len(afm.Kern) == 0 {
		return nil, 0
	}

	// A pair which names a glyph the font program does not have is skipped,
	// for the reason makeLigatures gives.
	var skipped int
	kern := gtab.Gpos2_1{}
	for _, pair := range afm.Kern {
		left, leftOK := name2gid[pair.Left]
		right, rightOK := name2gid[pair.Right]
		if !leftOK || !rightOK {
			skipped++
			continue
		}
		// AFM sets no range for an adjustment, but a GPOS value record holds
		// an int16.  The comparison also drops NaN.
		adjust := math.Round(pair.Adjust * q)
		if !(adjust >= math.MinInt16 && adjust <= math.MaxInt16) {
			skipped++
			continue
		}
		kern[glyph.Pair{
			Left:  left,
			Right: right,
		}] = &gtab.PairAdjust{
			First: &gtab.GposValueRecord{
				XAdvance: funit.Int16(adjust),
			},
		}
	}
	if len(kern) == 0 {
		return nil, skipped
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
	return gpos, skipped
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
