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

// Package mvar reads and writes the "MVAR" table, which stores variation
// data for miscellaneous font-wide metrics.
// https://learn.microsoft.com/en-us/typography/opentype/spec/mvar
//
// Of the tags a font may list, this library applies deltas at instancing
// time for: hasc (Ascent), hdsc (Descent), hlgp (LineGap), cpht (CapHeight),
// xhgt (XHeight), undo (UnderlinePosition), and unds (UnderlineThickness).
// All other tags round-trip through [Table] but are not applied, because the
// corresponding table values are regenerated from Font fields on write.
// Italic angle is not variable via MVAR: the registered value tags have no
// 'ital' entry, and slant is instead conveyed by the 'slnt' variation axis.
package mvar

import (
	"errors"
	"math"
	"sort"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/variation"
)

// headerSize is the fixed size of the MVAR header.
const headerSize = 12

// valueRecordSize is the strict-write size of one MVAR value record: a
// 4-byte tag plus two uint16 delta-set indexes.
const valueRecordSize = 8

var (
	errShortTable         = errors.New("mvar: table too short")
	errUnsupportedVersion = errors.New("mvar: unsupported table version")
	errRecordTooSmall     = errors.New("mvar: value record size too small")
	errDuplicateTag       = errors.New("mvar: duplicate tag")
	errTooManyRecords     = errors.New("mvar: too many records")
	errTableTooLarge      = errors.New("mvar: table too large")
	errBadTagLength       = errors.New("mvar: tag must be 4 bytes")
)

// Table represents the contents of an "MVAR" table.
type Table struct {
	// Store holds the shared delta values referenced by Records.  It is nil
	// when the table has no records.
	Store *variation.ItemVariationStore

	// Records lists the metrics with variation data, each naming a tag and
	// its delta-set index.  Encode sorts Records by Tag.
	Records []Record
}

// Record associates a metric tag with a delta-set index into the item
// variation store.
type Record struct {
	Tag                    string // four-character metric tag, e.g. "hasc"
	OuterIndex, InnerIndex uint16
}

// Read reads and decodes an "MVAR" table.  Allocations are charged against
// budget.
//
// The read is permissive: valueRecordSize may exceed the strict 8-byte
// size, in which case the extra bytes of each record are skipped, and the
// input records need not be sorted by tag. Records is always returned
// sorted by tag with duplicate tags collapsed to their first occurrence, so
// that a subsequent [Table.Encode] reproduces the same table.
func Read(r parser.ReadSeekSizer, budget *membudget.Budget) (*Table, error) {
	p := parser.New(r, budget)

	buf, err := p.ReadBytes(headerSize)
	if err != nil {
		return nil, err
	}
	majorVersion := u16(buf, 0)
	recordSize := int(u16(buf, 6))
	recordCount := int(u16(buf, 8))
	storeOffset := u16(buf, 10)

	if majorVersion != 1 {
		return nil, errUnsupportedVersion
	}
	if recordCount > 0 && recordSize < valueRecordSize {
		return nil, errRecordTooSmall
	}

	// bounds check before any allocation
	if int64(recordCount)*int64(recordSize) > p.Size()-int64(headerSize) {
		return nil, errShortTable
	}

	t := &Table{}
	if storeOffset != 0 {
		t.Store, err = variation.ReadItemVariationStore(p, int64(storeOffset))
		if err != nil {
			return nil, err
		}
	}

	if recordCount > 0 {
		t.Records, err = membudget.AllocSlice[Record](budget, recordCount)
		if err != nil {
			return nil, err
		}
	}
	if err := p.SeekPos(int64(headerSize)); err != nil {
		return nil, err
	}
	for i := range t.Records {
		rec, err := p.ReadBytes(recordSize)
		if err != nil {
			return nil, err
		}
		t.Records[i] = Record{
			Tag:        string(rec[0:4]),
			OuterIndex: u16(rec, 4),
			InnerIndex: u16(rec, 6),
		}
	}

	// records need not be sorted in the input; normalize to tag order so
	// that a read-write-read cycle is stable regardless of input order
	sort.SliceStable(t.Records, func(i, j int) bool { return t.Records[i].Tag < t.Records[j].Tag })

	// a malformed table may repeat a tag, which Encode cannot represent;
	// keep only the first (in original file order) occurrence of each tag
	if len(t.Records) > 0 {
		deduped := t.Records[:1]
		for _, rec := range t.Records[1:] {
			if rec.Tag == deduped[len(deduped)-1].Tag {
				continue
			}
			deduped = append(deduped, rec)
		}
		t.Records = deduped
	}

	return t, nil
}

// Encode returns the binary form of the MVAR table. Records are sorted by
// Tag; a table with zero records and no store encodes with a zero store
// offset and a zero record count. Encode returns an error if two records
// share a tag, if a Tag is not 4 bytes long, if there are more than 65535
// records, or if the item variation store would not fit within the 16-bit
// offset field.
func (t *Table) Encode() ([]byte, error) {
	records := append([]Record(nil), t.Records...)
	sort.Slice(records, func(i, j int) bool { return records[i].Tag < records[j].Tag })
	for i := 1; i < len(records); i++ {
		if records[i].Tag == records[i-1].Tag {
			return nil, errDuplicateTag
		}
	}
	for i := range records {
		if len(records[i].Tag) != 4 {
			return nil, errBadTagLength
		}
	}
	if len(records) > math.MaxUint16 {
		return nil, errTooManyRecords
	}

	// the value records immediately follow the header; the item variation
	// store is placed after them and located via its own offset field
	recordsEnd := headerSize + len(records)*valueRecordSize
	var storeBytes []byte
	var storeOffset int
	if t.Store != nil {
		storeBytes = t.Store.Encode()
		storeOffset = recordsEnd
	}
	if storeOffset > math.MaxUint16 {
		return nil, errTableTooLarge
	}

	buf := make([]byte, 0, recordsEnd+len(storeBytes))
	buf = appendU16(buf, 1) // majorVersion
	buf = appendU16(buf, 0) // minorVersion
	buf = appendU16(buf, 0) // reserved
	buf = appendU16(buf, valueRecordSize)
	buf = appendU16(buf, uint16(len(records)))
	buf = appendU16(buf, uint16(storeOffset))
	for _, rec := range records {
		buf = appendTag(buf, rec.Tag)
		buf = appendU16(buf, rec.OuterIndex)
		buf = appendU16(buf, rec.InnerIndex)
	}
	buf = append(buf, storeBytes...)
	return buf, nil
}

// Delta returns the unrounded delta for the metric identified by tag at the
// normalized axis coordinates coords, together with true when the tag is
// present. A tag absent from Records, or a nil Store, yields (0, false).
func (t *Table) Delta(tag string, coords []variation.F2Dot14) (float64, bool) {
	if t.Store == nil {
		return 0, false
	}
	for _, rec := range t.Records {
		if rec.Tag == tag {
			return t.Store.Evaluate(rec.OuterIndex, rec.InnerIndex, coords), true
		}
	}
	return 0, false
}

func u16(b []byte, i int) uint16 {
	return uint16(b[i])<<8 | uint16(b[i+1])
}

func appendU16(buf []byte, v uint16) []byte {
	return append(buf, byte(v>>8), byte(v))
}

// appendTag appends a four-character tag. Callers must ensure tag is
// exactly 4 bytes long.
func appendTag(buf []byte, tag string) []byte {
	return append(buf, tag...)
}
