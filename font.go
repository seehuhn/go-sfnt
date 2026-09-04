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
	"math"
	"strings"
	"time"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/geom/path"
	"seehuhn.de/go/geom/rect"
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
	"seehuhn.de/go/sfnt/hvar"
	"seehuhn.de/go/sfnt/mvar"
	"seehuhn.de/go/sfnt/opentype/gdef"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/os2"
	"seehuhn.de/go/sfnt/stat"
)

// TODO(voss): read https://github.com/googlefonts/gf-docs/tree/main/VerticalMetrics

// Outlines represents the glyph data of a TrueType or OpenType font.
// This must be one of [*glyf.Outlines] or [*cff.Outlines].
type Outlines interface {
	// NumGlyphs returns the number of glyphs in the font.
	NumGlyphs() int

	// IsBlank returns true if the glyph with the given ID does not add marks to the page.
	IsBlank(gid glyph.ID) bool

	// GlyphMatrix returns the effective font matrix for the given glyph.
	// For CID-keyed CFF fonts this composes the per-FD font matrix with the
	// top-level matrix; otherwise the top-level matrix is returned unchanged.
	GlyphMatrix(top matrix.Matrix, gid glyph.ID) matrix.Matrix

	// GlyphBBox returns the bounding box of the glyph with the given ID,
	// after the matrix m has been applied to the glyph outline.  The matrix
	// must already account for any per-glyph matrix, see GlyphMatrix.
	GlyphBBox(m matrix.Matrix, gid glyph.ID) (bbox rect.Rect)

	// GlyphBBoxPDF returns the bounding box of the glyph with the given ID
	// in PDF glyph space units.
	GlyphBBoxPDF(m matrix.Matrix, gid glyph.ID) (bbox rect.Rect)

	// Path returns the glyph outline as a path, in font design units.
	Path(gid glyph.ID) path.Path
}

// Font contains information about a TrueType or OpenType font.
//
// TODO(voss): clarify the relation between IsOblique, IsItalic, and
// ItalicAngle != 0.
//
// TODO(voss): document which fields are mandatory/optional.
type Font struct {
	// FontName (optional) is the PostScript name of the font.
	//
	// The rules for the name depend on the kind of glyph outlines the font
	// uses:
	//
	//   - For a font with CFF outlines, the name may be at most 127 bytes of
	//     UTF-8, and may hold neither white space, nor a character which
	//     cannot be shown, nor any of "%", "(", ")", "<", ">", "[", "]",
	//     "{", "}" and "/".
	//   - For a font with glyf or CFF2 outlines, the name may be at most 63
	//     characters, taken from the ASCII codes 33 to 126 with the same ten
	//     characters left out.
	//
	// [Font.CheckFontName] reports whether a name obeys these rules, and
	// [Font.MaxFontNameLen] gives the length limit.
	//
	// A font with CFF outlines can carry a name the "name" table cannot hold,
	// since its Name INDEX records the name instead; such a font is written
	// without a PostScript name entry in the "name" table.
	FontName string

	// FamilyName (optional) is the name of the family the font belongs to,
	// for example "MyFont 72".
	FamilyName string

	// Subfamily (optional) is the style name of the font within its family,
	// for example "Bold Italic".
	Subfamily string

	// FullName (optional) is the human-readable name of the font,
	// for example "MyFont 72 Smallcaps Book".
	FullName string

	Width  os2.Width
	Weight os2.Weight

	IsRegular bool // glyphs are in the standard weight/style for the font
	IsBold    bool // glyphs are emboldened
	IsItalic  bool // font contains italic or oblique glyphs
	IsOblique bool // font contains oblique glyphs
	IsSerif   bool // glyph shapes have serifs
	IsScript  bool // glyphs resemble cursive handwriting

	// CodePageRange records which code pages the font declares coverage for,
	// as reported by the OS/2 table.  These bits are an advisory hint set by
	// the font's author and are not guaranteed to match the actual cmap
	// coverage.
	CodePageRange os2.CodePageRange

	// UnicodeRange records which Unicode blocks the font declares coverage
	// for, as reported by the OS/2 table.  These bits are an advisory hint
	// set by the font's author and are not guaranteed to match the actual
	// cmap coverage.
	UnicodeRange os2.UnicodeRange

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

	// metrics in font design units (UnitsPerEm)
	Ascent             funit.Int16
	Descent            funit.Int16 // negative
	LineGap            funit.Int16 // LineGap = Leading - Ascent + Descent
	CapHeight          funit.Int16
	XHeight            funit.Int16
	UnderlinePosition  funit.Float64 // negative
	UnderlineThickness funit.Float64

	FontMatrix matrix.Matrix

	ItalicAngle float64 // degrees counterclockwise from vertical

	// Outlines contains the glyph data of the font.
	// This must be one of [*glyf.Outlines] or [*cff.Outlines].
	Outlines Outlines

	CMapTable cmap.Table

	Gdef *gdef.Table
	Gsub *gtab.Info
	Gpos *gtab.Info

	Fvar *fvar.Table
	Avar *avar.Table
	Stat *stat.Table
	Gvar *gvar.Table
	Cvar *cvar.Table
	Hvar *hvar.Table
	Mvar *mvar.Table

	// VariationsPostScriptName (optional) is the prefix from which
	// the PostScript name of a variable font instance is built.  It may hold
	// ASCII letters and digits only, and must be empty for non-variable fonts.
	VariationsPostScriptName string
}

// Clone makes a shallow copy of the font object.
func (f *Font) Clone() *Font {
	f2 := *f
	return &f2
}

// GetFontInfo returns an Adobe FontInfo structure for the given font.
// The result is a newly allocated structure and is not shared with the font.
func (f *Font) GetFontInfo() *type1.FontInfo {
	fontInfo := &type1.FontInfo{
		FontName:   f.FontName,
		FullName:   f.FullName,
		FamilyName: f.FamilyName,
		Weight:     f.Weight.String(),
		Version:    f.Version.String(),

		Copyright: strings.ReplaceAll(f.Copyright, "©", "(c)"),
		Notice:    f.Trademark,

		ItalicAngle:  f.ItalicAngle,
		IsFixedPitch: f.IsFixedPitch(),

		UnderlinePosition:  f.UnderlinePosition,
		UnderlineThickness: f.UnderlineThickness,

		FontMatrix: f.FontMatrix,
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
// Returns nil if the font does not contain CFF outlines.
func (f *Font) AsCFF() *cff.Font {
	outlines, ok := f.Outlines.(*cff.Outlines)
	if !ok {
		return nil
	}
	return &cff.Font{
		FontInfo: f.GetFontInfo(),
		Outlines: outlines,
	}
}

// IsCFF2 returns true if the font contains CFF2 glyph outlines.
func (f *Font) IsCFF2() bool {
	_, ok := f.Outlines.(*cff.OutlinesCFF2)
	return ok
}

// AsCFF2 returns the CFF2 font data for the given font.
// Returns nil if the font does not contain CFF2 outlines.
func (f *Font) AsCFF2() *cff.FontCFF2 {
	outlines, ok := f.Outlines.(*cff.OutlinesCFF2)
	if !ok {
		return nil
	}
	return &cff.FontCFF2{
		FontMatrix:   f.FontMatrix,
		OutlinesCFF2: outlines,
	}
}

// FontBBox returns the bounding box of the font, in font design units.
// This is the smallest rectangle enclosing all individual glyph bounding
// boxes.
func (f *Font) FontBBox() rect.Rect {
	return f.fontBBox(float64(f.UnitsPerEm))
}

// FontBBoxPDF returns the font bounding box in PDF glyph space units.
// This is the smallest rectangle enclosing all individual glyphs bounding boxes.
func (f *Font) FontBBoxPDF() rect.Rect {
	return f.fontBBox(1000)
}

// fontBBox returns the smallest rectangle enclosing all glyph bounding boxes,
// with the outlines scaled so that one em spans unitsPerEm units.
func (f *Font) fontBBox(unitsPerEm float64) (bbox rect.Rect) {
	for i := range f.NumGlyphs() {
		bbox.Extend(f.glyphBBox(unitsPerEm, glyph.ID(i)))
	}
	return bbox
}

// glyphBBox returns the bounding box of a single glyph, with the outline
// scaled so that one em spans unitsPerEm units.  The scale is composed into
// the font matrix rather than applied afterwards, so that outlines already
// drawn on the target grid are reproduced exactly.
func (f *Font) glyphBBox(unitsPerEm float64, gid glyph.ID) rect.Rect {
	M := f.Outlines.GlyphMatrix(f.FontMatrix, gid)
	return f.Outlines.GlyphBBox(M.Mul(matrix.Scale(unitsPerEm, unitsPerEm)), gid)
}

// NumGlyphs returns the number of glyphs in the font.
func (f *Font) NumGlyphs() int {
	return f.Outlines.NumGlyphs()
}

func (f *Font) BuiltinEncoding() []string {
	switch f := f.Outlines.(type) {
	case *cff.Outlines:
		return f.BuiltinEncoding()
	default:
		return nil
	}
}

// Widths returns the advance widths of the glyphs in the font
// in glyph design units.
func (f *Font) Widths() []float64 {
	widths := make([]float64, f.NumGlyphs())
	switch outlines := f.Outlines.(type) {
	case *cff.Outlines:
		for gid, g := range outlines.Glyphs {
			widths[gid] = g.Width
		}
		return widths
	case *cff.OutlinesCFF2:
		if outlines.Widths == nil {
			return widths
		}
		copy(widths, outlines.Widths)
		return widths
	case *glyf.Outlines:
		for i := range widths {
			widths[i] = float64(outlines.Widths[i])
		}
		return widths
	default:
		panic("unexpected font type")
	}
}

// WidthsPDF returns the advance widths of the glyphs in the font,
// in PDF text space units.
//
// For CID-keyed CFF fonts, per-FD font matrices are composed with the
// top-level font matrix.
func (f *Font) WidthsPDF() []float64 {
	widths := make([]float64, f.NumGlyphs())
	switch o := f.Outlines.(type) {
	case *cff.Outlines:
		for gid, g := range o.Glyphs {
			q := o.GlyphAdvanceScale(f.FontMatrix, glyph.ID(gid))
			widths[gid] = g.Width * q
		}
		return widths
	case *cff.OutlinesCFF2:
		if o.Widths == nil {
			return nil
		}
		for gid, w := range o.Widths {
			q := o.GlyphAdvanceScale(f.FontMatrix, glyph.ID(gid))
			widths[gid] = w * q
		}
		return widths
	case *glyf.Outlines:
		if o.Widths == nil {
			return nil
		}
		for gid, w := range o.Widths {
			widths[gid] = float64(w) / float64(f.UnitsPerEm)
		}
	default:
		panic("unexpected font type")
	}
	return widths
}

// WidthsMapPDF returns a map of glyph names to advance widths in PDF glyph
// space units (1/1000th of a text space unit).
//
// If the font does not contain CFF outlines or is CID-keyed, nil is returned.
func (f *Font) WidthsMapPDF() map[string]float64 {
	o, isCFF := f.Outlines.(*cff.Outlines)
	if !isCFF || o.IsCIDKeyed() {
		return nil
	}

	q := o.GlyphAdvanceScale(f.FontMatrix, 0) * 1000

	widths := make(map[string]float64)
	for _, glyph := range o.Glyphs {
		widths[glyph.Name] = glyph.Width * q
	}
	return widths
}

// GlyphBBoxesPDF returns per-glyph bounding boxes in PDF glyph space units
// (1/1000 of a text space unit).
//
// For CID-keyed CFF fonts, per-FD font matrices are composed with the
// top-level font matrix.
func (f *Font) GlyphBBoxesPDF() []rect.Rect {
	n := f.NumGlyphs()
	extents := make([]rect.Rect, n)
	for i := range n {
		extents[i] = f.glyphBBox(1000, glyph.ID(i))
	}
	return extents
}

// GlyphWidth returns the advance width of the glyph with the given glyph ID,
// in font design units.
func (f *Font) GlyphWidth(gid glyph.ID) float64 {
	switch o := f.Outlines.(type) {
	case *cff.Outlines:
		return o.Glyphs[gid].Width
	case *cff.OutlinesCFF2:
		if o.Widths == nil {
			return 0
		}
		return o.Widths[gid]
	case *glyf.Outlines:
		if o.Widths == nil {
			return 0
		}
		return float64(o.Widths[gid])
	default:
		panic("unexpected font type")
	}
}

// GlyphWidthPDF returns the advance width in PDF glyph space units.
func (f *Font) GlyphWidthPDF(gid glyph.ID) float64 {
	switch o := f.Outlines.(type) {
	case *cff.Outlines:
		q := o.GlyphAdvanceScale(f.FontMatrix, gid)
		return o.Glyphs[gid].Width * (q * 1000)

	case *cff.OutlinesCFF2:
		if o.Widths == nil {
			return 0
		}
		q := o.GlyphAdvanceScale(f.FontMatrix, gid)
		return o.Widths[gid] * (q * 1000)

	case *glyf.Outlines:
		if o.Widths == nil {
			return 0
		}
		return float64(o.Widths[gid]) / (float64(f.UnitsPerEm) / 1000)

	default:
		panic("unexpected font type")
	}
}

// GlyphBBox returns the glyph bounding box for one glyph in font design
// units.
func (f *Font) GlyphBBox(gid glyph.ID) rect.Rect {
	return f.glyphBBox(float64(f.UnitsPerEm), gid)
}

// glyphHeight returns the height of the glyph's ink, in font design units.
//
// A malformed font matrix can push the result out of range; such heights are
// clamped, and a height which is not a positive number is reported as zero.
func (f *Font) glyphHeight(gid glyph.ID) funit.Int16 {
	h := math.Round(f.GlyphBBox(gid).URy)
	if !(h > 0) { // the negated test also rejects NaN
		return 0
	}
	return clampInt16(h)
}

// measureHeight returns the height of the first of the given characters the
// font has a non-blank glyph for, in font design units.  It is used to derive
// a cap height or x-height for fonts which report none; see
// [type1.CapHeightChars].
func (f *Font) measureHeight(subtable cmap.Subtable, chars string) funit.Int16 {
	for _, r := range chars {
		gid := subtable.Lookup(r)
		if gid == 0 || int(gid) >= f.NumGlyphs() {
			continue
		}
		if h := f.glyphHeight(gid); h > 0 {
			return h
		}
	}
	return 0
}

// GlyphName returns the name of a glyph.
// If the name is not known, the empty string is returned.
func (f *Font) GlyphName(gid glyph.ID) string {
	switch o := f.Outlines.(type) {
	case *cff.Outlines:
		return o.Glyphs[gid].Name
	case *cff.OutlinesCFF2:
		return "" // CFF2 glyphs carry no names
	case *glyf.Outlines:
		if o.Names == nil {
			return ""
		}
		return o.Names[gid]
	default:
		panic("unexpected font type")
	}
}

// IsFixedPitch returns true if all glyphs in the font have the same width.
func (f *Font) IsFixedPitch() bool {
	ww := f.WidthsPDF()
	if len(ww) == 0 {
		return false
	}

	// Two widths count as equal if they round to the same hmtx UnitsPerEm
	// value.  WidthsPDF is in text space units (1 em), so the threshold is
	// half a UnitsPerEm step.
	tol := 0.5 / float64(f.UnitsPerEm)
	var width float64
	for _, w := range ww {
		if w == 0 {
			continue
		}
		if width == 0 {
			width = w
		} else if math.Abs(width-w) >= tol {
			return false
		}
	}

	return true
}
