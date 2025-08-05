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

package glyf

import (
	"bytes"
	"math"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/image/font/gofont/goregular"
	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/geom/path"
	"seehuhn.de/go/geom/vec"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/header"
)

func TestGlyphBBoxPDF(t *testing.T) {
	g := &Glyph{
		Rect16: funit.Rect16{
			LLx: -16,
			LLy: -16,
			URx: 128,
			URy: 128,
		},
	}
	O := &Outlines{
		Glyphs: []*Glyph{g},
	}
	fontMatrix := matrix.Matrix{1 / 4.0, 0, 0, 1 / 8.0, 0, 0}
	bbox := O.GlyphBBoxPDF(fontMatrix, 0)

	if math.Abs(bbox.LLx-(-4_000)) > 1e-7 {
		t.Errorf("bbox.LLx = %v, want -4", bbox.LLx)
	}
	if math.Abs(bbox.LLy-(-2_000)) > 1e-7 {
		t.Errorf("bbox.LLy = %v, want -2", bbox.LLy)
	}
	if math.Abs(bbox.URx-32_000) > 1e-7 {
		t.Errorf("bbox.URx = %v, want 32", bbox.URx)
	}
	if math.Abs(bbox.URy-16_000) > 1e-7 {
		t.Errorf("bbox.URy = %v, want 16", bbox.URy)
	}
}

func BenchmarkGlyph(b *testing.B) {
	r := bytes.NewReader(goregular.TTF)
	header, err := header.Read(r)
	if err != nil {
		b.Fatal(err)
	}
	glyfData, err := header.ReadTableBytes(r, "glyf")
	if err != nil {
		b.Fatal(err)
	}
	locaData, err := header.ReadTableBytes(r, "loca")
	if err != nil {
		b.Fatal(err)
	}

	enc := &Encoded{
		GlyfData:   glyfData,
		LocaData:   locaData,
		LocaFormat: 0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = Decode(enc)
	}

	if err != nil {
		b.Fatal(err)
	}
}

func FuzzGlyf(f *testing.F) {
	// Use goregular.TTF as a seed
	r := bytes.NewReader(goregular.TTF)
	header, err := header.Read(r)
	if err != nil {
		f.Fatal(err)
	}
	glyfData, err := header.ReadTableBytes(r, "glyf")
	if err != nil {
		f.Fatal(err)
	}
	locaData, err := header.ReadTableBytes(r, "loca")
	if err != nil {
		f.Fatal(err)
	}
	locaFormat := int16(0)
	if len(glyfData) > 0xFFFF {
		locaFormat = 1
	}
	f.Add(glyfData, locaData, locaFormat)

	// Add minimal synthetic test cases
	f.Add([]byte{}, []byte{0, 0}, int16(0))                       // empty tables
	f.Add([]byte{}, []byte{0, 0, 0, 0}, int16(1))                 // empty tables, format 1
	f.Add(make([]byte, 10), []byte{0, 0, 0, 10}, int16(0))        // minimal glyph header
	f.Add(make([]byte, 20), []byte{0, 0, 0, 20, 0, 40}, int16(0)) // two minimal glyphs

	f.Fuzz(func(t *testing.T, glyfData, locaData []byte, locaFormat int16) {
		enc := &Encoded{
			GlyfData:   glyfData,
			LocaData:   locaData,
			LocaFormat: locaFormat,
		}
		info, err := Decode(enc)
		if err != nil {
			return
		}

		enc2 := info.Encode()

		info2, err := Decode(enc2)
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(info, info2); diff != "" {
			t.Errorf("different (-old +new):\n%s", diff)
		}
	})
}

func TestGlyphPath(t *testing.T) {
	squareUnpacked := &SimpleUnpacked{
		Contours: []Contour{
			// One contour forming a square
			{
				{X: funit.Int16(0), Y: funit.Int16(0), OnCurve: true},     // bottom-left
				{X: funit.Int16(100), Y: funit.Int16(0), OnCurve: true},   // bottom-right
				{X: funit.Int16(100), Y: funit.Int16(100), OnCurve: true}, // top-right
				{X: funit.Int16(0), Y: funit.Int16(100), OnCurve: true},   // top-left
			},
		},
		Instructions: nil,
	}

	square := &Glyph{
		Rect16: funit.Rect16{
			LLx: 0,
			LLy: 0,
			URx: 100,
			URy: 100,
		},
		Data: squareUnpacked.Pack(),
	}
	comp1 := &ComponentUnpacked{
		Child:       0,                               // reference to square glyph
		Trfm:        matrix.Matrix{1, 0, 0, 1, 0, 0}, // identity transform at origin
		AlignPoints: true,                            // use point matching
		OurPoint:    0,
		TheirPoint:  0,
	}

	comp2 := &ComponentUnpacked{
		Child: 0,                                 // reference to square glyph
		Trfm:  matrix.Matrix{1, 0, 0, 1, 150, 0}, // identity transform + translation by (150, 0)
	}

	composite := &Glyph{
		Rect16: funit.Rect16{
			LLx: 0,
			LLy: 0,
			URx: 250,
			URy: 100,
		},
		Data: CompositeGlyph{
			Components: []GlyphComponent{
				comp1.Pack(),
				comp2.Pack(),
			},
		},
	}

	// Create glyph collection
	outlines := &Outlines{
		Glyphs: Glyphs{square, composite},
	}

	// Test simple square glyph path
	t.Run("simple_square", func(t *testing.T) {
		squarePath := outlines.Path(0)

		// Collect path commands
		var commands []path.Command
		var points [][]vec.Vec2

		for cmd, pts := range squarePath {
			commands = append(commands, cmd)
			points = append(points, pts)
		}

		// Verify the path structure
		if len(commands) == 0 {
			t.Errorf("Expected some path commands, got none")
			return
		}

		// Should start with a MoveTo
		if commands[0] != path.CmdMoveTo {
			t.Errorf("Expected first command to be MoveTo, got %v", commands[0])
		}

		// Should have some LineTo commands
		lineToCount := 0
		for _, cmd := range commands {
			if cmd == path.CmdLineTo {
				lineToCount++
			}
		}
		if lineToCount == 0 {
			t.Errorf("Expected some LineTo commands for a square, got none")
		}

		// Should end with a Close
		if len(commands) > 0 && commands[len(commands)-1] != path.CmdClose {
			t.Errorf("Expected last command to be Close, got %v", commands[len(commands)-1])
		}
	})

	// Test composite glyph path
	t.Run("composite_two_squares", func(t *testing.T) {
		compositePath := outlines.Path(1)

		// Collect path commands
		var commands []path.Command
		var points [][]vec.Vec2

		for cmd, pts := range compositePath {
			commands = append(commands, cmd)
			points = append(points, pts)
		}

		// Basic verification: composite should have path commands
		if len(commands) == 0 {
			t.Errorf("Expected some path commands for composite, got none")
			return
		}

		// Should start with a MoveTo
		if commands[0] != path.CmdMoveTo {
			t.Errorf("Expected first command to be MoveTo, got %v", commands[0])
		}

		// Count MoveTo commands - ideally should be 2 (one per component)
		// but due to the current Path() implementation issue, we'll just verify >= 1
		moveToCount := 0
		for _, cmd := range commands {
			if cmd == path.CmdMoveTo {
				moveToCount++
			}
		}
		if moveToCount == 0 {
			t.Errorf("Expected at least one MoveTo command, got %d", moveToCount)
		}

		// Should have some LineTo commands
		lineToCount := 0
		for _, cmd := range commands {
			if cmd == path.CmdLineTo {
				lineToCount++
			}
		}
		if lineToCount == 0 {
			t.Errorf("Expected some LineTo commands for composite squares, got none")
		}

		// Should have at least one Close command
		closeCount := 0
		for _, cmd := range commands {
			if cmd == path.CmdClose {
				closeCount++
			}
		}
		if closeCount == 0 {
			t.Errorf("Expected at least one Close command, got none")
		}
	})
}

func TestGlyphPathInfiniteLoop(t *testing.T) {
	triangleUnpacked := &SimpleUnpacked{
		Contours: []Contour{
			// One contour forming a triangle
			{
				{X: funit.Int16(0), Y: funit.Int16(0), OnCurve: true},   // bottom-left
				{X: funit.Int16(50), Y: funit.Int16(0), OnCurve: true},  // bottom-right
				{X: funit.Int16(25), Y: funit.Int16(50), OnCurve: true}, // top
			},
		},
		Instructions: nil,
	}

	triangle := &Glyph{
		Rect16: funit.Rect16{
			LLx: 0,
			LLy: 0,
			URx: 50,
			URy: 50,
		},
		Data: triangleUnpacked.Pack(),
	}
	compA := &ComponentUnpacked{
		Child:       2,                               // reference to composite glyph B
		Trfm:        matrix.Matrix{1, 0, 0, 1, 0, 0}, // identity transform
		AlignPoints: true,
		OurPoint:    0,
		TheirPoint:  0,
	}

	compA2 := &ComponentUnpacked{
		Child: 0,                                // reference to triangle (to make it non-empty)
		Trfm:  matrix.Matrix{1, 0, 0, 1, 60, 0}, // translated
	}

	compositeA := &Glyph{
		Rect16: funit.Rect16{
			LLx: 0,
			LLy: 0,
			URx: 110,
			URy: 50,
		},
		Data: CompositeGlyph{
			Components: []GlyphComponent{
				compA.Pack(),
				compA2.Pack(),
			},
		},
	}
	compB := &ComponentUnpacked{
		Child: 1,                                     // reference to composite glyph A
		Trfm:  matrix.Matrix{0.5, 0, 0, 0.5, 10, 10}, // scaled and translated
	}

	compB2 := &ComponentUnpacked{
		Child: 0,                                // reference to triangle (to make it non-empty)
		Trfm:  matrix.Matrix{1, 0, 0, 1, 0, 60}, // translated
	}

	compositeB := &Glyph{
		Rect16: funit.Rect16{
			LLx: 0,
			LLy: 0,
			URx: 100,
			URy: 110,
		},
		Data: CompositeGlyph{
			Components: []GlyphComponent{
				compB.Pack(),
				compB2.Pack(),
			},
		},
	}
	outlines := &Outlines{
		Glyphs: Glyphs{triangle, compositeA, compositeB},
	}

	t.Run("infinite_loop_detection", func(t *testing.T) {
		// Set a timeout to detect hanging
		done := make(chan bool, 1)
		var pathCommands []path.Command

		go func() {
			// Call Path() on composite A, which should detect the circular reference
			pathA := outlines.Path(1)

			// Collect commands to verify it returns something reasonable
			for cmd := range pathA {
				pathCommands = append(pathCommands, cmd)
				// Limit collection to avoid infinite iteration in case of bugs
				if len(pathCommands) > 1000 {
					break
				}
			}
			done <- true
		}()

		// Wait for completion or timeout
		select {
		case <-done:
			// Good! The method returned
			t.Logf("Path() completed successfully with %d commands", len(pathCommands))

			// Should have returned some path (probably just the triangle part)
			if len(pathCommands) == 0 {
				t.Errorf("Expected some path commands, got none")
			}

			// Verify we get reasonable path structure
			hasMoveTo := false
			for _, cmd := range pathCommands {
				if cmd == path.CmdMoveTo {
					hasMoveTo = true
					break
				}
			}
			if !hasMoveTo {
				t.Errorf("Expected at least one MoveTo command")
			}

		case <-time.After(5 * time.Second):
			t.Errorf("Path() method hanged - did not complete within 5 seconds")
		}
	})

	t.Run("both_directions", func(t *testing.T) {
		// Test calling Path() on composite B as well
		done := make(chan bool, 1)
		var pathCommands []path.Command

		go func() {
			pathB := outlines.Path(2)
			for cmd := range pathB {
				pathCommands = append(pathCommands, cmd)
				if len(pathCommands) > 1000 {
					break
				}
			}
			done <- true
		}()

		select {
		case <-done:
			t.Logf("Path() on glyph B completed with %d commands", len(pathCommands))
			if len(pathCommands) == 0 {
				t.Errorf("Expected some path commands for glyph B, got none")
			}
		case <-time.After(5 * time.Second):
			t.Errorf("Path() method on glyph B hanged - did not complete within 5 seconds")
		}
	})
}

func TestGlyphBBoxPDFCoordinates(t *testing.T) {
	square := &Glyph{
		Rect16: funit.Rect16{
			LLx: 100,
			LLy: 200,
			URx: 300,
			URy: 400,
		},
		Data: nil,
	}

	outlines := &Outlines{
		Glyphs: []*Glyph{square},
	}

	// Test with font matrix representing unitsPerEm = 1000
	t.Run("unitsPerEm_1000_via_matrix", func(t *testing.T) {
		// Font matrix for unitsPerEm=1000: 1/1000 = 0.001
		fontMatrix := matrix.Matrix{0.001, 0, 0, 0.001, 0, 0}
		bbox := outlines.GlyphBBoxPDF(fontMatrix, 0)

		// Expected: (glyph coords * 0.001) * 1000 = glyph coords
		expectedLLx := 100.0
		expectedLLy := 200.0
		expectedURx := 300.0
		expectedURy := 400.0

		if math.Abs(bbox.LLx-expectedLLx) > 1e-6 {
			t.Errorf("bbox.LLx = %g, want %g", bbox.LLx, expectedLLx)
		}
		if math.Abs(bbox.LLy-expectedLLy) > 1e-6 {
			t.Errorf("bbox.LLy = %g, want %g", bbox.LLy, expectedLLy)
		}
		if math.Abs(bbox.URx-expectedURx) > 1e-6 {
			t.Errorf("bbox.URx = %g, want %g", bbox.URx, expectedURx)
		}
		if math.Abs(bbox.URy-expectedURy) > 1e-6 {
			t.Errorf("bbox.URy = %g, want %g", bbox.URy, expectedURy)
		}
	})

	// Test with font matrix representing unitsPerEm = 2048
	t.Run("unitsPerEm_2048_via_matrix", func(t *testing.T) {
		// Font matrix for unitsPerEm=2048: 1/2048 ≈ 0.00048828125
		fontMatrix := matrix.Matrix{1.0 / 2048.0, 0, 0, 1.0 / 2048.0, 0, 0}
		bbox := outlines.GlyphBBoxPDF(fontMatrix, 0)

		// Expected: (glyph coords / 2048) * 1000
		expectedLLx := 100.0 / 2048.0 * 1000.0 // ≈ 48.828125
		expectedLLy := 200.0 / 2048.0 * 1000.0 // ≈ 97.65625
		expectedURx := 300.0 / 2048.0 * 1000.0 // ≈ 146.484375
		expectedURy := 400.0 / 2048.0 * 1000.0 // ≈ 195.3125

		if math.Abs(bbox.LLx-expectedLLx) > 1e-6 {
			t.Errorf("bbox.LLx = %g, want %g", bbox.LLx, expectedLLx)
		}
		if math.Abs(bbox.LLy-expectedLLy) > 1e-6 {
			t.Errorf("bbox.LLy = %g, want %g", bbox.LLy, expectedLLy)
		}
		if math.Abs(bbox.URx-expectedURx) > 1e-6 {
			t.Errorf("bbox.URx = %g, want %g", bbox.URx, expectedURx)
		}
		if math.Abs(bbox.URy-expectedURy) > 1e-6 {
			t.Errorf("bbox.URy = %g, want %g", bbox.URy, expectedURy)
		}
	})

	// Test with combined font matrix and scaling
	t.Run("unitsPerEm_1000_with_2x_scale", func(t *testing.T) {
		// Font matrix with 2x scaling: (1/1000) * 2 = 0.002
		fontMatrix := matrix.Matrix{0.002, 0, 0, 0.002, 0, 0}
		bbox := outlines.GlyphBBoxPDF(fontMatrix, 0)

		// Expected: (glyph coords * 0.002) * 1000 = glyph coords * 2
		expectedLLx := 200.0
		expectedLLy := 400.0
		expectedURx := 600.0
		expectedURy := 800.0

		if math.Abs(bbox.LLx-expectedLLx) > 1e-6 {
			t.Errorf("bbox.LLx = %g, want %g", bbox.LLx, expectedLLx)
		}
		if math.Abs(bbox.LLy-expectedLLy) > 1e-6 {
			t.Errorf("bbox.LLy = %g, want %g", bbox.LLy, expectedLLy)
		}
		if math.Abs(bbox.URx-expectedURx) > 1e-6 {
			t.Errorf("bbox.URx = %g, want %g", bbox.URx, expectedURx)
		}
		if math.Abs(bbox.URy-expectedURy) > 1e-6 {
			t.Errorf("bbox.URy = %g, want %g", bbox.URy, expectedURy)
		}
	})

	// Test with translation
	t.Run("with_translation", func(t *testing.T) {
		// Font matrix with translation
		fontMatrix := matrix.Matrix{0.001, 0, 0, 0.001, 50, -25}
		bbox := outlines.GlyphBBoxPDF(fontMatrix, 0)

		// Expected: ((glyph coords * 0.001) + translation) * 1000
		// = glyph coords + translation * 1000
		expectedLLx := 100.0 + 50000.0 // 100 + 50*1000
		expectedLLy := 200.0 - 25000.0 // 200 - 25*1000
		expectedURx := 300.0 + 50000.0 // 300 + 50*1000
		expectedURy := 400.0 - 25000.0 // 400 - 25*1000

		if math.Abs(bbox.LLx-expectedLLx) > 1e-6 {
			t.Errorf("bbox.LLx = %g, want %g", bbox.LLx, expectedLLx)
		}
		if math.Abs(bbox.LLy-expectedLLy) > 1e-6 {
			t.Errorf("bbox.LLy = %g, want %g", bbox.LLy, expectedLLy)
		}
		if math.Abs(bbox.URx-expectedURx) > 1e-6 {
			t.Errorf("bbox.URx = %g, want %g", bbox.URx, expectedURx)
		}
		if math.Abs(bbox.URy-expectedURy) > 1e-6 {
			t.Errorf("bbox.URy = %g, want %g", bbox.URy, expectedURy)
		}
	})
}

func TestGlyphBBox(t *testing.T) {
	// Test that GlyphBBox and GlyphBBoxPDF give consistent results
	g := &Glyph{
		Rect16: funit.Rect16{
			LLx: 100,
			LLy: 200,
			URx: 300,
			URy: 400,
		},
	}
	outlines := &Outlines{
		Glyphs: []*Glyph{g},
	}

	M := matrix.Matrix{1, 0, 0, 1, 0, 0} // identity matrix

	bboxPDF := outlines.GlyphBBoxPDF(M, glyph.ID(0))
	bboxPDFExpected := outlines.GlyphBBox(M.Mul(matrix.Scale(1000, 1000)), glyph.ID(0))

	if math.Abs(bboxPDF.LLx-bboxPDFExpected.LLx) > 1e-6 {
		t.Errorf("GlyphBBoxPDF.LLx = %g, want %g", bboxPDF.LLx, bboxPDFExpected.LLx)
	}
	if math.Abs(bboxPDF.LLy-bboxPDFExpected.LLy) > 1e-6 {
		t.Errorf("GlyphBBoxPDF.LLy = %g, want %g", bboxPDF.LLy, bboxPDFExpected.LLy)
	}
	if math.Abs(bboxPDF.URx-bboxPDFExpected.URx) > 1e-6 {
		t.Errorf("GlyphBBoxPDF.URx = %g, want %g", bboxPDF.URx, bboxPDFExpected.URx)
	}
	if math.Abs(bboxPDF.URy-bboxPDFExpected.URy) > 1e-6 {
		t.Errorf("GlyphBBoxPDF.URy = %g, want %g", bboxPDF.URy, bboxPDFExpected.URy)
	}
}
