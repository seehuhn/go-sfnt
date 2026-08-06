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

package name

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"seehuhn.de/go/sfnt/cmap"
)

func TestUTF16(t *testing.T) {
	cases := []string{
		"",
		"hello",
		"♠♡♢♣",
	}
	for _, c := range cases {
		buf := utf16Encode(c)
		d := utf16Decode(buf)
		if d != c {
			t.Errorf("%q -> % x -> %q", c, buf, d)
		}
	}
}

func FuzzNames(f *testing.F) {
	info := &Info{
		Mac: Tables{
			"en": {
				Copyright:   "Copyright (c) 2022 Jochen Voss <voss@seehuhn.de>",
				Description: "This is a test.",
			},
			"de": {
				Copyright:   "Copyright (c) 2022 Jochen Voss <voss@seehuhn.de>",
				Description: "Dies ist ein Test.",
			},
		},
		Windows: Tables{
			"en-US": {
				Copyright:   "Copyright (c) 2022 Jochen Voss <voss@seehuhn.de>",
				Description: "This is a test.",
			},
			"de-DE": {
				Copyright:   "Copyright (c) 2022 Jochen Voss <voss@seehuhn.de>",
				Description: "Dies ist ein Test.",
			},
		},
	}
	f.Add(info.Encode(1))

	f.Fuzz(func(t *testing.T, in []byte) {
		n1, err := Decode(in)
		if err != nil {
			return
		}

		ss := make(cmap.Table)
		ss[cmap.Key{PlatformID: 3, EncodingID: 1}] = nil

		buf := n1.Encode(1)
		n2, err := Decode(buf)
		if err != nil {
			t.Fatal(err)
		}

		if diff := cmp.Diff(n1, n2); diff != "" {
			t.Errorf("different (-old +new):\n%s", diff)
		}
	})
}

// A Macintosh record is written in MacRoman, which covers the Latin scripts
// only, and [mac.Encode] substitutes for what it cannot represent rather than
// failing.  Such a string is left out of the Macintosh record, since a reader
// would otherwise take the substitutions for the name.  This also keeps the
// Macintosh and Windows records of name ID 6 saying the same thing, which the
// OpenType specification requires of them.
func TestEncodeOmitsUnrepresentableMacRecords(t *testing.T) {
	for _, tc := range []struct {
		label  string
		psName string
		wantIn bool // is the Macintosh record expected?
	}{
		{"ASCII", "Foo-Regular", true},
		{"Latin-1, which MacRoman covers", "Grüße-Regular", true},
		{"outside MacRoman", "宋体-Regular", false},
	} {
		t.Run(tc.label, func(t *testing.T) {
			tbl := &Table{PostScriptName: tc.psName}
			info := &Info{
				Mac:     Tables{"en": tbl},
				Windows: Tables{"en-US": tbl},
			}

			back, err := Decode(info.Encode(1))
			if err != nil {
				t.Fatal(err)
			}

			var macName string
			if got := back.Mac["en"]; got != nil {
				macName = got.PostScriptName
			}
			if tc.wantIn {
				if macName != tc.psName {
					t.Errorf("the Macintosh record says %q, want %q", macName, tc.psName)
				}
			} else if macName != "" {
				t.Errorf("the Macintosh record says %q, want no record", macName)
			}

			winName := back.Windows["en-US"].PostScriptName
			if winName != tc.psName {
				t.Errorf("the Windows record says %q, want %q", winName, tc.psName)
			}
			if macName != "" && macName != winName {
				t.Errorf("the records disagree: %q and %q", macName, winName)
			}
		})
	}
}
