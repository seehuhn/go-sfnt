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
	"cmp"
	"fmt"
	"slices"

	"seehuhn.de/go/sfnt/opentype/internal/offsetpool"
	"seehuhn.de/go/sfnt/opentype/internal/readcache"
	"seehuhn.de/go/sfnt/parser"
)

// FeatureIndex enumerates features.
// It is used as an index into the FeatureListInfo.
// Valid values are in the range from 0 to 0xFFFE.
// The special value 0xFFFF is used to indicate the absence of required
// features in the `Features` struct.
type FeatureIndex uint16

// FeatureListInfo contains the contents of an OpenType "Feature List" table.
type FeatureListInfo []*Feature

// Feature describes an OpenType font feature.
type Feature struct {
	// Tag describes the function of this feature.
	// https://docs.microsoft.com/en-us/typography/opentype/spec/featuretags
	Tag string

	// Lookups is a list of lookup indices that implement this feature.
	Lookups []LookupIndex

	// Params holds feature-specific parameters, or nil if the feature has
	// none.
	Params FeatureParams
}

func (f Feature) String() string {
	return fmt.Sprintf("%s:%v", f.Tag, f.Lookups)
}

// https://docs.microsoft.com/en-us/typography/opentype/spec/chapter2#feature-list-table
func readFeatureList(p *parser.Parser, pos int64) (FeatureListInfo, error) {
	err := p.SeekPos(pos)
	if err != nil {
		return nil, err
	}

	featureCount, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	type featureRecord struct {
		tag  string
		offs uint16
	}
	var featureList []*featureRecord
	for i := 0; i < int(featureCount); i++ {
		buf, err := p.ReadBytes(6)
		if err != nil {
			return nil, err
		}

		featureList = append(featureList, &featureRecord{
			tag:  string(buf[:4]),
			offs: uint16(buf[4])<<8 | uint16(buf[5]),
		})
	}

	// tables reached through several offsets are read once and shared
	bodies := readcache.New[*featureBody]()
	params := map[string]*readcache.Cache[FeatureParams]{}
	info := make(FeatureListInfo, 0, len(featureList))
	for _, rec := range featureList {
		featurePos := pos + int64(rec.offs)
		body, err := bodies.Get(featurePos, func(pos int64) (*featureBody, error) {
			return readFeatureBody(p, pos)
		})
		if err != nil {
			return nil, err
		}

		feature := &Feature{
			Tag:     rec.tag,
			Lookups: body.lookups,
		}
		if body.paramsOffset != 0 {
			// the offset is measured from the start of the Feature table
			c := params[rec.tag]
			if c == nil {
				c = readcache.New[FeatureParams]()
				params[rec.tag] = c
			}
			feature.Params, err = c.Get(featurePos+int64(body.paramsOffset), func(pos int64) (FeatureParams, error) {
				return readFeatureParams(p, pos, rec.tag)
			})
			if err != nil {
				return nil, err
			}
		}
		info = append(info, feature)
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

// repair removes the parts of a feature list read from a file which cannot
// be written back, and keeps the rest.  Feature parameters are dropped if
// their offset does not fit.  If the Feature tables cannot all be reached
// from the feature list, the lookups of some features are dropped, keeping
// as many Feature tables as possible.  Such features keep their tag, so
// that the feature indices used in the script list remain valid.
//
// The features must not be shared with other feature lists.
func (info FeatureListInfo) repair() {
	for _, f := range info {
		if f.Params != nil && 4+2*len(f.Lookups) > 0xFFFF {
			f.Params = nil
		}
	}
	if _, err := info.layout(); err == nil {
		return
	}

	// Place the distinct Feature tables in order of increasing size, as
	// layout does, and keep those which fit.  The empty Feature table
	// which replaces the others is the smallest, so it is placed first.
	empty := featureKey(&Feature{})
	sizes := map[featureKeyType]int{empty: 4}
	paramSizes := map[FeatureParams]int{}
	for _, f := range info {
		sizes[featureKey(f)] = f.encodeLen(paramSizes)
	}
	keys := make([]featureKeyType, 0, len(sizes))
	for key := range sizes {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b featureKeyType) int {
		return cmp.Compare(sizes[a], sizes[b])
	})
	keep := map[featureKeyType]bool{}
	pos := 2 + 6*len(info)
	for _, key := range keys {
		if pos > 0xFFFF {
			break
		}
		keep[key] = true
		pos += sizes[key]
	}

	for _, f := range info {
		if !keep[featureKey(f)] {
			f.Lookups = nil
			f.Params = nil
		}
	}
}

// featureBody holds the fields of a Feature table which do not depend on the
// feature tag.
type featureBody struct {
	paramsOffset uint16
	lookups      []LookupIndex
}

func readFeatureBody(p *parser.Parser, pos int64) (*featureBody, error) {
	err := p.SeekPos(pos)
	if err != nil {
		return nil, err
	}
	buf, err := p.ReadBytes(4)
	if err != nil {
		return nil, err
	}
	body := &featureBody{
		paramsOffset: uint16(buf[0])<<8 | uint16(buf[1]),
	}
	n := int(buf[2])<<8 | int(buf[3])
	if n > 0 {
		body.lookups, err = p.Budget.AllocSlice[LookupIndex](n)
		if err != nil {
			return nil, err
		}
	}
	for i := range body.lookups {
		idx, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		body.lookups[i] = LookupIndex(idx)
	}
	return body, nil
}

// featureListLayout holds the positions of the tables of a feature list.
type featureListLayout struct {
	offs []uint16
	pool *offsetpool.Pool
}

func (l *featureListLayout) encodeLen() int {
	return 2 + 6*len(l.offs) + l.pool.Len()
}

// featureKeyType identifies the encoding of a Feature table, for use with
// [offsetpool.Pool.AddFunc].  The encoding does not depend on the tag.
type featureKeyType struct {
	lookups any
	params  FeatureParams
}

func featureKey(f *Feature) featureKeyType {
	return featureKeyType{offsetpool.SliceKey(f.Lookups), f.Params}
}

// encodeLen returns the length of the encoded Feature table.  The lengths
// of the feature parameter tables are memoised in paramSizes, since many
// features may share one table.
func (f *Feature) encodeLen(paramSizes map[FeatureParams]int) int {
	n := 4 + 2*len(f.Lookups)
	if f.Params != nil {
		size, ok := paramSizes[f.Params]
		if !ok {
			size = len(f.Params.encode())
			paramSizes[f.Params] = size
		}
		n += size
	}
	return n
}

// layout computes the positions of the Feature tables.  Features with
// identical encodings share one table.  The tables are stored in the order
// of the features if possible, and otherwise in order of increasing size.
// An error is returned if a Feature table cannot be reached through its
// Offset16.
func (info FeatureListInfo) layout() (*featureListLayout, error) {
	for _, f := range info {
		// the feature params table, if any, follows the lookup indices
		if f.Params != nil {
			paramsOffset := 4 + 2*len(f.Lookups)
			if paramsOffset > 0xFFFF {
				return nil, fmt.Errorf("feature params offset %d out of range", paramsOffset)
			}
		}
	}

	order := make([]int, len(info))
	for i := range order {
		order[i] = i
	}
	if lay, err := info.layoutInOrder(order); err == nil {
		return lay, nil
	}
	sizes := make([]int, len(info))
	paramSizes := map[FeatureParams]int{}
	for i, f := range info {
		sizes[i] = f.encodeLen(paramSizes)
	}
	slices.SortStableFunc(order, func(i, j int) int { return cmp.Compare(sizes[i], sizes[j]) })
	return info.layoutInOrder(order)
}

// layoutInOrder stores the Feature tables in the given order.
func (info FeatureListInfo) layoutInOrder(order []int) (*featureListLayout, error) {
	pool := offsetpool.New("feature", 2+6*len(info))
	offs := make([]uint16, len(info))
	for _, i := range order {
		f := info[i]
		var err error
		offs[i], err = pool.AddFunc(featureKey(f), f.encodeBody)
		if err != nil {
			return nil, err
		}
	}
	return &featureListLayout{offs: offs, pool: pool}, nil
}

// encodeBody encodes the Feature table.
func (f *Feature) encodeBody() []byte {
	enc := make([]byte, 4, 4+2*len(f.Lookups))
	if f.Params != nil {
		paramsOffset := 4 + 2*len(f.Lookups)
		enc[0] = byte(paramsOffset >> 8)
		enc[1] = byte(paramsOffset)
	}
	enc[2] = byte(len(f.Lookups) >> 8)
	enc[3] = byte(len(f.Lookups))
	for _, l := range f.Lookups {
		enc = append(enc, byte(l>>8), byte(l))
	}
	if f.Params != nil {
		enc = append(enc, f.Params.encode()...)
	}
	return enc
}

func (info FeatureListInfo) encode() []byte {
	if info == nil {
		return nil
	}
	l := mustFitList(info.layout())

	buf := make([]byte, 2+6*len(info), l.encodeLen())
	buf[0] = byte(len(info) >> 8)
	buf[1] = byte(len(info))
	for i, f := range info {
		copy(buf[2+6*i:6+6*i], f.Tag)
		buf[6+6*i] = byte(l.offs[i] >> 8)
		buf[7+6*i] = byte(l.offs[i])
	}
	return append(buf, l.pool.Bytes()...)
}

// Default features for use with the [Info.FindLookups] method.
var (
	// GsubDefaultFeatures lists a set of default features for "GSUB" tables.
	GsubDefaultFeatures = map[string]bool{
		"calt": true, // Contextual Alternates
		"ccmp": true, // Glyph Composition / Decomposition
		"clig": true, // Contextual Ligatures
		"liga": true, // Standard Ligatures
		"locl": true, // Localized Forms
	}

	// GposDefaultFeatures lists a set of default features for "GPOS" tables.
	GposDefaultFeatures = map[string]bool{
		"kern": true, // Kerning
		"mark": true, // Mark Positioning
		"mkmk": true, // Mark to Mark Positioning
	}
)
