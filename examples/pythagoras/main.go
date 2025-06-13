// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2025  Jochen Voss <voss@seehuhn.de>
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

// pythagoras generates a TrueType font containing Pythagoras tree fractals
// as composite glyphs, demonstrating recursive glyph composition.
//
// A Pythagoras tree is a fractal constructed from squares:
//   - Level 0: A single square
//   - Level k>0: A base square with two smaller Pythagoras trees (level k-1)
//     attached to its top edge, scaled by √2/2 and rotated ±45°
//
// The font contains 4 glyphs:
// - GID 0: Base square (level 0)
// - GID 1: Level 1 tree (mapped to 'A')
// - GID 2: Level 2 tree (mapped to 'B')
// - GID 3: Level 3 tree (mapped to 'C')
//
// This demonstrates the use of ComponentUnpacked.ArgsArePointIndices
// for precise alignment of composite glyph components.
package main

import (
	"log"
	"math"
	"os"
	"time"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/maxp"
	"seehuhn.de/go/sfnt/os2"
)

const (
	unitsPerEm = 1024
	squareSize = 320

	alpha = 0.6 // angle to the top point in radians
)

func main() {
	// Create the font structure
	now := time.Now()
	info := &sfnt.Font{
		FamilyName: "Pythagoras",
		Weight:     os2.WeightNormal,
		Width:      os2.WidthNormal,
		IsRegular:  true,

		Version:          0x00010000, // version 1.0
		CreationTime:     now,
		ModificationTime: now,

		Copyright: "Copyright (C) 2025  Jochen Voss <voss@seehuhn.de>",
		PermUse:   os2.PermInstall,

		UnitsPerEm: unitsPerEm,

		SampleText: "ABC",

		Ascent:    unitsPerEm,
		Descent:   0,
		LineGap:   unitsPerEm,
		CapHeight: unitsPerEm,
	}

	// Create the glyphs
	glyphs := createPythagorasGlyphs()

	widths := make([]funit.Int16, len(glyphs))
	for i := range widths {
		widths[i] = unitsPerEm
	}

	info.Outlines = &glyf.Outlines{
		Glyphs: glyphs,
		Widths: widths,
		Maxp: &maxp.TTFInfo{
			MaxPoints:             4,
			MaxContours:           1,
			MaxCompositePoints:    15 * (4 + 4),
			MaxCompositeContours:  15,
			MaxZones:              2,
			MaxTwilightPoints:     0,
			MaxStorage:            0,
			MaxFunctionDefs:       0,
			MaxInstructionDefs:    0,
			MaxStackElements:      0,
			MaxSizeOfInstructions: 0,
			MaxComponentElements:  2,
			MaxComponentDepth:     3,
		},
	}

	// Create character mapping (A=65, B=66, C=67)
	subtable := make(cmap.Format4)
	subtable[65] = 1 // A -> Pythagoras level 1
	subtable[66] = 2 // B -> Pythagoras level 2
	subtable[67] = 3 // C -> Pythagoras level 3

	// Create cmap table with the subtable
	cmapTable := make(cmap.Table)
	cmapTable[cmap.Key{PlatformID: 3, EncodingID: 1}] = subtable.Encode(0) // Windows Unicode BMP, language 0
	info.CMapTable = cmapTable

	// Write the font file
	fd, err := os.Create("test.ttf")
	if err != nil {
		log.Fatal(err)
	}
	defer fd.Close()

	_, err = info.Write(fd)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Generated test.ttf with Pythagoras tree fractals")
	log.Println("A = level 1, B = level 2, C = level 3")
}

// createPythagorasGlyphs creates all the glyphs for the Pythagoras tree
func createPythagorasGlyphs() glyf.Glyphs {
	square := createSquareGlyph()

	level1 := createPythagorasLevel(1)
	level2 := createPythagorasLevel(2)
	level3 := createPythagorasLevel(3)

	return glyf.Glyphs{square, level1, level2, level3}
}

// createSquareGlyph creates the base square glyph (level 0 Pythagoras tree)
func createSquareGlyph() *glyf.Glyph {
	left := funit.Int16((unitsPerEm - squareSize) / 2)

	squareUnpacked := &glyf.SimpleUnpacked{
		Contours: []glyf.Contour{
			{
				{X: left, Y: 0, OnCurve: true},                       // point 0: bottom-left
				{X: left, Y: squareSize, OnCurve: true},              // point 1: top-left
				{X: left + squareSize, Y: squareSize, OnCurve: true}, // point 2: top-right
				{X: left + squareSize, Y: 0, OnCurve: true},          // point 3: bottom-right
			},
		},
	}

	return &glyf.Glyph{
		Rect16: funit.Rect16{
			LLx: 0,
			LLy: 0,
			URx: unitsPerEm,
			URy: unitsPerEm,
		},
		Data: squareUnpacked.Pack(),
	}
}

// createPythagorasLevel creates a Pythagoras tree of the specified level
func createPythagorasLevel(k int) *glyf.Glyph {
	// A Pythagoras tree of level k consists of:
	// 1. A base square (level 0)
	// 2. Two smaller Pythagoras trees of level k-1 attached to the top

	subtreeGID := glyph.ID(k - 1)

	base := &glyf.ComponentUnpacked{
		Child:        0,
		Trfm:         matrix.Matrix{1, 0, 0, 1, 0, 0},
		UseMyMetrics: true,
	}

	// TODO(voss): try to use ComponentUnpacked.AlignPoints here.
	// I tried, but somehow this seems to interact with the rotation
	// in ways which seem to contradict the TrueType spec.

	ca := math.Cos(alpha)
	sa := math.Sin(alpha)
	D := float64((unitsPerEm - squareSize) / 2)

	scale := ca
	dx := D - scale*ca*D
	dy := float64(squareSize) - scale*sa*D
	left := &glyf.ComponentUnpacked{
		Child: subtreeGID,
		Trfm: matrix.Matrix{
			scale * ca, scale * sa, -scale * sa, scale * ca,
			dx, dy,
		},
		RoundXYToGrid: true,
	}

	scale = sa
	dx = D + squareSize - scale*sa*(D+squareSize)
	dy = squareSize + scale*ca*(D+squareSize)
	right := &glyf.ComponentUnpacked{
		Child: subtreeGID,
		Trfm: matrix.Matrix{
			scale * sa, -scale * ca, scale * ca, scale * sa,
			dx, dy,
		},
		RoundXYToGrid: true,
	}

	return &glyf.Glyph{
		Rect16: funit.Rect16{
			LLx: 0,
			LLy: 0,
			URx: squareSize,
			URy: squareSize,
		},
		Data: glyf.CompositeGlyph{
			Components: []glyf.GlyphComponent{
				base.Pack(),
				left.Pack(),
				right.Pack(),
			},
		},
	}
}
