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

package main

import (
	"log"
	"math"
	"os"
	"time"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/cmap"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/os2"
)

func main() {
	now := time.Now()
	info := &sfnt.Font{
		FamilyName: "Test",
		Weight:     os2.WeightNormal,
		Width:      os2.WidthNormal,

		Version:          0x00010000, // version 1.001
		CreationTime:     now,
		ModificationTime: now,

		Copyright: "Copyright (C) 2022  Jochen Voss <voss@seehuhn.de>",
		PermUse:   os2.PermInstall,

		UnitsPerEm: 1000,

		Ascent:  700,
		Descent: -300,
		LineGap: 200,
	}

	cffInfo := &cff.Outlines{}

	g := cff.NewGlyph(".notdef", 550) // GID 0
	g.MoveTo(0, 0)
	g.LineTo(500, 0)
	g.LineTo(500, 700)
	g.LineTo(0, 700)
	cffInfo.Glyphs = append(cffInfo.Glyphs, g)

	g = cff.NewGlyph("space", 550) // GID 1
	cffInfo.Glyphs = append(cffInfo.Glyphs, g)

	g = cff.NewGlyph("A", 550) // GID 2
	g.MoveTo(0, 0)
	g.LineTo(500, 0)
	g.LineTo(250, 710)
	cffInfo.Glyphs = append(cffInfo.Glyphs, g)

	g = cff.NewGlyph("B", 550) // GID 3
	g.MoveTo(0, 0)
	g.LineTo(200, 0)
	g.CurveTo(300, 0, 500, 75, 500, 175)
	g.CurveTo(500, 275, 300, 350, 200, 350)
	g.CurveTo(300, 350, 500, 425, 500, 525)
	g.CurveTo(500, 625, 300, 700, 200, 700)
	g.LineTo(0, 700)
	cffInfo.Glyphs = append(cffInfo.Glyphs, g)

	g = cff.NewGlyph("C", 650) // GID 4
	circle(g, 300, 300, 300)
	cffInfo.Glyphs = append(cffInfo.Glyphs, g)

	cffInfo.Private = []*type1.PrivateDict{
		{
			BlueValues: []funit.Int16{-10, 0, 700, 710},
		},
	}
	cffInfo.FDSelect = func(gi glyph.ID) int { return 0 }
	info.Outlines = cffInfo

	cmap := cmap.Format4{}
	cmap[' '] = 1
	cmap[0x00A0] = 1
	cmap['A'] = 2
	cmap['B'] = 3
	cmap['C'] = 4
	info.InstallCMap(cmap)

	info.CodePageRange.Set(os2.CP1252) // Latin 1

	// ----------------------------------------------------------------------

	out, err := os.Create("test.otf")
	if err != nil {
		log.Fatal(err)
	}
	_, err = info.Write(out)
	if err != nil {
		log.Fatal(err)
	}
	err = out.Close()
	if err != nil {
		log.Fatal(err)
	}
}

func circle(g *cff.Glyph, x, y, radius float64) {
	nSegment := 4
	dPhi := 2 * math.Pi / float64(nSegment)
	k := 4.0 / 3.0 * radius * math.Tan(dPhi/4)

	phi := 0.0
	x0 := x + radius*math.Cos(phi)
	y0 := y + radius*math.Sin(phi)
	g.MoveTo(x0, y0)

	for i := 0; i < nSegment; i++ {
		x1 := x0 - k*math.Sin(phi)
		y1 := y0 + k*math.Cos(phi)
		phi += dPhi
		x3 := x + radius*math.Cos(phi)
		y3 := y + radius*math.Sin(phi)
		x2 := x3 + k*math.Sin(phi)
		y2 := y3 - k*math.Cos(phi)
		g.CurveTo(x1, y1, x2, y2, x3, y3)
		x0 = x3
		y0 = y3
	}
}
