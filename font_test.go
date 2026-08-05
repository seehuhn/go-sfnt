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
	"strings"
	"testing"

	"seehuhn.de/go/sfnt/os2"
)

// legalPostScriptChar reports whether r may appear in a PostScript name.
// This is an independent statement of the rule (printable ASCII excluding
// the ten delimiters and whitespace), so the test verifies the spec rather
// than echoing the implementation's own character class.
func legalPostScriptChar(r rune) bool {
	if r < '!' || r > '~' { // whitespace, control codes, non-ASCII
		return false
	}
	return !strings.ContainsRune("()<>[]{}/%", r) // PostScript delimiters
}

func TestPostScriptName(t *testing.T) {
	info := &Font{
		FamilyName: `A(n)d[r]o{m}e/d<a> N%ebula`,
		Weight:     os2.WeightBold,
		IsItalic:   true,
	}
	psName := info.PostScriptName()
	if psName != "AndromedaNebula-BoldItalic" {
		t.Errorf("wrong postscript name: %q", psName)
	}

	// Drive every byte value through the name and check the contract: the
	// result must contain only legal characters, and it must keep exactly
	// the legal characters of "family-subfamily" (illegal ones removed, the
	// rest preserved in order).
	var rr []rune
	for i := range 255 {
		rr = append(rr, rune(i))
	}
	info.FamilyName = string(rr)
	psName = info.PostScriptName()

	for _, r := range psName {
		if !legalPostScriptChar(r) {
			t.Errorf("PostScript name contains illegal character %q: %q", r, psName)
		}
	}

	var want strings.Builder
	for _, r := range info.FamilyName + "-BoldItalic" {
		if legalPostScriptChar(r) {
			want.WriteRune(r)
		}
	}
	if psName != want.String() {
		t.Errorf("PostScript name = %q, want %q", psName, want.String())
	}
}

// A font with neither a stored nor a derivable name has no PostScript name.
func TestPostScriptNameMissing(t *testing.T) {
	info := &Font{}
	if got := info.PostScriptName(); got != "" {
		t.Errorf("PostScript name = %q, want %q", got, "")
	}
}

// A derived name has to obey the length limit too, since the family name it is
// built from is not length-limited.
func TestPostScriptNameDerivedLength(t *testing.T) {
	info := &Font{FamilyName: strings.Repeat("x", 500)}
	psName := info.PostScriptName()
	if err := checkPSName(psName); err != nil {
		t.Errorf("derived name is not writable: %v", err)
	}
}

// Names taken from a font file are reduced to valid PostScript names.
func TestSanitizePSName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"Foo-Regular", "Foo-Regular"},
		{"ABCDEF+Foo", "ABCDEF+Foo"},
		{"Foo Bar (draft)", "FooBardraft"},
		{"Foo\x00\x01Bar", "FooBar"},
		{"Grüße", "Gre"},
		{strings.Repeat("x", psNameMaxLen), strings.Repeat("x", psNameMaxLen)},
		// over the limit even after filtering: dropped, not truncated
		{strings.Repeat("x", psNameMaxLen+1), ""},
		// filtering brings it back under the limit
		{strings.Repeat("x", psNameMaxLen) + " ", strings.Repeat("x", psNameMaxLen)},
	}
	for _, c := range cases {
		got := sanitizePSName(c.in)
		if got != c.want {
			t.Errorf("sanitizePSName(%q) = %q, want %q", c.in, got, c.want)
		}
		if err := checkPSName(got); err != nil {
			t.Errorf("sanitizePSName(%q) = %q, which is not writable: %v", c.in, got, err)
		}
	}
}

func TestCheckPSName(t *testing.T) {
	valid := []string{"", "Foo-Regular", "ABCDEF+Foo", strings.Repeat("x", psNameMaxLen)}
	for _, s := range valid {
		if err := checkPSName(s); err != nil {
			t.Errorf("checkPSName(%q) = %v, want nil", s, err)
		}
	}

	invalid := []string{
		"Foo Bar", "Foo(Bar)", "Foo/Bar", "Foo%Bar", "Foo[Bar]", "Foo{Bar}",
		"Foo<Bar>", "Foo\x00Bar", "Foo\nBar", "Grüße",
		strings.Repeat("x", psNameMaxLen+1),
	}
	for _, s := range invalid {
		if err := checkPSName(s); err == nil {
			t.Errorf("checkPSName(%q) = nil, want error", s)
		}
	}
}
