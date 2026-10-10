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
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/geom/path"
	"seehuhn.de/go/postscript/cid"
	"seehuhn.de/go/sfnt/glyph"
)

// TestPathImplicitClose verifies that Path() correctly closes subpaths
// before each MoveTo, as required by CFF/Type2 semantics.
func TestPathImplicitClose(t *testing.T) {
	// Create a glyph with two subpaths where the first has no explicit close
	// (similar to glyph "P" which has a bowl and stem as separate subpaths)
	g := &Glyph{
		Name: "test",
		Cmds: []GlyphOp{
			// first subpath (e.g., inner bowl)
			{Op: OpMoveTo, Args: []float64{10, 10}},
			{Op: OpLineTo, Args: []float64{20, 10}},
			{Op: OpLineTo, Args: []float64{20, 20}},
			{Op: OpLineTo, Args: []float64{10, 20}},
			// second subpath starts here - should implicitly close first
			{Op: OpMoveTo, Args: []float64{0, 0}},
			{Op: OpLineTo, Args: []float64{30, 0}},
			{Op: OpLineTo, Args: []float64{30, 30}},
			{Op: OpLineTo, Args: []float64{0, 30}},
		},
	}
	o := &Outlines{
		Glyphs: []*Glyph{g},
	}

	// collect path commands
	var cmds []path.Command
	for cmd := range o.Path(0) {
		cmds = append(cmds, cmd)
	}

	// expected: MoveTo, LineTo, LineTo, LineTo, Close, MoveTo, LineTo, LineTo, LineTo, Close
	expected := []path.Command{
		path.CmdMoveTo,
		path.CmdLineTo,
		path.CmdLineTo,
		path.CmdLineTo,
		path.CmdClose, // implicit close before second MoveTo
		path.CmdMoveTo,
		path.CmdLineTo,
		path.CmdLineTo,
		path.CmdLineTo,
		path.CmdClose, // final close
	}

	if len(cmds) != len(expected) {
		t.Fatalf("got %d commands, want %d", len(cmds), len(expected))
	}

	for i, cmd := range cmds {
		if cmd != expected[i] {
			t.Errorf("command %d: got %v, want %v", i, cmd, expected[i])
		}
	}
}

// TestPathSingleSubpath verifies that a single subpath is closed correctly.
func TestPathSingleSubpath(t *testing.T) {
	g := &Glyph{
		Name: "test",
		Cmds: []GlyphOp{
			{Op: OpMoveTo, Args: []float64{0, 0}},
			{Op: OpLineTo, Args: []float64{10, 0}},
			{Op: OpLineTo, Args: []float64{10, 10}},
		},
	}
	o := &Outlines{
		Glyphs: []*Glyph{g},
	}

	var cmds []path.Command
	for cmd := range o.Path(0) {
		cmds = append(cmds, cmd)
	}

	expected := []path.Command{
		path.CmdMoveTo,
		path.CmdLineTo,
		path.CmdLineTo,
		path.CmdClose,
	}

	if len(cmds) != len(expected) {
		t.Fatalf("got %d commands, want %d", len(cmds), len(expected))
	}

	for i, cmd := range cmds {
		if cmd != expected[i] {
			t.Errorf("command %d: got %v, want %v", i, cmd, expected[i])
		}
	}
}

// TestPathEmptyGlyph verifies that an empty glyph produces no path commands.
func TestPathEmptyGlyph(t *testing.T) {
	g := &Glyph{
		Name: "empty",
		Cmds: []GlyphOp{},
	}
	o := &Outlines{
		Glyphs: []*Glyph{g},
	}

	var cmds []path.Command
	for cmd := range o.Path(0) {
		cmds = append(cmds, cmd)
	}

	if len(cmds) != 0 {
		t.Errorf("got %d commands for empty glyph, want 0", len(cmds))
	}
}

// TestGlyphMatrixMissingFD verifies that GlyphMatrix falls back to the
// supplied top matrix when a malformed CID-keyed font selects an FD that
// has no per-FD matrix, rather than panicking.
func TestGlyphMatrixMissingFD(t *testing.T) {
	top := matrix.Matrix{0.002, 0, 0, 0.002, 0, 0}
	o := &Outlines{
		ROS:          &cid.SystemInfo{Registry: "x", Ordering: "y"},
		FontMatrices: nil, // malformed: CID-keyed but no per-FD matrices
		FDSelect:     func(glyph.ID) int { return 0 },
	}
	got := o.GlyphMatrix(top, 0)
	if got != top {
		t.Errorf("GlyphMatrix with missing FD: got %v, want %v", got, top)
	}
}

// TestRepairFontMatrices checks that font matrices which leave a glyph
// without a finite effective font matrix or advance scale are replaced on
// read, and that the repaired font survives a write/read cycle.
func TestRepairFontMatrices(t *testing.T) {
	def := defaultFontMatrix
	id := matrix.Identity
	huge := matrix.Scale(1e200, 1e200)
	sheared := matrix.Matrix{1, 1e300, 1e300, 1, 0, 0}
	cases := []struct {
		name    string
		top     matrix.Matrix
		fds     []matrix.Matrix
		wantTop matrix.Matrix
		wantFDs []matrix.Matrix
	}{
		{"usable", huge, nil, huge, nil},
		{"shear overflows", sheared, nil, def, nil},
		{"usable composition", huge, []matrix.Matrix{matrix.Scale(1e-200, 1e-200), def}, huge, []matrix.Matrix{matrix.Scale(1e-200, 1e-200), def}},
		{"FD at fault", huge, []matrix.Matrix{def, huge}, huge, []matrix.Matrix{def, def}},
		{"top at fault", sheared, []matrix.Matrix{def, huge}, id, []matrix.Matrix{def, huge}},
		{"both at fault", sheared, []matrix.Matrix{def, sheared}, id, []matrix.Matrix{def, def}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := writeRead(t, fontWithMatrices(c.top, c.fds))
			if got.FontInfo.FontMatrix != c.wantTop {
				t.Errorf("top matrix: got %v, want %v", got.FontInfo.FontMatrix, c.wantTop)
			}
			if diff := cmp.Diff(c.wantFDs, got.FontMatrices); diff != "" {
				t.Errorf("FD matrices (-want +got):\n%s", diff)
			}
			for gid := range got.Glyphs {
				fm := got.GlyphMatrix(got.FontInfo.FontMatrix, glyph.ID(gid))
				if !usableFontMatrix(fm) {
					t.Errorf("glyph %d: unusable font matrix %v", gid, fm)
				}
			}

			again := writeRead(t, got)
			if again.FontInfo.FontMatrix != got.FontInfo.FontMatrix {
				t.Errorf("top matrix changed: %v, then %v", got.FontInfo.FontMatrix, again.FontInfo.FontMatrix)
			}
			if diff := cmp.Diff(got.FontMatrices, again.FontMatrices); diff != "" {
				t.Errorf("FD matrices changed (-first +second):\n%s", diff)
			}
		})
	}
}

// TestRepairFontMatricesCFF2 checks the repair of the font matrices of a CFF2
// font, whose per-FD matrices default to the identity.
func TestRepairFontMatricesCFF2(t *testing.T) {
	huge := matrix.Scale(1e200, 1e200)
	top := huge
	fds := []matrix.Matrix{matrix.Identity, huge}
	repairFontMatrices(&top, fds, defaultFontMatrix, matrix.Identity)
	if top != huge {
		t.Errorf("top matrix: got %v, want %v", top, huge)
	}
	if want := []matrix.Matrix{matrix.Identity, matrix.Identity}; !cmp.Equal(fds, want) {
		t.Errorf("FD matrices: got %v, want %v", fds, want)
	}
}
