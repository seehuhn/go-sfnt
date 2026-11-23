// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"bytes"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/text/language"
	"seehuhn.de/go/sfnt/parser"
)

func TestScriptListFeatureIndexSentinel(t *testing.T) {
	// 0xFFFF is the "no feature" sentinel for requiredFeatureIndex only; in
	// the optional list it must be dropped, not turned into a reference to
	// feature 0.
	info := ScriptListInfo{}
	info[language.English] = &Features{
		Optional: []FeatureIndex{1, 0xFFFF, 3},
	}
	data := encodeList(t, info)

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	got, err := readScriptList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one language entry, got %d", len(got))
	}
	want := []FeatureIndex{1, 3}
	for _, feat := range got {
		if !reflect.DeepEqual(feat.Optional, want) {
			t.Errorf("featureIndices = %v, want %v", feat.Optional, want)
		}
	}
}

func FuzzScriptList(f *testing.F) {
	info := ScriptListInfo{}
	info[language.MustParse("und-Zzzz")] = &Features{
		Optional: []FeatureIndex{0},
	}
	f.Add(encodeList(f, info))
	info = ScriptListInfo{}
	info[language.English] = &Features{
		Optional: []FeatureIndex{1, 2, 3, 4},
	}
	info[language.MustParse("und-Latn")] = &Features{
		Required: new(FeatureIndex(7)),
		Optional: []FeatureIndex{5, 6},
	}
	f.Add(encodeList(f, info))
	info = ScriptListInfo{}
	info[language.MustParse("und-Arab")] = &Features{
		Required: new(FeatureIndex(0)),
		Optional: []FeatureIndex{1, 2},
	}
	f.Add(encodeList(f, info))
	info = ScriptListInfo{}
	info[language.Arabic] = &Features{
		Optional: []FeatureIndex{1, 3, 5},
	}
	info[language.Azerbaijani] = &Features{
		Optional: []FeatureIndex{2, 4, 6},
	}
	f.Add(encodeList(f, info))

	f.Fuzz(func(t *testing.T, data []byte) {
		p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		info, err := readScriptList(p, 0)
		if err != nil {
			return
		}

		data2 := encodeList(t, info)

		// if len(data2) > len(data) {
		// 	fmt.Printf("A % x\n", data)
		// 	fmt.Printf("B % x\n", data2)
		// 	t.Errorf("encode: %d > %d", len(data2), len(data))
		// }

		p = parser.New(bytes.NewReader(data2), parser.NewBudget(int64(len(data2))))
		info2, err := readScriptList(p, 0)
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(info, info2) {
			t.Error("different")
		}
	})
}

// bigFeatures returns a language system with n optional features.
func bigFeatures(n int) *Features {
	opt := make([]FeatureIndex, n)
	for i := range opt {
		opt[i] = FeatureIndex(i)
	}
	return &Features{Optional: opt}
}

func TestScriptListOffsetRange(t *testing.T) {
	small := &Features{Optional: []FeatureIndex{1}}

	// In each case, the small table only fits if the large one is stored
	// last.
	fitting := map[string]ScriptListInfo{
		"script": {
			language.MustParse("und-Cyrl-x-cyrl"): bigFeatures(40000),
			language.MustParse("und-Latn-x-latn"): small,
		},
		"langSys": {
			language.MustParse("und-Latn-x-latn"):    bigFeatures(40000),
			language.MustParse("en-Latn-x-latn-eng"): small,
		},
		"within limit": {
			language.MustParse("und-Cyrl-x-cyrl"):    bigFeatures(20000),
			language.MustParse("und-Latn-x-latn"):    bigFeatures(10000),
			language.MustParse("en-Latn-x-latn-eng"): small,
		},
	}
	for name, info := range fitting {
		t.Run(name, func(t *testing.T) {
			data := encodeList(t, info)
			p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
			got, err := readScriptList(p, 0)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(info, got); diff != "" {
				t.Errorf("round trip failed (-want +got):\n%s", diff)
			}
		})
	}

	big := func(first int) *Features {
		f := bigFeatures(30000)
		for i := range f.Optional {
			f.Optional[i] += FeatureIndex(first)
		}
		return f
	}
	tooLarge := map[string]ScriptListInfo{
		"three scripts": {
			language.MustParse("und-Cyrl-x-cyrl"): big(0),
			language.MustParse("und-Latn-x-latn"): big(1),
			language.MustParse("und-Grek-x-grek"): big(2),
		},
		"three languages": {
			language.MustParse("und-Latn-x-latn"):    big(0),
			language.MustParse("en-Latn-x-latn-eng"): big(1),
			language.MustParse("de-Latn-x-latn-deu"): big(2),
		},
	}
	for name, info := range tooLarge {
		t.Run(name, func(t *testing.T) {
			if _, err := info.encode(); err == nil {
				t.Error("encode succeeded")
			}
		})
	}
}

// TestScriptListSharedTables checks that a script list in which many
// records reach the same Script table, and many language system records
// reach the same LangSys table, can be written back.  Without sharing, the
// output would need more than 64 KiB.
func TestScriptListSharedTables(t *testing.T) {
	const nScripts = 20
	const nLangs = 30
	const nFeatures = 300

	var scripts []otfScript
	for s := range scriptBcp47 {
		scripts = append(scripts, s)
	}
	slices.Sort(scripts)
	scripts = scripts[:nScripts]
	var langs []otfLang
	for l := range langBcp47 {
		if len(l) == 4 {
			langs = append(langs, l)
		}
	}
	slices.Sort(langs)
	langs = langs[:nLangs]

	scriptPos := 2 + 6*nScripts
	langSysPos := 4 + 6*nLangs // relative to the Script table
	data := []byte{0, nScripts}
	for _, s := range scripts {
		data = append(data, s...)
		data = append(data, byte(scriptPos>>8), byte(scriptPos))
	}
	data = append(data, byte(langSysPos>>8), byte(langSysPos), 0, nLangs)
	for _, l := range langs {
		data = append(data, l...)
		data = append(data, byte(langSysPos>>8), byte(langSysPos))
	}
	data = append(data, 0, 0, 0xFF, 0xFF, byte(nFeatures>>8), byte(nFeatures&0xFF))
	for i := range nFeatures {
		data = append(data, byte(i>>8), byte(i))
	}

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	info, err := readScriptList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(info) != nScripts*(nLangs+1) {
		t.Fatalf("got %d language systems, want %d", len(info), nScripts*(nLangs+1))
	}

	l, err := info.layout()
	if err != nil {
		t.Fatal(err)
	}
	enc := encodeList(t, info)
	if len(enc) != l.encodeLen() {
		t.Errorf("encode/encodeLen mismatch: %d vs %d", len(enc), l.encodeLen())
	}
	if len(enc) > len(data) {
		t.Errorf("re-encoded list has %d bytes, input has %d", len(enc), len(data))
	}
	p = parser.New(bytes.NewReader(enc), parser.NewBudget(int64(len(enc))))
	info2, err := readScriptList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(info, info2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// TestScriptListSharingIdenticalOnly checks that only identical Script and
// LangSys tables share storage.
func TestScriptListSharingIdenticalOnly(t *testing.T) {
	a := &Features{Optional: []FeatureIndex{1, 2, 3}}
	b := &Features{Optional: []FeatureIndex{1, 2, 4}}
	tag := func(s string) language.Tag { return language.MustParse(s) }

	size := func(info ScriptListInfo) int {
		l, err := info.layout()
		if err != nil {
			t.Fatal(err)
		}
		return l.encodeLen()
	}

	langSys := 6 + 2*3
	// two scripts with identical Script tables
	same := ScriptListInfo{tag("und-Latn-x-latn"): a, tag("und-Cyrl-x-cyrl"): a}
	if got, want := size(same), 2+2*6+4+langSys; got != want {
		t.Errorf("identical scripts: got %d bytes, want %d", got, want)
	}
	diff := ScriptListInfo{tag("und-Latn-x-latn"): a, tag("und-Cyrl-x-cyrl"): b}
	if got, want := size(diff), 2+2*6+2*(4+langSys); got != want {
		t.Errorf("different scripts: got %d bytes, want %d", got, want)
	}
	// language systems within one script
	sameLang := ScriptListInfo{tag("und-Latn-x-latn"): a, tag("en-Latn-x-latn-eng"): a}
	if got, want := size(sameLang), 2+6+4+6+langSys; got != want {
		t.Errorf("identical language systems: got %d bytes, want %d", got, want)
	}
	diffLang := ScriptListInfo{tag("und-Latn-x-latn"): a, tag("en-Latn-x-latn-eng"): b}
	if got, want := size(diffLang), 2+6+4+6+2*langSys; got != want {
		t.Errorf("different language systems: got %d bytes, want %d", got, want)
	}
}

// TestScriptListRepairLangSys checks that language systems whose LangSys
// tables cannot all be written back are dropped on read, keeping as many as
// possible.  The LangSys tables overlap in the input, and each holds about
// 2000 feature indices.
func TestScriptListRepairLangSys(t *testing.T) {
	const nLangs = 100
	const base = 2000

	var langs []otfLang
	for l := range langBcp47 {
		if len(l) == 4 {
			langs = append(langs, l)
		}
	}
	slices.Sort(langs)
	langs = langs[:nLangs]

	data := []byte{0, 1, 'l', 'a', 't', 'n', 0, 8}
	langSysPos := 4 + 6*nLangs // relative to the Script table
	data = append(data, 0, 0, 0, nLangs)
	for i, l := range langs {
		off := langSysPos + 6*i
		data = append(data, l...)
		data = append(data, byte(off>>8), byte(off))
	}
	put := func(w int) { data = append(data, byte(w>>8), byte(w)) }
	for k := range 4*nLangs + base + 10 {
		// a LangSys table starts at every third word
		switch k % 3 {
		case 0:
			put(0)
		case 1:
			put(k / 3)
		default:
			put(base + k/3)
		}
	}

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	info, err := readScriptList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	// each LangSys table takes about 4 kB, so about 15 fit
	if len(info) < 10 || len(info) >= nLangs {
		t.Errorf("%d language systems kept", len(info))
	}

	data2 := encodeList(t, info)
	p = parser.New(bytes.NewReader(data2), parser.NewBudget(int64(len(data2))))
	info2, err := readScriptList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(info, info2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// TestScriptListRepairScripts checks that scripts whose Script tables cannot
// all be written back are dropped on read, keeping as many as possible.  The
// Script tables differ in their language tag and share one LangSys table of
// about 20 kB in the input, which is copied into each of them on write.
func TestScriptListRepairScripts(t *testing.T) {
	const k = 10
	n := 10000

	var scripts []otfScript
	for script := range scriptBcp47 {
		if len(script) == 4 {
			scripts = append(scripts, script)
		}
	}
	slices.Sort(scripts)
	scripts = scripts[:k]
	// a different language for each script, so that the Script tables differ
	var langs []otfLang
	for l := range langBcp47 {
		if len(l) == 4 {
			langs = append(langs, l)
		}
	}
	slices.Sort(langs)

	hdr := 2 + 6*k
	langSysPos := hdr + 10*k
	data := []byte{0, k}
	for i, script := range scripts {
		off := hdr + 10*i
		data = append(data, script...)
		data = append(data, byte(off>>8), byte(off))
	}
	for i := range scripts {
		off := langSysPos - (hdr + 10*i)
		data = append(data, 0, 0, 0, 1)
		data = append(data, langs[i]...)
		data = append(data, byte(off>>8), byte(off))
	}
	data = append(data, 0, 0, 0xFF, 0xFF, byte(n>>8), byte(n))
	data = append(data, make([]byte, 2*n)...)

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	info, err := readScriptList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	// each Script table takes about 20 kB when written, so four fit
	if len(info) != 4 {
		t.Errorf("%d language systems kept, want 4", len(info))
	}

	data2 := encodeList(t, info)
	p = parser.New(bytes.NewReader(data2), parser.NewBudget(int64(len(data2))))
	info2, err := readScriptList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(info, info2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// TestScriptListMath checks that language systems under the "math" script
// are read and written.
func TestScriptListMath(t *testing.T) {
	data := []byte{
		0, 1, // scriptCount
		'm', 'a', 't', 'h', 0, 8, // script record
		0, 4, 0, 0, // Script table: default LangSys at offset 4, no others
		0, 0, 0xFF, 0xFF, 0, 1, 0, 7, // LangSys: one feature, index 7
	}
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	info, err := readScriptList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := ScriptListInfo{
		language.MustParse("und-Zmth-x-math"): &Features{
			Optional: []FeatureIndex{7},
		},
	}
	if diff := cmp.Diff(want, info); diff != "" {
		t.Fatalf("read failed (-want +got):\n%s", diff)
	}

	enc := encodeList(t, info)
	p = parser.New(bytes.NewReader(enc), parser.NewBudget(int64(len(enc))))
	info2, err := readScriptList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(info, info2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
	if !bytes.Contains(enc, []byte("math")) {
		t.Error("encoded script list lacks the math script tag")
	}
}

// TestScriptListTagForm checks that a tag without private-use subtags is
// read back in the explicit form.
func TestScriptListTagForm(t *testing.T) {
	info := ScriptListInfo{language.MustParse("und-Latn"): {}}
	got := readScriptListData(t, encodeList(t, info))
	if _, ok := got[language.MustParse("und-Latn-x-latn")]; !ok || len(got) != 1 {
		t.Errorf("unexpected tags in %v", got)
	}
}

// readScriptListData reads an encoded script list, and fails the test if
// the list cannot be read.
func readScriptListData(t *testing.T, data []byte) ScriptListInfo {
	t.Helper()
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	info, err := readScriptList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

// TestScriptListAllScripts checks that every script in the script table
// survives a round trip through a script list.
func TestScriptListAllScripts(t *testing.T) {
	info := ScriptListInfo{}
	for script := range scriptBcp47 {
		tag, err := otfToBCP47(script, "")
		if err != nil {
			t.Errorf("script %q: %v", script, err)
			continue
		}
		info[tag] = &Features{Optional: []FeatureIndex{FeatureIndex(len(info))}}
	}
	if len(info) != len(scriptBcp47) {
		t.Errorf("got %d tags for %d scripts", len(info), len(scriptBcp47))
	}

	got := readScriptListData(t, encodeList(t, info))
	if diff := cmp.Diff(info, got); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// TestScriptListPaddedScript checks that a script whose OpenType tag is
// padded with spaces can be written and read back.
func TestScriptListPaddedScript(t *testing.T) {
	features := &Features{Optional: []FeatureIndex{1}}
	info := ScriptListInfo{language.MustParse("und-Laoo"): features}
	data := encodeList(t, info)
	if !bytes.Contains(data, []byte("lao ")) {
		t.Error("encoded script list lacks the lao script tag")
	}

	got := readScriptListData(t, data)
	want := ScriptListInfo{language.MustParse("und-Laoo-x-lao"): features}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
	if again := encodeList(t, got); !bytes.Equal(data, again) {
		t.Error("re-encoding the decoded list gives different bytes")
	}
}

// scriptListData returns an encoded script list with one script.  The
// script has a default language system and, if lang is not empty, a second
// language system for lang.
func scriptListData(script, lang string) []byte {
	data := []byte{0, 1} // scriptCount
	data = append(data, script...)
	data = append(data, 0, 8) // offset of the Script table
	if lang == "" {
		data = append(data, 0, 4, 0, 0) // default LangSys at offset 4, no others
	} else {
		data = append(data, 0, 10, 0, 1) // default LangSys at offset 10, one other
		data = append(data, lang...)
		data = append(data, 0, 18)
	}
	data = append(data, 0, 0, 0xFF, 0xFF, 0, 1, 0, 7) // LangSys: one feature, index 7
	if lang != "" {
		data = append(data, 0, 0, 0xFF, 0xFF, 0, 1, 0, 8)
	}
	return data
}

// TestScriptListUnregisteredTags checks that script and language system
// tags which are missing from the tag tables are kept on read and written
// back.
func TestScriptListUnregisteredTags(t *testing.T) {
	cases := []struct {
		name, script, lang string
	}{
		{"script", "qqqq", ""},
		{"padded script", "qq  ", ""},
		{"language", "latn", "QQQ "},
		{"script and language", "qqqq", "QQQQ"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info := readScriptListData(t, scriptListData(c.script, c.lang))
			want := 1
			if c.lang != "" {
				want = 2
			}
			if len(info) != want {
				t.Fatalf("got %d language systems, want %d", len(info), want)
			}

			enc := encodeList(t, info)
			for _, tag := range []string{c.script, c.lang} {
				if !bytes.Contains(enc, []byte(tag)) {
					t.Errorf("encoded script list lacks the tag %q", tag)
				}
			}
			if diff := cmp.Diff(info, readScriptListData(t, enc)); diff != "" {
				t.Errorf("round trip failed (-first +second):\n%s", diff)
			}
		})
	}
}

// TestScriptListMalformedTags checks that a script list with tags which
// cannot be spelled in a BCP 47 tag can be read, and that the result
// survives a write/read cycle.
func TestScriptListMalformedTags(t *testing.T) {
	cases := []struct {
		name, script, lang string
	}{
		{"punctuation in script", "la-n", ""},
		{"upper-case script", "LATN", ""},
		{"lower-case default script", "dflt", ""},
		{"blank script", "    ", ""},
		{"inner space in script", "l tn", ""},
		{"punctuation in language", "latn", "EN-G"},
		{"lower-case language", "latn", "eng "},
		{"blank language", "latn", "    "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info := readScriptListData(t, scriptListData(c.script, c.lang))
			enc := encodeList(t, info)
			if diff := cmp.Diff(info, readScriptListData(t, enc)); diff != "" {
				t.Errorf("round trip failed (-first +second):\n%s", diff)
			}
		})
	}
}

// TestScriptListSharedTablesBudget checks that a script list is rejected if
// its shared tables expand to far more language systems than the size of
// the input allows.  Every script record refers to the same Script table,
// which has many language systems.
func TestScriptListSharedTablesBudget(t *testing.T) {
	const nScripts = 600
	const nLangs = 600

	scriptPos := 2 + 6*nScripts
	langSysPos := 4 + 6*nLangs // relative to the Script table
	data := []byte{byte(nScripts >> 8), byte(nScripts & 0xFF)}
	for i := range nScripts {
		data = fmt.Appendf(data, "q%03d", i)
		data = append(data, byte(scriptPos>>8), byte(scriptPos&0xFF))
	}
	data = append(data, 0, 0, byte(nLangs>>8), byte(nLangs&0xFF))
	for i := range nLangs {
		data = fmt.Appendf(data, "Q%03d", i)
		data = append(data, byte(langSysPos>>8), byte(langSysPos&0xFF))
	}
	data = append(data, 0, 0, 0xFF, 0xFF, 0, 0)

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if _, err := readScriptList(p, 0); err == nil {
		t.Error("script list beyond the budget was read")
	}
}

// TestEncodeScriptListTags checks that Encode accepts valid script-list
// tags, and rejects tags without an OpenType form and tags which name the
// same language system.
func TestEncodeScriptListTags(t *testing.T) {
	tags := func(tags ...string) ScriptListInfo {
		info := ScriptListInfo{}
		for i, tag := range tags {
			info[language.MustParse(tag)] = &Features{Optional: []FeatureIndex{FeatureIndex(i)}}
		}
		return info
	}
	features := FeatureListInfo{{Tag: "aaaa"}, {Tag: "bbbb"}, {Tag: "cccc"}, {Tag: "dddd"}}

	valid := map[string]ScriptListInfo{
		"plain tags":            tags("und-Latn", "en-Latn", "de-Latn", "und-Cyrl"),
		"explicit tags":         tags("und-Latn-x-latn", "en-Latn-x-latn-eng", "und-Deva-x-deva", "und-Deva-x-dev2"),
		"mixed tags":            tags("und-Latn", "en-Latn-x-latn-eng", "ru-Cyrl", "und-Laoo-x-lao"),
		"same language":         tags("en-Latn-x-latn-eng", "en-Cyrl-x-cyrl-eng"),
		"default and language":  tags("und-Zzzz", "en-Zzzz-x-dflt-eng"),
		"single default system": tags("und-Latn"),
	}
	for name, scripts := range valid {
		t.Run(name, func(t *testing.T) {
			info := &Info{ScriptList: scripts, FeatureList: features}
			if _, err := info.Encode(); err != nil {
				t.Error(err)
			}
		})
	}

	duplicate := map[string]ScriptListInfo{
		"language":        tags("en-Latn", "en-Latn-x-latn-eng"),
		"default":         tags("und-Latn", "und-Latn-x-latn"),
		"regions":         tags("en-US", "en-GB"),
		"padded language": tags("und-Latn-x-latn-ho", "hoc-Latn"),
	}
	invalid := map[string]ScriptListInfo{
		"long script":      tags("und-x-abcdefgh"),
		"long language":    tags("und-Latn-x-latn-abcdefgh"),
		"too many subtags": tags("und-Latn-x-latn-eng-abc"),
	}
	for name, scripts := range invalid {
		t.Run("invalid "+name, func(t *testing.T) {
			info := &Info{ScriptList: scripts, FeatureList: features}
			if _, err := info.Encode(); err == nil {
				t.Error("expected an error")
			}
		})
	}

	for name, scripts := range duplicate {
		t.Run("duplicate "+name, func(t *testing.T) {
			info := &Info{ScriptList: scripts, FeatureList: features}
			if _, err := info.Encode(); err == nil {
				t.Error("expected an error")
			}
		})
	}
}
