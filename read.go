// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package sfnt

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"os"
	"slices"
	"strings"
	"sync"

	"golang.org/x/text/language"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/membudget"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/avar"
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/cvar"
	"seehuhn.de/go/sfnt/fvar"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/gvar"
	"seehuhn.de/go/sfnt/head"
	"seehuhn.de/go/sfnt/header"
	"seehuhn.de/go/sfnt/hmtx"
	"seehuhn.de/go/sfnt/hvar"
	"seehuhn.de/go/sfnt/kern"
	"seehuhn.de/go/sfnt/maxp"
	"seehuhn.de/go/sfnt/mvar"
	"seehuhn.de/go/sfnt/name"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/gdef"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/os2"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/post"
	"seehuhn.de/go/sfnt/stat"
	"seehuhn.de/go/sfnt/vmtx"
	"seehuhn.de/go/sfnt/vorg"
)

// ReadFile reads a TrueType or OpenType font from a file.
func ReadFile(fname string) (*Font, error) {
	fd, err := os.Open(fname)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	fi, err := fd.Stat()
	if err != nil {
		return nil, err
	}
	return Read(fd, parser.NewBudget(fi.Size()))
}

// Read reads a TrueType or OpenType font from an io.Reader.
// If r does not implement the io.ReaderAt interface, the whole
// font file will be read into memory.
//
// All memory allocated while parsing is charged against budget, which
// bounds the total a single call may use.  Use [parser.NewBudget] to size
// a budget in proportion to the input.
func Read(r io.Reader, budget *membudget.Budget) (*Font, error) {
	rr, ok := r.(io.ReaderAt)
	if !ok {
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		rr = bytes.NewReader(data)
	}

	dir, err := header.Read(rr)
	if err != nil {
		return nil, fmt.Errorf("sfnt header: %w", err)
	}

	if !(dir.Has("glyf", "loca") || dir.Has("CFF ") || dir.Has("CFF2")) {
		return nil, errors.New("sfnt: no TrueType/OpenType glyph data found")
	}

	// we try to read the tables in the order given by
	// https://docs.microsoft.com/en-us/typography/opentype/spec/recom#optimized-table-ordering

	var headInfo *head.Info
	headFd, err := dir.TableReader(rr, "head")
	if err != nil && !header.IsMissing(err) {
		return nil, err
	}
	if headFd != nil {
		headInfo, err = head.Read(headFd)
		if err != nil {
			return nil, fmt.Errorf("head table: %w", err)
		}
	}

	hheaData, err := dir.ReadTableBytes(rr, "hhea")
	if err != nil && !header.IsMissing(err) {
		return nil, err
	}
	// decoded below when reading "hmtx"

	var maxpInfo *maxp.Info
	maxpFd, err := dir.TableReader(rr, "maxp")
	if err != nil && !header.IsMissing(err) {
		return nil, err
	}
	if maxpFd != nil {
		maxpInfo, err = maxp.Read(maxpFd)
		if err != nil {
			return nil, fmt.Errorf("maxp table: %w", err)
		}
	}

	var os2Info *os2.Info
	os2Fd, err := dir.TableReader(rr, "OS/2")
	if err != nil && !header.IsMissing(err) {
		return nil, err
	}
	if os2Fd != nil {
		os2Info, err = os2.Read(os2Fd)
		if err != nil {
			return nil, fmt.Errorf("OS/2 table: %w", err)
		}
	}

	var hmtxInfo *hmtx.Info
	hmtxData, err := dir.ReadTableBytes(rr, "hmtx")
	if err != nil && !header.IsMissing(err) {
		return nil, err
	}
	if hheaData != nil {
		hmtxInfo, err = hmtx.Decode(hheaData, hmtxData)
		if err != nil {
			return nil, fmt.Errorf("hmtx table: %w", err)
		}
	}

	var cmapTable cmap.Table
	cmapData, err := dir.ReadTableBytes(rr, "cmap")
	if err != nil && !header.IsMissing(err) {
		return nil, err
	}
	if cmapData != nil {
		cmapTable, err = cmap.Decode(cmapData)
		if err != nil {
			// treat a malformed cmap as if absent
			cmapTable = nil
			err = nil
		}
	}

	var nameTable *name.Table
	nameData, err := dir.ReadTableBytes(rr, "name")
	if err != nil && !header.IsMissing(err) {
		return nil, err
	}
	if nameData != nil {
		nameInfo, err := name.Decode(nameData)
		if err != nil {
			return nil, fmt.Errorf("name table: %w", err)
		}
		winTab, winConf := nameInfo.Windows.Choose(language.AmericanEnglish)
		macTab, macConf := nameInfo.Mac.Choose(language.AmericanEnglish)
		primary, fallback, fallbackConf := winTab, macTab, macConf
		if (winConf < language.High && macConf > winConf) || winTab == nil {
			primary, fallback, fallbackConf = macTab, winTab, winConf
		}
		if fallbackConf < language.Low {
			// names in an unrelated language
			fallback = nil
		}
		nameTable = name.Merged(primary, fallback)
	}

	var postInfo *post.Info
	postFd, err := dir.TableReader(rr, "post")
	if err != nil && !header.IsMissing(err) {
		return nil, err
	}
	if postFd != nil {
		postInfo, err = post.Read(postFd, budget)
		if err != nil {
			return nil, fmt.Errorf("post table: %w", err)
		}
	}

	var numGlyphs int
	if maxpInfo != nil {
		numGlyphs = maxpInfo.NumGlyphs
	}
	if hmtxInfo != nil && len(hmtxInfo.Widths) > 0 {
		if numGlyphs == 0 {
			numGlyphs = len(hmtxInfo.Widths)
		} else if len(hmtxInfo.Widths) > numGlyphs {
			// Fix up a problem found in some fonts
			hmtxInfo.Widths = hmtxInfo.Widths[:numGlyphs]
		} else if len(hmtxInfo.Widths) != numGlyphs {
			return nil, errors.New("sfnt: hmtx and maxp glyph count mismatch")
		}
	}

	// Read the glyph data.
	var Outlines Outlines
	var fontInfo *type1.FontInfo
	var cff2Font *cff.FontCFF2
	var cff2UPM float64
	switch {
	case dir.Has("CFF2"):
		// CFF2 outlines are preferred when present (the spec forbids a font
		// from also carrying "CFF "/"glyf").  CFF2 appears under the OTTO
		// scaler type, but we accept it under any scaler type.
		cff2Fd, err := dir.TableReader(rr, "CFF2")
		if err != nil {
			return nil, err
		}
		cff2Font, err = cff.ReadCFF2(cff2Fd, budget)
		if err != nil {
			return nil, fmt.Errorf("CFF2 table: %w", err)
		}
		o := cff2Font.OutlinesCFF2
		Outlines = o

		cff2UPM = unitsPerEmFrom(headInfo, o.GlyphMatrix(cff2Font.FontMatrix, 0))

		if numGlyphs != 0 && len(o.Glyphs) != numGlyphs {
			return nil, errors.New("sfnt: cff2 glyph count mismatch")
		}
		// A font without usable metrics is written out with one zero width per
		// glyph, so the widths are materialized even when hmtx supplies none;
		// otherwise a write/read cycle would turn the nil into zeros.
		o.Widths = make([]float64, len(o.Glyphs))
		if hmtxInfo != nil && len(hmtxInfo.Widths) > 0 {
			// hmtx widths are in UnitsPerEm.  Convert to CFF2 design units
			// (which is what OutlinesCFF2.Widths holds) using each glyph's
			// effective font matrix.
			for i, h := range hmtxInfo.Widths {
				q := o.GlyphAdvanceScale(cff2Font.FontMatrix, glyph.ID(i))
				o.Widths[i] = advanceFromHmtx(h, q, cff2UPM)
			}
		}
	case dir.ScalerType == header.ScalerTypeCFF:
		var cffInfo *cff.Font
		cffFd, err := dir.TableReader(rr, "CFF ")
		if err != nil {
			return nil, err
		}
		cffInfo, err = cff.Read(cffFd, budget)
		if err != nil {
			return nil, fmt.Errorf("CFF table: %w", err)
		}
		fontInfo = cffInfo.FontInfo
		Outlines = cffInfo.Outlines

		if numGlyphs != 0 && len(cffInfo.Glyphs) != numGlyphs {
			return nil, errors.New("sfnt: cff glyph count mismatch")
		}

		// The hmtx table is where an OpenType font keeps its advance widths,
		// and the writer always stores them there, in units of UnitsPerEm.
		// A charstring width is kept if the writer would turn it into the
		// hmtx width, so that fractional widths survive; otherwise the hmtx
		// width is authoritative.  Where the table is missing, as in a font
		// embedded in a PDF file, the hmtx width is the one the writer would
		// store, and a charstring width the table cannot hold, a negative
		// one for example, is dropped.
		upm := deriveUnitsPerEm(cffInfo.Outlines, cffInfo.FontInfo, headInfo)
		for i, g := range cffInfo.Glyphs {
			q := cffInfo.Outlines.GlyphAdvanceScale(cffInfo.FontInfo.FontMatrix, glyph.ID(i))
			h, ok := hmtxAdvance(g.Width*q, upm)
			if hmtxInfo != nil && len(hmtxInfo.Widths) > 0 {
				ok = ok && h == hmtxInfo.Widths[i]
				h = hmtxInfo.Widths[i]
			}
			if !ok {
				g.Width = advanceFromHmtx(h, q, upm)
			}
		}
	case dir.ScalerType == header.ScalerTypeTrueType, dir.ScalerType == header.ScalerTypeApple:
		if headInfo == nil {
			return nil, &header.ErrMissing{TableName: "head"}
		}
		if maxpInfo == nil {
			return nil, &header.ErrMissing{TableName: "maxp"}
		}

		locaData, err := dir.ReadTableBytes(rr, "loca")
		if err != nil {
			return nil, err
		}
		glyfData, err := dir.ReadTableBytes(rr, "glyf")
		if err != nil {
			return nil, err
		}
		enc := &glyf.Encoded{
			GlyfData:   glyfData,
			LocaData:   locaData,
			LocaFormat: headInfo.LocaFormat,
		}
		ttGlyphs, err := glyf.Decode(enc)
		if err != nil {
			return nil, fmt.Errorf("glyf table: %w", err)
		}

		tables := make(map[string][]byte)
		for _, name := range []string{"cvt ", "fpgm", "prep", "gasp"} {
			if !dir.Has(name) {
				continue
			}
			data, err := dir.ReadTableBytes(rr, name)
			if err != nil {
				return nil, err
			}
			tables[name] = data
		}

		if numGlyphs != 0 && len(ttGlyphs) != numGlyphs {
			return nil, errors.New("sfnt: ttf glyph count mismatch")
		}

		// A font without usable metrics is written out with one zero width
		// per glyph, so the widths are materialized here rather than left
		// nil; otherwise a write/read cycle would turn the nil into zeros.
		widths := make([]funit.Uint16, len(ttGlyphs))
		if hmtxInfo != nil && len(hmtxInfo.Widths) > 0 {
			widths = hmtxInfo.Widths
		}

		var names []string
		if postInfo != nil && postInfo.Names != nil {
			names = make([]string, len(ttGlyphs))
			// excess names are dropped, missing names are left blank
			copy(names, postInfo.Names)
		}
		Outlines = &glyf.Outlines{
			Widths: widths,
			Glyphs: ttGlyphs,
			Tables: tables,
			Maxp:   maxpInfo.TTF,
			Names:  names,
		}
	default:
		panic("unexpected scaler type")
	}

	// glyph IDs must be valid before anything looks them up
	cmapTable = dropInvalidCMapGlyphs(cmapTable, Outlines.NumGlyphs())

	// decoded only if needed, since large subtables are expensive to decode
	cmapBest := sync.OnceValue(func() cmap.Subtable {
		c, _ := cmapTable.GetBest()
		return c
	})

	// Merge the information from the various tables.
	info := &Font{
		Outlines:  Outlines,
		CMapTable: cmapTable,
	}

	if nameTable != nil {
		info.FamilyName = nameTable.Family
		info.Subfamily = nameTable.Subfamily
		info.FullName = nameTable.FullName
	}
	if info.FamilyName == "" && fontInfo != nil {
		info.FamilyName = fontInfo.FamilyName
	}
	if info.FullName == "" && fontInfo != nil {
		info.FullName = fontInfo.FullName
	}
	// Entry 6 of the "name" table is the PostScript name of the font, so a
	// well-formed entry wins.  A CFF font program names itself as well, and the
	// two need not agree: one CFF font program may be shared between several
	// fonts, in which case its name belongs to only one of them.  The CFF name
	// is used where entry 6 is missing, as in an OpenType/CFF font embedded in a
	// PDF file, where the "name" table need not be present, and where entry 6
	// holds something it is not allowed to hold.
	if nameTable != nil && canBeNameID6(nameTable.PostScriptName) {
		info.FontName = nameTable.PostScriptName
	}
	if info.FontName == "" && fontInfo != nil {
		info.FontName = info.repairFontName(fontInfo.FontName)
	}
	if info.FontName == "" && nameTable != nil {
		info.FontName = info.repairFontName(nameTable.PostScriptName)
	}
	if os2Info != nil {
		info.Width = os2Info.WidthClass
		info.Weight = os2Info.WeightClass
	}
	if info.Weight == 0 && fontInfo != nil {
		info.Weight = os2.WeightFromString(fontInfo.Weight)
	}

	if nameTable != nil {
		info.Description = nameTable.Description
		info.SampleText = nameTable.SampleText
	}

	if ver, ok := getNameTableVersion(nameTable); ok {
		info.Version = ver
	} else if headInfo != nil {
		info.Version = headInfo.FontRevision.Round()
	} else if ver, ok := getCFFVersion(fontInfo); ok {
		info.Version = ver
	}
	if headInfo != nil {
		info.CreationTime = headInfo.Created
		info.ModificationTime = headInfo.Modified
	}

	if nameTable != nil {
		info.Copyright = nameTable.Copyright
		info.Trademark = nameTable.Trademark
		info.License = nameTable.License
		info.LicenseURL = nameTable.LicenseURL
	} else if fontInfo != nil {
		info.Copyright = fontInfo.Copyright
		info.Trademark = fontInfo.Notice
	}
	if os2Info != nil {
		info.PermUse = os2Info.PermUse
	}

	if cff2Font != nil {
		info.UnitsPerEm = uint16(math.Round(cff2UPM))
		info.FontMatrix = cff2Font.FontMatrix
	} else {
		info.UnitsPerEm = uint16(math.Round(deriveUnitsPerEm(Outlines, fontInfo, headInfo)))
		if fontInfo != nil {
			info.FontMatrix = fontInfo.FontMatrix
		} else {
			q := 1 / float64(info.UnitsPerEm)
			info.FontMatrix = [6]float64{q, 0, 0, q, 0, 0}
		}
	}

	if hmtxInfo != nil {
		info.HheaAscent = hmtxInfo.Ascent
		info.HheaDescent = hmtxInfo.Descent
		info.HheaLineGap = hmtxInfo.LineGap
	}
	if os2Info != nil {
		info.Ascent = os2Info.Ascent
		info.Descent = os2Info.Descent
		info.LineGap = os2Info.LineGap
		info.WinAscent = os2Info.WinAscent
		info.WinDescent = os2Info.WinDescent
	} else if hmtxInfo != nil {
		info.Ascent = hmtxInfo.Ascent
		info.Descent = hmtxInfo.Descent
		info.LineGap = hmtxInfo.LineGap
	}
	// make the write-time fallbacks explicit so that a round trip is stable
	info.HheaAscent, info.HheaDescent, info.HheaLineGap = info.hheaMetrics()
	info.WinAscent, info.WinDescent = info.winMetrics()

	// vertical metrics are optional, so tables which cannot be used are
	// ignored rather than failing the read
	if dir.Has("vhea", "vmtx") {
		vheaData, err1 := dir.ReadTableBytes(rr, "vhea")
		vmtxData, err2 := dir.ReadTableBytes(rr, "vmtx")
		var vorgData []byte
		if !info.IsGlyf() && dir.Has("VORG") {
			vorgData, _ = dir.ReadTableBytes(rr, "VORG")
		}
		if err1 == nil && err2 == nil {
			info.Vertical = info.readVertical(vheaData, vmtxData, vorgData)
		}
	}

	if os2Info != nil {
		info.CapHeight = os2Info.CapHeight
		info.XHeight = os2Info.XHeight
	}
	if info.CapHeight == 0 {
		if c := cmapBest(); c != nil {
			info.CapHeight = info.measureHeight(c, type1.CapHeightChars)
		}
	}
	if info.XHeight == 0 {
		if c := cmapBest(); c != nil {
			info.XHeight = info.measureHeight(c, type1.XHeightChars)
		}
	}

	if postInfo != nil {
		info.ItalicAngle = postInfo.ItalicAngle
	} else if fontInfo != nil {
		info.ItalicAngle = fontInfo.ItalicAngle
	} else if hmtxInfo != nil {
		info.ItalicAngle = hmtxInfo.CaretAngle * 180 / math.Pi
	}
	// Round the italic angle so that the value can be exactly represented
	// in the post table.
	info.ItalicAngle = math.Round(info.ItalicAngle*65536) / 65536

	if postInfo != nil {
		info.UnderlinePosition = funit.Float64(postInfo.UnderlinePosition)
		info.UnderlineThickness = funit.Float64(postInfo.UnderlineThickness)
	} else if fontInfo != nil {
		info.UnderlinePosition = fontInfo.UnderlinePosition
		info.UnderlineThickness = fontInfo.UnderlineThickness
	}

	// Currently we set IsItalic if there is any evidence of the font being
	// slanted.  Are we too eager setting this?
	// TODO(voss): check that this gives good results
	info.IsItalic = info.ItalicAngle != 0
	if headInfo != nil && headInfo.IsItalic {
		info.IsItalic = true
	}
	if os2Info != nil && (os2Info.IsItalic || os2Info.IsOblique) {
		info.IsItalic = true
	}
	if nameTable != nil && strings.Contains(nameTable.Subfamily, "Italic") {
		info.IsItalic = true
	}

	if os2Info != nil {
		info.IsOblique = os2Info.IsOblique
	}

	if os2Info != nil {
		info.IsBold = os2Info.IsBold
	} else if headInfo != nil {
		info.IsBold = headInfo.IsBold
	}
	// TODO(voss): we could also check info.Weight == font.WeightBold
	if nameTable != nil &&
		strings.Contains(nameTable.Subfamily, "Bold") &&
		!strings.Contains(nameTable.Subfamily, "Semi Bold") &&
		!strings.Contains(nameTable.Subfamily, "Extra Bold") {
		info.IsBold = true
	}

	if !(info.IsItalic || info.IsBold) {
		if os2Info != nil {
			info.IsRegular = os2Info.IsRegular
		}
		// if nameTable != nil && nameTable.Subfamily == "Regular" {
		// 	info.IsRegular = true
		// }
	}

	if os2Info != nil {
		switch os2Info.FamilyClass >> 8 {
		case 1, 2, 3, 4, 5, 7:
			info.IsSerif = true
		case 10:
			info.IsScript = true
		}
	}

	if os2Info != nil {
		info.CodePageRange = os2Info.CodePageRange
		info.UnicodeRange = os2Info.UnicodeRange
	}

	if dir.Has("GDEF") {
		gdefFd, err := dir.TableReader(rr, "GDEF")
		if err == nil {
			info.Gdef, err = gdef.Read(gdefFd, budget)
			if err != nil {
				// skip malformed GDEF table
				info.Gdef = nil
			}
		}
	}

	// TODO(voss): should we try to read the "morx" table, if there is no
	// "GSUB" table?
	if dir.Has("GSUB") {
		gsubFd, err := dir.TableReader(rr, "GSUB")
		if err == nil {
			info.Gsub, err = gtab.Read(gsubFd, budget, gtab.TypeGsub)
			if err != nil {
				// skip malformed GSUB table
				info.Gsub = nil
			}
		}
	}
	dropInvalidGsubGlyphs(info.Gsub, info.NumGlyphs())
	if info.Gsub == nil && !info.IsFixedPitch() {
		if c := cmapBest(); c != nil {
			info.Gsub = standardLigatures(c)
		}
	}

	if dir.Has("GPOS") {
		gposFd, err := dir.TableReader(rr, "GPOS")
		if err == nil {
			info.Gpos, err = gtab.Read(gposFd, budget, gtab.TypeGpos)
			if err != nil {
				// skip malformed GPOS table
				info.Gpos = nil
			}
		}
	}
	if info.Gpos == nil && dir.Has("kern") {
		kernFd, err := dir.TableReader(rr, "kern")
		if err == nil {
			kern, err := kern.Read(kernFd, budget)
			if err == nil {
				subtable := &gtab.Gpos2_1{Adjust: map[glyph.Pair]*gtab.PairAdjust{}}
				for pair, val := range kern {
					subtable.Adjust[pair] = &gtab.PairAdjust{
						First: &gtab.GposValueRecord{XAdvance: val},
					}
				}
				info.Gpos = &gtab.Info{
					ScriptList: map[language.Tag]*gtab.Features{
						language.MustParse("und-Zzzz"): {Required: 0, Optional: []gtab.FeatureIndex{}},
					},
					FeatureList: []*gtab.Feature{
						{Tag: "kern", Lookups: []gtab.LookupIndex{0}},
					},
					LookupList: []*gtab.LookupTable{
						{
							Meta:      &gtab.LookupMetaInfo{LookupType: 2},
							Subtables: []gtab.Subtable{subtable},
						},
					},
				}
			}
		}
	}

	// Variation tables.  fvar is the gate: without it (or with no axes) the
	// font is not variable and the remaining tables are skipped.  Each table
	// is dropped silently on its own parse error; the font still loads.
	if dir.Has("fvar") {
		fvarFd, err := dir.TableReader(rr, "fvar")
		if err == nil {
			info.Fvar, err = fvar.Read(fvarFd, budget)
			if err != nil {
				info.Fvar = nil
			}
		}
	}
	if info.Fvar != nil && len(info.Fvar.Axes) == 0 {
		info.Fvar = nil
	}
	if info.Fvar != nil {
		axisCount := len(info.Fvar.Axes)

		// resolve informational names and retain name ID 25
		if nameTable != nil {
			for i := range info.Fvar.Axes {
				info.Fvar.Axes[i].Name = nameTable.Get(name.ID(info.Fvar.Axes[i].NameID))
			}
			for i := range info.Fvar.Instances {
				inst := &info.Fvar.Instances[i]
				inst.Name = nameTable.Get(name.ID(inst.NameID))
				if inst.PostScriptNameID != 0xFFFF {
					inst.PostScriptName = repairNameID6(nameTable.Get(name.ID(inst.PostScriptNameID)))
				}
			}
			info.VariationsPostScriptName = repairVariationsName(nameTable.VariationsPostScriptName)
		}

		if dir.Has("avar") {
			avarFd, err := dir.TableReader(rr, "avar")
			if err == nil {
				info.Avar, err = avar.Read(avarFd, budget)
				if err != nil {
					info.Avar = nil
				}
			}
		}
		// a version 1 avar must supply one segment map per axis
		if info.Avar != nil && info.Avar.IsSupported() && len(info.Avar.SegmentMaps) != axisCount {
			info.Avar = nil
		}

		if dir.Has("STAT") {
			statFd, err := dir.TableReader(rr, "STAT")
			if err == nil {
				info.Stat, err = stat.Read(statFd, budget)
				if err != nil {
					info.Stat = nil
				}
			}
		}
		if info.Stat != nil && nameTable != nil {
			resolveStatNames(info.Stat, nameTable)
		}

		// canonicalize the NameID fields so they match what Write would
		// assign; this keeps read-write-read round trips stable even when
		// the file's stored IDs use a different numbering.
		canonicalizeVariationNames(info.Fvar, info.Stat)

		// gvar applies only to glyf outlines
		_, isGlyf := info.Outlines.(*glyf.Outlines)
		if isGlyf && dir.Has("gvar") {
			gvarData, err := dir.ReadTableBytes(rr, "gvar")
			if err == nil {
				info.Gvar, err = gvar.Decode(gvarData, budget)
				if err != nil {
					info.Gvar = nil
				}
			}
		}
		if info.Gvar != nil {
			if info.Gvar.AxisCount != axisCount {
				info.Gvar = nil
			} else {
				// pad or truncate the per-glyph blocks to the real glyph
				// count (permissive)
				n := info.NumGlyphs()
				if len(info.Gvar.PerGlyph) > n {
					info.Gvar.PerGlyph = info.Gvar.PerGlyph[:n]
				} else if len(info.Gvar.PerGlyph) < n {
					padded := make([]gvar.GlyphData, n)
					copy(padded, info.Gvar.PerGlyph)
					info.Gvar.PerGlyph = padded
				}
			}
		}

		// cvar applies only to glyf fonts with a "cvt " table
		if glyfOutlines, ok := info.Outlines.(*glyf.Outlines); ok && dir.Has("cvar") {
			if cvt := glyfOutlines.Tables["cvt "]; cvt != nil {
				cvarData, err := dir.ReadTableBytes(rr, "cvar")
				if err == nil {
					info.Cvar, err = cvar.Decode(cvarData, axisCount, len(cvt)/2, budget)
					if err != nil {
						info.Cvar = nil
					}
				}
			}
		}

		if dir.Has("HVAR") {
			hvarFd, err := dir.TableReader(rr, "HVAR")
			if err == nil {
				info.Hvar, err = hvar.Read(hvarFd, budget)
				if err != nil {
					info.Hvar = nil
				}
			}
		}

		if dir.Has("MVAR") {
			mvarFd, err := dir.TableReader(rr, "MVAR")
			if err == nil {
				info.Mvar, err = mvar.Read(mvarFd, budget)
				if err != nil {
					info.Mvar = nil
				}
			}
		}
	}

	return info, nil
}

// readVertical builds the vertical metrics of the font from the "vhea",
// "vmtx" and "VORG" tables.  The outlines, UnitsPerEm and FontMatrix of f
// must already be set.  vorgData may be nil.
//
// The result is nil if the tables cannot be decoded, if "vmtx" has fewer
// entries than the font has glyphs, or if a vertical origin or a top side
// bearing does not fit into 16 bits.  Excess "vmtx" entries are ignored.  The
// vertical origin is taken from "VORG" where that table can be decoded, and
// is derived from the top side bearing otherwise.
func (f *Font) readVertical(vheaData, vmtxData, vorgData []byte) *VerticalMetrics {
	info, err := vmtx.Decode(vheaData, vmtxData)
	if err != nil {
		return nil
	}
	n := f.NumGlyphs()
	if len(info.Heights) < n {
		return nil
	}

	var origins *vorg.Table
	if vorgData != nil {
		origins, err = vorg.Decode(vorgData)
		if err != nil {
			origins = nil
		}
	}

	originY := make([]funit.Int16, n)
	for i := range n {
		gid := glyph.ID(i)
		top := f.glyphTop(gid)
		var y int
		if origins != nil {
			y = int(origins.Get(gid))
		} else {
			y = top + int(info.TSB[i])
		}
		if !fitsInt16(y) || !fitsInt16(y-top) {
			return nil
		}
		originY[i] = funit.Int16(y)
	}

	return &VerticalMetrics{
		Ascent:         info.Ascent,
		Descent:        info.Descent,
		LineGap:        info.LineGap,
		CaretSlopeRise: info.CaretSlopeRise,
		CaretSlopeRun:  info.CaretSlopeRun,
		CaretOffset:    info.CaretOffset,
		Heights:        info.Heights[:n],
		OriginY:        originY,
	}
}

// fitsInt16 reports whether x can be stored in a signed 16-bit integer.
func fitsInt16(x int) bool {
	return x >= math.MinInt16 && x <= math.MaxInt16
}

// resolveStatNames fills in the informational Name fields of a STAT table from
// the "name" table.
func resolveStatNames(t *stat.Table, nameTable *name.Table) {
	for i := range t.DesignAxes {
		t.DesignAxes[i].Name = nameTable.Get(name.ID(t.DesignAxes[i].NameID))
	}
	for _, av := range t.AxisValues {
		switch v := av.(type) {
		case *stat.Format1:
			v.Name = nameTable.Get(name.ID(v.NameID))
		case *stat.Format2:
			v.Name = nameTable.Get(name.ID(v.NameID))
		case *stat.Format3:
			v.Name = nameTable.Get(name.ID(v.NameID))
		case *stat.Format4:
			v.Name = nameTable.Get(name.ID(v.NameID))
		}
	}
	t.ElidedFallbackName = nameTable.Get(name.ID(t.ElidedFallbackNameID))
}

func getNameTableVersion(t *name.Table) (head.Version, bool) {
	if t == nil {
		return 0, false
	}
	v, err := head.VersionFromString(t.Version)
	if err != nil {
		return 0, false
	}
	return v, true
}

func getCFFVersion(fontInfo *type1.FontInfo) (head.Version, bool) {
	if fontInfo == nil || fontInfo.Version == "" {
		return 0, false
	}
	v, err := head.VersionFromString(fontInfo.Version)
	if err != nil {
		return 0, false
	}
	return v, true
}

// unitsPerEmFrom picks the best available source for UnitsPerEm.  The head
// table wins when present; otherwise the value is derived from the effective
// glyph matrix m, a zero matrix standing for "no matrix available".  A value a
// font file cannot store is skipped, since every glyph metric is measured in
// these units.  Fallback is the conventional 1000.
//
// The result is a whole number, since the widths read are converted using the
// value the font will be written with.
func unitsPerEmFrom(headInfo *head.Info, m matrix.Matrix) float64 {
	if headInfo != nil && validUnitsPerEm(float64(headInfo.UnitsPerEm)) {
		return float64(headInfo.UnitsPerEm)
	}
	if m[0] != 0 && validUnitsPerEm(1/m[0]) {
		return math.Round(1 / m[0])
	}
	return 1000
}

// deriveUnitsPerEm is unitsPerEmFrom for a font whose matrix comes from a CFF
// table.  For CID-keyed CFF the top FontMatrix is typically the identity, so
// it is composed with FD 0's per-FD matrix to recover a sensible scale.
func deriveUnitsPerEm(outlines Outlines, fontInfo *type1.FontInfo, headInfo *head.Info) float64 {
	var fm matrix.Matrix
	if fontInfo != nil {
		fm = fontInfo.FontMatrix
		if cffOutlines, ok := outlines.(*cff.Outlines); ok {
			fm = cffOutlines.GlyphMatrix(fm, 0)
		}
	}
	return unitsPerEmFrom(headInfo, fm)
}

// dropInvalidCMapGlyphs removes the mappings to glyphs which do not exist in
// the font, so that every glyph ID found via the cmap table can be used to
// index the font's glyphs.  Subtables which need no change are kept as they
// are; subtables in formats which cannot be decoded are never used to look up
// glyphs and are also kept.
func dropInvalidCMapGlyphs(table cmap.Table, numGlyphs int) cmap.Table {
	// Fonts often store identical subtables under several keys, e.g. for
	// the Unicode and Windows platforms.  Subtables found to need no
	// change are recorded here, so that copies are not decoded again.
	var clean [][]byte

	for _, key := range sortedCMapKeys(table) {
		data := table[key]
		if slices.ContainsFunc(clean, func(d []byte) bool { return bytes.Equal(d, data) }) {
			continue
		}
		c, err := cmap.Table{key: data}.GetNoLang(key.PlatformID, key.EncodingID)
		if err != nil {
			continue
		}

		changed := false
		switch c := c.(type) {
		case *cmap.Format0:
			for i, g := range c.Data {
				if int(g) >= numGlyphs {
					c.Data[i] = 0
					changed = true
				}
			}
			if changed {
				table[key] = c.Encode(key.Language)
			}
		case *cmap.Format6:
			for i, g := range c.GlyphIDs {
				if int(g) >= numGlyphs {
					c.GlyphIDs[i] = 0
					changed = true
				}
			}
			if changed {
				table[key] = c.Encode(key.Language)
			}
		case cmap.Format4:
			// removing mappings can split segments, so the result may
			// not fit into a format 4 subtable
			for code, g := range c {
				if int(g) >= numGlyphs {
					delete(c, code)
					changed = true
				}
			}
			if changed {
				delete(table, key)
				putFormat4(table, key, c, table)
			}
		case cmap.Format12:
			for code, g := range c {
				if int(g) >= numGlyphs {
					delete(c, code)
					changed = true
				}
			}
			if changed {
				table[key] = c.Encode(key.Language)
			}
		}
		if !changed {
			clean = append(clean, data)
		}
	}
	return table
}

// dropInvalidGsubGlyphs removes the GSUB substitutions which produce glyphs
// not present in the font, so that applying the lookups never yields an
// invalid glyph ID.  An alternate set keeps its valid alternates and a
// ligature set its valid ligatures; all other substitutions with an invalid
// output are removed as a whole.  Subtables left without any rules are
// removed from their lookup.
//
// Rules which match missing glyphs are kept.  They never fire, since neither
// the cmap table nor the remaining substitutions produce such glyphs.
func dropInvalidGsubGlyphs(info *gtab.Info, numGlyphs int) {
	if info == nil {
		return
	}

	valid := func(gid glyph.ID) bool { return int(gid) < numGlyphs }
	allValid := func(gids []glyph.ID) bool {
		return !slices.ContainsFunc(gids, func(gid glyph.ID) bool { return !valid(gid) })
	}
	validLig := func(lig gtab.Ligature) bool { return valid(lig.Out) }
	allValidLigs := func(ligs []gtab.Ligature) bool {
		return !slices.ContainsFunc(ligs, func(lig gtab.Ligature) bool { return !validLig(lig) })
	}
	nonEmpty := func(x []glyph.ID) bool { return len(x) > 0 }
	nonEmptyLigs := func(x []gtab.Ligature) bool { return len(x) > 0 }

	for _, lookup := range info.LookupList {
		for _, subtable := range lookup.Subtables {
			switch l := subtable.(type) {
			case *gtab.Gsub1_1:
				for gid := range l.Cov {
					if !valid(gid + l.Delta) {
						delete(l.Cov, gid)
					}
				}
			case *gtab.Gsub1_2:
				l.Cov, l.SubstituteGlyphIDs = filterCoverage(l.Cov, l.SubstituteGlyphIDs, valid)
			case *gtab.Gsub2_1:
				l.Cov, l.Repl = filterCoverage(l.Cov, l.Repl, allValid)
			case *gtab.Gsub3_1:
				if !slices.ContainsFunc(l.Alternates, func(alts []glyph.ID) bool { return !allValid(alts) }) {
					continue
				}
				alternates := make([][]glyph.ID, len(l.Alternates))
				for i, alts := range l.Alternates {
					alternates[i] = slices.DeleteFunc(slices.Clone(alts),
						func(gid glyph.ID) bool { return !valid(gid) })
				}
				l.Cov, l.Alternates = filterCoverage(l.Cov, alternates, nonEmpty)
			case *gtab.Gsub4_1:
				if !slices.ContainsFunc(l.Repl, func(ligs []gtab.Ligature) bool { return !allValidLigs(ligs) }) {
					continue
				}
				repl := make([][]gtab.Ligature, len(l.Repl))
				for i, ligs := range l.Repl {
					repl[i] = slices.DeleteFunc(slices.Clone(ligs),
						func(lig gtab.Ligature) bool { return !validLig(lig) })
				}
				l.Cov, l.Repl = filterCoverage(l.Cov, repl, nonEmptyLigs)
			case *gtab.Gsub8_1:
				l.Input, l.SubstituteGlyphIDs = filterCoverage(l.Input, l.SubstituteGlyphIDs, valid)
			}
		}
		lookup.Subtables = slices.DeleteFunc(lookup.Subtables, isEmptyGsub)
	}
}

// isEmptyGsub reports whether a non-contextual GSUB subtable has no rules.
func isEmptyGsub(subtable gtab.Subtable) bool {
	switch l := subtable.(type) {
	case *gtab.Gsub1_1:
		return len(l.Cov) == 0
	case *gtab.Gsub1_2:
		return len(l.Cov) == 0
	case *gtab.Gsub2_1:
		return len(l.Cov) == 0
	case *gtab.Gsub3_1:
		return len(l.Cov) == 0
	case *gtab.Gsub4_1:
		return len(l.Cov) == 0
	case *gtab.Gsub8_1:
		return len(l.Input) == 0
	default:
		return false
	}
}

// filterCoverage restricts a coverage table, and the data indexed by its
// coverage indices, to the entries for which keep returns true.  If entries
// are removed, the remaining ones are renumbered in order of increasing glyph
// ID; otherwise cov and data are returned unchanged.
func filterCoverage[T any](cov coverage.Table, data []T, keep func(T) bool) (coverage.Table, []T) {
	if !slices.ContainsFunc(data, func(x T) bool { return !keep(x) }) {
		return cov, data
	}

	newCov := make(coverage.Table, len(cov))
	newData := make([]T, 0, len(cov))
	for _, gid := range cov.Glyphs() {
		x := data[cov[gid]]
		if keep(x) {
			newCov[gid] = len(newData)
			newData = append(newData, x)
		}
	}
	return newCov, newData
}

// putFormat4 stores the mapping c, decoded from the subtable with the given
// key, in the table out.
//
// A format 4 subtable is limited to 16-bit offsets and length.  If c does not
// fit, the mapping is stored as a format 12 subtable under the platform's
// full-repertoire encoding instead.  The mapping is left out if the platform
// has no such encoding, or if the new key is already used in orig or out.
// Callers visit the keys in sorted order, so that the result does not depend
// on map iteration order when several subtables compete for the same key.
func putFormat4(out cmap.Table, key cmap.Key, c cmap.Format4, orig cmap.Table) {
	data, err := c.TryEncode(key.Language)
	if err == nil {
		out[key] = data
		return
	}

	newKey, ok := format12Key(key)
	if !ok {
		return
	}
	if newKey != key {
		if _, taken := orig[newKey]; taken {
			return
		}
		if _, taken := out[newKey]; taken {
			return
		}
	}
	f12 := make(cmap.Format12, len(c))
	for code, g := range c {
		f12[uint32(code)] = g
	}
	out[newKey] = f12.Encode(key.Language)
}

// format12Key returns the key under which a Unicode subtable beyond the BMP
// is stored for the given platform and encoding.  The second return value is
// false if the platform has no such encoding.
func format12Key(key cmap.Key) (cmap.Key, bool) {
	switch key.PlatformID {
	case 0:
		key.EncodingID = 4
	case 3:
		if key.EncodingID != 1 && key.EncodingID != 10 {
			return cmap.Key{}, false
		}
		key.EncodingID = 10
	default:
		return cmap.Key{}, false
	}
	return key, true
}

// sortedCMapKeys returns the keys of the table, ordered by platform ID,
// encoding ID and language.
func sortedCMapKeys(table cmap.Table) []cmap.Key {
	keys := slices.Collect(maps.Keys(table))
	slices.SortFunc(keys, func(a, b cmap.Key) int {
		return cmp.Or(
			cmp.Compare(a.PlatformID, b.PlatformID),
			cmp.Compare(a.EncodingID, b.EncodingID),
			cmp.Compare(a.Language, b.Language))
	})
	return keys
}
