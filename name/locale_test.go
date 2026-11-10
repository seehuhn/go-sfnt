// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package name

import (
	"testing"

	"golang.org/x/text/language"
)

func TestLanguageTags(t *testing.T) {
	for _, list := range []map[uint16]string{appleBCP, msBCP} {
		for _, lang := range list {
			tag := language.MustParse(lang)
			// region, _ := tag.Region()
			script, _ := tag.Script()
			if script.String() == "Zzzz" {
				t.Error(lang)
			}
		}
	}
}
