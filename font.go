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
	"regexp"
	"strings"
	"time"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/head"
	"seehuhn.de/go/sfnt/opentype/gdef"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/os2"
)

// TODO(voss): read https://github.com/googlefonts/gf-docs/tree/main/VerticalMetrics

// Font contains information about a TrueType or OpenType font.
//
// TODO(voss): clarify the relation between IsOblique, IsItalic, and
// ItalicAngle != 0.
type Font struct {
	FamilyName string
	Width      os2.Width
	Weight     os2.Weight
	IsBold     bool // glyphs are emboldened
	IsItalic   bool // font contains italic or oblique glyphs
	IsRegular  bool // glyphs are in the standard weight/style for the font
	IsOblique  bool // font contains oblique glyphs
	IsSerif    bool
	IsScript   bool // Glyphs resemble cursive handwriting.

	CodePageRange os2.CodePageRange

	Version          head.Version
	CreationTime     time.Time
	ModificationTime time.Time
	Description      string
	SampleText       string

	Copyright  string
	Trademark  string
	License    string
	LicenseURL string
	PermUse    os2.Permissions

	UnitsPerEm uint16

	Ascent    funit.Int16
	Descent   funit.Int16 // negative
	LineGap   funit.Int16 // LineGap = BaseLineSkip - Ascent + Descent
	CapHeight funit.Int16
	XHeight   funit.Int16

	ItalicAngle        float64       // Italic angle (degrees counterclockwise from vertical)
	UnderlinePosition  funit.Float64 // Underline position (negative)
	UnderlineThickness funit.Float64 // Underline thickness

	CMapTable cmap.Table
	CMap      cmap.Subtable // maps unicode to GID
	Outlines  interface{}   // either *cff.Outlines or *glyf.Outlines

	Gdef *gdef.Table
	Gsub *gtab.Info
	Gpos *gtab.Info
}

// Clone makes a shallow copy of the font object.
func (f *Font) Clone() *Font {
	f2 := *f
	return &f2
}

// GetFontInfo returns an Adobe FontInfo structure for the given font.
func (f *Font) GetFontInfo() *type1.FontInfo {
	q := 1 / float64(f.UnitsPerEm)
	fontInfo := &type1.FontInfo{
		FontName:   f.PostscriptName(),
		FullName:   f.FullName(),
		FamilyName: f.FamilyName,
		Weight:     f.Weight.String(),
		Version:    f.Version.String(),

		Copyright: strings.ReplaceAll(f.Copyright, "©", "(c)"),
		Notice:    f.Trademark,

		FontMatrix: []float64{q, 0, 0, q, 0, 0},

		ItalicAngle:  f.ItalicAngle,
		IsFixedPitch: f.IsFixedPitch(),

		UnderlinePosition:  f.UnderlinePosition,
		UnderlineThickness: f.UnderlineThickness,
	}
	return fontInfo
}

// IsGlyf returns true if the font contains TrueType glyph outlines.
func (f *Font) IsGlyf() bool {
	_, ok := f.Outlines.(*glyf.Outlines)
	return ok
}

// IsCFF returns true if the font contains CFF glyph outlines.
func (f *Font) IsCFF() bool {
	_, ok := f.Outlines.(*cff.Outlines)
	return ok
}

// AsCFF returns the CFF font data for the given font.
// Panics if the font does not contain CFF outlines.
func (f *Font) AsCFF() *cff.Font {
	return &cff.Font{
		FontInfo: f.GetFontInfo(),
		Outlines: f.Outlines.(*cff.Outlines),
	}
}

// FullName returns the full name of the font.
func (f *Font) FullName() string {
	return f.FamilyName + " " + f.Subfamily()
}

// Subfamily returns the subfamily name of the font.
func (f *Font) Subfamily() string {
	var words []string
	if f.Width != 0 && f.Width != os2.WidthNormal {
		words = append(words, f.Width.String())
	}
	if f.Weight != 0 && f.Weight != os2.WeightNormal {
		words = append(words, f.Weight.SimpleString())
	} else if f.IsBold {
		words = append(words, "Bold")
	}
	if f.IsOblique {
		words = append(words, "Oblique")
	} else if f.IsItalic {
		words = append(words, "Italic")
	}
	if len(words) == 0 {
		return "Regular"
	}
	return strings.Join(words, " ")
}

// PostscriptName returns the PostScript name of the font.
func (f *Font) PostscriptName() string {
	name := f.FamilyName + "-" + f.Subfamily()
	re := regexp.MustCompile(`[^!-$&-'*-.0-;=?-Z\\^-z|~]+`)
	return re.ReplaceAllString(name, "")
}

// BBox returns the bounding box of the font.
func (f *Font) BBox() (bbox funit.Rect16) {
	first := true
	for i := 0; i < f.NumGlyphs(); i++ {
		ext := f.GlyphExtent(glyph.ID(i))
		if ext.IsZero() {
			continue
		}

		if first {
			bbox = ext
		} else {
			bbox.Extend(ext)
		}
	}
	return
}

// NumGlyphs returns the number of glyphs in the font.
func (f *Font) NumGlyphs() int {
	switch outlines := f.Outlines.(type) {
	case *cff.Outlines:
		return len(outlines.Glyphs)
	case *glyf.Outlines:
		return len(outlines.Glyphs)
	default:
		panic("unexpected font type")
	}
}

// GlyphWidth returns the advance width of the glyph with the given glyph ID,
// in font design units.
func (f *Font) GlyphWidth(gid glyph.ID) funit.Int16 {
	switch f := f.Outlines.(type) {
	case *cff.Outlines:
		return f.Glyphs[gid].Width
	case *glyf.Outlines:
		if f.Widths == nil {
			return 0
		}
		return f.Widths[gid]
	default:
		panic("unexpected font type")
	}
}

// Widths returns the advance widths of the glyphs in the font.
func (f *Font) Widths() []funit.Int16 {
	switch outlines := f.Outlines.(type) {
	case *cff.Outlines:
		widths := make([]funit.Int16, f.NumGlyphs())
		for gid, g := range outlines.Glyphs {
			widths[gid] = g.Width
		}
		return widths
	case *glyf.Outlines:
		return outlines.Widths
	default:
		panic("unexpected font type")
	}
}

// Extents returns the glyph bounding boxes for the font.
func (f *Font) Extents() []funit.Rect16 {
	extents := make([]funit.Rect16, f.NumGlyphs())
	switch f := f.Outlines.(type) {
	case *cff.Outlines:
		for i, g := range f.Glyphs {
			extents[i] = g.Extent()
		}
	case *glyf.Outlines:
		for i, g := range f.Glyphs {
			if g == nil {
				continue
			}
			extents[i] = g.Rect16
		}
	default:
		panic("unexpected font type")
	}
	return extents
}

// GlyphExtent returns the glyph bounding box for one glyph in font design
// units.
func (f *Font) GlyphExtent(gid glyph.ID) funit.Rect16 {
	switch f := f.Outlines.(type) {
	case *cff.Outlines:
		return f.Glyphs[gid].Extent()
	case *glyf.Outlines:
		g := f.Glyphs[gid]
		if g == nil {
			return funit.Rect16{}
		}
		return g.Rect16
	default:
		panic("unexpected font type")
	}
}

func (f *Font) glyphHeight(gid glyph.ID) funit.Int16 {
	switch f := f.Outlines.(type) {
	case *cff.Outlines:
		return f.Glyphs[gid].Extent().URy
	case *glyf.Outlines:
		g := f.Glyphs[gid]
		if g == nil {
			return 0
		}
		return g.Rect16.URy
	default:
		panic("unexpected font type")
	}
}

// GlyphName returns the name of a glyph.
// If the name is not known, the empty string is returned.
func (f *Font) GlyphName(gid glyph.ID) string {
	switch f := f.Outlines.(type) {
	case *cff.Outlines:
		return f.Glyphs[gid].Name
	case *glyf.Outlines:
		if f.Names == nil {
			return ""
		}
		return f.Names[gid]
	default:
		panic("unexpected font type")
	}
}

// IsFixedPitch returns true if all glyphs in the font have the same width.
func (f *Font) IsFixedPitch() bool {
	ww := f.Widths()
	if len(ww) == 0 {
		return false
	}

	var width funit.Int16
	for _, w := range ww {
		if w == 0 {
			continue
		}
		if width == 0 {
			width = w
		} else if width != w {
			return false
		}
	}

	return true
}

func (f *Font) Layout(rr []rune, gsubLookups, gposLookups []gtab.LookupIndex) glyph.Seq {
	// TODO(voss): should this take a string as an argument, instead of rr?
	seq := make(glyph.Seq, len(rr))
	for i, r := range rr {
		gid := f.CMap.Lookup(r)
		seq[i].Gid = gid
		seq[i].Text = []rune{r}
	}

	if f.Gsub != nil {
		for _, lookupIndex := range gsubLookups {
			seq = f.Gsub.LookupList.ApplyLookup(seq, lookupIndex, f.Gdef)
		}
	}

	for i := range seq {
		gid := seq[i].Gid
		if !f.Gdef.IsMark(gid) {
			seq[i].Advance = f.GlyphWidth(gid)
		}
	}
	for _, lookupIndex := range gposLookups {
		seq = f.Gpos.LookupList.ApplyLookup(seq, lookupIndex, f.Gdef)
	}

	return seq
}
