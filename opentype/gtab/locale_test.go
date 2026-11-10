// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"fmt"
	"testing"

	"golang.org/x/text/language"
)

func TestOtfToBCP(t *testing.T) {
	cases := []struct {
		script otfScript
		lang   otfLang
	}{
		{"DFLT", ""},

		{"latn", ""},
		{"cyrl", ""},
		{"grek", ""},
		{"hebr", ""},
		{"arab", ""},
		{"thai", ""},
		{"hani", ""},
		{"kana", ""},
		{"latn", "ENG "},
		{"latn", "ROM "},
		{"latn", "TRK "},
		{"latn", "DEU "},
		{"cyrl", "SRB "},
		{"cyrl", "RUS "},
		{"hani", "ZHT "},
	}
	for _, test := range cases {
		tag, err := otfToBCP47(test.script, test.lang)
		if err != nil {
			t.Error(err)
			continue
		}
		script, lang, err := bcp47ToOtf(tag)
		if err != nil {
			t.Error(err)
			continue
		}
		if script != test.script || lang != test.lang {
			t.Errorf("got %s, %s; want %s, %s", script, lang, test.script, test.lang)
		}
	}
}

func TestBcpToOtf(t *testing.T) {
	cases := []struct {
		tag    language.Tag
		script otfScript
		lang   otfLang
	}{
		{language.MustParse("und-Zzzz"), "DFLT", ""},
		{language.MustParse("und-Latn"), "latn", ""},

		{language.English, "latn", "ENG "},
		{language.AmericanEnglish, "latn", "ENG "},
		{language.BritishEnglish, "latn", "ENG "},
		{language.German, "latn", "DEU "},
		{language.Greek, "grek", "ELL "},
		{language.Russian, "cyrl", "RUS "},
		{language.SimplifiedChinese, "hani", "ZHS "},
		{language.TraditionalChinese, "hani", "ZHT "},
		{language.Chinese, "hani", "ZHP "},
	}
	for i, test := range cases {
		script, lang, err := bcp47ToOtf(test.tag)
		if err != nil {
			t.Error(err)
			continue
		}
		if script != test.script || lang != test.lang {
			t.Errorf("%d: got %s, %s; want %s, %s", i, script, lang, test.script, test.lang)
		}
	}
}

func TestScriptTags(t *testing.T) {
	for _, bcpTag := range scriptBcp47 {
		_, err := language.ParseScript(bcpTag)
		if err != nil {
			t.Error(err)
		}
	}
}

func TestLangTags(t *testing.T) {
	for _, bcpTag := range langBcp47 {
		tag, err := language.Parse(bcpTag)
		if err != nil {
			t.Error(err)
		}
		if tag.String() != bcpTag {
			fmt.Println(bcpTag, "->", tag.String())
		}
	}
}

// TestReverseTablesUnambiguous checks that every BCP 47 value shared by
// several OpenType language tags has a preferred tag, that every script
// value is shared by at most an original tag and its v.2 variant, and that
// the script inversion picks the v.2 variant.
func TestReverseTablesUnambiguous(t *testing.T) {
	count := make(map[string]int)
	for _, val := range langBcp47 {
		count[val]++
	}
	for val, n := range count {
		if n > 1 {
			if _, ok := preferredOtfLang[val]; !ok {
				t.Errorf("no preferred OpenType tag for %q", val)
			}
		}
	}
	for val, pref := range preferredOtfLang {
		if count[val] < 2 {
			t.Errorf("preference for %q is stale: the value is not shared", val)
		}
		if langBcp47[pref] != val {
			t.Errorf("preferred tag %q does not map to %q", pref, val)
		}
		if bcp47Lang[val] != pref {
			t.Errorf("bcp47Lang[%q] = %q, want %q", val, bcp47Lang[val], pref)
		}
	}
	scripts := make(map[string][]otfScript)
	for key, val := range scriptBcp47 {
		scripts[val] = append(scripts[val], key)
	}
	for val, keys := range scripts {
		if len(keys) == 1 {
			continue
		}
		v2 := 0
		for _, key := range keys {
			if isV2Script(key) {
				v2++
			}
		}
		if len(keys) != 2 || v2 != 1 {
			t.Errorf("script %q has tags %v, want one original and one v.2 tag", val, keys)
		}
		if got := bcp47Script[val]; !isV2Script(got) {
			t.Errorf("bcp47Script[%q] = %q, want the v.2 tag", val, got)
		}
	}
}
