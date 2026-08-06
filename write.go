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

package sfnt

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"time"

	"seehuhn.de/go/postscript/funit"

	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/fvar"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/head"
	"seehuhn.de/go/sfnt/header"
	"seehuhn.de/go/sfnt/hmtx"
	"seehuhn.de/go/sfnt/maxp"
	"seehuhn.de/go/sfnt/name"
	"seehuhn.de/go/sfnt/os2"
	"seehuhn.de/go/sfnt/post"
	"seehuhn.de/go/sfnt/stat"
)

// Write writes the binary form of the font to the given writer.
func (f *Font) Write(w io.Writer) (int64, error) {
	if err := f.checkPSNames(); err != nil {
		return 0, err
	}

	tableData := make(map[string][]byte)

	// name-ID coordination for the variation tables must precede makeName so
	// that the allocated strings can be registered in the "name" table.  The
	// returned tables are clones; the caller's Font is never mutated.
	fvarTable, statTable, nameExtra := f.assignVariationNameIDs()

	hheaData, hmtxData := f.makeHmtx()
	tableData["hhea"] = hheaData
	tableData["hmtx"] = hmtxData

	if f.CMapTable != nil {
		tableData["cmap"] = f.CMapTable.Encode()
	}

	tableData["OS/2"] = f.makeOS2()
	tableData["name"] = f.makeName(nameExtra)
	postData, err := f.makePost()
	if err != nil {
		return 0, err
	}
	tableData["post"] = postData

	var locaFormat int16
	var scalerType uint32
	var maxpTtf *maxp.TTFInfo
	switch outlines := f.Outlines.(type) {
	case *cff.Outlines:
		cffData, err := f.makeCFF(outlines)
		if err != nil {
			return 0, err
		}
		tableData["CFF "] = cffData
		scalerType = header.ScalerTypeCFF
	case *cff.OutlinesCFF2:
		cff2Data, err := f.makeCFF2(outlines)
		if err != nil {
			return 0, err
		}
		tableData["CFF2"] = cff2Data
		scalerType = header.ScalerTypeCFF
	case *glyf.Outlines:
		enc := outlines.Glyphs.Encode()
		tableData["glyf"] = enc.GlyfData
		tableData["loca"] = enc.LocaData
		locaFormat = enc.LocaFormat
		maps.Copy(tableData, outlines.Tables)
		scalerType = header.ScalerTypeTrueType
		maxpTtf = outlines.Maxp
	default:
		panic("unexpected font type")
	}

	maxpInfo := &maxp.Info{
		NumGlyphs: f.NumGlyphs(),
		TTF:       maxpTtf,
	}
	tableData["maxp"] = maxpInfo.Encode()

	tableData["head"] = f.makeHead(locaFormat)

	if f.Gdef != nil {
		tableData["GDEF"] = f.Gdef.Encode()
	}
	if f.Gsub != nil {
		tableData["GSUB"] = f.Gsub.Encode()
	}
	if f.Gpos != nil {
		tableData["GPOS"] = f.Gpos.Encode()
	}

	if fvarTable != nil {
		tableData["fvar"] = fvarTable.Encode()
	}
	if f.Avar != nil {
		tableData["avar"] = f.Avar.Encode()
	}
	if statTable != nil {
		tableData["STAT"] = statTable.Encode()
	}
	// gvar applies only to glyf outlines, mirroring the read gate.
	if _, ok := f.Outlines.(*glyf.Outlines); ok && f.Gvar != nil {
		gvarData, err := f.Gvar.Encode()
		if err != nil {
			return 0, err
		}
		tableData["gvar"] = gvarData
	}
	// cvar applies only to glyf fonts with a "cvt " table, mirroring the
	// read gate.
	if o, ok := f.Outlines.(*glyf.Outlines); ok && f.Cvar != nil {
		if cvt, ok := o.Tables["cvt "]; ok {
			cvarData, err := f.Cvar.Encode(len(cvt) / 2)
			if err != nil {
				return 0, err
			}
			tableData["cvar"] = cvarData
		}
	}
	if f.Hvar != nil {
		tableData["HVAR"] = f.Hvar.Encode()
	}
	if f.Mvar != nil {
		mvarData, err := f.Mvar.Encode()
		if err != nil {
			return 0, err
		}
		tableData["MVAR"] = mvarData
	}

	return header.Write(w, scalerType, tableData)
}

// assignVariationNameIDs clones the font's "fvar" and "STAT" tables and
// assigns deterministic "name" table IDs (>= 256) to their name strings via
// [canonicalizeVariationNames].  It returns the clones together with the
// strings to register in the "name" table, keyed by the allocated ID.  The
// caller's Font is never mutated.
func (f *Font) assignVariationNameIDs() (*fvar.Table, *stat.Table, map[name.ID]string) {
	var fv *fvar.Table
	if f.Fvar != nil {
		fv = cloneFvar(f.Fvar)
	}
	var st *stat.Table
	if f.Stat != nil {
		st = cloneStat(f.Stat)
	}
	extra := canonicalizeVariationNames(fv, st)
	return fv, st, extra
}

// WriteTrueTypePDF writes the binary form of a TrueType font to the given
// writer.  The output contains the tables required by PDF (head, hhea, hmtx,
// loca, maxp, glyf, and cmap when present), any tables in
// outlines.Tables (typically cvt, fpgm, prep, gasp), a "post" table
// when outlines.Names is non-nil, and a minimal "name" table holding the
// font's PostScript name.  The "OS/2", "GSUB", "GPOS", "GDEF", and "kern"
// tables are omitted, as they are not required for PDF embedding.
//
// If the font does not use TrueType outlines, the function returns an error.
//
// The optional arguments, if given, must be a sequence of pairs of strings and
// byte slices.  Each pair is interpreted as the name of a table and the
// corresponding data.  These tables are included in the output and override
// the default tables, where applicable.
func (f *Font) WriteTrueTypePDF(w io.Writer, extraTables ...any) (int64, error) {
	outlines, ok := f.Outlines.(*glyf.Outlines)
	if !ok {
		return 0, errors.New("sfnt: WriteTrueTypePDF requires TrueType outlines")
	}
	if err := f.CheckFontName(f.FontName); err != nil {
		return 0, err
	}

	tableData := make(map[string][]byte)

	if f.CMapTable != nil {
		tableData["cmap"] = f.CMapTable.Encode()
	}
	tableData["hhea"], tableData["hmtx"] = f.makeHmtx()
	enc := outlines.Glyphs.Encode()
	tableData["glyf"] = enc.GlyfData
	tableData["loca"] = enc.LocaData
	maps.Copy(tableData, outlines.Tables)

	maxpInfo := &maxp.Info{
		NumGlyphs: f.NumGlyphs(),
		TTF:       outlines.Maxp,
	}
	tableData["maxp"] = maxpInfo.Encode()

	tableData["head"] = f.makeHead(enc.LocaFormat)

	if outlines.Names != nil {
		postData, err := f.makePost()
		if err != nil {
			return 0, err
		}
		tableData["post"] = postData
	}

	if nameData := f.makeMinimalName(); nameData != nil {
		tableData["name"] = nameData
	}

	for i := 0; i+1 < len(extraTables); i += 2 {
		tableData[extraTables[i].(string)] = extraTables[i+1].([]byte)
	}

	return header.Write(w, header.ScalerTypeTrueType, tableData)
}

// WriteOpenTypeCFFPDF writes a minimal OpenType file, which includes only the
// tables required for PDF embedding.
//
// CFF2 outlines cannot be embedded this way (PDF supports static CFF only) and
// the function returns an error for them.
func (f *Font) WriteOpenTypeCFFPDF(w io.Writer) error {
	outlines, ok := f.Outlines.(*cff.Outlines)
	if !ok {
		return errors.New("sfnt: WriteOpenTypeCFFPDF requires CFF outlines")
	}
	if err := f.CheckFontName(f.FontName); err != nil {
		return err
	}

	tableData := make(map[string][]byte)

	if f.CMapTable != nil {
		tableData["cmap"] = f.CMapTable.Encode()
	}

	cffData, err := f.makeCFF(outlines)
	if err != nil {
		return err
	}
	tableData["CFF "] = cffData

	_, err = header.Write(w, header.ScalerTypeCFF, tableData)
	return err
}

func (f *Font) makeHead(locaFormat int16) []byte {
	headInfo := head.Info{
		FontRevision:  f.Version,
		HasYBaseAt0:   true,
		HasXBaseAt0:   true,
		UnitsPerEm:    f.UnitsPerEm,
		Created:       f.CreationTime,
		Modified:      f.ModificationTime,
		FontBBox:      f.FontBBox(),
		IsBold:        f.IsBold,
		IsItalic:      f.IsItalic,
		LowestRecPPEM: 7, // TODO(voss)
		LocaFormat:    locaFormat,
	}
	return headInfo.Encode()
}

func (f *Font) makeHmtx() ([]byte, []byte) {
	// The hmtx table stores widths and bboxes in UnitsPerEm units.  For CFF
	// outlines that requires applying the (possibly per-FD) font matrix; the
	// PDF helpers handle this, so derive UnitsPerEm values from there.
	upm := float64(f.UnitsPerEm)
	widths := make([]funit.Uint16, f.NumGlyphs())
	for i, w := range f.WidthsPDF() {
		// advance widths are UFWORD; clamp to the representable range
		v := math.Round(w * upm)
		v = max(0, min(v, 0xFFFF))
		widths[i] = funit.Uint16(v)
	}

	bboxScale := upm / 1000
	extents := make([]funit.Rect16, f.NumGlyphs())
	for i, b := range f.GlyphBBoxesPDF() {
		extents[i] = funit.Rect16{
			LLx: funit.Int16(math.Round(b.LLx * bboxScale)),
			LLy: funit.Int16(math.Round(b.LLy * bboxScale)),
			URx: funit.Int16(math.Round(b.URx * bboxScale)),
			URy: funit.Int16(math.Round(b.URy * bboxScale)),
		}
	}

	hmtxInfo := &hmtx.Info{
		Widths:       widths,
		GlyphExtents: extents,
		Ascent:       f.Ascent,
		Descent:      f.Descent,
		LineGap:      f.LineGap,
		CaretAngle:   f.ItalicAngle / 180 * math.Pi,
	}

	return hmtxInfo.Encode()
}

func (f *Font) makeOS2() []byte {
	// OS/2 xAvgCharWidth is in UnitsPerEm units.
	upm := float64(f.UnitsPerEm)
	avgGlyphWidth := 0
	count := 0
	for _, w := range f.WidthsPDF() {
		if w > 0 {
			avgGlyphWidth += int(math.Round(w * upm))
			count++
		}
	}
	if count > 0 {
		avgGlyphWidth = (avgGlyphWidth + count/2) / count
	}

	var familyClass int16
	if f.IsSerif {
		familyClass = 3 << 8
	} else if f.IsScript {
		familyClass = 10 << 8
	}

	var firstCharIndex, lastCharIndex uint16
	if cmap, _ := f.CMapTable.GetBest(); cmap != nil {
		low, high := cmap.CodeRange()
		firstCharIndex = uint16(low)
		if low > 0xFFFF {
			firstCharIndex = 0xFFFF
		}
		lastCharIndex = uint16(high)
		if high > 0xFFFF {
			lastCharIndex = 0xFFFF
		}
	}

	bbox := f.FontBBox()
	winAscent := bbox.URy
	winDescent := -bbox.LLy
	// TODO(voss): larger values may be needed, if GPOS rules move some
	// glyphs outside this range.

	os2Info := &os2.Info{
		WeightClass: f.Weight,
		WidthClass:  f.Width,

		IsBold:    f.IsBold,
		IsItalic:  f.IsItalic,
		IsRegular: f.IsRegular,
		IsOblique: f.IsOblique,

		FirstCharIndex: firstCharIndex,
		LastCharIndex:  lastCharIndex,

		Ascent:     f.Ascent,
		Descent:    f.Descent,
		LineGap:    f.LineGap,
		WinAscent:  winAscent,
		WinDescent: winDescent,
		CapHeight:  f.CapHeight,
		XHeight:    f.XHeight,

		AvgGlyphWidth: funit.Int16(avgGlyphWidth),

		FamilyClass: familyClass,

		CodePageRange: f.CodePageRange,
		UnicodeRange:  f.UnicodeRange,

		PermUse: f.PermUse,
	}
	return os2Info.Encode()
}

// makeName builds the "name" table.  extra carries additional strings keyed by
// their allocated name ID (>= 256), typically from assignVariationNameIDs.
func (f *Font) makeName(extra map[name.ID]string) []byte {
	day := f.ModificationTime
	if day.IsZero() {
		day = f.CreationTime
	}
	if day.IsZero() {
		day = time.Now()
	}
	dayString := day.Format("2006-01-02")

	// the unique identifier names the font, so it is left out when the font
	// carries no name to put in it
	var identifier string
	if unique := f.FullName; unique != "" {
		identifier = unique + "; " + f.Version.String() + "; " + dayString
	}

	nameTable := &name.Table{
		Family:         f.FamilyName,
		Subfamily:      f.Subfamily,
		Description:    f.Description,
		Copyright:      f.Copyright,
		Trademark:      f.Trademark,
		License:        f.License,
		LicenseURL:     f.LicenseURL,
		Identifier:     identifier,
		FullName:       f.FullName,
		Version:        "Version " + f.Version.String(),
		PostScriptName: nameID6(f),
		SampleText:     f.SampleText,

		VariationsPostScriptName: f.VariationsPostScriptName,
		Extra:                    extra,
	}
	nameInfo := &name.Info{
		Mac: name.Tables{
			"en": nameTable,
		},
		Windows: name.Tables{
			"en-US": nameTable,
		},
	}

	return nameInfo.Encode(1)
}

func (f *Font) makePost() ([]byte, error) {
	r := func(x funit.Float64) funit.Int16 {
		return funit.Int16(math.Round(float64(x)))
	}
	postInfo := &post.Info{
		ItalicAngle:        f.ItalicAngle,
		UnderlinePosition:  r(f.UnderlinePosition),
		UnderlineThickness: r(f.UnderlineThickness),
		IsFixedPitch:       f.IsFixedPitch(),
	}
	if outlines, ok := f.Outlines.(*glyf.Outlines); ok && outlines.Names != nil {
		if len(outlines.Names) != f.NumGlyphs() {
			return nil, fmt.Errorf("sfnt: glyph name count %d does not match glyph count %d",
				len(outlines.Names), f.NumGlyphs())
		}
		postInfo.Names = outlines.Names
	}
	return postInfo.Encode(), nil
}

func (f *Font) makeCFF2(outlines *cff.OutlinesCFF2) ([]byte, error) {
	myCff := &cff.FontCFF2{
		FontMatrix:   f.FontMatrix,
		OutlinesCFF2: outlines,
	}

	buf := &bytes.Buffer{}
	if err := myCff.Write(buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (f *Font) makeCFF(outlines *cff.Outlines) ([]byte, error) {
	fontInfo := f.GetFontInfo()
	myCff := &cff.Font{
		FontInfo: fontInfo,
		Outlines: outlines,
	}

	buf := &bytes.Buffer{}
	err := myCff.Write(buf)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// InstallCMap replaces the cmap table in the font with the given subtable.
func (f *Font) InstallCMap(s cmap.Subtable) {
	uniEncoding := uint16(3)
	winEncoding := uint16(1)
	if _, high := s.CodeRange(); high > 0xFFFF {
		uniEncoding = 4
		winEncoding = 10
	}
	cmapSubtable := s.Encode(0)
	f.CMapTable = cmap.Table{
		{PlatformID: 0, EncodingID: uniEncoding}: cmapSubtable,
		{PlatformID: 3, EncodingID: winEncoding}: cmapSubtable,
	}
}

// makeMinimalName builds a "name" table holding the font's PostScript name and
// nothing else, or nil if the font has no name.
//
// PDF does not need the "name" table and does not require it to be present,
// but without it a font program read back out of a PDF file has lost its name:
// nothing else in the file records what the font calls itself.  Of the entries
// the table can hold, a PDF processor consults only the PostScript name, so
// only that one is written; the copyright, licence and version strings a full
// table carries would cost far more than the rest of the subset.
func (f *Font) makeMinimalName() []byte {
	psName := nameID6(f)
	if psName == "" {
		return nil
	}
	info := &name.Info{
		Windows: name.Tables{
			"en-US": &name.Table{PostScriptName: psName},
		},
	}
	return info.Encode(1)
}

// nameID6 returns the PostScript name to write to the "name" table, or the
// empty string where the font's name cannot be written there.
//
// The entry is optional, so omitting it leaves a conforming font, whereas a
// name outside the restricted set the entry allows would not be one; see
// [nameID6Forbidden].  This only happens for a font with CFF outlines, whose
// Name INDEX carries the name instead; anywhere else [Font.CheckFontName] has
// already refused the name.
func nameID6(f *Font) string {
	psName := f.FontName
	if !canBeNameID6(psName) {
		return ""
	}
	return psName
}
