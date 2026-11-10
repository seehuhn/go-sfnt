// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

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
