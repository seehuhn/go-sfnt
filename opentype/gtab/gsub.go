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
	"fmt"
	"slices"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/coverage"
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

	reader, ok := gsubReaders[10*meta.LookupType+format]
	if !ok {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/opentype/gtab",
			Reason: fmt.Sprintf("unknown GSUB subtable format %d.%d",
				meta.LookupType, format),
		}
	}
	return reader(p, pos)
}

var gsubReaders = map[uint16]func(p *parser.Parser, pos int64) (Subtable, error){
	1_1: readGsub1_1,
	1_2: readGsub1_2,
	2_1: readGsub2_1,
	3_1: readGsub3_1,
	4_1: readGsub4_1,
	5_1: readSeqContext1,
	5_2: readSeqContext2,
	5_3: readSeqContext3,
	6_1: readChainedSeqContext1,
	6_2: readChainedSeqContext2,
	6_3: readChainedSeqContext3,
	7_1: readExtensionSubtable,
	8_1: readGsub8_1,
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

// encodeLen implements the [Subtable] interface.
func (l *Gsub1_1) encodeLen() int {
	return 6 + l.Cov.ToTable().EncodeLen()
}

// encode implements the [Subtable] interface.
func (l *Gsub1_1) encode() []byte {
	cov := l.Cov.ToTable()
	buf := make([]byte, 6+cov.EncodeLen())
	// buf[0] = 0
	buf[1] = 1
	// buf[2] = 0
	buf[3] = 6
	buf[4] = byte(l.Delta >> 8)
	buf[5] = byte(l.Delta)
	copy(buf[6:], cov.Encode())
	return buf
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

	cov, err := coverage.Read(p, subtablePos+int64(coverageOffset))
	if err != nil {
		return nil, err
	}

	if len(cov) > len(substituteGlyphIDs) {
		cov.Prune(len(substituteGlyphIDs))
	} else {
		substituteGlyphIDs = substituteGlyphIDs[:len(cov)]
	}

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

// encodeLen implements the [Subtable] interface.
func (l *Gsub1_2) encodeLen() int {
	return 6 + 2*len(l.SubstituteGlyphIDs) + l.Cov.EncodeLen()
}

// encode implements the [Subtable] interface.
func (l *Gsub1_2) encode() []byte {
	n := len(l.SubstituteGlyphIDs)
	covOffs := 6 + 2*n

	buf := make([]byte, covOffs+l.Cov.EncodeLen())
	// buf[0] = 0
	buf[1] = 2
	buf[2] = byte(covOffs >> 8)
	buf[3] = byte(covOffs)
	buf[4] = byte(n >> 8)
	buf[5] = byte(n)
	for i := 0; i < n; i++ {
		buf[6+2*i] = byte(l.SubstituteGlyphIDs[i] >> 8)
		buf[6+2*i+1] = byte(l.SubstituteGlyphIDs[i])
	}
	copy(buf[covOffs:], l.Cov.Encode())
	return buf
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

	cov, err := coverage.Read(p, subtablePos+int64(coverageOffset))
	if err != nil {
		return nil, err
	}

	if len(cov) > len(sequenceOffsets) {
		cov.Prune(len(sequenceOffsets))
	} else {
		sequenceOffsets = sequenceOffsets[:len(cov)]
	}

	sequenceCount := len(sequenceOffsets)
	repl := make([][]glyph.ID, sequenceCount)
	for i := 0; i < sequenceCount; i++ {
		err := p.SeekPos(subtablePos + int64(sequenceOffsets[i]))
		if err != nil {
			return nil, err
		}
		repl[i], err = readGIDSlice(p)
		if err != nil {
			return nil, err
		}
	}

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

// encodeLen implements the [Subtable] interface.
func (l *Gsub2_1) encodeLen() int {
	total := 6 + 2*len(l.Repl)
	for _, repl := range l.Repl {
		total += 2 + 2*len(repl)
	}
	total += l.Cov.EncodeLen()
	return total
}

// encode implements the [Subtable] interface.
func (l *Gsub2_1) encode() []byte {
	sequenceCount := len(l.Repl)
	covOffs := 6 + 2*sequenceCount

	sequenceOffsets := make([]uint16, sequenceCount)
	for i, repl := range l.Repl {
		sequenceOffsets[i] = uint16(covOffs)
		covOffs += 2 + 2*len(repl)
	}

	buf := make([]byte, covOffs+l.Cov.EncodeLen())
	// buf[0] = 0
	buf[1] = 1
	buf[2] = byte(covOffs >> 8)
	buf[3] = byte(covOffs)
	buf[4] = byte(len(l.Repl) >> 8)
	buf[5] = byte(len(l.Repl))
	pos := 6
	for i := range l.Repl {
		buf[pos] = byte(sequenceOffsets[i] >> 8)
		buf[pos+1] = byte(sequenceOffsets[i])
		pos += 2
	}
	for _, repl := range l.Repl {
		buf[pos] = byte(len(repl) >> 8)
		buf[pos+1] = byte(len(repl))
		pos += 2
		for _, gid := range repl {
			buf[pos] = byte(gid >> 8)
			buf[pos+1] = byte(gid)
			pos += 2
		}
	}
	copy(buf[covOffs:], l.Cov.Encode())
	return buf
}

// Gsub3_1 is an Alternate Substitution GSUB subtable (type 3, format 1).
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

	cov, err := coverage.Read(p, subtablePos+int64(coverageOffset))
	if err != nil {
		return nil, err
	}

	if len(cov) > len(alternateSetOffsets) {
		cov.Prune(len(alternateSetOffsets))
	} else {
		alternateSetOffsets = alternateSetOffsets[:len(cov)]
	}

	alternateSetCount := len(alternateSetOffsets)
	alt := make([][]glyph.ID, alternateSetCount)
	for i := 0; i < alternateSetCount; i++ {
		err := p.SeekPos(subtablePos + int64(alternateSetOffsets[i]))
		if err != nil {
			return nil, err
		}
		glyphCount, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		alt[i] = make([]glyph.ID, glyphCount)
		for j := 0; j < int(glyphCount); j++ {
			gid, err := p.ReadUint16()
			if err != nil {
				return nil, err
			}
			alt[i][j] = glyph.ID(gid)
		}
	}

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

	alt := l.Alternates[idx]
	if len(alt) == 0 {
		return -1 // TODO(voss): can this happen?
	}

	seq[a].GID = alt[0]

	return a + 1
}

// encodeLen implements the [Subtable] interface.
func (l *Gsub3_1) encodeLen() int {
	total := 6 + 2*len(l.Alternates)
	for _, repl := range l.Alternates {
		total += 2 + 2*len(repl)
	}
	total += l.Cov.EncodeLen()
	return total
}

// encode implements the [Subtable] interface.
func (l *Gsub3_1) encode() []byte {
	alternateSetCount := len(l.Alternates)
	covOffs := 6 + 2*alternateSetCount

	alternateSetOffsets := make([]uint16, alternateSetCount)
	for i, repl := range l.Alternates {
		alternateSetOffsets[i] = uint16(covOffs)
		covOffs += 2 + 2*len(repl)
	}

	buf := make([]byte, covOffs+l.Cov.EncodeLen())
	// buf[0] = 0
	buf[1] = 1
	buf[2] = byte(covOffs >> 8)
	buf[3] = byte(covOffs)
	buf[4] = byte(len(l.Alternates) >> 8)
	buf[5] = byte(len(l.Alternates))
	pos := 6
	for i := range l.Alternates {
		buf[pos] = byte(alternateSetOffsets[i] >> 8)
		buf[pos+1] = byte(alternateSetOffsets[i])
		pos += 2
	}
	for _, alt := range l.Alternates {
		buf[pos] = byte(len(alt) >> 8)
		buf[pos+1] = byte(len(alt))
		pos += 2
		for _, gid := range alt {
			buf[pos] = byte(gid >> 8)
			buf[pos+1] = byte(gid)
			pos += 2
		}
	}
	copy(buf[covOffs:], l.Cov.Encode())
	return buf
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

	cov, err := coverage.Read(p, subtablePos+int64(coverageOffset))
	if err != nil {
		return nil, err
	}

	if len(cov) > len(ligatureSetOffsets) {
		cov.Prune(len(ligatureSetOffsets))
	} else {
		ligatureSetOffsets = ligatureSetOffsets[:len(cov)]
	}

	repl := make([][]Ligature, len(ligatureSetOffsets))
	for i, ligatureSetOffset := range ligatureSetOffsets {
		ligatureSetPos := subtablePos + int64(ligatureSetOffset)
		err := p.SeekPos(ligatureSetPos)
		if err != nil {
			return nil, err
		}
		ligatureOffsets, err := p.ReadUint16Slice()
		if err != nil {
			return nil, err
		}

		repl[i] = make([]Ligature, len(ligatureOffsets))
		for j, ligatureOffset := range ligatureOffsets {
			err = p.SeekPos(ligatureSetPos + int64(ligatureOffset))
			if err != nil {
				return nil, err
			}
			ligatureGlyph, err := p.ReadUint16()
			if err != nil {
				return nil, err
			}
			componentCount, err := p.ReadUint16()
			if err != nil {
				return nil, err
			}
			componentGlyphIDs := make([]glyph.ID, componentCount-1)
			for k := range componentGlyphIDs {
				gid, err := p.ReadUint16()
				if err != nil {
					return nil, err
				}
				componentGlyphIDs[k] = glyph.ID(gid)
			}

			repl[i][j].In = componentGlyphIDs
			repl[i][j].Out = glyph.ID(ligatureGlyph)
		}
	}

	total := 6 + 2*len(repl)
	for _, replI := range repl {
		total += 2 + 2*len(replI)
		for _, lig := range replI {
			total += 4 + 2*len(lig.In)
		}
	}
	// Now total is the coverage offset when encoding the subtable without
	// overlapping data.
	if total > 0xFFFF {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/opentype/gtab",
			Reason:    "GSUB 4.1 too large",
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
	var text []rune
ligLoop:
	for j := range ligSet {
		lig := &ligSet[j]

		matchPos = matchPos[:0]
		skipPos = matchPos[:0]
		text = text[:0]

		matchPos = append(matchPos, a)
		text = append(text, seq[a].Text...)
		p := a + 1
		for _, ligGid := range lig.In {
			for p < b && !keep.Keep(seq[p].GID) {
				skipPos = append(skipPos, p)
				p++
			}

			if p >= b || seq[p].GID != ligGid { // no match
				continue ligLoop
			}

			matchPos = append(matchPos, p)
			text = append(text, seq[p].Text...)
			p++
		}

		// Insert the ligature glyph.
		seq[a] = glyph.Info{GID: lig.Out, Text: text}

		// Move all the skipped glyphs after position a.
		for i, skip := range skipPos {
			seq[a+i+1] = seq[skip]
		}

		// Move the tail to the correct position.
		ctx.seq = slices.Delete(seq, a+1+len(skipPos), a+len(lig.In)+1+len(skipPos))

		ctx.fixStackMerge(matchPos)

		return a + 1 + len(skipPos)
	}

	return -1
}

// encodeLen implements the [Subtable] interface.
func (l *Gsub4_1) encodeLen() int {
	total := 6 + 2*len(l.Repl)
	for _, repl := range l.Repl {
		total += 2 + 2*len(repl)
		for _, lig := range repl {
			total += 4 + 2*len(lig.In)
		}
	}
	total += l.Cov.EncodeLen()
	return total
}

// encode implements the [Subtable] interface.
func (l *Gsub4_1) encode() []byte {
	ligatureSetCount := len(l.Repl)
	total := 6 + 2*ligatureSetCount
	ligatureSetOffsets := make([]uint16, ligatureSetCount)
	for i, repl := range l.Repl {
		ligatureSetOffsets[i] = uint16(total)
		total += 2 + 2*len(repl)
		for _, lig := range repl {
			total += 4 + 2*len(lig.In)
		}
	}
	coverageOffset := total
	total += l.Cov.EncodeLen()
	if coverageOffset > 0xFFFF {
		panic("coverage offset overflow")
	}

	buf := make([]byte, 0, total)

	buf = append(buf,
		0, 1, // version
		byte(coverageOffset>>8), byte(coverageOffset),
		byte(ligatureSetCount>>8), byte(ligatureSetCount),
	)
	for _, offs := range ligatureSetOffsets {
		buf = append(buf, byte(offs>>8), byte(offs))
	}
	for _, repl := range l.Repl {
		ligatureCount := len(repl)
		buf = append(buf, byte(ligatureCount>>8), byte(ligatureCount))
		pos := 2 + 2*ligatureCount
		for _, lig := range repl {
			buf = append(buf, byte(pos>>8), byte(pos))
			pos += 4 + 2*len(lig.In)
		}
		for _, lig := range repl {
			componentCount := len(lig.In) + 1
			buf = append(buf,
				byte(lig.Out>>8), byte(lig.Out),
				byte(componentCount>>8), byte(componentCount),
			)
			for _, gid := range lig.In {
				buf = append(buf, byte(gid>>8), byte(gid))
			}
		}
	}
	buf = append(buf, l.Cov.Encode()...)

	return buf
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

	input, err := coverage.Read(p, subtablePos+int64(coverageOffset))
	if err != nil {
		return nil, err
	}

	backtrack := make([]coverage.Table, len(backtrackCoverageOffsets))
	for i, offs := range backtrackCoverageOffsets {
		backtrack[i], err = coverage.Read(p, subtablePos+int64(offs))
		if err != nil {
			return nil, err
		}
	}
	lookahead := make([]coverage.Table, len(lookaheadCoverageOffsets))
	for i, offs := range lookaheadCoverageOffsets {
		lookahead[i], err = coverage.Read(p, subtablePos+int64(offs))
		if err != nil {
			return nil, err
		}
	}

	if len(input) > len(substituteGlyphIDs) {
		input.Prune(len(substituteGlyphIDs))
	} else {
		substituteGlyphIDs = substituteGlyphIDs[:len(input)]
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

// encodeLen implements the [Subtable] interface.
func (l *Gsub8_1) encodeLen() int {
	total := 10
	total += 2 * len(l.Backtrack)
	total += 2 * len(l.Lookahead)
	total += 2 * len(l.SubstituteGlyphIDs)
	total += l.Input.EncodeLen()
	for _, cov := range l.Backtrack {
		total += cov.EncodeLen()
	}
	for _, cov := range l.Lookahead {
		total += cov.EncodeLen()
	}
	return total
}

// encode implements the [Subtable] interface.
func (l *Gsub8_1) encode() []byte {
	backtrackGlyphCount := len(l.Backtrack)
	lookaheadGlyphCount := len(l.Lookahead)
	glyphCount := len(l.SubstituteGlyphIDs)

	total := 10
	total += 2 * backtrackGlyphCount
	total += 2 * lookaheadGlyphCount
	total += 2 * glyphCount
	coverageOffset := total
	total += l.Input.EncodeLen()
	backtrackCoverageOffsets := make([]uint16, backtrackGlyphCount)
	for i, cov := range l.Backtrack {
		backtrackCoverageOffsets[i] = uint16(total)
		total += cov.EncodeLen()
	}
	lookaheadCoverageOffsets := make([]uint16, lookaheadGlyphCount)
	for i, cov := range l.Lookahead {
		lookaheadCoverageOffsets[i] = uint16(total)
		total += cov.EncodeLen()
	}

	buf := make([]byte, 0, total)
	buf = append(buf,
		0, 1, // format
		byte(coverageOffset>>8), byte(coverageOffset),
		byte(backtrackGlyphCount>>8), byte(backtrackGlyphCount),
	)
	for _, offset := range backtrackCoverageOffsets {
		buf = append(buf, byte(offset>>8), byte(offset))
	}
	buf = append(buf, byte(lookaheadGlyphCount>>8), byte(lookaheadGlyphCount))
	for _, offset := range lookaheadCoverageOffsets {
		buf = append(buf, byte(offset>>8), byte(offset))
	}
	buf = append(buf, byte(glyphCount>>8), byte(glyphCount))
	for _, gid := range l.SubstituteGlyphIDs {
		buf = append(buf, byte(gid>>8), byte(gid))
	}

	buf = append(buf, l.Input.Encode()...)
	for _, cov := range l.Backtrack {
		buf = append(buf, cov.Encode()...)
	}
	for _, cov := range l.Lookahead {
		buf = append(buf, cov.Encode()...)
	}

	return buf
}
