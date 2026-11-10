// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"seehuhn.de/go/sfnt/opentype/gtab/testcases"
)

// TestGsubHarfbuzz feeds each test case to hb-shape and verifies that
// HarfBuzz produces the same glyph string as the library.  The test is
// scoped to the section-6 cases (GSUB 8) for now; expanding the prefix
// filter brings other sections into the comparison.
func TestGsubHarfbuzz(t *testing.T) {
	hbShape, err := exec.LookPath("hb-shape")
	if err != nil {
		t.Skip("hb-shape not found in PATH")
	}

	fontGen, err := testcases.NewFontGen()
	if err != nil {
		t.Fatal(err)
	}

	tmp := t.TempDir()
	cleanup := strings.NewReplacer("[", "", "|", "", "]", "", "\n", "")

	for testIdx, test := range testcases.Gsub {
		if !strings.HasPrefix(test.Name, "6_") && test.Name != "1_15" {
			continue
		}
		t.Run(test.Name, func(t *testing.T) {
			info, err := fontGen.GsubTestFont(testIdx)
			if err != nil {
				t.Fatal(err)
			}
			fontPath := filepath.Join(tmp, test.Name+".otf")
			fd, err := os.Create(fontPath)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := info.Write(fd); err != nil {
				fd.Close()
				t.Fatal(err)
			}
			if err := fd.Close(); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command(hbShape, "--no-clusters", "--no-positions", fontPath, test.In)
			out, err := cmd.Output()
			if err != nil {
				t.Fatalf("hb-shape failed: %v", err)
			}
			hbOut := cleanup.Replace(string(out))
			if hbOut != test.Out {
				t.Errorf("HarfBuzz disagrees: hb-shape=%q, library/expected=%q (input %q)",
					hbOut, test.Out, test.In)
			}
		})
	}
}
