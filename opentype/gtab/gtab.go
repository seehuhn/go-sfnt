// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"encoding/binary"
	"fmt"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/opentype/internal/offsetpool"
	"seehuhn.de/go/sfnt/parser"
)

// Info contains the information from an OpenType "GSUB" or "GPOS" table.
type Info struct {
	// The ScriptList lists the font features available for each natural
	// language.  Features are given as indices into the FeatureList.
	ScriptList ScriptListInfo

	// The FeatureList enumerates all font features available in the font.
	// Features are implemented by lookups from the LookupList.
	FeatureList FeatureListInfo

	// The LookupList enumerates all the OpenType lookups used to implement
	// the font features.
	LookupList LookupList

	// Variations, when non-nil, holds the GSUB/GPOS FeatureVariations data
	// (a version 1.1 feature).  The records are consulted in order; the first
	// record whose conditions all hold applies its feature substitutions.  See
	// [FeatureVariationRecord].
	//
	// Variations is mutually exclusive with VariationsRaw.
	Variations []FeatureVariationRecord

	// VariationsRaw holds the verbatim FeatureVariations table bytes when the
	// table uses a condition format that is not modeled.  Such tables cannot be
	// interpreted but round-trip byte-for-byte.  When non-nil, Variations is
	// nil.
	VariationsRaw []byte
}

// Type chooses between "GSUB" and "GPOS" tables.
// The possible values are [TypeGsub] and [TypeGpos].
type Type byte

func (tp Type) String() string {
	switch tp {
	case TypeGsub:
		return "GSUB"
	case TypeGpos:
		return "GPOS"
	default:
		return fmt.Sprintf("Type(%d)", tp)
	}

}

// These are the allowed types for use in the [Read] function.
const (
	// TypeGsub is an OpenType "GSUB" table.
	TypeGsub = iota + 1

	// TypeGpos is an OpenType "GPOS" table.
	TypeGpos
)

// Read reads and decodes an OpenType "GSUB" or "GPOS" table from r.
// The tp argument must be one of [TypeGsub] or [TypeGpos].
// Allocations are charged against budget.
//
// The format of the data read is defined here:
//   - https://docs.microsoft.com/en-us/typography/opentype/spec/gsub#gsub-header
//   - https://docs.microsoft.com/en-us/typography/opentype/spec/gpos#gpos-header
func Read(r parser.ReadSeekSizer, budget *membudget.Budget, tp Type) (*Info, error) {
	var sr subtableReader
	switch tp {
	case TypeGsub:
		sr = readGsubSubtable
	case TypeGpos:
		sr = readGposSubtable
	default:
		return nil, fmt.Errorf("unsupported Gtab table type %d", tp)
	}
	return readGtab(r, budget, tp, sr)
}

func readGtab(r parser.ReadSeekSizer, budget *membudget.Budget, tp Type, sr subtableReader) (*Info, error) {
	p := parser.New(r, budget)

	var header struct {
		MajorVersion      uint16
		MinorVersion      uint16
		ScriptListOffset  uint16
		FeatureListOffset uint16
		LookupListOffset  uint16
	}
	var FeatureVariationsOffset uint32

	err := binary.Read(p, binary.BigEndian, &header)
	if err != nil {
		return nil, err
	}
	if header.MajorVersion != 1 || header.MinorVersion > 1 {
		return nil, &parser.NotSupportedError{
			SubSystem: "sfnt/opentype/gtab",
			Feature: fmt.Sprintf("%s table version %d.%d",
				tp, header.MajorVersion, header.MinorVersion),
		}
	}
	endOfHeader := uint32(10)
	if header.MinorVersion == 1 {
		FeatureVariationsOffset, err = p.ReadUint32()
		if err != nil {
			return nil, err
		}
		endOfHeader += 4
	}

	if header.ScriptListOffset == 0 || header.LookupListOffset == 0 {
		return &Info{
			ScriptList: make(ScriptListInfo),
		}, nil
	}

	fileSize := p.Size()
	for _, offset := range []uint32{
		uint32(header.ScriptListOffset),
		uint32(header.FeatureListOffset),
		uint32(header.LookupListOffset),
	} {
		if offset < endOfHeader || int64(offset) >= fileSize {
			return nil, &parser.InvalidFontError{
				SubSystem: "sfnt/opentype/gtab",
				Reason: fmt.Sprintf("%s header has invalid offset %d",
					tp, offset),
			}
		}
	}
	if FeatureVariationsOffset != 0 &&
		(FeatureVariationsOffset < endOfHeader || int64(FeatureVariationsOffset) >= fileSize) {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/opentype/gtab",
			Reason:    fmt.Sprintf("%s header has invalid FeatureVariationsOffset", tp),
		}
	}

	info := &Info{}
	info.ScriptList, err = readScriptList(p, int64(header.ScriptListOffset))
	if err != nil {
		return nil, err
	}
	info.FeatureList, err = readFeatureList(p, int64(header.FeatureListOffset))
	if err != nil {
		return nil, err
	}
	info.LookupList, err = readLookupList(p, int64(header.LookupListOffset), sr)
	if err != nil {
		return nil, err
	}

	if FeatureVariationsOffset != 0 {
		records, raw, err := readFeatureVariations(p, int64(FeatureVariationsOffset),
			len(info.FeatureList), len(info.LookupList))
		if err == nil {
			// on error, drop the variations but keep the rest (permissive)
			info.Variations = records
			info.VariationsRaw = raw
		}
	}

	if !info.headerFits() {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/opentype/gtab",
			Reason:    fmt.Sprintf("%s lists too large to encode", tp),
		}
	}

	return info, nil
}

// headerFits reports whether the script, feature and lookup lists can be
// reached from the header of the encoded table.  The lists must be
// encodable on their own, as checked by the readers.
func (info *Info) headerFits() bool {
	hdr := 10
	if info.Variations != nil || info.VariationsRaw != nil {
		hdr = 14
	}
	sizes := []int{-1, -1, -1}
	if info.ScriptList != nil {
		sizes[0] = mustFitList(info.ScriptList.layout()).encodeLen()
	}
	if info.FeatureList != nil {
		sizes[1] = mustFitList(info.FeatureList.layout()).encodeLen()
	}
	if hdr+max(sizes[0], 0)+max(sizes[1], 0) <= 0xFFFF {
		// the standard order works, whatever the length of the lookup list
		return true
	}
	if info.LookupList != nil {
		sizes[2] = len(info.LookupList.encode())
	}
	_, ok := offsetpool.Place(hdr, sizes)
	return ok
}

// mustFitList returns the layout of a table which holds the data of the
// whole GSUB or GPOS table, such as the script and feature lists, and
// panics if the table cannot be encoded.  Such tables cannot be split.
func mustFitList[T any](lay T, err error) T {
	if err != nil {
		panic("sfnt/gtab: " + err.Error())
	}
	return lay
}

// Encode returns the binary representation of a "GSUB" or "GPOS" table.
func (info *Info) Encode() []byte {
	lists := [3][]byte{
		info.ScriptList.encode(),
		info.FeatureList.encode(),
		info.LookupList.encode(),
	}

	var variations []byte
	if info.VariationsRaw != nil {
		variations = info.VariationsRaw
	} else if info.Variations != nil {
		variations = encodeFeatureVariations(info.Variations)
	}

	// version 1.1 adds a uint32 FeatureVariationsOffset to the header
	hdr := 10
	minorVersion := 0
	if variations != nil {
		hdr = 14
		minorVersion = 1
	}

	sizes := make([]int, len(lists))
	total := hdr
	for i, list := range lists {
		sizes[i] = -1
		if list != nil {
			sizes[i] = len(list)
			total += len(list)
		}
	}
	offs, ok := offsetpool.Place(hdr, sizes)
	if !ok {
		panic("sfnt/gtab: script, feature and lookup lists too large")
	}
	featureVariationsOffset := total
	total += len(variations)

	buf := make([]byte, total)
	copy(buf, []byte{
		0, 1, // major version
		0, byte(minorVersion),
		byte(offs[0] >> 8), byte(offs[0]),
		byte(offs[1] >> 8), byte(offs[1]),
		byte(offs[2] >> 8), byte(offs[2]),
	})
	if variations != nil {
		binary.BigEndian.PutUint32(buf[10:], uint32(featureVariationsOffset))
	}
	for i, list := range lists {
		copy(buf[offs[i]:], list)
	}
	copy(buf[featureVariationsOffset:], variations)

	return buf
}
