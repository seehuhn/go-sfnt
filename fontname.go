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
	"errors"
	"fmt"
	"regexp"
	"unicode"

	"seehuhn.de/go/postscript/type1"
)

// A PostScript font name can be carried in two places, each with its own
// rules.  A CFF Name INDEX takes any PostScript font name, which is what
// [type1.CheckFontName] describes.  The "name" table is narrower: entry 6 is
// restricted to a subset of ASCII and to 63 characters, so a name outside that
// reaches a CFF font but not a "name" table.
//
// Which of the two must carry the name depends on the glyph outlines, and that
// is the only thing which varies: a font with CFF outlines always has a Name
// INDEX and writes entry 6 as well where the name fits, whereas a font with
// TrueType or CFF2 outlines has nowhere else to put it.

// maxNameID6Len is the greatest number of characters the PostScript name of
// the "name" table may hold.
const maxNameID6Len = 63

// nameID6Forbidden matches the characters which may not appear in the
// PostScript name of the "name" table.  That entry is restricted to the
// printable ASCII subset, codes 33 to 126, less the ten PostScript delimiters,
// and the Macintosh and Windows records of it must agree.
//
// The restriction belongs to the "name" table rather than to font names in
// general: a CFF Name INDEX states no encoding and carries the bytes it is
// given, whereas a Macintosh record is written in MacRoman and cannot hold a
// name outside Latin script at all.  Conforming fonts give entry 6 in ASCII
// whatever script the family name uses, so a name which does not fit here is
// one this library derived or read from elsewhere, and the entry is omitted
// rather than written in a form which would not match.
var nameID6Forbidden = regexp.MustCompile(`[^!-$&-'*-.0-;=?-Z\\^-z|~]`)

// checkNameID6 returns an error if s cannot be written as the PostScript name
// of the "name" table.  The empty string is allowed; the entry is optional, so
// a font without it is still a conforming font.
func checkNameID6(s string) error {
	if nameID6Forbidden.MatchString(s) {
		return errors.New("sfnt: invalid character in PostScript name")
	}
	// every character allowed here is one byte long
	if len(s) > maxNameID6Len {
		return fmt.Errorf("sfnt: PostScript name too long (%d characters)", len(s))
	}
	return nil
}

// canBeNameID6 reports whether s can be written as the PostScript name of the
// "name" table, and is a name rather than the absence of one.
func canBeNameID6(s string) bool {
	return s != "" && checkNameID6(s) == nil
}

// repairNameID6 makes a PostScript name stored in the "name" table usable, by
// removing the characters the entry may not hold.
//
// A name which needs more than the entry allows is dropped rather than cut
// down to the characters which fit, since a name missing its non-ASCII
// characters would stand for a different font.
func repairNameID6(s string) string {
	s = type1.RepairFontName(s)
	if !canBeNameID6(s) {
		return ""
	}
	return s
}

// nonAlnum matches the characters outside the ASCII letters and digits.  This
// is the character set a variable font instance is named from, and it is
// narrower than the PostScript name of the "name" table because a generated
// name is assembled by appending one group per axis: every part has to stay
// clear of the "_" and "." the assembly separates them with.  The restriction
// applies to the prefix the "name" table carries as much as to the parts
// [instanceName] derives for itself.
var nonAlnum = regexp.MustCompile(`[^A-Za-z0-9]`)

// keepAlnum removes the characters outside the ASCII letters and digits.
func keepAlnum(s string) string {
	return nonAlnum.ReplaceAllString(s, "")
}

// isAlnum reports whether s is a non-empty string of ASCII letters and digits.
func isAlnum(s string) bool {
	return s != "" && !nonAlnum.MatchString(s)
}

// checkVariationsName returns an error if s cannot be written as the
// variations PostScript name prefix of the "name" table.  The empty string is
// allowed and stands for the absence of the entry.
func checkVariationsName(s string) error {
	if nonAlnum.MatchString(s) {
		return errors.New("sfnt: invalid character in variations PostScript name")
	}
	return nil
}

// repairVariationsName makes a variations PostScript name prefix read from a
// font file usable.  The separators a font name conventionally uses carry no
// meaning in a prefix, since the generated groups bring their own, so they are
// removed; a prefix which needs more than the ASCII letters and digits is
// dropped instead, because removing those characters would leave a fragment
// standing for a different family.
func repairVariationsName(s string) string {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return ""
		}
	}
	return keepAlnum(s)
}

// MaxFontNameLen returns the greatest number of bytes [Font.FontName] may
// occupy in this font.  The limit depends on which place must carry the name,
// and so on the kind of glyph outlines the font uses.
func (f *Font) MaxFontNameLen() int {
	if f.IsCFF() {
		return type1.MaxFontNameLen
	}
	return maxNameID6Len
}

// CheckFontName returns an error if s cannot be used as [Font.FontName] for
// this font, because the place which has to carry the name cannot hold it.
//
// This is for callers which take a name from somewhere other than the font,
// such as the /BaseFont entry of a PDF file: those names follow rules of their
// own, and one which does not fit has to be left out rather than written in a
// form which would not match.  Writing a font whose name it cannot carry
// fails.
func (f *Font) CheckFontName(s string) error {
	if f.IsCFF() {
		return type1.CheckFontName(s)
	}
	return checkNameID6(s)
}

// repairFontName makes a PostScript name read from a font file usable, using
// the rules of the place which must carry it.  Repairing on read is what keeps
// the name this library reports the same before and after the font is written
// out again.
func (f *Font) repairFontName(s string) string {
	if f.IsCFF() {
		return type1.RepairFontName(s)
	}
	return repairNameID6(s)
}

// checkPSNames verifies every PostScript name the font would write to a
// complete font file.  Names read from a font file are repaired on read, so a
// failure here means the caller supplied an invalid name.
func (f *Font) checkPSNames() error {
	if err := f.CheckFontName(f.FontName); err != nil {
		return err
	}
	if err := checkVariationsName(f.VariationsPostScriptName); err != nil {
		return err
	}
	if f.VariationsPostScriptName != "" && f.Fvar == nil {
		return errors.New("sfnt: variations PostScript name without variations")
	}
	if f.Fvar != nil {
		// A named instance's PostScript name is stored in the "name" table of
		// this font and becomes the PostScript name of the instance when the
		// font is pinned, so both ends of its life ask for the same rules.
		for _, inst := range f.Fvar.Instances {
			if err := checkNameID6(inst.PostScriptName); err != nil {
				return err
			}
		}
	}
	return nil
}
