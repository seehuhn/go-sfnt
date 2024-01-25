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
	"io"
	"math"
	"time"

	"seehuhn.de/go/postscript/funit"

	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/head"
	"seehuhn.de/go/sfnt/header"
	"seehuhn.de/go/sfnt/hmtx"
	"seehuhn.de/go/sfnt/maxp"
	"seehuhn.de/go/sfnt/name"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/os2"
	"seehuhn.de/go/sfnt/post"
)

// Write writes the binary form of the font to the given writer.
func (f *Font) Write(w io.Writer) (int64, error) {
	tableData := make(map[string][]byte)

	hheaData, hmtxData := f.makeHmtx()
	tableData["hhea"] = hheaData
	tableData["hmtx"] = hmtxData

	if f.CMapTable != nil {
		tableData["cmap"] = f.CMapTable.Encode()
	}

	tableData["OS/2"] = f.makeOS2()
	tableData["name"] = f.makeName()
	tableData["post"] = f.makePost()

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
		for name, data := range outlines.Tables {
			tableData[name] = data
		}
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
		tableData["GSUB"] = f.Gsub.Encode(gtab.GsubExtensionLookupType)
	}
	if f.Gpos != nil {
		tableData["GPOS"] = f.Gpos.Encode(gtab.GposExtensionLookupType)
	}

	return header.Write(w, scalerType, tableData)
}

// WriteTrueTypePDF writes the binary form of a TrueType font to the given
// writer.  Only the tables needed for PDF embedding are included.
//
// if the font does not use TrueType outlines, the function panics.
func (f *Font) WriteTrueTypePDF(w io.Writer) (int64, error) {
	tableData := make(map[string][]byte)

	if f.CMapTable != nil {
		tableData["cmap"] = f.CMapTable.Encode()
	}
	tableData["hhea"], tableData["hmtx"] = f.makeHmtx()

	outlines := f.Outlines.(*glyf.Outlines)
	enc := outlines.Glyphs.Encode()
	tableData["glyf"] = enc.GlyfData
	tableData["loca"] = enc.LocaData
	for name, data := range outlines.Tables {
		tableData[name] = data
	}

	maxpInfo := &maxp.Info{
		NumGlyphs: f.NumGlyphs(),
		TTF:       outlines.Maxp,
	}
	tableData["maxp"] = maxpInfo.Encode()

	tableData["head"] = f.makeHead(enc.LocaFormat)

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
		FontBBox:      f.BBox(),
		IsBold:        f.IsBold,
		IsItalic:      f.ItalicAngle != 0,
		LowestRecPPEM: 7, // TODO(voss)
		LocaFormat:    locaFormat,
	}
	return headInfo.Encode()
}

func (f *Font) makeHmtx() ([]byte, []byte) {
	hmtxInfo := &hmtx.Info{
		Widths:       f.Widths(),
		GlyphExtents: f.GlyphBBoxes(),
		Ascent:       f.Ascent,
		Descent:      f.Descent,
		LineGap:      f.LineGap,
		CaretAngle:   f.ItalicAngle / 180 * math.Pi,
	}

	return hmtxInfo.Encode()
}

func (f *Font) makeOS2() []byte {
	avgGlyphWidth := 0
	count := 0
	ww := f.Widths()
	for _, w := range ww {
		if w > 0 {
			avgGlyphWidth += int(w)
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

	bbox := f.BBox()
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

		PermUse: f.PermUse,
	}
	return os2Info.Encode()
}

func (f *Font) makeName() []byte {
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

func (f *Font) makePost() []byte {
	r := func(x funit.Float64) funit.Int16 {
		return funit.Int16(math.Round(float64(x)))
	}
	postInfo := &post.Info{
		ItalicAngle:        f.ItalicAngle,
		UnderlinePosition:  r(f.UnderlinePosition),
		UnderlineThickness: r(f.UnderlineThickness),
		IsFixedPitch:       f.IsFixedPitch(),
	}
	if outlines, ok := f.Outlines.(*glyf.Outlines); ok {
		postInfo.Names = outlines.Names
	}
	return postInfo.Encode()
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
