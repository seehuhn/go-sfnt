// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"errors"
	"fmt"
	"slices"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/internal/offsetpool"
	"seehuhn.de/go/sfnt/opentype/internal/readcache"
	"seehuhn.de/go/sfnt/parser"
)

// readGsubSubtable reads a GSUB subtable.
// This function can be used as the SubtableReader argument to readLookupList().
func readGsubSubtable(p *parser.Parser, pos int64, meta *LookupMetaInfo) (Subtable, error) {
	err := p.SeekPos(pos)
	if err != nil {
		return nil, err
	}

	format, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}

	reader, ok := gsubReaders[subtableKey{meta.LookupType, format}]
	if !ok {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/opentype/gtab",
			Reason: fmt.Sprintf("unknown GSUB subtable format %d.%d",
				meta.LookupType, format),
		}
	}
	return reader(p, pos)
}

var gsubReaders = map[subtableKey]func(p *parser.Parser, pos int64) (Subtable, error){
	{1, 1}:                       readGsub1_1,
	{1, 2}:                       readGsub1_2,
	{2, 1}:                       readGsub2_1,
	{3, 1}:                       readGsub3_1,
	{4, 1}:                       readGsub4_1,
	{5, 1}:                       readSeqContext1,
	{5, 2}:                       readSeqContext2,
	{5, 3}:                       readSeqContext3,
	{6, 1}:                       readChainedSeqContext1,
	{6, 2}:                       readChainedSeqContext2,
	{6, 3}:                       readChainedSeqContext3,
	{gsubExtensionLookupType, 1}: readExtensionSubtable,
	{8, 1}:                       readGsub8_1,
}

// Gsub1_1 is a Single Substitution subtable (GSUB type 1, format 1).
// Lookups of this type allow to replace a single glyph with another glyph.
// The original glyph must be contained in the coverage table.
// The new glyph is determined by adding `delta` to the original glyph's GID.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gsub#11-single-substitution-format-1
type Gsub1_1 struct {
	Cov   coverage.Set
	Delta glyph.ID
}

func readGsub1_1(p *parser.Parser, subtablePos int64) (Subtable, error) {
	buf, err := p.ReadBytes(4)
	if err != nil {
		return nil, err
	}
	coverageOffset := int64(buf[0])<<8 | int64(buf[1])
	deltaGlyphID := glyph.ID(buf[2])<<8 | glyph.ID(buf[3])
	cov, err := coverage.ReadSet(p, subtablePos+coverageOffset)
	if err != nil {
		return nil, err
	}
	res := &Gsub1_1{
		Cov:   cov,
		Delta: deltaGlyphID,
	}
	return res, nil
}

// apply implements the [Subtable] interface.
func (l *Gsub1_1) apply(ctx *Context, a, b int) int {
	seq := ctx.seq
	gid := seq[a].GID
	if _, ok := l.Cov[gid]; !ok {
		return -1
	}

	seq[a].GID += l.Delta
	return a + 1
}

// layout implements the [Subtable] interface.  The coverage table follows
// the fixed-size header, so the subtable always fits.
func (l *Gsub1_1) layout() (subtableLayout, error) {
	return &gsub1_1Layout{delta: l.Delta, cov: l.Cov.ToTable().Encode()}, nil
}

// gsub1_1Layout is the encoded layout of a [Gsub1_1] subtable: the header,
// followed by the coverage table.
type gsub1_1Layout struct {
	delta glyph.ID
	cov   []byte // the encoded coverage table
}

func (lay *gsub1_1Layout) encodeLen() int {
	return 6 + len(lay.cov)
}

func (lay *gsub1_1Layout) encode() []byte {
	buf := make([]byte, 0, lay.encodeLen())
	buf = append(buf,
		0, 1, // format
		0, 6, // coverage offset
		byte(lay.delta>>8), byte(lay.delta),
	)
	return append(buf, lay.cov...)
}

// Gsub1_2 is a Single Substitution GSUB subtable (type 1, format 2).
// Lookups of this type allow to replace a single glyph with another glyph.
// The original glyph must be contained in the coverage table.
// The new glyph is found by looking up the replacement GID in the
// SubstituteGlyphIDs table (indexed by the coverage index of the original GID).
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gsub#12-single-substitution-format-2
type Gsub1_2 struct {
	Cov                coverage.Table
	SubstituteGlyphIDs []glyph.ID // indexed by coverage index
}

func readGsub1_2(p *parser.Parser, subtablePos int64) (Subtable, error) {
	coverageOffset, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	substituteGlyphIDs, err := readGIDSlice(p)
	if err != nil {
		return nil, err
	}

	listing, err := coverage.ReadListing(p, subtablePos+int64(coverageOffset))
	if err != nil {
		return nil, err
	}
	cov, substituteGlyphIDs := coverage.Align(listing, substituteGlyphIDs)

	res := &Gsub1_2{
		Cov:                cov,
		SubstituteGlyphIDs: substituteGlyphIDs,
	}
	return res, nil
}

// apply implements the [Subtable] interface.
func (l *Gsub1_2) apply(ctx *Context, a, b int) int {
	seq := ctx.seq
	gid := seq[a].GID
	idx, ok := l.Cov[gid]
	if !ok {
		return -1
	}

	seq[a].GID = l.SubstituteGlyphIDs[idx]
	return a + 1
}

// layout implements the [Subtable] interface.  The coverage table follows
// the substitute glyph IDs.
func (l *Gsub1_2) layout() (subtableLayout, error) {
	if err := checkCoverage("Gsub1_2", l.Cov, len(l.SubstituteGlyphIDs)); err != nil {
		return nil, err
	}
	covOffset, err := fitOffset16("Gsub1_2", 6+2*len(l.SubstituteGlyphIDs))
	if err != nil {
		return nil, err
	}
	return &gsub1_2Layout{l: l, covOffset: covOffset, cov: l.Cov.Encode()}, nil
}

// gsub1_2Layout is the encoded layout of a [Gsub1_2] subtable: the header
// and the substitute glyph IDs, followed by the coverage table.
type gsub1_2Layout struct {
	l         *Gsub1_2
	covOffset uint16
	cov       []byte // the encoded coverage table
}

func (lay *gsub1_2Layout) encodeLen() int {
	return int(lay.covOffset) + len(lay.cov)
}

func (lay *gsub1_2Layout) encode() []byte {
	ids := lay.l.SubstituteGlyphIDs
	n := len(ids)
	buf := make([]byte, 0, lay.encodeLen())
	buf = append(buf,
		0, 2, // format
		byte(lay.covOffset>>8), byte(lay.covOffset),
		byte(n>>8), byte(n),
	)
	for _, gid := range ids {
		buf = append(buf, byte(gid>>8), byte(gid))
	}
	return append(buf, lay.cov...)
}

// Gsub2_1 is a Multiple Substitution GSUB subtable (type 2, format 1).
// Lookups of this type allow to replace a single glyph with multiple glyphs.
// The original glyph must be contained in the coverage table.
// The new glyphs are found by looking up the replacement GIDs in the
// `Repl` table (indexed by the coverage index of the original GID).
// Replacement sequences must have at least one glyph.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gsub#21-multiple-substitution-format-1
type Gsub2_1 struct {
	Cov  coverage.Table
	Repl [][]glyph.ID // indexed by coverage index
}

func readGsub2_1(p *parser.Parser, subtablePos int64) (Subtable, error) {
	coverageOffset, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	sequenceOffsets, err := p.ReadUint16Slice()
	if err != nil {
		return nil, err
	}

	listing, err := coverage.ReadListing(p, subtablePos+int64(coverageOffset))
	if err != nil {
		return nil, err
	}
	cov, sequenceOffsets := coverage.Align(listing, sequenceOffsets)

	sequenceCount := len(sequenceOffsets)
	repl, err := p.Budget.AllocSlice[[]glyph.ID](sequenceCount)
	if err != nil {
		return nil, err
	}
	seqCache := readcache.New[[]glyph.ID]()
	for i := range sequenceCount {
		repl[i], err = seqCache.Get(subtablePos+int64(sequenceOffsets[i]),
			func(pos int64) ([]glyph.ID, error) {
				if err := p.SeekPos(pos); err != nil {
					return nil, err
				}
				return readGIDSlice(p)
			})
		if err != nil {
			return nil, err
		}
	}

	// A sequence must have at least one glyph.  An empty sequence is
	// dropped together with its glyph, so that a following subtable can
	// apply to the glyph.
	cov, repl = dropEmptyEntries(cov, repl)

	res := &Gsub2_1{
		Cov:  cov,
		Repl: repl,
	}
	return res, nil
}

// apply implements the [Subtable] interface.
func (l *Gsub2_1) apply(ctx *Context, a, b int) int {
	seq := ctx.seq
	gid := seq[a].GID
	idx, ok := l.Cov[gid]
	if !ok {
		return -1
	}

	repl := l.Repl[idx]
	seq[a].GID = repl[0]
	k := len(repl)
	if k > 1 {
		// insert k-1 new glyphs after position a
		seq = slices.Grow(seq, k-1)
		seq = seq[:len(seq)+k-1]
		copy(seq[a+k:], seq[a+1:])
		for i := 1; i < k; i++ {
			seq[a+i] = glyph.Info{GID: repl[i]}
		}
		ctx.seq = seq

		// Fix up input sequences and positions in parent lookups.
		ctx.fixStackInsert(a, k)
	}

	return a + k
}

// covSetLayout is the encoded layout of a subtable which holds a coverage
// table and one table per covered glyph: the header with the offsets,
// followed by the per-glyph tables and the coverage table.  Identical
// tables are stored once.  The Gsub2_1, Gsub3_1 and Gsub4_1 subtables have
// this form.
type covSetLayout struct {
	cov  int   // pool index of the coverage table
	sets []int // pool indices of the per-glyph tables
	pool *offsetpool.Deferred
}

func newCovSetLayout(name string, n int) *covSetLayout {
	return &covSetLayout{
		sets: make([]int, n),
		pool: offsetpool.NewDeferred(name, 6+2*n),
	}
}

// addSet stores the table for coverage index i.  The key and encode
// arguments are as for [offsetpool.Pool.AddFunc].
func (lay *covSetLayout) addSet(i int, key any, encode func() ([]byte, error)) error {
	idx, err := lay.pool.AddFunc(key, encode)
	if err != nil {
		return err
	}
	lay.sets[i] = idx
	return nil
}

// addCoverage stores the coverage table.  It is called after the per-glyph
// tables, so that the coverage table follows them where this fits.
func (lay *covSetLayout) addCoverage(cov coverage.Table) error {
	idx, err := lay.pool.Add(cov.Encode())
	if err != nil {
		return err
	}
	lay.cov = idx
	return nil
}

func (lay *covSetLayout) encodeLen() int {
	return 6 + 2*len(lay.sets) + lay.pool.Len()
}

func (lay *covSetLayout) encode() []byte {
	offs, data := lay.pool.Place()
	buf := make([]byte, 0, lay.encodeLen())
	buf = appendUint16(buf, 1) // format
	buf = appendUint16(buf, int(offs[lay.cov]))
	buf = appendUint16(buf, len(lay.sets))
	buf = appendOffsets(buf, offs, lay.sets)
	return append(buf, data...)
}

// layout implements the [Subtable] interface.
func (l *Gsub2_1) layout() (subtableLayout, error) {
	if err := checkCoverage("Gsub2_1", l.Cov, len(l.Repl)); err != nil {
		return nil, err
	}
	lay := newCovSetLayout("Gsub2_1", len(l.Repl))
	for i, repl := range l.Repl {
		if len(repl) == 0 {
			return nil, errors.New("Gsub2_1: empty replacement sequence")
		}
		if len(repl) > 0xFFFF {
			return nil, errors.New("Gsub2_1: replacement sequence too long")
		}
		err := lay.addSet(i, offsetpool.SliceKey(repl), func() ([]byte, error) { return encodeGIDSlice(repl), nil })
		if err != nil {
			return nil, err
		}
	}
	if err := lay.addCoverage(l.Cov); err != nil {
		return nil, err
	}
	return lay, nil
}

// Gsub3_1 is an Alternate Substitution GSUB subtable (type 3, format 1).
// Lookups of this type let the user choose between alternate glyphs for
// a given input glyph. The original glyph must be contained in the
// coverage table. The alternates are found by looking up the
// `Alternates` table by the coverage index of the original GID.
// Each alternate set must have at least one glyph.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gsub#31-alternate-substitution-format-1
type Gsub3_1 struct {
	Cov        coverage.Table
	Alternates [][]glyph.ID
}

func readGsub3_1(p *parser.Parser, subtablePos int64) (Subtable, error) {
	coverageOffset, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	alternateSetOffsets, err := p.ReadUint16Slice()
	if err != nil {
		return nil, err
	}

	listing, err := coverage.ReadListing(p, subtablePos+int64(coverageOffset))
	if err != nil {
		return nil, err
	}
	cov, alternateSetOffsets := coverage.Align(listing, alternateSetOffsets)

	alternateSetCount := len(alternateSetOffsets)
	alt, err := p.Budget.AllocSlice[[]glyph.ID](alternateSetCount)
	if err != nil {
		return nil, err
	}
	altCache := readcache.New[[]glyph.ID]()
	for i := range alternateSetCount {
		alt[i], err = altCache.Get(subtablePos+int64(alternateSetOffsets[i]),
			func(pos int64) ([]glyph.ID, error) {
				if err := p.SeekPos(pos); err != nil {
					return nil, err
				}
				return readGIDSlice(p)
			})
		if err != nil {
			return nil, err
		}
	}

	// An alternate set must have at least one glyph.  An empty set is
	// dropped together with its glyph, so that a following subtable can
	// apply to the glyph.
	cov, alt = dropEmptyEntries(cov, alt)

	res := &Gsub3_1{
		Cov:        cov,
		Alternates: alt,
	}
	return res, nil
}

// apply implements the [Subtable] interface.
func (l *Gsub3_1) apply(ctx *Context, a, b int) int {
	seq := ctx.seq
	gid := seq[a].GID
	idx, ok := l.Cov[gid]
	if !ok {
		return -1
	}

	seq[a].GID = l.Alternates[idx][0]

	return a + 1
}

// layout implements the [Subtable] interface.
func (l *Gsub3_1) layout() (subtableLayout, error) {
	if err := checkCoverage("Gsub3_1", l.Cov, len(l.Alternates)); err != nil {
		return nil, err
	}
	lay := newCovSetLayout("Gsub3_1", len(l.Alternates))
	for i, alt := range l.Alternates {
		if len(alt) == 0 {
			return nil, errors.New("Gsub3_1: empty alternate set")
		}
		if len(alt) > 0xFFFF {
			return nil, errors.New("Gsub3_1: too many alternates")
		}
		err := lay.addSet(i, offsetpool.SliceKey(alt), func() ([]byte, error) { return encodeGIDSlice(alt), nil })
		if err != nil {
			return nil, err
		}
	}
	if err := lay.addCoverage(l.Cov); err != nil {
		return nil, err
	}
	return lay, nil
}

// Gsub4_1 is a Ligature Substitution GSUB subtable (type 4, format 1).
// Lookups of this type replace a sequence of glyphs with a single glyph.
//
// The order of entries in Repl defines the preference for using the ligatures,
// for example "ffl" is only applied if it comes before "ff".
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gsub#41-ligature-substitution-format-1
type Gsub4_1 struct {
	Cov  coverage.Table
	Repl [][]Ligature // indexed by coverage index
}

// Ligature represents a substitution of a sequence of glyphs into a single glyph
// in a [Gsub4_1] subtable.
type Ligature struct {
	// In is the sequence of input glyphs that is replaced by Out, excluding
	// the first glyph in the sequence (since this is in Cov).
	In []glyph.ID

	// Out is the glyph that replaces the input sequence.
	Out glyph.ID
}

func readGsub4_1(p *parser.Parser, subtablePos int64) (Subtable, error) {
	coverageOffset, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	ligatureSetOffsets, err := p.ReadUint16Slice()
	if err != nil {
		return nil, err
	}

	listing, err := coverage.ReadListing(p, subtablePos+int64(coverageOffset))
	if err != nil {
		return nil, err
	}
	cov, ligatureSetOffsets := coverage.Align(listing, ligatureSetOffsets)

	repl, err := p.Budget.AllocSlice[[]Ligature](len(ligatureSetOffsets))
	if err != nil {
		return nil, err
	}
	ligCache := readcache.New[Ligature]()
	readLigature := func(pos int64) (Ligature, error) {
		if err := p.SeekPos(pos); err != nil {
			return Ligature{}, err
		}
		ligatureGlyph, err := p.ReadUint16()
		if err != nil {
			return Ligature{}, err
		}
		componentCount, err := p.ReadUint16()
		if err != nil {
			return Ligature{}, err
		}
		if componentCount == 0 {
			return Ligature{}, &parser.InvalidFontError{
				SubSystem: "sfnt/opentype/gtab",
				Reason:    "ligature with zero component count",
			}
		}
		componentGlyphIDs, err := p.Budget.AllocSlice[glyph.ID](int(componentCount) - 1)
		if err != nil {
			return Ligature{}, err
		}
		for k := range componentGlyphIDs {
			gid, err := p.ReadUint16()
			if err != nil {
				return Ligature{}, err
			}
			componentGlyphIDs[k] = glyph.ID(gid)
		}
		return Ligature{In: componentGlyphIDs, Out: glyph.ID(ligatureGlyph)}, nil
	}
	setCache := readcache.New[[]Ligature]()
	readSet := func(setPos int64) ([]Ligature, error) {
		if err := p.SeekPos(setPos); err != nil {
			return nil, err
		}
		ligatureOffsets, err := p.ReadUint16Slice()
		if err != nil {
			return nil, err
		}
		set, err := p.Budget.AllocSlice[Ligature](len(ligatureOffsets))
		if err != nil {
			return nil, err
		}
		for j, ligatureOffset := range ligatureOffsets {
			set[j], err = ligCache.Get(setPos+int64(ligatureOffset), readLigature)
			if err != nil {
				return nil, err
			}
		}
		return set, nil
	}
	for i, ligatureSetOffset := range ligatureSetOffsets {
		repl[i], err = setCache.Get(subtablePos+int64(ligatureSetOffset), readSet)
		if err != nil {
			return nil, err
		}
	}

	return &Gsub4_1{
		Cov:  cov,
		Repl: repl,
	}, nil
}

// apply implements the [Subtable] interface.
func (l *Gsub4_1) apply(ctx *Context, a, b int) int {
	seq := ctx.seq

	gid := seq[a].GID
	ligSetIdx, ok := l.Cov[gid]
	if !ok {
		return -1
	}
	ligSet := l.Repl[ligSetIdx]

	keep := ctx.keep

	var matchPos []int
	var skipPos []int
	var skipComp []uint16 // ligature component each skipped glyph follows
	var text []rune
ligLoop:
	for j := range ligSet {
		lig := &ligSet[j]

		matchPos = matchPos[:0]
		skipPos = skipPos[:0]
		skipComp = skipComp[:0]
		text = text[:0]

		matchPos = append(matchPos, a)
		text = append(text, seq[a].Text...)
		p := a + 1
		for c, ligGid := range lig.In {
			for p < b && !keep.Keep(seq[p].GID) {
				skipPos = append(skipPos, p)
				skipComp = append(skipComp, uint16(c))
				p++
			}

			if p >= b || seq[p].GID != ligGid { // no match
				continue ligLoop
			}

			matchPos = append(matchPos, p)
			text = append(text, seq[p].Text...)
			p++
		}

		// Insert the ligature glyph.  When marks were skipped between
		// components, tag the ligature and those marks with a shared
		// ligature id (and the component each mark follows) so that
		// mark-to-ligature positioning can attach each mark to the right
		// component.
		ligInfo := glyph.Info{GID: lig.Out, Text: text}
		var ligID uint16
		if len(skipPos) > 0 {
			ligID = ctx.newLigID()
			ligInfo.LigID = ligID
		}
		seq[a] = ligInfo

		// Move all the skipped glyphs after position a.
		for i, skip := range skipPos {
			m := seq[skip]
			m.LigID = ligID
			m.LigComp = skipComp[i]
			seq[a+i+1] = m
		}

		// Move the tail to the correct position.
		ctx.seq = slices.Delete(seq, a+1+len(skipPos), a+len(lig.In)+1+len(skipPos))

		ctx.fixStackMerge(matchPos)

		return a + 1 + len(skipPos)
	}

	return -1
}

// ligatureKey identifies the encoding of a Ligature table.
type ligatureKey struct {
	in  any
	out glyph.ID
}

func newLigatureKey(lig Ligature) ligatureKey {
	return ligatureKey{offsetpool.SliceKey(lig.In), lig.Out}
}

// encodeLigatureSet encodes a LigatureSet table.  Identical Ligature tables
// within the set are stored once.
func encodeLigatureSet(ligs []Ligature) ([]byte, error) {
	m := len(ligs)
	pool := offsetpool.NewDeferred("Gsub4_1 LigatureSet", 2+2*m)
	indices := make([]int, m)
	for i, lig := range ligs {
		if len(lig.In) >= 0xFFFF {
			return nil, errors.New("Gsub4_1: ligature too long")
		}
		var err error
		indices[i], err = pool.AddFunc(newLigatureKey(lig), func() ([]byte, error) {
			enc := make([]byte, 0, 4+2*len(lig.In))
			enc = appendUint16(enc, int(lig.Out))
			enc = appendUint16(enc, len(lig.In)+1)
			for _, gid := range lig.In {
				enc = appendUint16(enc, int(gid))
			}
			return enc, nil
		})
		if err != nil {
			return nil, err
		}
	}
	offs, data := pool.Place()
	buf := make([]byte, 0, 2+2*m+len(data))
	buf = appendUint16(buf, m)
	buf = appendOffsets(buf, offs, indices)
	return append(buf, data...), nil
}

// layout implements the [Subtable] interface.
func (l *Gsub4_1) layout() (subtableLayout, error) {
	if err := checkCoverage("Gsub4_1", l.Cov, len(l.Repl)); err != nil {
		return nil, err
	}
	lay := newCovSetLayout("Gsub4_1", len(l.Repl))
	// ligature sets with the same ligatures are encoded once
	var keys offsetpool.SeqKeys[ligatureKey]
	for i, ligs := range l.Repl {
		key := keys.Key(offsetpool.SliceKey(ligs), len(ligs), func(yield func(ligatureKey) bool) {
			for _, lig := range ligs {
				if !yield(newLigatureKey(lig)) {
					return
				}
			}
		})
		err := lay.addSet(i, key, func() ([]byte, error) {
			return encodeLigatureSet(ligs)
		})
		if err != nil {
			return nil, err
		}
	}
	if err := lay.addCoverage(l.Cov); err != nil {
		return nil, err
	}
	return lay, nil
}

// Gsub8_1 is a Reverse Chaining Contextual Single Substitution GSUB subtable
// (type 8, format 1).
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/gsub#81-reverse-chaining-contextual-single-substitution-format-1-coverage-based-glyph-contexts
type Gsub8_1 struct {
	Input              coverage.Table
	Backtrack          []coverage.Table
	Lookahead          []coverage.Table
	SubstituteGlyphIDs []glyph.ID // indexed by input coverage index
}

func readGsub8_1(p *parser.Parser, subtablePos int64) (Subtable, error) {
	coverageOffset, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	backtrackCoverageOffsets, err := p.ReadUint16Slice()
	if err != nil {
		return nil, err
	}
	lookaheadCoverageOffsets, err := p.ReadUint16Slice()
	if err != nil {
		return nil, err
	}
	substituteGlyphIDs, err := readGIDSlice(p)
	if err != nil {
		return nil, err
	}

	// coverage tables reached through several offsets are read once
	covCache := readcache.New[*coverage.Listing]()
	readCov := func(pos int64) (*coverage.Listing, error) {
		return coverage.ReadListing(p, pos)
	}

	listing, err := covCache.Get(subtablePos+int64(coverageOffset), readCov)
	if err != nil {
		return nil, err
	}
	input, substituteGlyphIDs := coverage.Align(listing, substituteGlyphIDs)

	backtrack, err := p.Budget.AllocSlice[coverage.Table](len(backtrackCoverageOffsets))
	if err != nil {
		return nil, err
	}
	for i, offs := range backtrackCoverageOffsets {
		listing, err := covCache.Get(subtablePos+int64(offs), readCov)
		if err != nil {
			return nil, err
		}
		backtrack[i] = listing.Table()
	}
	lookahead, err := p.Budget.AllocSlice[coverage.Table](len(lookaheadCoverageOffsets))
	if err != nil {
		return nil, err
	}
	for i, offs := range lookaheadCoverageOffsets {
		listing, err := covCache.Get(subtablePos+int64(offs), readCov)
		if err != nil {
			return nil, err
		}
		lookahead[i] = listing.Table()
	}

	res := &Gsub8_1{
		Input:              input,
		Backtrack:          backtrack,
		Lookahead:          lookahead,
		SubstituteGlyphIDs: substituteGlyphIDs,
	}
	return res, nil
}

// apply implements the [Subtable] interface.
func (l *Gsub8_1) apply(ctx *Context, a, b int) int {
	seq := ctx.seq
	keep := ctx.keep

	gid := seq[a].GID
	idx, ok := l.Input[gid]
	if !ok {
		return -1
	}

	p := a
	glyphsNeeded := len(l.Backtrack)
	for _, cov := range l.Backtrack {
		p--
		glyphsNeeded--
		for p-glyphsNeeded >= 0 && !keep.Keep(seq[p].GID) {
			p--
		}
		if p-glyphsNeeded < 0 || !cov.Contains(seq[p].GID) {
			return -1
		}
	}

	p = a
	glyphsNeeded = len(l.Lookahead)
	for _, cov := range l.Lookahead {
		p++
		glyphsNeeded--
		for p+glyphsNeeded < len(seq) && !keep.Keep(seq[p].GID) {
			p++
		}
		if p+glyphsNeeded >= len(seq) || !cov.Contains(seq[p].GID) {
			return -1
		}
	}

	seq[a].GID = l.SubstituteGlyphIDs[idx]
	return a + 1
}

// gsub8_1Layout is the encoded layout of a [Gsub8_1] subtable: the header
// followed by the coverage tables.  Identical tables are stored once.
type gsub8_1Layout struct {
	l         *Gsub8_1
	input     int   // pool index of the input coverage table
	backtrack []int // pool indices of the backtrack coverage tables
	lookahead []int // pool indices of the lookahead coverage tables
	pool      *offsetpool.Deferred
}

// layout implements the [Subtable] interface.
func (l *Gsub8_1) layout() (subtableLayout, error) {
	if err := checkCoverage("Gsub8_1", l.Input, len(l.SubstituteGlyphIDs)); err != nil {
		return nil, err
	}
	// a coverage table shared by several entries is checked once
	checked := make(map[any]bool)
	for _, cov := range slices.Concat(l.Backtrack, l.Lookahead) {
		key := offsetpool.MapKey(cov)
		if checked[key] {
			continue
		}
		checked[key] = true
		if err := checkCoverage("Gsub8_1", cov, len(cov)); err != nil {
			return nil, err
		}
	}
	nb, na := len(l.Backtrack), len(l.Lookahead)
	pool := offsetpool.NewDeferred("Gsub8_1", 10+2*(nb+na+len(l.SubstituteGlyphIDs)))
	lay := &gsub8_1Layout{
		l:         l,
		backtrack: make([]int, nb),
		lookahead: make([]int, na),
		pool:      pool,
	}
	var err error
	lay.input, err = pool.AddFunc(offsetpool.MapKey(l.Input), func() ([]byte, error) { return l.Input.Encode(), nil })
	if err != nil {
		return nil, err
	}
	for i, cov := range l.Backtrack {
		lay.backtrack[i], err = pool.AddFunc(offsetpool.MapKey(cov), func() ([]byte, error) { return cov.Encode(), nil })
		if err != nil {
			return nil, err
		}
	}
	for i, cov := range l.Lookahead {
		lay.lookahead[i], err = pool.AddFunc(offsetpool.MapKey(cov), func() ([]byte, error) { return cov.Encode(), nil })
		if err != nil {
			return nil, err
		}
	}
	return lay, nil
}

func (lay *gsub8_1Layout) encodeLen() int {
	return 10 + 2*(len(lay.backtrack)+len(lay.lookahead)+len(lay.l.SubstituteGlyphIDs)) + lay.pool.Len()
}

func (lay *gsub8_1Layout) encode() []byte {
	l := lay.l
	nb, na, ng := len(lay.backtrack), len(lay.lookahead), len(l.SubstituteGlyphIDs)
	offs, data := lay.pool.Place()
	buf := make([]byte, 0, 10+2*(nb+na+ng)+len(data))
	buf = appendUint16(buf, 1) // format
	buf = appendUint16(buf, int(offs[lay.input]))
	buf = appendUint16(buf, nb)
	buf = appendOffsets(buf, offs, lay.backtrack)
	buf = appendUint16(buf, na)
	buf = appendOffsets(buf, offs, lay.lookahead)
	buf = appendUint16(buf, ng)
	for _, gid := range l.SubstituteGlyphIDs {
		buf = appendUint16(buf, int(gid))
	}
	return append(buf, data...)
}

// dropEmptyEntries removes the glyphs whose item is empty from a coverage
// table and from the items indexed by its coverage indices.  The arguments
// are not modified.
func dropEmptyEntries(cov coverage.Table, items [][]glyph.ID) (coverage.Table, [][]glyph.ID) {
	cov, items, _ = coverage.Filter(cov, items, func(_ glyph.ID, item []glyph.ID) bool {
		return len(item) > 0
	})
	return cov, items
}
