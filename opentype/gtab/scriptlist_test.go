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

package gtab

import (
	"bytes"
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
		Required: 0xFFFF,
		Optional: []FeatureIndex{1, 0xFFFF, 3},
	}
	data := info.encode()

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
		Required: 0xFFFF,
		Optional: []FeatureIndex{0},
	}
	f.Add(info.encode())
	info = ScriptListInfo{}
	info[language.English] = &Features{
		Required: 0xFFFF,
		Optional: []FeatureIndex{1, 2, 3, 4},
	}
	info[language.MustParse("und-Latn")] = &Features{
		Required: 7,
		Optional: []FeatureIndex{5, 6},
	}
	f.Add(info.encode())
	info = ScriptListInfo{}
	info[language.MustParse("und-Arab")] = &Features{
		Required: FeatureIndex(0),
		Optional: []FeatureIndex{1, 2},
	}
	f.Add(info.encode())
	info = ScriptListInfo{}
	info[language.Arabic] = &Features{
		Required: FeatureIndex(0xFFFF),
		Optional: []FeatureIndex{1, 3, 5},
	}
	info[language.Azerbaijani] = &Features{
		Required: FeatureIndex(0xFFFF),
		Optional: []FeatureIndex{2, 4, 6},
	}
	f.Add(info.encode())

	f.Fuzz(func(t *testing.T, data []byte) {
		p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		info, err := readScriptList(p, 0)
		if err != nil {
			return
		}

		data2 := info.encode()

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

func TestEncodeUnmappedTag(t *testing.T) {
	for _, tag := range []language.Tag{
		language.MustParse("und-Qaaa"), // private-use script
		language.MustParse("qaa-Latn"), // private-use language
	} {
		info := ScriptListInfo{tag: &Features{Required: 0xFFFF}}
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("encoding %v did not panic", tag)
				}
			}()
			info.encode()
		}()
	}
}

// bigFeatures returns a language system with n optional features.
func bigFeatures(n int) *Features {
	opt := make([]FeatureIndex, n)
	for i := range opt {
		opt[i] = FeatureIndex(i)
	}
	return &Features{Required: 0xFFFF, Optional: opt}
}

func TestScriptListOffsetRange(t *testing.T) {
	small := &Features{Required: 0xFFFF, Optional: []FeatureIndex{1}}

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
			data := info.encode()
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
			assertPanics(t, func() { info.encode() })
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
	enc := info.encode()
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
	a := &Features{Required: 0xFFFF, Optional: []FeatureIndex{1, 2, 3}}
	b := &Features{Required: 0xFFFF, Optional: []FeatureIndex{1, 2, 4}}
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

	data2 := info.encode()
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

	data2 := info.encode()
	p = parser.New(bytes.NewReader(data2), parser.NewBudget(int64(len(data2))))
	info2, err := readScriptList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(info, info2); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}
