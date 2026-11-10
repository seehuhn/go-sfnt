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
	"bytes"
	"cmp"
	"fmt"
	"slices"

	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/internal/offsetpool"
	"seehuhn.de/go/sfnt/opentype/markarray"
)

// anchorPool collects the Anchor tables of a mark, base, ligature or mark2
// array.  The tables are written after the offsets pointing at them, and the
// pool assigns each anchor an offset relative to the start of the array.
//
// Fonts commonly attach the same anchor to many glyphs, so anchors with
// identical encodings share a single copy.  Without this the arrays of a
// large font can outgrow the 64 KiB an Offset16 can reach.
type anchorPool struct {
	*offsetpool.Deferred
	devIDs *deviceIDs
}

// newAnchorPool returns a pool for anchors stored at the given offset from
// the start of the array.  name identifies the subtable in the error returned
// when the array outgrows the offsets addressing it.
func newAnchorPool(name string, base int) anchorPool {
	return anchorPool{offsetpool.NewDeferred(name, base), &deviceIDs{}}
}

// add registers a and returns its pool index.  A nil anchor is absent from
// the array and yields index -1.  An error is returned if the anchors cannot
// be reached through Offset16 fields.
func (p anchorPool) add(a *anchor.Table) (int, error) {
	if a == nil {
		return -1, nil
	}
	return p.AddFunc(p.key(a), func() []byte { return a.Append(nil) })
}

// anchorKey identifies the encoding of an anchor table.
type anchorKey struct {
	x, y       int16
	hasPoint   bool
	point      uint16
	xDev, yDev uint64
}

// key returns a key for [offsetpool.Pool.AddFunc] which identifies the
// encoding of a.  Anchors are keyed by their content rather than their
// address, so that many distinct anchors which refer to one large Device
// table encode it only once.
func (p anchorPool) key(a *anchor.Table) anchorKey {
	k := anchorKey{
		x:    int16(a.X),
		y:    int16(a.Y),
		xDev: p.devIDs.id(a.XDev),
		yDev: p.devIDs.id(a.YDev),
	}
	if a.ContourPoint != nil {
		k.hasPoint = true
		k.point = *a.ContourPoint
	}
	return k
}

// markArray is the layout of a MarkArray table: a mark count, one
// (class, offset) record per mark, and the anchors those offsets reach.
// Gpos4_1, Gpos5_1 and Gpos6_1 all begin with one.
type markArray struct {
	recs    []markarray.Record
	anchors []int // pool indices
	pool    anchorPool
}

// newMarkArray pools the anchors of recs.  name identifies the enclosing
// subtable in the error returned when the array grows out of reach.
func newMarkArray(name string, recs []markarray.Record) (*markArray, error) {
	pool := newAnchorPool(name, 2+4*len(recs))
	anchors := make([]int, len(recs))
	for i := range recs {
		var err error
		anchors[i], err = pool.add(&recs[i].Table)
		if err != nil {
			return nil, err
		}
	}
	return &markArray{recs: recs, anchors: anchors, pool: pool}, nil
}

// size returns the encoded length of the array.
func (a *markArray) size() int { return 2 + 4*len(a.recs) + a.pool.Len() }

// append appends the encoded array to buf.
func (a *markArray) append(buf []byte) []byte {
	offs, data := a.pool.Place()
	buf = appendUint16(buf, len(a.recs))
	for i, rec := range a.recs {
		buf = appendUint16(buf, int(rec.Class))
		buf = appendUint16(buf, int(poolOffset(offs, a.anchors[i])))
	}
	return append(buf, data...)
}

// anchorMatrix is the layout of a BaseArray, LigatureAttach or Mark2Array
// table: a row count, a rectangular block of cols offsets per row, and the
// anchors those offsets reach.  An absent anchor is stored as offset 0.
//
// The block must be rectangular, since the anchors follow it: a short row
// would displace every anchor after it.  Callers establish this by way of
// their countMarkClasses method, which supplies cols.
type anchorMatrix struct {
	rows    [][]*anchor.Table
	cols    int
	anchors [][]int // pool indices
	pool    anchorPool
}

// newAnchorMatrix pools the anchors of rows, each of which has cols entries.
// name identifies the enclosing subtable in the error returned when the table
// grows out of reach.
func newAnchorMatrix(name string, rows [][]*anchor.Table, cols int) (*anchorMatrix, error) {
	pool := newAnchorPool(name, 2+2*len(rows)*cols)
	anchors := make([][]int, len(rows))
	for i, row := range rows {
		anchors[i] = make([]int, len(row))
		for j, a := range row {
			var err error
			anchors[i][j], err = pool.add(a)
			if err != nil {
				return nil, err
			}
		}
	}
	return &anchorMatrix{rows: rows, cols: cols, anchors: anchors, pool: pool}, nil
}

// size returns the encoded length of the table.
func (m *anchorMatrix) size() int { return 2 + 2*len(m.rows)*m.cols + m.pool.Len() }

// append appends the encoded table to buf.
func (m *anchorMatrix) append(buf []byte) []byte {
	offs, data := m.pool.Place()
	buf = appendUint16(buf, len(m.rows))
	for _, row := range m.anchors {
		buf = appendOffsets(buf, offs, row)
	}
	return append(buf, data...)
}

// attachArray is the array of base, ligature or mark2 data which follows the
// mark array in a mark attachment subtable.
type attachArray interface {
	size() int
	append(buf []byte) []byte
}

// markAttach is the layout of a mark attachment subtable (Gpos4_1, Gpos5_1
// or Gpos6_1): the header, the two coverage tables, the mark array and the
// array of attachment data.  The coverage tables are stored once if they
// are identical.
type markAttach struct {
	markClassCount int
	markCov        []byte
	attachCov      []byte // nil if stored together with markCov
	marks          *markArray
	attach         attachArray
	offs           []int // offsets of the four tables, in header order
}

// newMarkAttach computes the layout of a mark attachment subtable.  An error
// is returned if an offset in the subtable header does not fit into an
// Offset16.
func newMarkAttach(name string, markCov, attachCov coverage.Table, markClassCount int, marks *markArray, attach attachArray) (*markAttach, error) {
	m := &markAttach{
		markClassCount: markClassCount,
		markCov:        markCov.Encode(),
		attachCov:      attachCov.Encode(),
		marks:          marks,
		attach:         attach,
	}
	shared := bytes.Equal(m.markCov, m.attachCov)
	if shared {
		m.attachCov = nil
	}

	// Each array addresses its anchors from its own start and bounds itself,
	// so the subtable may reach past the 64 KiB an offset can cover.  What is
	// left to check are the offsets in the subtable header.
	var ok bool
	m.offs, ok = offsetpool.Place(12, m.sizes())
	if !ok {
		return nil, fmt.Errorf("%s offset out of range", name)
	}
	if shared {
		m.offs[1] = m.offs[0]
	}
	return m, nil
}

// sizes returns the encoded lengths of the four tables, in header order.
// The size is -1 for an attachment coverage table which is not stored.
func (m *markAttach) sizes() []int {
	sizes := []int{len(m.markCov), len(m.attachCov), m.marks.size(), m.attach.size()}
	if m.attachCov == nil {
		sizes[1] = -1
	}
	return sizes
}

// size returns the encoded length of the subtable.
func (m *markAttach) size() int {
	total := 12
	for _, size := range m.sizes() {
		total += max(size, 0)
	}
	return total
}

// append appends the encoded subtable to buf.
func (m *markAttach) append(buf []byte) []byte {
	buf = appendUint16(buf, 1) // format
	buf = appendUint16(buf, m.offs[0])
	buf = appendUint16(buf, m.offs[1])
	buf = appendUint16(buf, m.markClassCount)
	buf = appendUint16(buf, m.offs[2])
	buf = appendUint16(buf, m.offs[3])

	// the tables in order of increasing offset
	tables := []func([]byte) []byte{
		func(buf []byte) []byte { return append(buf, m.markCov...) },
		func(buf []byte) []byte { return append(buf, m.attachCov...) },
		m.marks.append,
		m.attach.append,
	}
	order := []int{0, 1, 2, 3}
	slices.SortStableFunc(order, func(i, j int) int { return cmp.Compare(m.offs[i], m.offs[j]) })
	for _, i := range order {
		buf = tables[i](buf)
	}
	return buf
}
