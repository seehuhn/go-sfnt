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

// Command varexpect regenerates the fontTools ground truth in
// testdata/varexpect: it writes the synthetic source fonts, then runs
// instancer.py on each to record what fontTools' varLib.instancer makes of
// them at a set of pinned instances.
//
// Both the fonts and the JSON are committed, so TestVarExpect compares
// Font.Instantiate against an independent implementation without needing
// Python at test time.  The fonts are committed rather than rebuilt on the
// fly because they are the oracle's input: leaving them out would let a
// change to the font writer silently redefine what the recorded answers
// describe.
//
// Regeneration is byte-stable: with unchanged builders, libraries and
// fontTools, this leaves the working tree alone, and any diff it produces
// reflects a real change.
package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/internal/debug"
)

// outDir is the testdata directory, relative to the module root.
const outDir = "testdata/varexpect"

// a source font together with the instances and glyphs to record for it
type source struct {
	name   string // file name of the font within outDir
	build  func() *sfnt.Font
	cases  []string // "NAME:tag=val,..." as instancer.py parses it
	glyphs []string
}

// The glyf font carries the awkward parts of gvar: a subset tuple needing
// IUP, a composite glyph, an intermediate region and a two-tuple glyph.  The
// CFF2 font carries the awkward parts of the blend machinery: two item
// variation data subtables of different width, an intermediate region, two
// Font DICTs, stem hints behind a hintmask, and cubic curves.
var sources = []source{
	{
		name:  "glyf.ttf",
		build: debug.MakeVarFont,
		cases: []string{
			"defaults:",
			"bold:wght=900",
			"narrow:wdth=75",
			"bold-narrow:wght=900,wdth=75",
			"mid:wght=700",
		},
		glyphs: []string{".notdef", "rect", "iup", "comp", "two", "swap"},
	},
	{
		name:  "cff2.otf",
		build: debug.MakeVarCFF2Font,
		cases: []string{
			"defaults:",
			"bold:wght=900",
			"narrow:wdth=75",
			"bold-narrow:wght=900,wdth=75",
			"mid:wght=700",
		},
		// CFF2 dropped the charset, so the glyphs have no names of their own
		// and fontTools falls back to their position in the glyph order.
		glyphs: []string{".notdef", "glyph00001", "glyph00002", "glyph00003", "glyph00004"},
	},
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	script, err := filepath.Abs("internal/varexpect/instancer.py")
	if err != nil {
		return err
	}
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("run \"go generate\" from the module root: %w", err)
	}

	for _, s := range sources {
		fontPath := filepath.Join(outDir, s.name)
		if err := writeFont(s.build(), fontPath); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}

		jsonPath := filepath.Join(outDir, jsonName(s.name))
		args := []string{script, fontPath, "-o", jsonPath}
		for _, c := range s.cases {
			args = append(args, "--case", c)
		}
		for _, g := range s.glyphs {
			args = append(args, "--glyph", g)
		}

		cmd := exec.Command("python3", args...)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: instancer.py: %w", s.name, err)
		}
		fmt.Printf("wrote %s and %s\n", fontPath, jsonPath)
	}
	return nil
}

// writeFont serializes f to path, leaving the file alone when the bytes have
// not changed so that regeneration does not churn timestamps.
func writeFont(f *sfnt.Font, path string) error {
	buf := &bytes.Buffer{}
	if _, err := f.Write(buf); err != nil {
		return err
	}
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, buf.Bytes()) {
		return nil
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}

// jsonName maps a font file name to the name of its ground-truth file.
func jsonName(font string) string {
	return font[:len(font)-len(filepath.Ext(font))] + ".json"
}
