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

package sfnt_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/internal/debug"
)

// TestVarExpectSourceFonts checks that the source fonts committed in
// testdata/varexpect still match what the builders produce.
//
// Those fonts are the input fontTools computed its answers from, so a
// mismatch means the recorded ground truth describes a font we no longer
// build.  Failing here rather than inside TestVarExpect keeps the two apart:
// this test says the writer or a builder changed, TestVarExpect says
// Instantiate disagrees with fontTools.
func TestVarExpectSourceFonts(t *testing.T) {
	cases := []struct {
		file  string
		build func() *sfnt.Font
	}{
		{"glyf.ttf", debug.MakeVarFont},
		{"cff2.otf", debug.MakeVarCFF2Font},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join("testdata/varexpect", c.file))
			if err != nil {
				t.Fatal(err)
			}
			buf := &bytes.Buffer{}
			if _, err := c.build().Write(buf); err != nil {
				t.Fatal(err)
			}
			if got := buf.Bytes(); !bytes.Equal(want, got) {
				t.Errorf("committed font differs from the builder at %s; re-run go generate",
					firstDiff(want, got))
			}
		})
	}
}

// firstDiff describes where two byte slices start to differ.
func firstDiff(a, b []byte) string {
	for i := range min(len(a), len(b)) {
		if a[i] != b[i] {
			return fmt.Sprintf("byte %d", i)
		}
	}
	return fmt.Sprintf("length %d vs %d", len(a), len(b))
}
