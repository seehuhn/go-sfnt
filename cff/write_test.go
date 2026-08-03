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

package cff

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"testing"

	"seehuhn.de/go/geom/matrix"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/postscript/cid"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/glyph"
)

// makeLargeFont returns a name-keyed font with n glyphs.  All glyph names
// except ".notdef" are non-standard, so each of them needs its own SID.
func makeLargeFont(n int) *Font {
	glyphs := make([]*Glyph, n)
	for i := range glyphs {
		g := &Glyph{Name: fmt.Sprintf("uni%05X", i), Width: 100}
		g.MoveTo(0, 0)
		g.LineTo(10, 0)
		glyphs[i] = g
	}
	if n > 0 {
		glyphs[0] = &Glyph{Name: ".notdef", Width: 100}
	}

	return &Font{
		FontInfo: &type1.FontInfo{
			FontName:   "Test-Regular",
			FontMatrix: matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
		},
		Outlines: &Outlines{
			Glyphs: glyphs,
			Private: []*type1.PrivateDict{{
				BlueScale: defaultBlueScale,
				BlueShift: defaultBlueShift,
				BlueFuzz:  defaultBlueFuzz,
			}},
			FDSelect: fdSelectSimple,
			Encoding: make([]glyph.ID, 256),
		},
	}
}

// TestWriteTooManyGlyphs checks that fonts which exceed the format limits are
// rejected instead of being written in truncated form.
func TestWriteTooManyGlyphs(t *testing.T) {
	// The CharStrings INDEX stores the glyph count in a 16-bit field.
	f := makeLargeFont(maxGlyphs + 1)
	if err := f.Write(io.Discard); err == nil {
		t.Error("expected an error")
	}

	// Every glyph name needs a SID, and SIDs above maxSID are reserved.
	f = makeLargeFont(maxGlyphs)
	if err := f.Write(io.Discard); err == nil {
		t.Error("expected an error")
	}
}

// TestWriteNameKeyedPrivateCount checks that name-keyed fonts must have
// exactly one private dictionary.  The Top DICT holds a single Private
// entry, so any further dictionaries would be written but never referenced,
// and reading the font back would silently lose them.
func TestWriteNameKeyedPrivateCount(t *testing.T) {
	f := makeLargeFont(3)
	f.Private = append(f.Private, &type1.PrivateDict{
		BlueScale: defaultBlueScale,
		BlueShift: defaultBlueShift,
		BlueFuzz:  defaultBlueFuzz,
	})
	if err := f.Write(io.Discard); err == nil {
		t.Error("font with two private dicts: expected an error")
	}

	f.Private = nil
	if err := f.Write(io.Discard); err == nil {
		t.Error("font with no private dict: expected an error")
	}
}

// makeManyFDFont returns a CID-keyed font with numFDs Font DICTs, using a
// separate Font DICT for each glyph.
func makeManyFDFont(numFDs int) *Font {
	glyphs := make([]*Glyph, numFDs)
	gidToCID := make([]cid.CID, numFDs)
	private := make([]*type1.PrivateDict, numFDs)
	fontMatrices := make([]matrix.Matrix, numFDs)
	for i := range glyphs {
		g := &Glyph{Width: 100}
		g.MoveTo(0, 0)
		g.LineTo(10, 0)
		glyphs[i] = g
		gidToCID[i] = cid.CID(i)
		private[i] = &type1.PrivateDict{
			BlueScale: defaultBlueScale,
			BlueShift: defaultBlueShift,
			BlueFuzz:  defaultBlueFuzz,
		}
		fontMatrices[i] = matrix.Identity
	}

	return &Font{
		FontInfo: &type1.FontInfo{
			FontName:   "Test-Regular",
			FontMatrix: matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
		},
		Outlines: &Outlines{
			Glyphs:       glyphs,
			Private:      private,
			FontMatrices: fontMatrices,
			FDSelect:     func(gid glyph.ID) int { return int(gid) },
			ROS: &cid.SystemInfo{
				Registry: "Adobe",
				Ordering: "Identity",
			},
			GIDToCID: gidToCID,
		},
	}
}

// TestWriteTooManyFontDICTs checks that fonts with more Font DICTs than
// FDSelect can address are rejected.  Without the check the Font DICT index
// would be truncated to eight bits and the font could not be read back.
func TestWriteTooManyFontDICTs(t *testing.T) {
	f := makeManyFDFont(maxFontDICTs)
	buf := &bytes.Buffer{}
	if err := f.Write(buf); err != nil {
		t.Fatalf("largest allowed font rejected: %v", err)
	}
	out, err := Read(bytes.NewReader(buf.Bytes()), membudget.New(1<<26))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	for gid := range maxFontDICTs {
		if fd := out.FDSelect(glyph.ID(gid)); fd != gid {
			t.Fatalf("FDSelect(%d) = %d, want %d", gid, fd, gid)
		}
	}

	f = makeManyFDFont(maxFontDICTs + 1)
	if err := f.Write(io.Discard); err == nil {
		t.Error("expected an error")
	}
}

// TestWriteFDSelectRange checks that Font DICT indices returned by FDSelect
// are validated when the font is written.  Without the check, out-of-range
// indices would be truncated to eight bits and Read would reject the output.
func TestWriteFDSelectRange(t *testing.T) {
	for _, bad := range []int{-1, 2, 300} {
		f := makeManyFDFont(2)
		f.FDSelect = func(glyph.ID) int { return bad }
		if err := f.Write(io.Discard); err == nil {
			t.Errorf("FDSelect returning %d: expected an error", bad)
		}
	}
}

// TestSelectWidthsUniform checks the width heuristic for fonts in which
// every glyph has the same advance width.  No glyph then encodes a width
// relative to nominalWidthX, but the value is still written to the private
// dict and must be a valid number.
func TestSelectWidthsUniform(t *testing.T) {
	f := makeLargeFont(3)
	def, nom := f.selectWidths()
	if def != 100 {
		t.Errorf("got default width %v, want 100", def)
	}
	if math.IsInf(nom, 0) || math.IsNaN(nom) {
		t.Errorf("got nominal width %v, want a finite number", nom)
	}
}

// TestWriteFractionalUnderline checks that fractional underline metrics
// survive a write-read cycle.  The DICT encoding supports real numbers, so
// the values must not be rounded to integers.
func TestWriteFractionalUnderline(t *testing.T) {
	f := makeLargeFont(2)
	f.FontInfo.UnderlinePosition = -100.5
	f.FontInfo.UnderlineThickness = 42.25

	buf := &bytes.Buffer{}
	if err := f.Write(buf); err != nil {
		t.Fatal(err)
	}
	out, err := Read(bytes.NewReader(buf.Bytes()), membudget.New(1<<26))
	if err != nil {
		t.Fatal(err)
	}

	if got := out.FontInfo.UnderlinePosition; got != -100.5 {
		t.Errorf("got underline position %v, want -100.5", got)
	}
	if got := out.FontInfo.UnderlineThickness; got != 42.25 {
		t.Errorf("got underline thickness %v, want 42.25", got)
	}
}

// TestWriteCharStringLength checks that glyphs whose charstring exceeds the
// 65535-byte limit of Type 2 charstring interpreters are rejected.
func TestWriteCharStringLength(t *testing.T) {
	f := makeLargeFont(1)
	g := f.Glyphs[0]
	g.MoveTo(0, 0)
	for i := range 40000 {
		g.LineTo(float64(i%100), float64(i%37))
	}
	if err := f.Write(io.Discard); err == nil {
		t.Error("expected an error")
	}
}

// TestWriteCFF2CharStringLength checks the corresponding limit for CFF2.
func TestWriteCFF2CharStringLength(t *testing.T) {
	cmds := make([]GlyphOpCFF2, 0, 40001)
	cmds = append(cmds, GlyphOpCFF2{Op: OpMoveTo, Args: blendArgs(0, 0)})
	for i := range 40000 {
		x := float64(i % 100)
		y := float64(i % 37)
		cmds = append(cmds, GlyphOpCFF2{Op: OpLineTo, Args: blendArgs(x, y)})
	}

	f := &FontCFF2{
		FontMatrix: matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
		OutlinesCFF2: &OutlinesCFF2{
			Glyphs:  []*GlyphCFF2{{Cmds: cmds}},
			Private: []*PrivateCFF2{{}},
		},
	}
	if err := f.Write(io.Discard); err == nil {
		t.Error("expected an error")
	}
}

// TestWriteCFF2FDSelectRange checks that Font DICT indices returned by
// FDSelect are validated when a CFF2 font is written.  Without the check the
// FDSelect table would contain indices which do not match the private dicts
// used for encoding the charstrings.
func TestWriteCFF2FDSelectRange(t *testing.T) {
	for _, bad := range []int{-1, 2, 300} {
		f := &FontCFF2{
			FontMatrix: matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
			OutlinesCFF2: &OutlinesCFF2{
				Glyphs:   []*GlyphCFF2{{}, {}},
				Private:  []*PrivateCFF2{{}, {}},
				FDSelect: func(glyph.ID) int { return bad },
			},
		}
		if err := f.Write(io.Discard); err == nil {
			t.Errorf("FDSelect returning %d: expected an error", bad)
		}
	}
}

// TestWriteCFF2TooManyFontDICTs checks the corresponding limit for CFF2.
// ReadCFF2 rejects such fonts, so writing them would break the round trip.
func TestWriteCFF2TooManyFontDICTs(t *testing.T) {
	const numFDs = maxFontDICTs + 1
	glyphs := make([]*GlyphCFF2, numFDs)
	private := make([]*PrivateCFF2, numFDs)
	for i := range glyphs {
		glyphs[i] = &GlyphCFF2{}
		private[i] = &PrivateCFF2{}
	}

	f := &FontCFF2{
		FontMatrix: matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
		OutlinesCFF2: &OutlinesCFF2{
			Glyphs:   glyphs,
			Private:  private,
			FDSelect: func(gid glyph.ID) int { return int(gid) },
		},
	}
	if err := f.Write(io.Discard); err == nil {
		t.Error("expected an error")
	}
}
