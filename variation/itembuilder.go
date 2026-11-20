// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package variation

import (
	"encoding/binary"
	"errors"
	"math"
	"slices"
)

// maxStoreEntries is the largest number of regions, subtables, or rows
// per subtable which an item variation store can hold.
const maxStoreEntries = 0xFFFF

var errTooManySubtables = errors.New("variation: too many subtables in item variation store")

// StoreBuilder assembles an [ItemVariationStore] from individual delta rows.
// Regions are shared between rows, and rows with the same set of regions
// are collected into one [ItemVariationData] subtable.
type StoreBuilder struct {
	regions   []Region
	regionIdx map[string]uint16

	groups  []*builderGroup
	open    map[string]int // region set -> subtable still accepting rows
	rowIdxs map[string][2]uint16
}

// builderGroup is one subtable under construction.
type builderGroup struct {
	cols []uint16 // region indexes, ascending
	rows [][]int32
}

// NewStoreBuilder returns an empty builder.
func NewStoreBuilder() *StoreBuilder {
	return &StoreBuilder{
		regionIdx: make(map[string]uint16),
		open:      make(map[string]int),
		rowIdxs:   make(map[string][2]uint16),
	}
}

// Add registers one delta row and returns the (outer, inner) index pair
// under which the row can be looked up in the store returned by
// [StoreBuilder.Build].  The delta deltas[i] applies within regions[i].
//
// If a region appears more than once in regions, the corresponding deltas
// are summed.  Regions with a zero delta are not stored.  A row without any
// non-zero delta is valid; it is placed in a subtable without region
// columns.  Adding a row equal to an earlier one returns the earlier
// indices.  Indices returned by Add remain valid after later calls to Add
// and Build.
//
// An all-zero row needs no device table or VariationIndex at all: the
// store's reader drops the rows of subtables without region columns.
//
// An error is returned if the summed delta of a region does not fit into an
// int32, or if the store would need more than 0xFFFF regions or more than
// 0xFFFF subtables.  In this case the builder is unchanged.
func (b *StoreBuilder) Add(regions []Region, deltas []int32) (outer, inner uint16, err error) {
	// sum the deltas per distinct region
	type entry struct {
		key   string
		index int // position in regions
		delta int64
	}
	var entries []entry
	pos := make(map[string]int)
	for i, r := range regions {
		key := regionKey(canonicalRegion(r))
		if j, ok := pos[key]; ok {
			entries[j].delta += int64(deltas[i])
			continue
		}
		pos[key] = len(entries)
		entries = append(entries, entry{key, i, int64(deltas[i])})
	}
	for _, e := range entries {
		if e.delta < math.MinInt32 || e.delta > math.MaxInt32 {
			return 0, 0, errors.New("variation: summed region delta out of range")
		}
	}
	entries = slices.DeleteFunc(entries, func(e entry) bool { return e.delta == 0 })

	newRegions := 0
	for _, e := range entries {
		if _, ok := b.regionIdx[e.key]; !ok {
			newRegions++
		}
	}
	if len(b.regions)+newRegions > maxStoreEntries {
		return 0, 0, errors.New("variation: too many regions in item variation store")
	}
	// a row with a new region needs a new subtable; check this before the
	// regions are registered, so that a failed call leaves no trace
	if newRegions > 0 && len(b.groups) >= maxStoreEntries {
		return 0, 0, errTooManySubtables
	}

	// column indices of the row, in the order of the region indexes
	type cell struct {
		idx   uint16
		delta int32
	}
	cells := make([]cell, len(entries))
	for i, e := range entries {
		idx, ok := b.regionIdx[e.key]
		if !ok {
			idx = uint16(len(b.regions))
			b.regions = append(b.regions, canonicalRegion(regions[e.index]))
			b.regionIdx[e.key] = idx
		}
		cells[i] = cell{idx, int32(e.delta)}
	}
	slices.SortFunc(cells, func(x, y cell) int { return int(x.idx) - int(y.idx) })

	var gk, rk []byte
	for _, c := range cells {
		gk = binary.BigEndian.AppendUint16(gk, c.idx)
	}
	rk = slices.Clone(gk)
	for _, c := range cells {
		rk = binary.BigEndian.AppendUint32(rk, uint32(c.delta))
	}
	if at, ok := b.rowIdxs[string(rk)]; ok {
		return at[0], at[1], nil
	}

	gi, ok := b.open[string(gk)]
	if !ok || len(b.groups[gi].rows) >= maxStoreEntries {
		if len(b.groups) >= maxStoreEntries {
			return 0, 0, errTooManySubtables
		}
		g := &builderGroup{cols: make([]uint16, len(cells))}
		for i, c := range cells {
			g.cols[i] = c.idx
		}
		gi = len(b.groups)
		b.groups = append(b.groups, g)
		b.open[string(gk)] = gi
	}
	g := b.groups[gi]
	row := make([]int32, len(cells))
	for i, c := range cells {
		row[i] = c.delta
	}
	g.rows = append(g.rows, row)

	outer, inner = uint16(gi), uint16(len(g.rows)-1)
	b.rowIdxs[string(rk)] = [2]uint16{outer, inner}
	return outer, inner, nil
}

// Build returns the item variation store holding all rows added so far.
// The store does not share memory with the builder.  Subtables and rows
// appear in the order of the indices returned by [StoreBuilder.Add]; the
// builder may continue to be used afterwards.
func (b *StoreBuilder) Build() *ItemVariationStore {
	s := &ItemVariationStore{
		Regions: make([]Region, len(b.regions)),
		Data:    make([]*ItemVariationData, len(b.groups)),
	}
	for i, r := range b.regions {
		s.Regions[i] = slices.Clone(r)
	}

	for i, g := range b.groups {
		// columns needing wider deltas come first, as the encoding
		// stores the wide columns as a prefix
		order := make([]int, len(g.cols))
		width := make([]int, len(g.cols))
		for j := range order {
			order[j] = j
			for _, row := range g.rows {
				width[j] = max(width[j], deltaWidth(row[j]))
			}
		}
		slices.SortStableFunc(order, func(x, y int) int { return width[y] - width[x] })

		d := &ItemVariationData{
			RegionIndexes: make([]uint16, len(order)),
			Deltas:        make([][]int32, len(g.rows)),
		}
		for j, c := range order {
			d.RegionIndexes[j] = g.cols[c]
		}
		for k, row := range g.rows {
			nr := make([]int32, len(order))
			for j, c := range order {
				nr[j] = row[c]
			}
			d.Deltas[k] = nr
		}
		s.Data[i] = d
	}
	return s
}

// deltaWidth returns 0, 1 or 2 if v fits in 8, 16 or 32 bits.
func deltaWidth(v int32) int {
	switch {
	case v < math.MinInt16 || v > math.MaxInt16:
		return 2
	case v < math.MinInt8 || v > math.MaxInt8:
		return 1
	}
	return 0
}

// canonicalRegion returns a copy of r in which the axes which do not affect
// the region are all zero.  These are the axes with a zero peak, the axes
// whose start, peak and end are out of order, and the axes whose range
// contains both negative and positive values.
func canonicalRegion(r Region) Region {
	c := slices.Clone(r)
	for i, a := range c {
		if a.Peak == 0 || a.Start > a.Peak || a.Peak > a.End || a.Start < 0 && a.End > 0 {
			c[i] = RegionAxis{}
		}
	}
	return c
}

// regionKey returns a map key which identifies a region by value.
func regionKey(r Region) string {
	buf := make([]byte, 0, 6*len(r))
	for _, a := range r {
		buf = binary.BigEndian.AppendUint16(buf, uint16(a.Start))
		buf = binary.BigEndian.AppendUint16(buf, uint16(a.Peak))
		buf = binary.BigEndian.AppendUint16(buf, uint16(a.End))
	}
	return string(buf)
}
