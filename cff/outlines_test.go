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

	"seehuhn.de/go/geom/path"
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

// TestPathInvalidGID verifies that invalid glyph IDs produce no path commands.
func TestPathInvalidGID(t *testing.T) {
	o := &Outlines{
		Glyphs: []*Glyph{},
	}

	var cmds []path.Command
	for cmd := range o.Path(glyph.ID(999)) {
		cmds = append(cmds, cmd)
	}

	if len(cmds) != 0 {
		t.Errorf("got %d commands for invalid GID, want 0", len(cmds))
	}
}
