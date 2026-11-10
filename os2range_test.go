// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package sfnt_test

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/image/font/gofont/goregular"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/internal/debug"
	"seehuhn.de/go/sfnt/os2"
	"seehuhn.de/go/sfnt/parser"
)

// TestOS2UnicodeRangeRoundTrip verifies that the OS/2 UnicodeRange bits set on
// a Font survive a write/read cycle, for both glyf and CFF outlines.
func TestOS2UnicodeRangeRoundTrip(t *testing.T) {
	glyfFont, err := sfnt.Read(bytes.NewReader(goregular.TTF),
		parser.NewBudget(int64(len(goregular.TTF))))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		font *sfnt.Font
	}{
		{"glyf", glyfFont},
		{"cff", debug.MakeSimpleFont()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.font
			// deliberately avoid bit 57, which is derived from LastCharIndex
			f.UnicodeRange.Set(os2.URBasicLatin)
			f.UnicodeRange.Set(os2.URCyrillic)
			f.UnicodeRange.Set(os2.URGreek)
			want := f.UnicodeRange

			buf := &bytes.Buffer{}
			if _, err := f.Write(buf); err != nil {
				t.Fatal(err)
			}
			data := buf.Bytes()
			f2, err := sfnt.Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
			if err != nil {
				t.Fatal(err)
			}

			got := f2.UnicodeRange
			// bit 57 ("Non-Plane 0") is recomputed on write/read, ignore it
			maskBit57(&want)
			maskBit57(&got)
			if d := cmp.Diff(want, got); d != "" {
				t.Errorf("UnicodeRange round trip failed (-want +got):\n%s", d)
			}
		})
	}
}

// maskBit57 clears the "Non-Plane 0" bit (57), which is derived rather than
// preserved across a write/read cycle.
func maskBit57(ur *os2.UnicodeRange) {
	ur[57/32] &^= 1 << (57 % 32)
}
