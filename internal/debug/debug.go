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

// Package debug provides a simple font for use in unit tests.
package debug

import (
	"bytes"
	"math"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"seehuhn.de/go/geom/path"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/os2"
)

// MakeSimpleFont creates a simple font for use in unit tests.
//
// TODO(voss): remove
func MakeSimpleFont() *sfnt.Font {
	info, err := sfnt.Read(bytes.NewReader(goregular.TTF))
	if err != nil {
		panic(err)
	}

	var includeGid []glyph.ID
	cmap := cmap.Format4{}
	encoding := make([]glyph.ID, 256)

	includeGid = append(includeGid, 0, 1, 2, 3)
	cmap[0x000D] = glyph.ID(2)
	cmap[0x0020] = glyph.ID(3)
	encoding[0] = glyph.ID(1)
	encoding[0x000D] = glyph.ID(2)
	encoding[0x0020] = glyph.ID(3)

	fontCMap, err := info.CMapTable.GetBest()
	if err != nil {
		panic(err)
	}

	var topMin, topMax funit.Int16
	var bottomMin, bottomMax funit.Int16
	for c := 'A'; c <= 'Z'; c++ {
		gid := fontCMap.Lookup(c)
		cmap[uint16(c)] = glyph.ID(len(includeGid))
		encoding[c] = glyph.ID(len(includeGid))
		includeGid = append(includeGid, gid)

		ext := info.GlyphBBox(gid)
		top := ext.URy
		if c == 'A' || top < topMin {
			topMin = top
		}
		if c == 'A' || top > topMax {
			topMax = top
		}

		if c == 'Q' {
			continue
		}
		bottom := ext.LLy
		if c == 'A' || bottom < bottomMin {
			bottomMin = bottom
		}
		if c == 'A' || bottom > bottomMax {
			bottomMax = bottom
		}
	}

	origOutlines := info.Outlines.(*glyf.Outlines)
	newOutlines := &cff.Outlines{
		Private: []*type1.PrivateDict{
			{
				BlueValues: []funit.Int16{
					bottomMin, bottomMax, topMin, topMax,
				},
				BlueScale: 0.039625,
				BlueShift: 7,
				BlueFuzz:  1,
			},
		},
		FDSelect: func(glyph.ID) int {
			return 0
		},
		Encoding: encoding,
	}

	for _, gid := range includeGid {
		origGlyph := origOutlines.Glyphs[gid]
		cffGlyph := cff.NewGlyph(info.GlyphName(gid), info.GlyphWidth(gid))

		if origGlyph != nil {
			glyphPath := origOutlines.Glyphs.Path(gid)
			for contour := range glyphPath.Contours() {
				cubicContour := path.ToCubic(contour)
				for cmd, pts := range cubicContour {
					switch cmd {
					case path.CmdMoveTo:
						cffGlyph.MoveTo(pts[0].X, pts[0].Y)
					case path.CmdLineTo:
						cffGlyph.LineTo(pts[0].X, pts[0].Y)
					case path.CmdCubeTo:
						cffGlyph.CurveTo(pts[0].X, pts[0].Y, pts[1].X, pts[1].Y, pts[2].X, pts[2].Y)
					case path.CmdClose:
						// CFF glyphs auto-close, no explicit close needed
					}
				}
			}
		}
		newOutlines.Glyphs = append(newOutlines.Glyphs, cffGlyph)
	}

	ext := info.GlyphBBox(fontCMap.Lookup('M'))
	xMid := math.Round(float64(ext.URx+ext.LLx) / 2)
	yMid := math.Round(float64(ext.URy+ext.LLy) / 2)
	a := math.Round(math.Min(xMid, yMid) * 0.8)

	cffGlyph := cff.NewGlyph("marker.left", float64(ext.URx))
	cffGlyph.MoveTo(xMid, yMid)
	cffGlyph.LineTo(xMid-a, yMid-a)
	cffGlyph.LineTo(xMid-a, yMid+a)
	encoding['>'] = glyph.ID(len(newOutlines.Glyphs))
	cmap[uint16('>')] = glyph.ID(len(newOutlines.Glyphs))
	newOutlines.Glyphs = append(newOutlines.Glyphs, cffGlyph)

	cffGlyph = cff.NewGlyph("marker.right", float64(ext.URx))
	cffGlyph.MoveTo(xMid, yMid)
	cffGlyph.LineTo(xMid+a, yMid+a)
	cffGlyph.LineTo(xMid+a, yMid-a)
	encoding['<'] = glyph.ID(len(newOutlines.Glyphs))
	cmap[uint16('<')] = glyph.ID(len(newOutlines.Glyphs))
	newOutlines.Glyphs = append(newOutlines.Glyphs, cffGlyph)

	cffGlyph = cff.NewGlyph("marker", float64(ext.URx))
	cffGlyph.MoveTo(xMid, yMid)
	cffGlyph.LineTo(xMid-a, yMid-a)
	cffGlyph.LineTo(xMid-a, yMid+a)
	cffGlyph.LineTo(xMid, yMid)
	cffGlyph.LineTo(xMid+a, yMid+a)
	cffGlyph.LineTo(xMid+a, yMid-a)
	encoding['='] = glyph.ID(len(newOutlines.Glyphs))
	cmap[uint16('=')] = glyph.ID(len(newOutlines.Glyphs))
	newOutlines.Glyphs = append(newOutlines.Glyphs, cffGlyph)

	now := time.Now()
	res := &sfnt.Font{
		FamilyName: "Debug",
		Width:      info.Width,
		Weight:     info.Weight,
		IsRegular:  true,

		CodePageRange: 1 << os2.CP1252,

		Version:          0,
		CreationTime:     now,
		ModificationTime: now,

		UnitsPerEm: info.UnitsPerEm,
		FontMatrix: info.FontMatrix,

		Ascent:    info.Ascent,
		Descent:   info.Descent,
		LineGap:   info.LineGap,
		CapHeight: info.CapHeight,
		XHeight:   info.XHeight,

		UnderlinePosition:  info.UnderlinePosition,
		UnderlineThickness: info.UnderlineThickness,

		Outlines: newOutlines,
	}
	res.InstallCMap(cmap)

	return res
}
