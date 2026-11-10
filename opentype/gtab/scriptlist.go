// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"cmp"
	"slices"
	"sort"

	"golang.org/x/text/language"
	"seehuhn.de/go/sfnt/opentype/internal/offsetpool"
	"seehuhn.de/go/sfnt/opentype/internal/readcache"
	"seehuhn.de/go/sfnt/parser"
)

// ScriptListInfo contains the information of an OpenType "ScriptList table".
// It maps BCP 47 tags to OpenType font features.
//
// Tags read from a font name the OpenType script and language system in
// their private-use subtags, for example "und-Deva-x-dev2" or
// "en-Latn-x-latn-eng", and are written back unchanged.  A tag without
// these subtags is written with the OpenType tags implied by its language
// and script; where a script has both an original and a "v.2" tag, the
// "v.2" tag is used, since it selects the current shaping model.  Use the
// explicit form to request the original tag.  A language or script with no
// OpenType tag cannot be written; encoding such a tag panics.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#scriptList
type ScriptListInfo map[language.Tag]*Features

// Features describes the mandatory and optional features for a script/language.
type Features struct {
	Required FeatureIndex // 0xFFFF, if no required feature
	Optional []FeatureIndex
}

type scriptTableEntry struct {
	script otfScript
	offset uint16
}

// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#script-list-table-and-script-record
func readScriptList(p *parser.Parser, pos int64) (ScriptListInfo, error) {
	err := p.SeekPos(pos)
	if err != nil {
		return nil, err
	}

	scriptCount, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	if 6*int64(scriptCount) > p.Size() {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/gtab",
			Reason:    "invalid scriptCount",
		}
	}

	var entries []scriptTableEntry
	for i := 0; i < int(scriptCount); i++ {
		buf, err := p.ReadBytes(6)
		if err != nil {
			return nil, err
		}

		entry := scriptTableEntry{
			script: otfScript(buf[:4]),
			offset: uint16(buf[4])<<8 | uint16(buf[5]),
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].offset < entries[j].offset
	})

	for _, entry := range entries {
		if int(entry.offset) < 2+6*len(entries) {
			return nil, &parser.InvalidFontError{
				SubSystem: "sfnt/gtab",
				Reason:    "invalid script table offset",
			}
		}
	}

	// tables reached through several offsets are read once and shared
	scripts := readcache.New[*scriptBody]()
	langSys := readcache.New[*Features]()
	info := ScriptListInfo{}
	for _, entry := range entries {
		body, err := scripts.Get(pos+int64(entry.offset), func(pos int64) (*scriptBody, error) {
			return readScriptBody(p, pos, langSys)
		})
		if err != nil {
			return nil, err
		}
		for _, rec := range body.records {
			tag, err := otfToBCP47(entry.script, rec.lang)
			if err != nil {
				// TODO(voss): what to do here?
				continue
			}
			info[tag] = rec.features
		}
	}

	info.repair()
	if _, err := info.layout(); err != nil {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/gtab",
			Reason:    err.Error(),
		}
	}

	return info, nil
}

// scriptBody holds the language systems of a Script table, in the order in
// which they are applied.  The default language system has an empty language.
type scriptBody struct {
	records []scriptLang
}

type scriptLang struct {
	lang     otfLang
	features *Features
}

// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#script-table-and-language-system-record
func readScriptBody(p *parser.Parser, pos int64, cache *readcache.Cache[*Features]) (*scriptBody, error) {
	err := p.SeekPos(pos)
	if err != nil {
		return nil, err
	}

	data, err := p.ReadBytes(4)
	if err != nil {
		return nil, err
	}

	defaultLangSysOffset := uint16(data[0])<<8 | uint16(data[1])
	langSysCount := uint16(data[2])<<8 | uint16(data[3])

	if defaultLangSysOffset > 0 && int(defaultLangSysOffset) < 4+6*int(langSysCount) {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/gtab",
			Reason:    "invalid defaultLangSysOffset",
		}
	}
	if 4+int64(langSysCount)*6 > p.Size() {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/gtab",
			Reason:    "invalid langSysCount",
		}
	}

	type record struct {
		lang   otfLang
		offset uint16
	}

	var records []record
	if defaultLangSysOffset != 0 {
		records = append(records, record{
			offset: defaultLangSysOffset,
		})
	}
	for i := 0; i < int(langSysCount); i++ {
		buf, err := p.ReadBytes(6)
		if err != nil {
			return nil, err
		}

		records = append(records, record{
			lang:   otfLang(buf[:4]),
			offset: uint16(buf[4])<<8 | uint16(buf[5]),
		})
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].offset < records[j].offset
	})

	body := &scriptBody{}
	for _, rec := range records {
		ff, err := cache.Get(pos+int64(rec.offset), func(pos int64) (*Features, error) {
			return readLangSysTable(p, pos)
		})
		if err != nil {
			return nil, err
		}
		body.records = append(body.records, scriptLang{lang: rec.lang, features: ff})
	}
	return body, nil
}

// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#language-system-table
func readLangSysTable(p *parser.Parser, pos int64) (*Features, error) {
	err := p.SeekPos(pos)
	if err != nil {
		return nil, err
	}

	data, err := p.ReadBytes(6)
	if err != nil {
		return nil, err
	}
	lookupOrderOffset := uint16(data[0])<<8 | uint16(data[1])
	requiredFeatureIndex := FeatureIndex(data[2])<<8 | FeatureIndex(data[3])
	featureIndexCount := uint16(data[4])<<8 | uint16(data[5])
	if lookupOrderOffset != 0 {
		return nil, &parser.NotSupportedError{
			SubSystem: "sfnt/gtab",
			Feature:   "use of reordering tables",
		}
	}

	featureIndices, err := p.Budget.AllocSlice[FeatureIndex](int(featureIndexCount))
	if err != nil {
		return nil, err
	}
	featureIndices = featureIndices[:0]
	for i := 0; i < int(featureIndexCount); i++ {
		idx, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		// 0xFFFF is the "no feature" sentinel for requiredFeatureIndex; it
		// is not valid inside featureIndices, so drop it rather than turn it
		// into a reference to feature 0
		if idx == 0xFFFF {
			continue
		}
		featureIndices = append(featureIndices, FeatureIndex(idx))
	}

	return &Features{
		Required: requiredFeatureIndex,
		Optional: featureIndices,
	}, nil
}

// scriptListLayout holds the positions of the tables of a script list.
type scriptListLayout struct {
	scripts []otfScript
	offs    []uint16
	pool    *offsetpool.Pool
}

func (l *scriptListLayout) encodeLen() int {
	return 2 + 6*len(l.scripts) + l.pool.Len()
}

// byScript groups the language tags by script.  The scripts are returned
// in sorted order.
func (info ScriptListInfo) byScript() ([]otfScript, map[otfScript][]language.Tag) {
	scriptLangs := map[otfScript][]language.Tag{}
	for tag := range info {
		script, _, err := bcp47ToOtf(tag)
		if err != nil {
			panic("gtab: cannot encode script list: " + err.Error())
		}
		scriptLangs[script] = append(scriptLangs[script], tag)
	}

	scripts := make([]otfScript, 0, len(scriptLangs))
	for script := range scriptLangs {
		scripts = append(scripts, script)
	}
	slices.Sort(scripts)
	return scripts, scriptLangs
}

// layout computes the positions of the Script tables.  Script tables with
// identical encodings share one copy, and so do the LangSys tables within
// a Script table.  The Script tables are stored in the order of the scripts
// if possible, and otherwise in order of increasing size.  An error is
// returned if a table cannot be reached through its Offset16.
func (info ScriptListInfo) layout() (*scriptListLayout, error) {
	scripts, scriptLangs := info.byScript()

	// script tables with the same language systems are encoded once
	var keys offsetpool.SeqKeys[langSysRef]
	scriptKeys := make([]any, len(scripts))
	encs := map[any][]byte{}
	for i, script := range scripts {
		refs := info.langSysRefs(scriptLangs[script])
		key := keys.Key(offsetpool.SliceKey(refs), len(refs), slices.Values(refs))
		scriptKeys[i] = key
		if _, ok := encs[key]; ok {
			continue
		}
		enc, err := encodeScriptTable(refs)
		if err != nil {
			return nil, err
		}
		encs[key] = enc
	}

	order := make([]int, len(scripts))
	for i := range order {
		order[i] = i
	}
	place := func() ([]uint16, *offsetpool.Pool, error) {
		pool := offsetpool.New("script", 2+6*len(scripts))
		offs := make([]uint16, len(scripts))
		for _, i := range order {
			key := scriptKeys[i]
			off, err := pool.AddFunc(key, func() []byte { return encs[key] })
			if err != nil {
				return nil, nil, err
			}
			offs[i] = off
		}
		return offs, pool, nil
	}
	offs, pool, err := place()
	if err != nil {
		slices.SortStableFunc(order, func(i, j int) int {
			return cmp.Compare(len(encs[scriptKeys[i]]), len(encs[scriptKeys[j]]))
		})
		offs, pool, err = place()
		if err != nil {
			return nil, err
		}
	}
	return &scriptListLayout{scripts: scripts, offs: offs, pool: pool}, nil
}

// repair removes the parts of a script list read from a file which cannot
// be written back, and keeps the rest.  If the LangSys tables of a script
// cannot all be reached from its Script table, language systems are
// dropped, keeping as many LangSys tables as possible.  If then the Script
// tables cannot all be reached from the script list, scripts are dropped,
// keeping as many Script tables as possible.
func (info ScriptListInfo) repair() {
	if _, err := info.layout(); err == nil {
		return
	}

	scripts, scriptLangs := info.byScript()
	for _, script := range scripts {
		tags := scriptLangs[script]
		refs := info.langSysRefs(tags)
		if _, err := encodeScriptTable(refs); err == nil {
			continue
		}
		nRecords := 0
		sizes := map[*Features]int{}
		for _, ref := range refs {
			if ref.lang != "" {
				nRecords++
			}
			sizes[ref.features] = 6 + 2*len(ref.features.Optional)
		}
		keep := keepSmallest(4+6*nRecords, sizes)
		for _, tag := range tags {
			if !keep[info[tag]] {
				delete(info, tag)
			}
		}
	}

	if _, err := info.layout(); err == nil {
		return
	}
	scripts, scriptLangs = info.byScript()
	sizes := map[otfScript]int{}
	for _, script := range scripts {
		enc, err := encodeScriptTable(info.langSysRefs(scriptLangs[script]))
		if err != nil {
			panic("sfnt/gtab: script table does not fit after repair")
		}
		sizes[script] = len(enc)
	}
	keep := keepSmallest(2+6*len(scripts), sizes)
	for _, script := range scripts {
		if !keep[script] {
			for _, tag := range scriptLangs[script] {
				delete(info, tag)
			}
		}
	}
}

// keepSmallest returns the tables which can be reached through Offset16
// fields when they are stored in order of increasing size after a header of
// length hdr.  The sizes give the encoded lengths of the tables.
func keepSmallest[K comparable](hdr int, sizes map[K]int) map[K]bool {
	keys := make([]K, 0, len(sizes))
	for key := range sizes {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b K) int { return cmp.Compare(sizes[a], sizes[b]) })
	keep := map[K]bool{}
	pos := hdr
	for _, key := range keys {
		if pos > 0xFFFF {
			break
		}
		keep[key] = true
		pos += sizes[key]
	}
	return keep
}

// langSysRef refers to the LangSys table of a language.  The language is
// empty for the default LangSys table.
type langSysRef struct {
	lang     otfLang
	features *Features
}

// langSysRefs returns the language systems for the given language tags,
// which all belong to the same script, sorted by OpenType language.  Ties
// between tags which give the same language are broken by the tag, so that
// the result does not depend on the order of tags.
func (info ScriptListInfo) langSysRefs(tags []language.Tag) []langSysRef {
	type entry struct {
		langSysRef
		tag string
	}
	entries := make([]entry, len(tags))
	for i, tag := range tags {
		_, lang, _ := bcp47ToOtf(tag)
		if lang != "" && len(lang) != 4 {
			panic("invalid language " + string(lang))
		}
		entries[i] = entry{langSysRef{lang, info[tag]}, tag.String()}
	}
	slices.SortFunc(entries, func(a, b entry) int {
		if c := cmp.Compare(a.lang, b.lang); c != 0 {
			return c
		}
		return cmp.Compare(a.tag, b.tag)
	})
	refs := make([]langSysRef, len(entries))
	for i, e := range entries {
		refs[i] = e.langSysRef
	}
	return refs
}

// encodeScriptTable encodes the Script table for the given language
// systems, as returned by [ScriptListInfo.langSysRefs].  If several
// language systems are the default, the last one is used.  The LangSys
// tables are stored in the order of the language systems if possible, and
// otherwise in order of increasing size.
func encodeScriptTable(refs []langSysRef) ([]byte, error) {
	defaultIdx := -1
	var records []int // indices of the non-default language systems
	for i, ref := range refs {
		if ref.lang == "" {
			defaultIdx = i
		} else {
			records = append(records, i)
		}
	}
	var used []int // indices of the LangSys tables to store
	if defaultIdx >= 0 {
		used = append(used, defaultIdx)
	}
	used = append(used, records...)

	place := func() ([]uint16, *offsetpool.Pool, error) {
		pool := offsetpool.New("langSys", 4+6*len(records))
		offs := make([]uint16, len(refs))
		for _, i := range used {
			ff := refs[i].features
			off, err := pool.AddFunc(ff, func() []byte { return encodeLangSys(ff) })
			if err != nil {
				return nil, nil, err
			}
			offs[i] = off
		}
		return offs, pool, nil
	}
	offs, pool, err := place()
	if err != nil {
		slices.SortStableFunc(used, func(i, j int) int {
			return cmp.Compare(len(refs[i].features.Optional), len(refs[j].features.Optional))
		})
		offs, pool, err = place()
		if err != nil {
			return nil, err
		}
	}

	var defaultOffs uint16
	if defaultIdx >= 0 {
		defaultOffs = offs[defaultIdx]
	}
	buf := make([]byte, 4, 4+6*len(records)+pool.Len())
	buf[0] = byte(defaultOffs >> 8)
	buf[1] = byte(defaultOffs)
	buf[2] = byte(len(records) >> 8)
	buf[3] = byte(len(records))
	for _, i := range records {
		buf = append(buf, refs[i].lang...)
		buf = append(buf, byte(offs[i]>>8), byte(offs[i]))
	}
	return append(buf, pool.Bytes()...), nil
}

// encodeLangSys encodes a LangSys table.
func encodeLangSys(ff *Features) []byte {
	buf := make([]byte, 6, 6+2*len(ff.Optional))
	// the first two bytes are lookupOrderOffset
	buf[2] = byte(ff.Required >> 8)
	buf[3] = byte(ff.Required)
	buf[4] = byte(len(ff.Optional) >> 8)
	buf[5] = byte(len(ff.Optional))
	for _, idx := range ff.Optional {
		buf = append(buf, byte(idx>>8), byte(idx))
	}
	return buf
}

func (info ScriptListInfo) encode() []byte {
	if info == nil {
		return nil
	}
	l := mustFitList(info.layout())

	buf := make([]byte, 2, l.encodeLen())
	buf[0] = byte(len(l.scripts) >> 8)
	buf[1] = byte(len(l.scripts))
	for i, script := range l.scripts {
		buf = append(buf, script...)
		buf = append(buf, byte(l.offs[i]>>8), byte(l.offs[i]))
	}
	return append(buf, l.pool.Bytes()...)
}
