// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package name

import (
	"testing"

	"golang.org/x/text/language"
)

func TestChoose(t *testing.T) {
	tt := Tables{}
	makeTable := func(lang string, numEntries int) {
		t := &Table{}
		for i := range numEntries {
			t.set(ID(i), "x")
		}
		t.set(1000, lang)
		tt[lang] = t
	}
	makeTable("en-GB", 3)
	makeTable("en-US", 2)

	table, conf := tt.Choose(language.AmericanEnglish)
	if got := table.get(1000); got != "en-US" || conf != language.Exact {
		t.Errorf("%q %d", got, conf)
	}

	table, conf = tt.Choose(language.BritishEnglish)
	if got := table.get(1000); got != "en-GB" || conf != language.Exact {
		t.Errorf("%q %d", got, conf)
	}

	table, conf = tt.Choose(language.MustParse("en-NZ"))
	if got := table.get(1000); got != "en-GB" || conf != language.High {
		t.Errorf("%q %d", got, conf)
	}

	table, conf = tt.Choose(language.German)
	if got := table.get(1000); got != "en-US" || conf != language.No {
		t.Errorf("%q %d", got, conf)
	}
}
