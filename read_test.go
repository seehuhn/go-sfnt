// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2023  Jochen Voss <voss@seehuhn.de>
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
	"io"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/header"
	"seehuhn.de/go/sfnt/parser"
)

// TestEmptyGDEF tests that we can read a font with an empty GDEF table.
// This is a regression test for https://github.com/seehuhn/go-sfnt/issues/1 .
func TestEmptyGDEF(t *testing.T) {
	// create a TrueType font with an empty GDEF table
	r := bytes.NewReader(goregular.TTF)
	info, err := header.Read(r)
	if err != nil {
		t.Fatal(err)
	}
	tables := make(map[string][]byte)
	for name := range info.Toc {
		data, err := info.ReadTableBytes(r, name)
		if err != nil {
			t.Fatal(err)
		}
		tables[name] = data
	}
	tables["GDEF"] = []byte{}
	w := &bytes.Buffer{}
	_, err = header.Write(w, info.ScalerType, tables)
	if err != nil {
		t.Fatal(err)
	}

	// read the font again
	data := w.Bytes()
	r = bytes.NewReader(data)
	_, err = Read(r, parser.NewBudget(int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}
}

// TestShortPostNames tests that a "post" table listing fewer names than the
// font has glyphs is accepted, and that the missing names are left empty.
func TestShortPostNames(t *testing.T) {
	r := bytes.NewReader(goregular.TTF)
	info, err := header.Read(r)
	if err != nil {
		t.Fatal(err)
	}
	tables := make(map[string][]byte)
	for name := range info.Toc {
		data, err := info.ReadTableBytes(r, name)
		if err != nil {
			t.Fatal(err)
		}
		tables[name] = data
	}

	// A version 1.0 "post" table names the 258 glyphs of the standard
	// Macintosh ordering, but the font has more glyphs than that.
	tables["post"] = append([]byte{0x00, 0x01, 0x00, 0x00}, tables["post"][4:32]...)

	w := &bytes.Buffer{}
	_, err = header.Write(w, info.ScalerType, tables)
	if err != nil {
		t.Fatal(err)
	}

	data := w.Bytes()
	font, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}

	outlines, ok := font.Outlines.(*glyf.Outlines)
	if !ok {
		t.Fatalf("unexpected outline type %T", font.Outlines)
	}
	if len(outlines.Names) != font.NumGlyphs() {
		t.Errorf("got %d names, want %d", len(outlines.Names), font.NumGlyphs())
	}
	if font.NumGlyphs() <= 258 {
		t.Fatal("test font has too few glyphs")
	}
	if name := font.GlyphName(glyph.ID(font.NumGlyphs() - 1)); name != "" {
		t.Errorf("unnamed glyph: got %q, want \"\"", name)
	}

	// the font must still be writable
	if _, err := font.Write(io.Discard); err != nil {
		t.Error(err)
	}
}
