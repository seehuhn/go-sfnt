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
	"fmt"
	"io"
	"maps"
	"math"
	"slices"
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
	if f.Gvar != nil {
		gvarData, err := f.Gvar.Encode()
		if err != nil {
			return 0, err
		}
		tableData["gvar"] = gvarData
	}
	if f.Cvar != nil {
		var cvtCount int
		if o, ok := f.Outlines.(*glyf.Outlines); ok {
			cvtCount = len(o.Tables["cvt "]) / 2
		}
		cvarData, err := f.Cvar.Encode(cvtCount)
		if err != nil {
			return 0, err
		}
		tableData["cvar"] = cvarData
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

// assignVariationNameIDs clones the font's "fvar" and "STAT" tables and assigns
// deterministic "name" table IDs (>= 256) to their name strings.  It returns
// the clones together with the strings to register in the "name" table, keyed
// by the allocated ID.  Identical strings share one ID.  A NameID field whose
// resolved Name is empty keeps its existing numeric value and contributes no
// string.  The caller's Font is never mutated.
func (f *Font) assignVariationNameIDs() (*fvar.Table, *stat.Table, map[name.ID]string) {
	extra := make(map[name.ID]string)
	next := name.ID(256)
	seen := make(map[string]name.ID)
	alloc := func(s string) uint16 {
		if id, ok := seen[s]; ok {
			return uint16(id)
		}
		id := next
		next++
		seen[s] = id
		extra[id] = s
		return uint16(id)
	}

	var fv *fvar.Table
	if f.Fvar != nil {
		fv = cloneFvar(f.Fvar)
		for i := range fv.Axes {
			if fv.Axes[i].Name != "" {
				fv.Axes[i].NameID = alloc(fv.Axes[i].Name)
			}
		}
		for i := range fv.Instances {
			inst := &fv.Instances[i]
			if inst.Name != "" {
				inst.NameID = alloc(inst.Name)
			}
			if inst.PostScriptName != "" {
				inst.PostScriptNameID = alloc(inst.PostScriptName)
			}
		}
	}

	var st *stat.Table
	if f.Stat != nil {
		st = cloneStat(f.Stat)
		for i := range st.DesignAxes {
			if st.DesignAxes[i].Name != "" {
				st.DesignAxes[i].NameID = alloc(st.DesignAxes[i].Name)
			}
		}
		for i, av := range st.AxisValues {
			st.AxisValues[i] = assignAxisValueName(av, alloc)
		}
		if st.ElidedFallbackName != "" {
			st.ElidedFallbackNameID = alloc(st.ElidedFallbackName)
		}
	}

	if len(extra) == 0 {
		extra = nil
	}
	return fv, st, extra
}

func cloneFvar(t *fvar.Table) *fvar.Table {
	c := *t
	c.Axes = slices.Clone(t.Axes)
	c.Instances = slices.Clone(t.Instances)
	for i := range c.Instances {
		c.Instances[i].Coordinates = slices.Clone(t.Instances[i].Coordinates)
	}
	return &c
}

func cloneStat(t *stat.Table) *stat.Table {
	c := *t
	c.DesignAxes = slices.Clone(t.DesignAxes)
	c.AxisValues = slices.Clone(t.AxisValues) // elements replaced by the caller
	return &c
}

// assignAxisValueName returns a copy of av with its NameID reassigned when its
// resolved Name is non-empty.
func assignAxisValueName(av stat.AxisValue, alloc func(string) uint16) stat.AxisValue {
	switch v := av.(type) {
	case *stat.Format1:
		c := *v
		if c.Name != "" {
			c.NameID = alloc(c.Name)
		}
		return &c
	case *stat.Format2:
		c := *v
		if c.Name != "" {
			c.NameID = alloc(c.Name)
		}
		return &c
	case *stat.Format3:
		c := *v
		if c.Name != "" {
			c.NameID = alloc(c.Name)
		}
		return &c
	case *stat.Format4:
		c := *v
		c.Values = slices.Clone(v.Values)
		if c.Name != "" {
			c.NameID = alloc(c.Name)
		}
		return &c
	default:
		return av
	}
}

// WriteTrueTypePDF writes the binary form of a TrueType font to the given
// writer.  The output contains the tables required by PDF (head, hhea, hmtx,
// loca, maxp, glyf, and cmap when present), any tables in
// outlines.Tables (typically cvt, fpgm, prep, gasp), and a "post" table
// when outlines.Names is non-nil.  The "name", "OS/2", "GSUB", "GPOS",
// "GDEF", and "kern" tables are omitted, as they are not required for
// PDF embedding.
//
// If the font does not use TrueType outlines, the function panics.
//
// The optional arguments, if given, must be a sequence of pairs of strings and
// byte slices.  Each pair is interpreted as the name of a table and the
// corresponding data.  These tables are included in the output and override
// the default tables, where applicable.
func (f *Font) WriteTrueTypePDF(w io.Writer, extraTables ...any) (int64, error) {
	tableData := make(map[string][]byte)

	if f.CMapTable != nil {
		tableData["cmap"] = f.CMapTable.Encode()
	}
	tableData["hhea"], tableData["hmtx"] = f.makeHmtx()

	outlines := f.Outlines.(*glyf.Outlines)
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

	for i := 0; i+1 < len(extraTables); i += 2 {
		tableData[extraTables[i].(string)] = extraTables[i+1].([]byte)
	}

	return header.Write(w, header.ScalerTypeTrueType, tableData)
}

// WriteOpenTypeCFFPDF writes a minimal OpenType file, which includes only the
// tables required for PDF embedding.
func (f *Font) WriteOpenTypeCFFPDF(w io.Writer) error {
	tableData := make(map[string][]byte)

	if f.CMapTable != nil {
		tableData["cmap"] = f.CMapTable.Encode()
	}

	outlines := f.Outlines.(*cff.Outlines)
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
		IsItalic:      f.ItalicAngle != 0,
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
		IsItalic:  f.ItalicAngle != 0,
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

	fullName := f.FullName()
	nameTable := &name.Table{
		Family:         f.FamilyName,
		Subfamily:      f.Subfamily(),
		Description:    f.Description,
		Copyright:      f.Copyright,
		Trademark:      f.Trademark,
		License:        f.License,
		LicenseURL:     f.LicenseURL,
		Identifier:     fullName + "; " + f.Version.String() + "; " + dayString,
		FullName:       fullName,
		Version:        "Version " + f.Version.String(),
		PostScriptName: f.PostScriptName(),
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
