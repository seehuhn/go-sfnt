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
	"testing"

	"github.com/google/go-cmp/cmp"

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
