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
	"math"
	"os"
	"strings"

	"golang.org/x/text/language"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/head"
	"seehuhn.de/go/sfnt/header"
	"seehuhn.de/go/sfnt/hmtx"
	"seehuhn.de/go/sfnt/kern"
	"seehuhn.de/go/sfnt/maxp"
	"seehuhn.de/go/sfnt/name"
	"seehuhn.de/go/sfnt/opentype/gdef"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/os2"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/post"
)

// ReadFile reads a TrueType or OpenType font from a file.
func ReadFile(fname string) (*Font, error) {
	fd, err := os.Open(fname)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	return Read(fd)
}

// Read reads a TrueType or OpenType font from an io.Reader.
// If r does not implement the io.ReaderAt interface, the whole
// font file will be read into memory.
func Read(r io.Reader) (*Font, error) {
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

	if !(dir.Has("glyf", "loca") || dir.Has("CFF ")) {
		if dir.Has("CFF2") {
			return nil, &parser.NotSupportedError{
				SubSystem: "sfnt",
				Feature:   "CFF2-based fonts",
			}
		}
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
	var cmapBest cmap.Subtable
	cmapData, err := dir.ReadTableBytes(rr, "cmap")
	if err != nil && !header.IsMissing(err) {
		return nil, err
	}
	if cmapData != nil {
		cmapTable, err = cmap.Decode(cmapData)
		if err != nil {
			return nil, fmt.Errorf("cmap table: %w", err)
		}
		cmapBest, _ = cmapTable.GetBest()
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
		nameTable = winTab
		if winConf < language.High && macConf > winConf || nameTable == nil {
			nameTable = macTab
		}
	}

	var postInfo *post.Info
	postFd, err := dir.TableReader(rr, "post")
	if err != nil && !header.IsMissing(err) {
		return nil, err
	}
	if postFd != nil {
		postInfo, err = post.Read(postFd)
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
	switch dir.ScalerType {
	case header.ScalerTypeCFF:
		var cffInfo *cff.Font
		cffFd, err := dir.TableReader(rr, "CFF ")
		if err != nil {
			return nil, err
		}
		cffInfo, err = cff.Read(cffFd)
		if err != nil {
			return nil, fmt.Errorf("CFF table: %w", err)
		}
		fontInfo = cffInfo.FontInfo
		Outlines = cffInfo.Outlines

		if numGlyphs != 0 && len(cffInfo.Glyphs) != numGlyphs {
			return nil, errors.New("sfnt: cff glyph count mismatch")
		} else if hmtxInfo != nil && len(hmtxInfo.Widths) > 0 {
			for i, w := range hmtxInfo.Widths {
				cffInfo.Glyphs[i].Width = float64(w)
			}
		}
	case header.ScalerTypeTrueType, header.ScalerTypeApple:
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

		var widths []funit.Int16
		if hmtxInfo != nil && len(hmtxInfo.Widths) > 0 {
			widths = hmtxInfo.Widths
		}

		var names []string
		if postInfo != nil {
			names = postInfo.Names
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

	// Merge the information from the various tables.
	info := &Font{
		Outlines:  Outlines,
		CMapTable: cmapTable,
	}

	if nameTable != nil {
		info.FamilyName = nameTable.Family
	}
	if info.FamilyName == "" && fontInfo != nil {
		info.FamilyName = fontInfo.FamilyName
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

	if headInfo != nil {
		info.UnitsPerEm = headInfo.UnitsPerEm
	} else if fontInfo != nil && fontInfo.FontMatrix[0] != 0 {
		info.UnitsPerEm = uint16(math.Round(1 / fontInfo.FontMatrix[0]))
	} else {
		info.UnitsPerEm = 1000
	}
	if fontInfo != nil {
		info.FontMatrix = fontInfo.FontMatrix
	} else {
		q := 1 / float64(info.UnitsPerEm)
		info.FontMatrix = [6]float64{q, 0, 0, q, 0, 0}
	}

	if os2Info != nil {
		info.Ascent = os2Info.Ascent
		info.Descent = os2Info.Descent
		info.LineGap = os2Info.LineGap
	} else if hmtxInfo != nil {
		info.Ascent = hmtxInfo.Ascent
		info.Descent = hmtxInfo.Descent
		info.LineGap = hmtxInfo.LineGap
	}

	if os2Info != nil {
		info.CapHeight = os2Info.CapHeight
		info.XHeight = os2Info.XHeight
	}
	if info.CapHeight == 0 && cmapBest != nil {
		gid := cmapBest.Lookup('H')
		if gid != 0 && int(gid) < info.NumGlyphs() {
			info.CapHeight = info.glyphHeight(gid)
		}
	}
	if info.XHeight == 0 && cmapBest != nil {
		gid := cmapBest.Lookup('x')
		if gid != 0 && int(gid) < info.NumGlyphs() {
			info.XHeight = info.glyphHeight(gid)
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
	}

	if dir.Has("GDEF") {
		gdefFd, err := dir.TableReader(rr, "GDEF")
		if err != nil {
			return nil, err
		}
		info.Gdef, err = gdef.Read(gdefFd)
		if err != nil {
			return nil, fmt.Errorf("GDEF table: %w", err)
		}
	}

	// TODO(voss): should we try to read the "morx" table, if there is no
	// "GSUB" table?
	if dir.Has("GSUB") {
		gsubFd, err := dir.TableReader(rr, "GSUB")
		if err != nil {
			return nil, err
		}
		info.Gsub, err = gtab.Read(gsubFd, gtab.TypeGsub)
		if err != nil {
			return nil, fmt.Errorf("GSUB table: %w", err)
		}
	} else if !info.IsFixedPitch() {
		cmap, _ := info.CMapTable.GetBest()
		if cmap != nil {
			info.Gsub = standardLigatures(cmap)
		}
	}

	if dir.Has("GPOS") {
		gposFd, err := dir.TableReader(rr, "GPOS")
		if err != nil {
			return nil, err
		}
		info.Gpos, err = gtab.Read(gposFd, gtab.TypeGpos)
		if err != nil {
			return nil, fmt.Errorf("GPOS table: %w", err)
		}
	} else if dir.Has("kern") {
		kernFd, err := dir.TableReader(rr, "kern")
		if err != nil {
			return nil, err
		}
		kern, err := kern.Read(kernFd)
		if err != nil {
			return nil, fmt.Errorf("kern table: %w", err)
		}

		subtable := gtab.Gpos2_1{}
		for pair, val := range kern {
			subtable[pair] = &gtab.PairAdjust{
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

	return info, nil
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
