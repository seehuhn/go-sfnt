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
	"sort"

	"golang.org/x/text/language"
	"seehuhn.de/go/sfnt/parser"
)

// LookupIndex enumerates lookups.
// It is used as an index into a [LookupList].
type LookupIndex uint16

// LookupList contains the information from an OpenType "Lookup List Table".
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#lookup-list-table
type LookupList []*LookupTable

// LookupTable represents a lookup table inside a "GSUB" or "GPOS" table of a
// font.
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#lookup-table
type LookupTable struct {
	Meta *LookupMetaInfo

	// Subtables contains subtables to try for this lookup.  The subtables
	// are tried in order, until one of them can be applied.
	//
	// The type of the subtables must match Meta.LookupType, but the
	// subtables may use any format within that type.
	Subtables []Subtable
}

// LookupMetaInfo contains information associated with a [LookupTable].
// Only information which is not specific to a particular subtable is
// included here.
type LookupMetaInfo struct {
	// LookupType identifies the type of the lookups inside a lookup table.
	// Different numbering schemes are used for GSUB and GPOS tables.
	LookupType uint16

	// LookupFlags contains flags which modify application of the lookup to a
	// glyph string.
	LookupFlags LookupFlags

	// An index into the MarkGlyphSets slice in the corresponding GDEF struct.
	// This is only used, if the MarkFilteringSet flag is set.  In this case,
	// all marks not present in the specified mark glyph set are skipped.
	MarkFilteringSet uint16
}

// LookupFlags contains bits which modify application of a lookup to a glyph string.
//
// LookupFlags can specify glyphs to be ignored in a variety of ways:
//   - all base glyphs
//   - all ligature glyphs
//   - all mark glyphs
//   - a subset of mark glyphs, specified by a mark filtering set
//   - a subset of mark glyphs, specified by a mark attachment type
//
// When this is used, the lookup is applied as if the ignored glyphs
// were not present in the input sequence.
//
// There is also a flag value to control the behaviour of GPOS lookup type
// 3 (cursive attachment).
//
// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#lookupFlags
type LookupFlags uint16

// Bit values for LookupFlag.
const (
	// RightToLeft indicates that for GPOS lookup type 3 (cursive
	// attachment), the last glyph in the sequence (rather than the first) will
	// be positioned on the baseline.
	RightToLeft LookupFlags = 0x0001

	// IgnoreBaseGlyphs indicates that the lookup ignores glyphs which
	// are classified as base glyphs in the GDEF table.
	IgnoreBaseGlyphs LookupFlags = 0x0002

	// IgnoreLigatures indicates that the lookup ignores glyphs which
	// are classified as ligatures in the GDEF table.
	IgnoreLigatures LookupFlags = 0x0004

	// IgnoreMarks indicates that the lookup ignores glyphs which are
	// classified as marks in the GDEF table.
	IgnoreMarks LookupFlags = 0x0008

	// UseMarkFilteringSet indicates that the lookup ignores all
	// glyphs classified as marks in the GDEF table, except for those
	// in the specified mark filtering set.
	UseMarkFilteringSet LookupFlags = 0x0010

	// MarkAttachTypeMask, if not zero, skips over all marks that are not
	// of the specified type.  Mark attachment classes must be defined in the
	// MarkAttachClass Table in the GDEF table.
	MarkAttachTypeMask LookupFlags = 0xFF00
)

// Subtable represents a subtable of a "GSUB" or "GPOS" lookup table.
//
// The following types are GSUB subtables:
//
//   - [*Gsub1_1]
//   - [*Gsub1_2]
//   - [*Gsub2_1]
//   - [*Gsub3_1]
//   - [*Gsub4_1]
//   - [*Gsub8_1]
//
// The following types are GPOS subtables:
//   - [*Gpos1_1]
//   - [*Gpos1_2]
//   - [*Gpos2_1]
//   - [*Gpos2_2]
//   - [*Gpos3_1]
//   - [*Gpos4_1]
//   - [*Gpos5_1]
//   - [*Gpos6_1]
//
// The following types can be used both in GSUB and GPOS tables:
//
//   - [*SeqContext1]
//   - [*SeqContext2]
//   - [*SeqContext3]
//   - [*ChainedSeqContext1]
//   - [*ChainedSeqContext2]
//   - [*ChainedSeqContext3]
type Subtable interface {
	// apply attempts to apply the subtable at position a.  The function
	// returns the new position.  If the subtable cannot be applied, a negative
	// position is returned.  Matching the input sequence is restricted to
	// positions a to b-1.
	//
	// The function ctx.Keep represents the lookup flags, glyphs for which
	// keep(seq[i].Gid) is false must be ignored. The caller already checks the
	// glyph at location a, so only subsequent glyphs need to be tested by the
	// Subtable implementation.
	//
	// The method can assume that ctx.Keep(seq[a].Gid) is true.  It is the
	// callers responsibility to ensure this.
	apply(ctx *Context, a, b int) int

	encodeLen() int

	encode() []byte
}

// subtableReader is a function that can decode a subtable.
// Different functions are required for "GSUB" and "GPOS" tables.
type subtableReader func(*parser.Parser, int64, *LookupMetaInfo) (Subtable, error)

func readLookupList(p *parser.Parser, pos int64, sr subtableReader) (LookupList, error) {
	err := p.SeekPos(pos)
	if err != nil {
		return nil, err
	}

	lookupOffsets, err := p.ReadUint16Slice()
	if err != nil {
		return nil, err
	}

	res := make(LookupList, len(lookupOffsets))

	numLookups := 0
	numSubTables := 0

	var subtableOffsets []uint16
	for i, offs := range lookupOffsets {
		lookupTablePos := pos + int64(offs)
		err := p.SeekPos(lookupTablePos)
		if err != nil {
			return nil, err
		}
		buf, err := p.ReadBytes(6)
		if err != nil {
			return nil, err
		}
		lookupType := uint16(buf[0])<<8 | uint16(buf[1])
		lookupFlag := LookupFlags(buf[2])<<8 | LookupFlags(buf[3])
		subTableCount := uint16(buf[4])<<8 | uint16(buf[5])
		numLookups++
		numSubTables += int(subTableCount)
		if numLookups+numSubTables > 6000 {
			// The condition ensures that we can always store the lookup
			// data (using extension subtables if necessary), without
			// exceeding the maximum offset size in the lookup list table.
			return nil, &parser.InvalidFontError{
				SubSystem: "sfnt/opentype/gtab",
				Reason:    "too many lookup (sub-)tables",
			}
		}
		subtableOffsets = subtableOffsets[:0]
		for j := 0; j < int(subTableCount); j++ {
			subtableOffset, err := p.ReadUint16()
			if err != nil {
				return nil, err
			}
			subtableOffsets = append(subtableOffsets, subtableOffset)
		}
		var markFilteringSet uint16
		if lookupFlag&UseMarkFilteringSet != 0 {
			markFilteringSet, err = p.ReadUint16()
			if err != nil {
				return nil, err
			}
		}

		meta := &LookupMetaInfo{
			LookupType:       lookupType,
			LookupFlags:      lookupFlag,
			MarkFilteringSet: markFilteringSet,
		}

		subtables := make([]Subtable, subTableCount)
		for j, subtableOffset := range subtableOffsets {
			subtable, err := sr(p, lookupTablePos+int64(subtableOffset), meta)
			if err != nil {
				return nil, err
			}
			subtables[j] = subtable
		}

		if tp, ok := isExtension(subtables); ok {
			if tp == meta.LookupType {
				return nil, &parser.InvalidFontError{
					SubSystem: "sfnt/opentype/gtab",
					Reason:    "invalid extension subtable",
				}
			}
			meta.LookupType = tp
			for j, subtable := range subtables {
				l, ok := subtable.(*extensionSubtable)
				if !ok || l.ExtensionLookupType != tp {
					return nil, &parser.InvalidFontError{
						SubSystem: "sfnt/opentype/gtab",
						Reason:    "inconsistent extension subtables",
					}
				}
				pos := lookupTablePos + int64(subtableOffsets[j]) + l.ExtensionOffset
				subtable, err := sr(p, pos, meta)
				if err != nil {
					return nil, err
				}
				subtables[j] = subtable
			}
		}

		res[i] = &LookupTable{
			Meta:      meta,
			Subtables: subtables,
		}
	}
	return res, nil
}

func isExtension(ss []Subtable) (uint16, bool) {
	if len(ss) == 0 {
		return 0, false
	}
	l, ok := ss[0].(*extensionSubtable)
	if !ok {
		return 0, false
	}
	return l.ExtensionLookupType, true
}

type chunkCode uint32

const (
	chunkHeader chunkCode = iota << 28
	chunkTable
	chunkSubtable
	chunkExtReplace

	chunkTypeMask     chunkCode = 0b1111_00000000000000_00000000000000
	chunkTableMask    chunkCode = 0b0000_11111111111111_00000000000000
	chunkSubtableMask chunkCode = 0b0000_00000000000000_11111111111111
)

// Lookup types for extension lookup records.
const (
	gposExtensionLookupType uint16 = 9
	gsubExtensionLookupType uint16 = 7
)

func (ll LookupList) encode() []byte {
	if ll == nil {
		return nil
	}

	var extLookupType uint16
	for _, l := range ll {
		for _, subtable := range l.Subtables {
			switch subtable.(type) {
			case *Gsub1_1, *Gsub1_2, *Gsub2_1, *Gsub3_1, *Gsub4_1, *Gsub8_1:
				extLookupType = gsubExtensionLookupType
				break
			case *Gpos1_1, *Gpos1_2, *Gpos2_1, *Gpos2_2, *Gpos3_1, *Gpos4_1, *Gpos5_1, *Gpos6_1:
				extLookupType = gposExtensionLookupType
				break
			}
		}
	}

	lookupCount := len(ll)
	if lookupCount >= 1<<14 {
		panic("too many lookup tables")
	}

	// Make a list of all chunks which need to be written.
	var chunks []layoutChunk
	chunks = append(chunks, layoutChunk{
		code: chunkHeader,
		size: 2 + 2*uint32(lookupCount),
	})
	for i, l := range ll {
		lookupHeaderLen := 6 + 2*len(l.Subtables)
		if l.Meta.LookupFlags&UseMarkFilteringSet != 0 {
			lookupHeaderLen += 2
		}
		tCode := chunkCode(i) << 14
		chunks = append(chunks, layoutChunk{
			code: chunkTable | tCode,
			size: uint32(lookupHeaderLen),
		})
		if len(l.Subtables) >= 1<<14 {
			panic("too many subtables")
		}
		for j, subtable := range l.Subtables {
			sCode := chunkCode(j)
			chunks = append(chunks, layoutChunk{
				code: chunkSubtable | tCode | sCode,
				size: uint32(subtable.encodeLen()),
			})
		}
	}

	// If needed, reorder the chunks or introduce extension records.
	isTooLarge := false
	var total uint32
	for i := range chunks {
		code := chunks[i].code
		if code&chunkTypeMask == chunkTable && total > 0xFFFF {
			isTooLarge = true
			break
		}
		total += chunks[i].size
	}
	if isTooLarge {
		chunks = ll.tryReorder(chunks)
	}

	// Layout the chunks.
	chunkPos := make(map[chunkCode]uint32, len(chunks))
	total = 0
	for i := range chunks {
		code := chunks[i].code
		chunkPos[code] = total
		total += chunks[i].size
	}

	// Construct the lookup table in memory.
	buf := make([]byte, 0, total)
	for k := range chunks {
		code := chunks[k].code
		switch code & chunkTypeMask {
		case chunkHeader:
			buf = append(buf, byte(lookupCount>>8), byte(lookupCount))
			for i := range ll {
				tCode := chunkCode(i) << 14
				lookupOffset := chunkPos[chunkTable|tCode]
				buf = append(buf, byte(lookupOffset>>8), byte(lookupOffset))
			}
		case chunkTable:
			tCode := code & chunkTableMask
			i := int(tCode) >> 14
			li := ll[i]
			subTableCount := len(li.Subtables)
			lookupType := li.Meta.LookupType
			if _, replaced := chunkPos[chunkExtReplace|tCode]; replaced {
				// fix the lookup type in case of replaced subtables
				lookupType = extLookupType
			}
			buf = append(buf,
				byte(lookupType>>8), byte(lookupType),
				byte(li.Meta.LookupFlags>>8), byte(li.Meta.LookupFlags),
				byte(subTableCount>>8), byte(subTableCount),
			)
			base := chunkPos[code]
			for j := range li.Subtables {
				sCode := chunkCode(j)
				subtablePos, replaced := chunkPos[chunkExtReplace|tCode|sCode]
				if !replaced {
					subtablePos = chunkPos[chunkSubtable|tCode|sCode]
				}
				subtableOffset := subtablePos - base
				buf = append(buf, byte(subtableOffset>>8), byte(subtableOffset))
			}
			if li.Meta.LookupFlags&UseMarkFilteringSet != 0 {
				buf = append(buf,
					byte(li.Meta.MarkFilteringSet>>8), byte(li.Meta.MarkFilteringSet),
				)
			}
		case chunkExtReplace:
			tCode := code & chunkTableMask
			sCode := code & chunkSubtableMask
			lookup := ll[tCode>>14]
			pos := chunkPos[code]
			extPos := chunkPos[chunkSubtable|tCode|sCode]
			subtable := &extensionSubtable{
				ExtensionLookupType: lookup.Meta.LookupType,
				ExtensionOffset:     int64(extPos - pos),
			}
			buf = append(buf, subtable.encode()...)
		case chunkSubtable:
			i := code & chunkTableMask >> 14
			j := code & chunkSubtableMask
			subtable := ll[i].Subtables[j]
			buf = append(buf, subtable.encode()...)
		}
	}
	return buf
}

type layoutChunk struct {
	code chunkCode
	size uint32
}

func (ll LookupList) tryReorder(chunks []layoutChunk) []layoutChunk {
	total := uint32(0)
	for i := range chunks {
		total += chunks[i].size
	}

	lookupSize := make(map[chunkCode]uint32)
	var lookups []chunkCode
	for i := range chunks {
		code := chunks[i].code
		tp := code & chunkTypeMask
		tCode := code & chunkTableMask
		if tp == chunkHeader {
			continue
		} else if tp == chunkTable {
			lookups = append(lookups, tCode)
		}
		lookupSize[tCode] += chunks[i].size
	}
	sort.SliceStable(lookups, func(i, j int) bool {
		return lookupSize[lookups[i]] < lookupSize[lookups[j]]
	})

	// Move the largest table to the end and introduce extension subtables
	// as needed.
	biggestLookup := lookups[len(lookups)-1]
	lastPos := total - lookupSize[biggestLookup]
	idx := len(lookups) - 2
	replace := make(map[chunkCode]bool)
	extra := 0
	for lastPos > 0xFFFF && idx >= 0 {
		tCode := lookups[idx]

		oldSize := lookupSize[tCode]
		l := ll[tCode>>14]
		lookupHeaderLen := 6 + 2*len(l.Subtables)
		if l.Meta.LookupFlags&UseMarkFilteringSet != 0 {
			lookupHeaderLen += 2
		}
		newSize := uint32(lookupHeaderLen) + 8*uint32(len(l.Subtables))

		if newSize < oldSize {
			replace[tCode] = true
			extra += len(l.Subtables)
			lastPos -= oldSize - newSize
		}

		idx--
	}
	if lastPos > 0xFFFF {
		panic("too much data for lookup list table")
	}

	res := make([]layoutChunk, 0, len(chunks)+extra)
	var moved, ext []layoutChunk
	for _, chunk := range chunks {
		code := chunk.code
		tp := code & chunkTypeMask
		tCode := code & chunkTableMask
		switch {
		case tp == chunkHeader:
			res = append(res, chunk)
		case tCode == biggestLookup:
			moved = append(moved, chunk)
		case replace[tCode]:
			sCode := code & chunkSubtableMask
			if tp == chunkSubtable {
				res = append(res, layoutChunk{
					code: chunkExtReplace | tCode | sCode,
					size: 8,
				})
				ext = append(ext, chunk)
			} else {
				res = append(res, chunk)
			}
		default:
			res = append(res, chunk)
		}
	}
	res = append(res, moved...)
	res = append(res, ext...)

	return res
}

// Extension Substitution/Positioning Subtable Format 1
// https://docs.microsoft.com/en-us/typography/opentype/spec/gsub#71-extension-substitution-subtable-format-1
// https://docs.microsoft.com/en-us/typography/opentype/spec/gpos#lookuptype-9-extension-positioning
type extensionSubtable struct {
	ExtensionLookupType uint16
	ExtensionOffset     int64
}

func readExtensionSubtable(p *parser.Parser, _ int64) (Subtable, error) {
	buf, err := p.ReadBytes(6)
	if err != nil {
		return nil, err
	}
	extensionLookupType := uint16(buf[0])<<8 | uint16(buf[1])
	extensionOffset := int64(buf[2])<<24 | int64(buf[3])<<16 | int64(buf[4])<<8 | int64(buf[5])
	res := &extensionSubtable{
		ExtensionLookupType: extensionLookupType,
		ExtensionOffset:     extensionOffset,
	}
	return res, nil
}

func (l *extensionSubtable) apply(*Context, int, int) int {
	panic("unreachable")
}

func (l *extensionSubtable) encodeLen() int {
	return 8
}

func (l *extensionSubtable) encode() []byte {
	return []byte{
		0, 1, // format
		byte(l.ExtensionLookupType >> 8), byte(l.ExtensionLookupType),
		byte(l.ExtensionOffset >> 24), byte(l.ExtensionOffset >> 16), byte(l.ExtensionOffset >> 8), byte(l.ExtensionOffset),
	}
}

// FindLookups returns the lookups required to implement the given
// features in the specified language.
func (info *Info) FindLookups(lang language.Tag, includeFeature map[string]bool) []LookupIndex {
	if info == nil || len(info.ScriptList) == 0 {
		return nil
	}

	tags := make([]language.Tag, 0, len(info.ScriptList))
	for tag := range info.ScriptList {
		tags = append(tags, tag)
	}
	// TODO(voss): make sure a sensible default comes first.
	//     Maybe this could be based on the number of features supported?

	matcher := language.NewMatcher(tags)
	_, index, _ := matcher.Match(lang)

	features := info.ScriptList[tags[index]]
	if features == nil {
		return nil
	}

	includeLookup := make(map[LookupIndex]bool)
	numFeatures := FeatureIndex(len(info.FeatureList))
	if features.Required < numFeatures {
		feature := info.FeatureList[features.Required]
		for _, l := range feature.Lookups {
			includeLookup[l] = true
		}
	}
	for _, f := range features.Optional {
		if f >= numFeatures {
			continue
		}
		feature := info.FeatureList[f]
		if !includeFeature[feature.Tag] {
			continue
		}
		for _, l := range feature.Lookups {
			includeLookup[l] = true
		}
	}

	numLookups := LookupIndex(len(info.LookupList))
	ll := make([]LookupIndex, 0, len(includeLookup))
	for l := range includeLookup {
		if l >= numLookups {
			continue
		}
		ll = append(ll, l)
	}
	sort.Slice(ll, func(i, j int) bool {
		return ll[i] < ll[j]
	})
	return ll
}
