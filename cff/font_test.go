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

package cff

import (
	"bytes"
	"fmt"
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"
	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/sfnt/glyph"
)

func TestGlyphBBoxPDF(t *testing.T) {
	g := &Glyph{
		Name: "test",
		Cmds: []GlyphOp{
			{Op: OpMoveTo, Args: []float64{-16, -16}},
			{Op: OpLineTo, Args: []float64{128, -16}},
			{Op: OpLineTo, Args: []float64{128, 128}},
			{Op: OpLineTo, Args: []float64{-16, 128}},
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

func FuzzFont(f *testing.F) {
	f.Fuzz(func(t *testing.T, data []byte) {
		cff1, err := Read(bytes.NewReader(data))
		if err != nil {
			return
		}

		buf := &bytes.Buffer{}
		err = cff1.Write(buf)
		if err != nil {
			fmt.Println(cff1)
			t.Fatal(err)
		}

		cff2, err := Read(bytes.NewReader(buf.Bytes()))
		if err != nil {
			return
		}

		cmpFDSelectFn := cmp.Comparer(func(fn1, fn2 FDSelectFn) bool {
			for gid := 0; gid < len(cff1.Glyphs); gid++ {
				if fn1(glyph.ID(gid)) != fn2(glyph.ID(gid)) {
					return false
				}
			}
			return true
		})
		cmpFloat := cmp.Comparer(func(x, y float64) bool {
			diff := math.Abs(x - y)
			// For CFF 16.16 fixed-point encoding, the precision is 1/65536
			// But for large numbers, we need to allow for more error
			maxVal := math.Max(math.Abs(x), math.Abs(y))
			if maxVal == 0 {
				return diff < 1.0/65536
			}
			// Use relative tolerance for large values
			tolerance := math.Max(1.0/65536, maxVal*1e-6)
			return diff < tolerance
		})
		if diff := cmp.Diff(cff1, cff2, cmpFDSelectFn, cmpFloat); diff != "" {
			t.Errorf("different (-old +new):\n%s", diff)
		}
	})
}
