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

package glyf

import (
	"bytes"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/geom/path"
	"seehuhn.de/go/geom/vec"
	"seehuhn.de/go/postscript/funit"
)

func TestEncode(t *testing.T) {
	info := &SimpleUnpacked{
		Contours: []Contour{
			{
				{X: 100, Y: 100, OnCurve: true},
				{X: 200, Y: 100, OnCurve: true},
				{X: 150, Y: 200, OnCurve: true},
			},
			{
				{X: 300, Y: 100, OnCurve: true},
				{X: 350, Y: 150, OnCurve: false},
				{X: 300, Y: 200, OnCurve: true},
				{X: 250, Y: 150, OnCurve: false},
			},
		},
		Instructions: []byte{0x01, 0x02, 0x03},
	}

	encoded := info.Pack()
	decoded, err := encoded.Unpack()
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if diff := cmp.Diff(info, decoded); diff != "" {
		t.Errorf("round trip failed:\n%s", diff)
	}
}

func TestEncodeEmptyGlyph(t *testing.T) {
	info := &SimpleUnpacked{}

	decoded, err := info.Pack().Unpack()
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if diff := cmp.Diff(info, decoded); diff != "" {
		t.Errorf("round trip failed:\n%s", diff)
	}
}

func TestEncodeWithRepetition(t *testing.T) {
	info := &SimpleUnpacked{
		Contours: []Contour{
			{
				{X: 0, Y: 100, OnCurve: true},
				{X: 100, Y: 100, OnCurve: true},
				{X: 200, Y: 100, OnCurve: true},
				{X: 300, Y: 100, OnCurve: true},
			},
		},
	}

	encoded := info.Pack()

	if len(encoded.Encoded) > 50 {
		t.Errorf("encoded size too large, repetition may not be working: %d bytes", len(encoded.Encoded))
	}

	decoded, err := encoded.Unpack()
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if diff := cmp.Diff(info, decoded); diff != "" {
		t.Errorf("round trip failed:\n%s", diff)
	}
}

func TestEncodeLargeCoordinates(t *testing.T) {
	info := &SimpleUnpacked{
		Contours: []Contour{
			{
				{X: 0, Y: 0, OnCurve: true},
				{X: 1000, Y: -500, OnCurve: true},
				{X: -2000, Y: 3000, OnCurve: true},
			},
		},
		Instructions: []byte{0xAA, 0xBB},
	}

	encoded := info.Pack()
	decoded, err := encoded.Unpack()
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if diff := cmp.Diff(info, decoded); diff != "" {
		t.Errorf("round trip failed:\n%s", diff)
	}
}

func TestGlyphInfo_AsGlyph(t *testing.T) {
	info := &SimpleUnpacked{
		Contours: []Contour{
			{
				{X: 100, Y: 110, OnCurve: true}, // bottom-left
				{X: 200, Y: 110, OnCurve: true}, // bottom-right
				{X: 200, Y: 210, OnCurve: true}, // top-right
				{X: 100, Y: 210, OnCurve: true}, // top-left
			},
		},
		Instructions: []byte{0x01, 0x02},
	}

	glyph := info.AsGlyph()

	expectedBBox := funit.Rect16{
		LLx: 100, LLy: 110,
		URx: 200, URy: 210,
	}
	if glyph.Rect16 != expectedBBox {
		t.Errorf("bounding box mismatch: got %+v, want %+v", glyph.Rect16, expectedBBox)
	}

	simpleGlyph, ok := glyph.Data.(SimpleGlyph)
	if !ok {
		t.Fatalf("expected SimpleGlyph, got %T", glyph.Data)
	}

	if simpleGlyph.NumContours != 1 {
		t.Errorf("expected 1 contour, got %d", simpleGlyph.NumContours)
	}

	if len(simpleGlyph.Encoded) == 0 {
		t.Error("encoded data should not be empty")
	}

	// verify round-trip
	decoded, err := simpleGlyph.Unpack()
	if err != nil {
		t.Fatalf("failed to decode glyph: %v", err)
	}

	if len(decoded.Contours) != len(info.Contours) {
		t.Errorf("contour count mismatch: got %d, want %d", len(decoded.Contours), len(info.Contours))
	}

	if len(decoded.Contours) > 0 && len(decoded.Contours[0]) != len(info.Contours[0]) {
		t.Errorf("point count mismatch: got %d, want %d", len(decoded.Contours[0]), len(info.Contours[0]))
	}

	if !bytes.Equal(decoded.Instructions, info.Instructions) {
		t.Errorf("instructions mismatch: got %02x, want %02x", decoded.Instructions, info.Instructions)
	}
}

// TestUnpackMalformedEndPts checks that non-monotonic endPtsOfContours
// values are rejected with an error rather than triggering a panic
// during contour construction.
func TestUnpackMalformedEndPts(t *testing.T) {
	// flag 0x31 = flagOnCurve | flagXSameOrPos | flagYSameOrPos,
	// so no x/y coordinate bytes are needed in the encoded payload.
	cases := []struct {
		name    string
		numCont int16
		encoded []byte
	}{
		{
			name:    "decreasing",
			numCont: 2,
			encoded: []byte{
				0x00, 0x0a, // endPtsOfContours[0] = 10
				0x00, 0x05, // endPtsOfContours[1] =  5
				0x00, 0x00, // instructionLength   =  0
				0x31, 0x31, 0x31, 0x31, 0x31, 0x31,
			},
		},
		{
			name:    "equal",
			numCont: 2,
			encoded: []byte{
				0x00, 0x05, // endPtsOfContours[0] = 5
				0x00, 0x05, // endPtsOfContours[1] = 5
				0x00, 0x00, // instructionLength  = 0
				0x31, 0x31, 0x31, 0x31, 0x31, 0x31,
			},
		},
		{
			name:    "max in middle",
			numCont: 3,
			encoded: []byte{
				0x00, 0x05, // endPtsOfContours[0] = 5
				0x00, 0x0a, // endPtsOfContours[1] = 10
				0x00, 0x03, // endPtsOfContours[2] =  3
				0x00, 0x00, // instructionLength   =  0
				0x31, 0x31, 0x31, 0x31,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sg := SimpleGlyph{NumContours: tc.numCont, Encoded: tc.encoded}
			if _, err := sg.Unpack(); err == nil {
				t.Errorf("expected error for malformed endPts, got nil")
			}
			// also verify that the public Path() surface does not panic
			for range sg.Path() {
				t.Errorf("malformed glyph yielded a path command")
			}
		})
	}
}

func TestGlyphInfo_AsGlyph_EmptyContours(t *testing.T) {
	info := &SimpleUnpacked{
		Contours:     []Contour{},
		Instructions: nil,
	}

	glyph := info.AsGlyph()

	if !glyph.Rect16.IsZero() {
		t.Errorf("bounding box should be zero for empty glyph: got %+v", glyph.Rect16)
	}

	simpleGlyph, ok := glyph.Data.(SimpleGlyph)
	if !ok {
		t.Fatalf("expected SimpleGlyph, got %T", glyph.Data)
	}

	if simpleGlyph.NumContours != 0 {
		t.Errorf("expected 0 contours, got %d", simpleGlyph.NumContours)
	}
}

type pathStep struct {
	Cmd path.Command
	Pts []vec.Vec2
}

func collectPath(p path.Path) []pathStep {
	var steps []pathStep
	for cmd, pts := range p {
		// the iterator reuses its point buffer
		steps = append(steps, pathStep{Cmd: cmd, Pts: slices.Clone(pts)})
	}
	return steps
}

func on(x, y funit.Int16) Point  { return Point{X: x, Y: y, OnCurve: true} }
func off(x, y funit.Int16) Point { return Point{X: x, Y: y} }

func move(x, y float64) pathStep { return pathStep{path.CmdMoveTo, []vec.Vec2{{X: x, Y: y}}} }
func line(x, y float64) pathStep { return pathStep{path.CmdLineTo, []vec.Vec2{{X: x, Y: y}}} }
func quad(cx, cy, x, y float64) pathStep {
	return pathStep{path.CmdQuadTo, []vec.Vec2{{X: cx, Y: cy}, {X: x, Y: y}}}
}

var closePath = pathStep{Cmd: path.CmdClose}

// TestSimpleGlyphPath checks the conversion of TrueType contours into path
// segments.  A contour is a closed cycle of points which may start at any
// point of the cycle, on-curve or not.
func TestSimpleGlyphPath(t *testing.T) {
	tests := []struct {
		name string
		cc   Contour
		want []pathStep
	}{
		{
			name: "polygon",
			cc:   Contour{on(0, 0), on(100, 0), on(0, 100)},
			want: []pathStep{move(0, 0), line(100, 0), line(0, 100), line(0, 0), closePath},
		},
		{
			// the off-curve point is a control point for the segment which
			// closes the contour
			name: "trailing control point",
			cc:   Contour{on(0, 0), on(100, 0), off(50, 80)},
			want: []pathStep{move(0, 0), line(100, 0), quad(50, 80, 0, 0), closePath},
		},
		{
			// the same cycle, but the stored contour starts at the control
			// point instead of at an on-curve point
			name: "leading control point",
			cc:   Contour{off(50, 80), on(0, 0), on(100, 0)},
			want: []pathStep{move(0, 0), line(100, 0), quad(50, 80, 0, 0), closePath},
		},
		{
			name: "implicit on-curve point",
			cc:   Contour{on(0, 0), off(40, 60), off(60, 60), on(100, 0)},
			want: []pathStep{move(0, 0), quad(40, 60, 50, 60), quad(60, 60, 100, 0), line(0, 0), closePath},
		},
		{
			// with no on-curve point at all, drawing starts halfway between
			// the last and the first point
			name: "all control points",
			cc:   Contour{off(0, 0), off(100, 0), off(100, 100), off(0, 100)},
			want: []pathStep{
				move(0, 50),
				quad(0, 0, 50, 0),
				quad(100, 0, 100, 50),
				quad(100, 100, 50, 100),
				quad(0, 100, 0, 50),
				closePath,
			},
		},
		{
			name: "control points on both sides of the start",
			cc:   Contour{off(0, 0), on(50, 50), off(100, 0)},
			want: []pathStep{move(50, 50), quad(100, 0, 50, 0), quad(0, 0, 50, 50), closePath},
		},
		{
			// implied on-curve points are computed in float64, so that
			// coordinates near the ends of the int16 range do not wrap
			name: "large coordinates",
			cc:   Contour{off(20000, 0), off(20000, 100), on(0, 100)},
			want: []pathStep{
				move(0, 100),
				quad(20000, 0, 20000, 50),
				quad(20000, 100, 0, 100),
				closePath,
			},
		},
		{
			name: "degenerate contour",
			cc:   Contour{on(50, 50)},
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sd := &SimpleUnpacked{Contours: []Contour{tc.cc}}
			if diff := cmp.Diff(tc.want, collectPath(sd.Path())); diff != "" {
				t.Errorf("wrong path (-want +got):\n%s", diff)
			}
		})
	}
}

// TestSimpleGlyphPathClosed checks that a contour is traced back to its
// starting point, wherever in the cycle the stored point list begins.
func TestSimpleGlyphPathClosed(t *testing.T) {
	contours := []Contour{
		{on(0, 0), on(100, 0), on(0, 100)},
		{on(0, 0), on(100, 0), off(50, 80)},
		{on(0, 0), off(40, 60), off(60, 60), on(100, 0)},
		{off(0, 0), off(100, 0), off(100, 100), off(0, 100)},
		{off(0, 0), on(50, 50), off(100, 0), on(80, 90), off(10, 10)},
	}

	for i, cc := range contours {
		for r := range len(cc) {
			rotated := slices.Concat(cc[r:], cc[:r])
			sd := &SimpleUnpacked{Contours: []Contour{rotated}}
			steps := collectPath(sd.Path())

			if len(steps) < 2 || steps[0].Cmd != path.CmdMoveTo {
				t.Fatalf("contour %d rotation %d: path does not start with MoveTo", i, r)
			}
			if steps[len(steps)-1].Cmd != path.CmdClose {
				t.Errorf("contour %d rotation %d: path does not end with Close", i, r)
				continue
			}

			start := steps[0].Pts[0]
			end := steps[len(steps)-2].Pts[len(steps[len(steps)-2].Pts)-1]
			if end != start {
				t.Errorf("contour %d rotation %d: contour ends at %v, want %v",
					i, r, end, start)
			}
		}
	}
}
