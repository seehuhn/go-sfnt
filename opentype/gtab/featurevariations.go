// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2026  Jochen Voss <voss@seehuhn.de>
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
	"encoding/binary"
	"errors"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/variation"
)

// FeatureVariationRecord describes a set of conditions together with the
// feature substitutions to apply when the conditions hold.
//
// The records in [Info.Variations] are consulted in order: the first record
// whose conditions all hold (see [FeatureVariationRecord.Matches]) is applied,
// and its substitutions replace the lookup lists of the named features
// wholesale.  Features not named by the winning record keep their default
// lookups, and records after the first match are ignored.
type FeatureVariationRecord struct {
	// Conditions must all hold for the record to apply.  The list is a
	// conjunction; an empty list always holds.
	Conditions []Condition

	// Substitutions lists the feature lookup-list replacements to apply when
	// the record's conditions hold.
	Substitutions []FeatureSubstitution
}

// Condition is a single condition table.  Only format 1 is modeled; condition
// tables of other formats force the whole FeatureVariations table to be kept
// as raw bytes (see [Info.VariationsRaw]), so they never appear here.
type Condition struct {
	// Format is the condition table format.  Modeled conditions have Format 1.
	Format uint16

	// AxisIndex selects the variation axis (format 1).
	AxisIndex uint16

	// Min and Max give the inclusive range of the normalized axis coordinate
	// for which the condition holds (format 1).
	Min, Max variation.F2Dot14
}

// FeatureSubstitution replaces the lookup list of one feature.
type FeatureSubstitution struct {
	// FeatureIndex identifies the feature in [Info.FeatureList] whose lookups
	// are replaced.
	FeatureIndex FeatureIndex

	// Lookups is the replacement list of lookup indices.
	Lookups []LookupIndex
}

// Matches reports whether every condition of the record holds for the given
// normalized (avar-mapped) axis coordinates.  An empty condition list matches.
//
// A format-1 condition holds when Min <= coords[AxisIndex] <= Max (both ends
// inclusive).  A condition whose AxisIndex is out of range for coords, or whose
// format is not modeled, never matches.
func (r *FeatureVariationRecord) Matches(coords []variation.F2Dot14) bool {
	for i := range r.Conditions {
		if !r.Conditions[i].matches(coords) {
			return false
		}
	}
	return true
}

func (c *Condition) matches(coords []variation.F2Dot14) bool {
	if c.Format != 1 {
		return false
	}
	if int(c.AxisIndex) >= len(coords) {
		return false
	}
	v := coords[c.AxisIndex]
	return c.Min <= v && v <= c.Max
}

// errUnknownConditionFormat signals that the whole FeatureVariations table
// must be kept as raw bytes.
var errUnknownConditionFormat = errors.New("gtab: unknown condition format")

// readFeatureVariations reads the FeatureVariations table at pos.  On success
// it returns either a modeled record list or, when any condition uses an
// unmodeled format, the verbatim table bytes in raw (with records nil).  Any
// parse error is returned so the caller can drop the variations permissively.
func readFeatureVariations(p *parser.Parser, pos int64, numFeatures, numLookups int) (records []FeatureVariationRecord, raw []byte, err error) {
	err = p.SeekPos(pos)
	if err != nil {
		return nil, nil, err
	}
	major, err := p.ReadUint16()
	if err != nil {
		return nil, nil, err
	}
	minor, err := p.ReadUint16()
	if err != nil {
		return nil, nil, err
	}
	if major != 1 || minor != 0 {
		return nil, nil, errors.New("gtab: unsupported FeatureVariations version")
	}
	count, err := p.ReadUint32()
	if err != nil {
		return nil, nil, err
	}

	type rawRecord struct{ cs, fts uint32 }
	rawRecords, err := membudget.AllocSlice[rawRecord](p.Budget, int(count))
	if err != nil {
		return nil, nil, err
	}
	for i := range rawRecords {
		rawRecords[i].cs, err = p.ReadUint32()
		if err != nil {
			return nil, nil, err
		}
		rawRecords[i].fts, err = p.ReadUint32()
		if err != nil {
			return nil, nil, err
		}
	}

	result, err := membudget.AllocSlice[FeatureVariationRecord](p.Budget, int(count))
	if err != nil {
		return nil, nil, err
	}
	for i := range rawRecords {
		var rec FeatureVariationRecord
		if rawRecords[i].cs != 0 {
			conds, err := readConditionSet(p, pos+int64(rawRecords[i].cs))
			if errors.Is(err, errUnknownConditionFormat) {
				return readVariationsRaw(p, pos)
			} else if err != nil {
				return nil, nil, err
			}
			rec.Conditions = conds
		}
		if rawRecords[i].fts != 0 {
			subs, err := readFeatureTableSubstitution(p, pos+int64(rawRecords[i].fts), numFeatures, numLookups)
			if err != nil {
				return nil, nil, err
			}
			rec.Substitutions = subs
		}
		result[i] = rec
	}
	return result, nil, nil
}

// readVariationsRaw captures the FeatureVariations table verbatim, from pos to
// the end of the table, for the raw-fallback case.
func readVariationsRaw(p *parser.Parser, pos int64) ([]FeatureVariationRecord, []byte, error) {
	n := p.Size() - pos
	if n <= 0 {
		return nil, nil, errors.New("gtab: empty FeatureVariations table")
	}
	raw, err := membudget.AllocSlice[byte](p.Budget, int(n))
	if err != nil {
		return nil, nil, err
	}
	err = p.SeekPos(pos)
	if err != nil {
		return nil, nil, err
	}
	_, err = p.Read(raw)
	if err != nil {
		return nil, nil, err
	}
	return nil, raw, nil
}

func readConditionSet(p *parser.Parser, pos int64) ([]Condition, error) {
	err := p.SeekPos(pos)
	if err != nil {
		return nil, err
	}
	count, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}
	offsets, err := membudget.AllocSlice[uint32](p.Budget, int(count))
	if err != nil {
		return nil, err
	}
	for i := range offsets {
		offsets[i], err = p.ReadUint32()
		if err != nil {
			return nil, err
		}
	}
	conds, err := membudget.AllocSlice[Condition](p.Budget, int(count))
	if err != nil {
		return nil, err
	}
	for i := range offsets {
		conds[i], err = readCondition(p, pos+int64(offsets[i]))
		if err != nil {
			return nil, err
		}
	}
	return conds, nil
}

func readCondition(p *parser.Parser, pos int64) (Condition, error) {
	err := p.SeekPos(pos)
	if err != nil {
		return Condition{}, err
	}
	format, err := p.ReadUint16()
	if err != nil {
		return Condition{}, err
	}
	if format != 1 {
		return Condition{}, errUnknownConditionFormat
	}
	axisIndex, err := p.ReadUint16()
	if err != nil {
		return Condition{}, err
	}
	minVal, err := variation.ReadF2Dot14(p)
	if err != nil {
		return Condition{}, err
	}
	maxVal, err := variation.ReadF2Dot14(p)
	if err != nil {
		return Condition{}, err
	}
	return Condition{Format: 1, AxisIndex: axisIndex, Min: minVal, Max: maxVal}, nil
}

// readFeatureTableSubstitution reads a FeatureTableSubstitution table.
// Substitutions naming an out-of-range feature or lookup index are dropped
// permissively.
func readFeatureTableSubstitution(p *parser.Parser, pos int64, numFeatures, numLookups int) ([]FeatureSubstitution, error) {
	err := p.SeekPos(pos)
	if err != nil {
		return nil, err
	}
	// major and minor version, ignored
	_, err = p.ReadUint32()
	if err != nil {
		return nil, err
	}
	count, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}
	type rawSub struct {
		feat uint16
		off  uint32
	}
	rawSubs, err := membudget.AllocSlice[rawSub](p.Budget, int(count))
	if err != nil {
		return nil, err
	}
	for i := range rawSubs {
		rawSubs[i].feat, err = p.ReadUint16()
		if err != nil {
			return nil, err
		}
		rawSubs[i].off, err = p.ReadUint32()
		if err != nil {
			return nil, err
		}
	}
	result, err := membudget.AllocSlice[FeatureSubstitution](p.Budget, int(count))
	if err != nil {
		return nil, err
	}
	k := 0
	for i := range rawSubs {
		if rawSubs[i].off == 0 || int(rawSubs[i].feat) >= numFeatures {
			continue
		}
		lookups, err := readAlternateFeature(p, pos+int64(rawSubs[i].off))
		if err != nil {
			return nil, err
		}
		if !lookupsInRange(lookups, numLookups) {
			continue
		}
		result[k] = FeatureSubstitution{
			FeatureIndex: FeatureIndex(rawSubs[i].feat),
			Lookups:      lookups,
		}
		k++
	}
	if k == 0 {
		return nil, nil
	}
	return result[:k], nil
}

func lookupsInRange(lookups []LookupIndex, numLookups int) bool {
	for _, li := range lookups {
		if int(li) >= numLookups {
			return false
		}
	}
	return true
}

// readAlternateFeature reads the alternate Feature table of a feature
// substitution.  Its featureParams offset is ignored.
func readAlternateFeature(p *parser.Parser, pos int64) ([]LookupIndex, error) {
	err := p.SeekPos(pos)
	if err != nil {
		return nil, err
	}
	// featureParams offset, ignored
	_, err = p.ReadUint16()
	if err != nil {
		return nil, err
	}
	count, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}
	lookups, err := membudget.AllocSlice[LookupIndex](p.Budget, int(count))
	if err != nil {
		return nil, err
	}
	for i := range lookups {
		v, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		lookups[i] = LookupIndex(v)
	}
	return lookups, nil
}

// encodeFeatureVariations serializes a modeled FeatureVariations table.
func encodeFeatureVariations(records []FeatureVariationRecord) []byte {
	n := len(records)
	headerLen := 8 + 8*n // version (4) + count (4) + records (8 each)

	type recOffsets struct{ cs, fts uint32 }
	offs := make([]recOffsets, n)
	var body []byte
	for i := range records {
		r := &records[i]
		if len(r.Conditions) > 0 {
			offs[i].cs = uint32(headerLen + len(body))
			body = append(body, encodeConditionSet(r.Conditions)...)
		}
		if len(r.Substitutions) > 0 {
			offs[i].fts = uint32(headerLen + len(body))
			body = append(body, encodeFeatureTableSubstitution(r.Substitutions)...)
		}
	}

	buf := make([]byte, 0, headerLen+len(body))
	buf = append(buf, 0, 1, 0, 0) // major version 1, minor version 0
	buf = binary.BigEndian.AppendUint32(buf, uint32(n))
	for i := range records {
		buf = binary.BigEndian.AppendUint32(buf, offs[i].cs)
		buf = binary.BigEndian.AppendUint32(buf, offs[i].fts)
	}
	buf = append(buf, body...)
	return buf
}

func encodeConditionSet(conds []Condition) []byte {
	n := len(conds)
	headerLen := 2 + 4*n // count (2) + offsets (4 each)

	offs := make([]uint32, n)
	var body []byte
	for i := range conds {
		offs[i] = uint32(headerLen + len(body))
		body = append(body, encodeCondition(&conds[i])...)
	}

	buf := make([]byte, 0, headerLen+len(body))
	buf = binary.BigEndian.AppendUint16(buf, uint16(n))
	for i := range conds {
		buf = binary.BigEndian.AppendUint32(buf, offs[i])
	}
	buf = append(buf, body...)
	return buf
}

func encodeCondition(c *Condition) []byte {
	buf := make([]byte, 0, 8)
	buf = binary.BigEndian.AppendUint16(buf, 1) // format 1
	buf = binary.BigEndian.AppendUint16(buf, c.AxisIndex)
	buf = binary.BigEndian.AppendUint16(buf, uint16(c.Min))
	buf = binary.BigEndian.AppendUint16(buf, uint16(c.Max))
	return buf
}

func encodeFeatureTableSubstitution(subs []FeatureSubstitution) []byte {
	n := len(subs)
	headerLen := 6 + 6*n // version (4) + count (2) + records (6 each)

	offs := make([]uint32, n)
	var body []byte
	for i := range subs {
		offs[i] = uint32(headerLen + len(body))
		body = append(body, encodeAlternateFeature(subs[i].Lookups)...)
	}

	buf := make([]byte, 0, headerLen+len(body))
	buf = append(buf, 0, 1, 0, 0) // major version 1, minor version 0
	buf = binary.BigEndian.AppendUint16(buf, uint16(n))
	for i := range subs {
		buf = binary.BigEndian.AppendUint16(buf, uint16(subs[i].FeatureIndex))
		buf = binary.BigEndian.AppendUint32(buf, offs[i])
	}
	buf = append(buf, body...)
	return buf
}

func encodeAlternateFeature(lookups []LookupIndex) []byte {
	buf := make([]byte, 0, 4+2*len(lookups))
	buf = binary.BigEndian.AppendUint16(buf, 0) // featureParams offset
	buf = binary.BigEndian.AppendUint16(buf, uint16(len(lookups)))
	for _, li := range lookups {
		buf = binary.BigEndian.AppendUint16(buf, uint16(li))
	}
	return buf
}
