// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2026  Jochen Voss <voss@seehuhn.de>
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

//go:generate python3 ../scripts/gen-var-expect.py $QUIRE_TESTFONTS/Junicode-VF.ttf -o testdata/varexpect/junicode.json --case defaults: --case bold-narrow:wght=700,wdth=87.5,ENLA=0 --case light:wght=300 --glyph A --glyph a --glyph B --glyph b --glyph O --glyph g --glyph f --glyph eacute
//go:generate python3 ../scripts/gen-var-expect.py $QUIRE_TESTFONTS/Elstob-VF.ttf -o testdata/varexpect/elstob.json --case defaults: --case bold-display:wght=800,opsz=18,GRAD=1,SPAC=1 --glyph A --glyph a --glyph O --glyph eacute

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"seehuhn.de/go/postscript/funit"

	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/internal/testfonts"
	"seehuhn.de/go/sfnt/parser"
)

// pointTolerance bounds the float-artifact slack between Instantiate's
// integer glyf coordinates and fontTools' independently computed ones; a
// larger discrepancy is a genuine disagreement, not a tolerance to paper
// over.
const pointTolerance = 0.01

// varExpectDoc is the JSON shape written by
// examples/scripts/gen-var-expect.py.
type varExpectDoc struct {
	FontToolsVersion string          `json:"fonttools_version"`
	SourceFont       string          `json:"source_font"`
	SourceSHA256     string          `json:"source_sha256"`
	Cases            []varExpectCase `json:"cases"`
}

type varExpectCase struct {
	Name    string                    `json:"name"`
	Coords  map[string]float64        `json:"coords"`
	Glyphs  map[string]varExpectGlyph `json:"glyphs"`
	Metrics varExpectMetrics          `json:"metrics"`
}

type varExpectGlyph struct {
	GID          int                `json:"gid"`
	AdvanceWidth int                `json:"advance_width"`
	Contours     [][]varExpectPoint `json:"contours"`
}

type varExpectMetrics struct {
	Ascent    int  `json:"ascent"`
	Descent   int  `json:"descent"`
	CapHeight *int `json:"cap_height"`
}

// varExpectPoint is a raw glyf point: [x, y, onCurve] in the source JSON.
type varExpectPoint struct {
	X, Y    float64
	OnCurve bool
}

func (p *varExpectPoint) UnmarshalJSON(data []byte) error {
	var raw [3]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	x, ok1 := raw[0].(float64)
	y, ok2 := raw[1].(float64)
	onCurve, ok3 := raw[2].(bool)
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("sfnt: malformed varexpect point %v", raw)
	}
	p.X, p.Y, p.OnCurve = x, y, onCurve
	return nil
}

// TestVarExpect compares [Font.Instantiate] against fontTools'
// varLib.instancer, using the ground truth recorded in
// testdata/varexpect/*.json (regenerate with `go generate`, which requires
// QUIRE_TESTFONTS to point at a directory containing the source fonts).
func TestVarExpect(t *testing.T) {
	files, err := filepath.Glob("testdata/varexpect/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no testdata/varexpect/*.json files found")
	}
	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			runVarExpectFile(t, path)
		})
	}
}

func runVarExpectFile(t *testing.T, jsonPath string) {
	t.Helper()

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc varExpectDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}

	fontPath := testfonts.Path(t, doc.SourceFont)
	fontData, err := os.ReadFile(fontPath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(fontData)
	if got := hex.EncodeToString(sum[:]); got != doc.SourceSHA256 {
		t.Fatalf("source font sha256 = %s, want %s (stale testdata, re-run go generate)", got, doc.SourceSHA256)
	}

	f, err := Read(bytes.NewReader(fontData), parser.NewBudget(int64(len(fontData))))
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range doc.Cases {
		t.Run(c.Name, func(t *testing.T) {
			runVarExpectCase(t, f, c)
		})
	}
}

func runVarExpectCase(t *testing.T, f *Font, c varExpectCase) {
	t.Helper()

	inst, err := f.Instantiate(c.Coords)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := int(inst.Ascent), c.Metrics.Ascent; got != want {
		t.Errorf("ascent = %d, want %d", got, want)
	}
	if got, want := int(inst.Descent), c.Metrics.Descent; got != want {
		t.Errorf("descent = %d, want %d", got, want)
	}
	if c.Metrics.CapHeight != nil {
		if got, want := int(inst.CapHeight), *c.Metrics.CapHeight; got != want {
			t.Errorf("cap height = %d, want %d", got, want)
		}
	}

	outlines, ok := inst.Outlines.(*glyf.Outlines)
	if !ok {
		t.Fatalf("instanced outlines have type %T, want *glyf.Outlines", inst.Outlines)
	}

	names := make([]string, 0, len(c.Glyphs))
	for name := range c.Glyphs {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		want := c.Glyphs[name]
		t.Run(name, func(t *testing.T) {
			checkVarExpectGlyph(t, outlines, glyph.ID(want.GID), want)
		})
	}
}

func checkVarExpectGlyph(t *testing.T, outlines *glyf.Outlines, gid glyph.ID, want varExpectGlyph) {
	t.Helper()

	if int(gid) >= len(outlines.Widths) {
		t.Fatalf("gid %d out of range (numGlyphs=%d)", gid, len(outlines.Widths))
	}
	if got := int(outlines.Widths[gid]); got != want.AdvanceWidth {
		t.Errorf("advance width = %d, want %d", got, want.AdvanceWidth)
	}

	gotContours, err := decomposeContours(outlines.Glyphs, gid, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotContours) != len(want.Contours) {
		t.Fatalf("contour count = %d, want %d", len(gotContours), len(want.Contours))
	}
	for ci, wantContour := range want.Contours {
		gotContour := gotContours[ci]
		if len(gotContour) != len(wantContour) {
			t.Fatalf("contour %d point count = %d, want %d", ci, len(gotContour), len(wantContour))
		}
		for pi, wantPt := range wantContour {
			gotPt := gotContour[pi]
			if gotPt.OnCurve != wantPt.OnCurve {
				t.Errorf("contour %d point %d on-curve = %v, want %v", ci, pi, gotPt.OnCurve, wantPt.OnCurve)
			}
			dx := math.Abs(float64(gotPt.X) - wantPt.X)
			dy := math.Abs(float64(gotPt.Y) - wantPt.Y)
			if dx > pointTolerance || dy > pointTolerance {
				t.Errorf("contour %d point %d = (%d, %d), want (%v, %v)",
					ci, pi, gotPt.X, gotPt.Y, wantPt.X, wantPt.Y)
			}
		}
	}
}

const maxVarExpectCompositeDepth = 64

// decomposeContours returns glyph gid's raw outline points, with composite
// components resolved into absolute coordinates (translation and 2x2
// transform only; point-matched components are not supported since none of
// the fixture glyphs use them). It mirrors fontTools'
// Glyph.getCoordinates(): no curve flattening, no implied on-curve points.
func decomposeContours(glyphs glyf.Glyphs, gid glyph.ID, depth int) ([]glyf.Contour, error) {
	if depth > maxVarExpectCompositeDepth {
		return nil, errors.New("sfnt: composite glyph nesting too deep")
	}
	if int(gid) >= len(glyphs) || glyphs[gid] == nil {
		return nil, nil
	}

	switch d := glyphs[gid].Data.(type) {
	case glyf.SimpleGlyph:
		su, err := d.Unpack()
		if err != nil {
			return nil, err
		}
		return su.Contours, nil

	case glyf.CompositeGlyph:
		var all []glyf.Contour
		for _, comp := range d.Components {
			cu, err := comp.Unpack()
			if err != nil {
				return nil, err
			}
			if cu.AlignPoints {
				return nil, errors.New("sfnt: point-matched composite component not supported by test decoder")
			}
			childContours, err := decomposeContours(glyphs, cu.Child, depth+1)
			if err != nil {
				return nil, err
			}
			for _, cc := range childContours {
				nc := make(glyf.Contour, len(cc))
				for i, p := range cc {
					x, y := transformComponentPoint(cu, float64(p.X), float64(p.Y))
					nc[i] = glyf.Point{
						X:       funit.Int16(math.Round(x)),
						Y:       funit.Int16(math.Round(y)),
						OnCurve: p.OnCurve,
					}
				}
				all = append(all, nc)
			}
		}
		return all, nil

	default:
		return nil, fmt.Errorf("sfnt: unexpected glyph data type %T", d)
	}
}

// transformComponentPoint applies a composite component's transform to a
// child point, honouring ScaledComponentOffset: unscaled ("Microsoft way")
// scales first and translates second, scaled ("Apple way") translates first
// and scales second.
func transformComponentPoint(cu *glyf.ComponentUnpacked, x, y float64) (float64, float64) {
	m := cu.Trfm
	dx, dy := m[4], m[5]
	if cu.ScaledComponentOffset {
		x, y = x+dx, y+dy
		return m[0]*x + m[2]*y, m[1]*x + m[3]*y
	}
	return m[0]*x + m[2]*y + dx, m[1]*x + m[3]*y + dy
}
