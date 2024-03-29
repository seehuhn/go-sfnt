// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2021  Jochen Voss <voss@seehuhn.de>
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

// Package header reads and writes TrueType and OpenType file headers.
// https://docs.microsoft.com/en-us/typography/opentype/spec/otff#table-directory
package header

import (
	"fmt"
	"io"
	"sort"

	"seehuhn.de/go/sfnt/parser"
)

const (
	// ScalerTypeTrueType is the scaler type for fonts which use TrueType
	// outlines.
	ScalerTypeTrueType uint32 = 0x00010000

	// ScalerTypeCFF is the scaler type for fonts which use CFF
	// outlines (version 1 or 2).
	ScalerTypeCFF uint32 = 0x4F54544F // "OTTO"

	// ScalerTypeApple is recognised as an alternative for ScalerTypeTrueType
	// on Apple systems.
	ScalerTypeApple uint32 = 0x74727565 // "true"
)

// Info contains information about the tables present in an sfnt font file.
type Info struct {
	ScalerType uint32
	Toc        map[string]Record
}

// A Record contains the offset and length of a table in an sfnt font file.
type Record struct {
	Offset uint32
	Length uint32
}

// Read reads the file header of an sfnt font file.
// All checksum data is ignored, but basic sanity checks for the
// table structure are performed.
func Read(r io.ReaderAt) (*Info, error) {
	var buf [16]byte
	_, err := r.ReadAt(buf[:6], 0)
	if err != nil {
		return nil, err
	}
	scalerType := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
	numTables := int(buf[4])<<8 | int(buf[5])

	if scalerType != ScalerTypeTrueType &&
		scalerType != ScalerTypeCFF &&
		scalerType != ScalerTypeApple {
		return nil, &parser.NotSupportedError{
			SubSystem: "sfnt/header",
			Feature:   fmt.Sprintf("scaler type 0x%08x", scalerType),
		}
	}
	if numTables > 280 {
		// the largest value observed amongst the fonts on my laptop is 28
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/header",
			Reason:    "too many tables",
		}
	}

	h := &Info{
		ScalerType: scalerType,
		Toc:        make(map[string]Record, numTables),
	}
	type alloc struct {
		Start uint32
		End   uint32
	}
	var coverage []alloc
	for i := 0; i < numTables; i++ {
		_, err := r.ReadAt(buf[:16], int64(12+i*16))
		if err != nil {
			return nil, err
		}
		for i := 0; i < 4; i++ {
			if buf[i] < 0x20 || buf[i] > 0x7e {
				return nil, &parser.InvalidFontError{
					SubSystem: "sfnt/header",
					Reason:    "invalid table name",
				}
			}
		}
		name := string(buf[:4])
		offset := uint32(buf[8])<<24 | uint32(buf[9])<<16 | uint32(buf[10])<<8 | uint32(buf[11])
		length := uint32(buf[12])<<24 | uint32(buf[13])<<16 | uint32(buf[14])<<8 | uint32(buf[15])

		if _, exists := h.Toc[name]; exists {
			return nil, &parser.InvalidFontError{
				SubSystem: "sfnt/header",
				Reason:    "duplicate table " + name,
			}
		}
		h.Toc[name] = Record{
			Offset: offset,
			Length: length,
		}
		coverage = append(coverage, alloc{
			Start: offset,
			End:   offset + length,
		})
	}
	if len(h.Toc) == 0 {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/header",
			Reason:    "no tables",
		}
	}

	// perform some sanity checks
	sort.Slice(coverage, func(i, j int) bool {
		if coverage[i].Start != coverage[j].Start {
			return coverage[i].Start < coverage[j].Start
		}
		return coverage[i].End < coverage[j].End
	})
	if coverage[0].Start < 12 {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/header",
			Reason:    "invalid table offset",
		}
	}
	for i := 1; i < len(coverage); i++ {
		if coverage[i-1].End > coverage[i].Start {
			return nil, &parser.InvalidFontError{
				SubSystem: "sfnt/header",
				Reason:    "overlapping tables",
			}
		}
	}
	_, err = r.ReadAt(buf[:1], int64(coverage[len(coverage)-1].End)-1)
	if err == io.EOF {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/header",
			Reason:    "table extends beyond EOF",
		}
	} else if err != nil {
		return nil, err
	}

	return h, nil
}

// Has returns true if all of the given tables are present in the font,
// and none of them are empty.
func (h *Info) Has(tableNames ...string) bool {
	for _, name := range tableNames {
		if rec, ok := h.Toc[name]; !ok || rec.Length == 0 {
			return false
		}
	}
	return true
}

// TableReader returns an io.Reader for the given table.
func (h *Info) TableReader(r io.ReaderAt, tableName string) (*io.SectionReader, error) {
	rec, ok := h.Toc[tableName]
	if !ok {
		return nil, &ErrMissing{TableName: tableName}
	}
	return io.NewSectionReader(r, int64(rec.Offset), int64(rec.Length)), nil
}

// ReadTableBytes returns the un-decoded table contents.
func (h *Info) ReadTableBytes(r io.ReaderAt, tableName string) ([]byte, error) {
	tableFd, err := h.TableReader(r, tableName)
	if err != nil {
		return nil, err
	}
	return io.ReadAll(tableFd)
}
