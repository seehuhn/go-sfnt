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

import (
	"bytes"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/parser"
)

// TestWriteTrueTypePDFPreservesNames checks that WriteTrueTypePDF retains
// glyph names from the "post" table when they are present in the source
// font.  Downstream consumers (text extraction in particular) rely on
// these names to recover text from symbolic TrueType embeds without a
// /ToUnicode CMap.
func TestWriteTrueTypePDFPreservesNames(t *testing.T) {
	src, err := Read(bytes.NewReader(goregular.TTF), parser.NewBudget(int64(len(goregular.TTF))))
	if err != nil {
		t.Fatal(err)
	}
	srcOutlines := src.Outlines.(*glyf.Outlines)
	if len(srcOutlines.Names) == 0 {
		t.Fatal("test precondition: source font carries no glyph names")
	}

	var buf bytes.Buffer
	if _, err := src.WriteTrueTypePDF(&buf); err != nil {
		t.Fatal(err)
	}

	dstData := buf.Bytes()
	dst, err := Read(bytes.NewReader(dstData), parser.NewBudget(int64(len(dstData))))
	if err != nil {
		t.Fatal(err)
	}
	dstOutlines := dst.Outlines.(*glyf.Outlines)
	if len(dstOutlines.Names) != len(srcOutlines.Names) {
		t.Fatalf("glyph name count: got %d, want %d",
			len(dstOutlines.Names), len(srcOutlines.Names))
	}
	for gid, name := range srcOutlines.Names {
		if dstOutlines.Names[gid] != name {
			t.Errorf("gid %d: got %q, want %q",
				gid, dstOutlines.Names[gid], name)
		}
	}
}

// TestWriteTrueTypePDFRejectsNameCountMismatch checks that writing a font
// whose glyph name slice does not match the glyph count returns an error
// rather than emitting a malformed "post" table.
func TestWriteTrueTypePDFRejectsNameCountMismatch(t *testing.T) {
	src, err := Read(bytes.NewReader(goregular.TTF), parser.NewBudget(int64(len(goregular.TTF))))
	if err != nil {
		t.Fatal(err)
	}
	outlines := src.Outlines.(*glyf.Outlines)
	outlines.Names = outlines.Names[:len(outlines.Names)-1]

	var buf bytes.Buffer
	if _, err := src.WriteTrueTypePDF(&buf); err == nil {
		t.Fatal("expected error for glyph name / glyph count mismatch")
	}
}

// TestWriteTrueTypePDFOmitsNamelessPost checks that no "post" table is
// written when the source font has no glyph names — the original
// minimisation behaviour.
func TestWriteTrueTypePDFOmitsNamelessPost(t *testing.T) {
	src, err := Read(bytes.NewReader(goregular.TTF), parser.NewBudget(int64(len(goregular.TTF))))
	if err != nil {
		t.Fatal(err)
	}
	src.Outlines.(*glyf.Outlines).Names = nil

	var buf bytes.Buffer
	if _, err := src.WriteTrueTypePDF(&buf); err != nil {
		t.Fatal(err)
	}

	dstData := buf.Bytes()
	dst, err := Read(bytes.NewReader(dstData), parser.NewBudget(int64(len(dstData))))
	if err != nil {
		t.Fatal(err)
	}
	if names := dst.Outlines.(*glyf.Outlines).Names; names != nil {
		t.Errorf("expected nil glyph names, got %d entries", len(names))
	}
}
