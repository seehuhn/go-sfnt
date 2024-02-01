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
	IsRegular  bool // glyphs are in the standard weight/style for the font
	IsBold     bool // glyphs are emboldened
	IsItalic   bool // font contains italic or oblique glyphs
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
	FontMatrix [6]float64

	Ascent    funit.Int16
	Descent   funit.Int16 // negative
	LineGap   funit.Int16 // LineGap = Leading - Ascent + Descent
	CapHeight funit.Int16
	XHeight   funit.Int16

	ItalicAngle        float64       // Italic angle (degrees counterclockwise from vertical)
	UnderlinePosition  funit.Float64 // Underline position (negative)
	UnderlineThickness funit.Float64 // Underline thickness

	Outlines interface{} // either *cff.Outlines or *glyf.Outlines

	CMapTable cmap.Table

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
	fontInfo := &type1.FontInfo{
		FontName:   f.PostScriptName(),
		FullName:   f.FullName(),
		FamilyName: f.FamilyName,
		Weight:     f.Weight.String(),
		Version:    f.Version.String(),

		Copyright: strings.ReplaceAll(f.Copyright, "©", "(c)"),
		Notice:    f.Trademark,

		FontMatrix: f.FontMatrix,

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

// PostScriptName returns the PostScript name of the font.
func (f *Font) PostScriptName() string {
	name := f.FamilyName + "-" + f.Subfamily()
	re := regexp.MustCompile(`[^!-$&-'*-.0-;=?-Z\\^-z|~]+`)
	return re.ReplaceAllString(name, "")
}

// BBox returns the bounding box of the font.
func (f *Font) BBox() (bbox funit.Rect16) {
	first := true
	for i := 0; i < f.NumGlyphs(); i++ {
		ext := f.GlyphBBox(glyph.ID(i))
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

// GlyphWidthPDF returns the advance width in PDF text space units.
func (f *Font) GlyphWidthPDF(gid glyph.ID) float64 {
	switch o := f.Outlines.(type) {
	case *cff.Outlines:
		// TODO(voss): ... + f.FontMatrix[4] ???
		return float64(o.Glyphs[gid].Width) * f.FontMatrix[0]
	case *glyf.Outlines:
		if o.Widths == nil {
			return 0
		}
		return float64(o.Widths[gid]) / float64(f.UnitsPerEm)
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

// WidthsPDF returns the advance widths of the glyphs in the font.
func (f *Font) WidthsPDF() []float64 {
	widths := make([]float64, f.NumGlyphs())
	switch outlines := f.Outlines.(type) {
	case *cff.Outlines:
		for gid, g := range outlines.Glyphs {
			widths[gid] = float64(g.Width) * f.FontMatrix[0]
		}
		return widths
	case *glyf.Outlines:
		if outlines.Widths == nil {
			return nil
		}
		for gid, w := range outlines.Widths {
			widths[gid] = float64(w) / float64(f.UnitsPerEm)
		}
	default:
		panic("unexpected font type")
	}
	return widths
}

// GlyphBBoxes returns the glyph bounding boxes for the font.
func (f *Font) GlyphBBoxes() []funit.Rect16 {
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

// GlyphBBox returns the glyph bounding box for one glyph in font design
// units.
func (f *Font) GlyphBBox(gid glyph.ID) funit.Rect16 {
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

// Layout returns the glyph sequence for the given text.
func (f *Font) Layout(cmap cmap.Subtable, gsubLookups, gposLookups []gtab.LookupIndex, s string) []glyph.Info {
	rr := []rune(s)

	seq := make([]glyph.Info, len(rr))
	for i, r := range rr {
		gid := cmap.Lookup(r)
		seq[i].GID = gid
		seq[i].Text = []rune{r}
	}

	if f.Gsub != nil {
		for _, lookupIndex := range gsubLookups {
			seq = f.Gsub.LookupList.ApplyLookup(seq, lookupIndex, f.Gdef)
		}
	}

	for i := range seq {
		gid := seq[i].GID
		if !f.Gdef.IsMark(gid) {
			seq[i].Advance = f.GlyphWidth(gid)
		}
	}
	for _, lookupIndex := range gposLookups {
		seq = f.Gpos.LookupList.ApplyLookup(seq, lookupIndex, f.Gdef)
	}

	return seq
}
