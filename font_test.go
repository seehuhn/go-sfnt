// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2022  Jochen Voss <voss@seehuhn.de>
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
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"
	"seehuhn.de/go/postscript/type1"
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/parser"
)

func TestCheckNameID6(t *testing.T) {
	valid := []string{
		"", "Foo-Regular", "ABCDEF+Foo", "Foo.Bar", "a_b",
		strings.Repeat("x", maxNameID6Len),
	}
	for _, s := range valid {
		if err := checkNameID6(s); err != nil {
			t.Errorf("checkNameID6(%q) = %v, want nil", s, err)
		}
	}

	invalid := []string{
		// the ten delimiters and white space
		"Foo Bar", "Foo(Bar)", "Foo/Bar", "Foo%Bar", "Foo[Bar]", "Foo{Bar}",
		"Foo<Bar>", "Foo\x00Bar", "Foo\nBar",
		// outside the printable ASCII subset
		"Grüße", "宋体", "Foo\x01Bar", "Foo\x7fBar",
		// longer than the entry may be
		strings.Repeat("x", maxNameID6Len+1),
	}
	for _, s := range invalid {
		if err := checkNameID6(s); err == nil {
			t.Errorf("checkNameID6(%q) = nil, want error", s)
		}
	}
}

// The "name" table cannot hold a name outside its ASCII subset, so such a name
// is dropped rather than reduced to the characters which fit: a name missing
// its non-ASCII characters would stand for a different font.
func TestRepairNameID6(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"", ""},
		{"Foo-Regular", "Foo-Regular"},
		{"ABCDEF+Foo", "ABCDEF+Foo"},
		{"Foo Bar (draft)", "FooBardraft"},
		{"Foo\x00\x01Bar", "FooBar"},

		// dropped, not cut down to the ASCII characters
		{"Grüße", ""},
		{"宋体", ""},
		{"Grüße-Regular", ""},

		// over the limit even after filtering: dropped, not truncated
		{strings.Repeat("x", maxNameID6Len), strings.Repeat("x", maxNameID6Len)},
		{strings.Repeat("x", maxNameID6Len+1), ""},
		// filtering brings it back under the limit
		{strings.Repeat("x", maxNameID6Len) + " ", strings.Repeat("x", maxNameID6Len)},
	} {
		got := repairNameID6(tc.in)
		if got != tc.want {
			t.Errorf("repairNameID6(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if err := checkNameID6(got); err != nil {
			t.Errorf("repairNameID6(%q) = %q, which is not writable: %v", tc.in, got, err)
		}
	}
}

// The variations PostScript name prefix allows only ASCII letters and digits.
// The separators a font name conventionally uses carry no meaning in a prefix
// and are removed, whereas a prefix needing more than ASCII is dropped.
func TestVariationsName(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"", ""},
		{"QuireVar", "QuireVar"},
		{"QuireVar-", "QuireVar"},
		{"Quire.Var_2", "QuireVar2"},
		{"Quire Var", "QuireVar"},

		// not expressible in the entry's character set at all
		{"Grüße", ""},
		{"宋体", ""},
		{"Quire\xffVar", ""},
	} {
		got := repairVariationsName(tc.in)
		if got != tc.want {
			t.Errorf("repairVariationsName(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if err := checkVariationsName(got); err != nil {
			t.Errorf("repairVariationsName(%q) = %q, which is not writable: %v",
				tc.in, got, err)
		}
	}

	for _, s := range []string{"QuireVar-", "Quire Var", "Grüße", "a_b"} {
		if err := checkVariationsName(s); err == nil {
			t.Errorf("checkVariationsName(%q) = nil, want error", s)
		}
	}
}

// The limit on a font name depends on which place has to carry it: a CFF Name
// INDEX takes any PostScript font name, whereas the "name" table is narrower.
func TestMaxFontNameLen(t *testing.T) {
	glyfFont := &Font{Outlines: &glyf.Outlines{}}
	if got, want := glyfFont.MaxFontNameLen(), maxNameID6Len; got != want {
		t.Errorf("glyf: MaxFontNameLen() = %d, want %d", got, want)
	}
	cffFont := &Font{Outlines: &cff.Outlines{}}
	if got, want := cffFont.MaxFontNameLen(), type1.MaxFontNameLen; got != want {
		t.Errorf("cff: MaxFontNameLen() = %d, want %d", got, want)
	}
	cff2Font := &Font{Outlines: &cff.OutlinesCFF2{}}
	if got, want := cff2Font.MaxFontNameLen(), maxNameID6Len; got != want {
		t.Errorf("cff2: MaxFontNameLen() = %d, want %d", got, want)
	}
}

// A name a font cannot carry is rejected on write rather than quietly left
// out, and the same name is accepted where a CFF Name INDEX can hold it.
func TestCheckFontName(t *testing.T) {
	const unicodeName = "宋体-Regular"

	glyfFont := &Font{Outlines: &glyf.Outlines{}}
	if err := glyfFont.CheckFontName(unicodeName); err == nil {
		t.Error("glyf: a name the font cannot carry was accepted")
	}
	if err := glyfFont.CheckFontName(strings.Repeat("x", maxNameID6Len+1)); err == nil {
		t.Error("glyf: an over-long name was accepted")
	}

	cffFont := &Font{Outlines: &cff.Outlines{}}
	if err := cffFont.CheckFontName(unicodeName); err != nil {
		t.Errorf("cff: %v", err)
	}

	// the empty string means "no name" and is allowed everywhere
	for _, f := range []*Font{glyfFont, cffFont} {
		if err := f.CheckFontName(""); err != nil {
			t.Errorf("the empty name was rejected: %v", err)
		}
	}
}

// Reading a font, writing it and reading it again must give the same name.  A
// "name" table holds entry 6 in a restricted ASCII form, so a font with
// TrueType outlines cannot be given a name outside that at all.
func TestPostScriptNameSurvivesWriteRead(t *testing.T) {
	for _, name := range []string{"Foo-Regular", "ABCDEF+Foo-Regular", ""} {
		f := ttfFixture(t)
		f.FontName = name

		var buf bytes.Buffer
		if _, err := f.WriteTrueTypePDF(&buf); err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		data := buf.Bytes()
		g, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		if err != nil {
			t.Fatalf("%q: %v", name, err)
		}
		if got := g.FontName; got != name {
			t.Errorf("wrote %q, read back %q", name, got)
		}
	}

	// A TrueType font has nowhere to put a name the "name" table cannot hold,
	// so writing one fails rather than silently losing it.
	for _, name := range []string{"Grüße-Regular", "宋体-Regular"} {
		f := ttfFixture(t)
		f.FontName = name

		var buf bytes.Buffer
		if _, err := f.WriteTrueTypePDF(&buf); err == nil {
			t.Errorf("%q: a name the font cannot carry was written", name)
		}
	}
}

// The style names a font file carries are stored, not derived, so they survive
// a write-read cycle even where they disagree with the style flags.
func TestStyleNamesSurviveWriteRead(t *testing.T) {
	f := ttfFixture(t)
	f.Subfamily = "Book"
	f.FullName = "Go Regular Book"

	g := writeReadTTF(t, f)

	if got := g.Subfamily; got != "Book" {
		t.Errorf("subfamily = %q, want %q", got, "Book")
	}
	if got := g.FullName; got != "Go Regular Book" {
		t.Errorf("full name = %q, want %q", got, "Go Regular Book")
	}
}

// Nothing derives a style name, so a font which names no style writes no
// "name" table entry for one.
func TestStyleNamesOmitted(t *testing.T) {
	f := ttfFixture(t)
	f.Subfamily = ""
	f.FullName = ""

	g := writeReadTTF(t, f)

	if got := g.Subfamily; got != "" {
		t.Errorf("subfamily = %q, want %q", got, "")
	}
	if got := g.FullName; got != "" {
		t.Errorf("full name = %q, want %q", got, "")
	}
}

// A font which does not name itself has no PostScript name.  Nothing is
// derived from the family name, since a derived name stands for a different
// font and the caller cannot tell the two apart.
func TestPostScriptNameNotDerived(t *testing.T) {
	f := ttfFixture(t)
	f.FontName = ""
	f.FamilyName = "Foo"
	f.Subfamily = "Bold"

	if got := writeReadTTF(t, f).FontName; got != "" {
		t.Errorf("font name = %q, want %q", got, "")
	}
}

// writeReadTTF writes f as a TrueType font and reads the result back.
func writeReadTTF(t *testing.T, f *Font) *Font {
	t.Helper()

	var buf bytes.Buffer
	if _, err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	g, err := Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// ttfFixture returns a TrueType font to write out and read back.
func ttfFixture(t *testing.T) *Font {
	t.Helper()

	f, err := Read(bytes.NewReader(goregular.TTF), parser.NewBudget(int64(len(goregular.TTF))))
	if err != nil {
		t.Fatal(err)
	}
	return f
}
