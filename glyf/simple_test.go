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
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestEncode(t *testing.T) {
	info := &GlyphInfo{
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

	encoded := info.Encode()
	decoded, err := encoded.Decode()
	if err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	if diff := cmp.Diff(info, decoded); diff != "" {
		t.Errorf("Round trip failed:\n%s", diff)
	}
}

func TestEncodeEmptyGlyph(t *testing.T) {
	info := &GlyphInfo{}

	decoded, err := info.Encode().Decode()
	if err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	if diff := cmp.Diff(info, decoded); diff != "" {
		t.Errorf("Round trip failed:\n%s", diff)
	}
}

func TestEncodeWithRepetition(t *testing.T) {
	info := &GlyphInfo{
		Contours: []Contour{
			{
				{X: 0, Y: 100, OnCurve: true},
				{X: 100, Y: 100, OnCurve: true},
				{X: 200, Y: 100, OnCurve: true},
				{X: 300, Y: 100, OnCurve: true},
			},
		},
	}

	encoded := info.Encode()

	if len(encoded.Encoded) > 50 {
		t.Errorf("Encoded size seems too large, repetition may not be working: %d bytes", len(encoded.Encoded))
	}

	decoded, err := encoded.Decode()
	if err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	if diff := cmp.Diff(info, decoded); diff != "" {
		t.Errorf("Round trip failed:\n%s", diff)
	}
}

func TestEncodeLargeCoordinates(t *testing.T) {
	info := &GlyphInfo{
		Contours: []Contour{
			{
				{X: 0, Y: 0, OnCurve: true},
				{X: 1000, Y: -500, OnCurve: true},
				{X: -2000, Y: 3000, OnCurve: true},
			},
		},
		Instructions: []byte{0xAA, 0xBB},
	}

	encoded := info.Encode()
	decoded, err := encoded.Decode()
	if err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	if diff := cmp.Diff(info, decoded); diff != "" {
		t.Errorf("Round trip failed:\n%s", diff)
	}
}
