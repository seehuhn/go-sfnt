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
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/markarray"
)

// anchorPool collects the Anchor tables of a mark, base, ligature or mark2
// array.  The tables are written after the offsets pointing at them, and the
// pool returns each anchor's offset relative to the start of the array.
//
// Fonts commonly attach the same anchor to many glyphs, so anchors with
// identical encodings share a single copy.  Without this the arrays of a
// large font can outgrow the 64 KiB an Offset16 can reach.
type anchorPool struct {
	name    string
	base    int
	seen    map[string]uint16
	encoded []byte
}

// newAnchorPool returns a pool for anchors stored at the given offset from
// the start of the array.  name identifies the subtable in the panic raised
// when the array outgrows the offsets addressing it.
func newAnchorPool(name string, base int) *anchorPool {
	return &anchorPool{name: name, base: base, seen: map[string]uint16{}}
}

// add registers a and returns its array-relative offset.  A nil anchor is
// absent from the array and yields offset 0.
//
// The offset is checked here rather than left to the caller, so that a full
// array is refused wherever the pool is filled.  Sizing passes build a pool
// without ever laying out the array around it, and would otherwise take a
// truncated offset for a valid one.
func (p *anchorPool) add(a *anchor.Table) uint16 {
	if a == nil {
		return 0
	}
	enc := a.Append(nil)
	key := string(enc)
	if off, ok := p.seen[key]; ok {
		return off
	}
	off := p.base + len(p.encoded)
	checkSubtableOffset16(p.name, off)
	p.seen[key] = uint16(off)
	p.encoded = append(p.encoded, enc...)
	return uint16(off)
}

// len returns the total byte length of the pool's encoded data.
func (p *anchorPool) len() int { return len(p.encoded) }

// bytes returns the concatenated bytes of the unique anchors, in offset
// order.  Callers append this to the end of the array.
func (p *anchorPool) bytes() []byte { return p.encoded }

// markArray is the layout of a MarkArray table: a mark count, one
// (class, offset) record per mark, and the anchors those offsets reach.
// Gpos4_1, Gpos5_1 and Gpos6_1 all begin with one.
type markArray struct {
	recs []markarray.Record
	offs []uint16
	pool *anchorPool
}

// newMarkArray pools the anchors of recs.  name identifies the enclosing
// subtable in the panic raised when the array grows out of reach.
func newMarkArray(name string, recs []markarray.Record) *markArray {
	pool := newAnchorPool(name, 2+4*len(recs))
	offs := make([]uint16, len(recs))
	for i := range recs {
		offs[i] = pool.add(&recs[i].Table)
	}
	a := &markArray{recs: recs, offs: offs, pool: pool}
	// Every offset the array stores is reached from the array's own start, so
	// bounding its length bounds them all.  The pool checks the offsets it
	// hands out; this also catches an array of unreachable records whose
	// anchors are all absent.
	checkSubtableOffset16(name, a.size())
	return a
}

// size returns the encoded length of the array.
func (a *markArray) size() int { return 2 + 4*len(a.recs) + a.pool.len() }

// append appends the encoded array to buf.
func (a *markArray) append(buf []byte) []byte {
	buf = append(buf, byte(len(a.recs)>>8), byte(len(a.recs)))
	for i, rec := range a.recs {
		buf = append(buf,
			byte(rec.Class>>8), byte(rec.Class),
			byte(a.offs[i]>>8), byte(a.offs[i]),
		)
	}
	return append(buf, a.pool.bytes()...)
}

// anchorMatrix is the layout of a BaseArray, LigatureAttach or Mark2Array
// table: a row count, a rectangular block of cols offsets per row, and the
// anchors those offsets reach.  An absent anchor is stored as offset 0.
//
// The block must be rectangular, since the anchors follow it: a short row
// would displace every anchor after it.  Callers establish this by way of
// their countMarkClasses method, which supplies cols.
type anchorMatrix struct {
	rows [][]*anchor.Table
	cols int
	offs [][]uint16
	pool *anchorPool
}

// newAnchorMatrix pools the anchors of rows, each of which has cols entries.
// name identifies the enclosing subtable in the panic raised when the table
// grows out of reach.
func newAnchorMatrix(name string, rows [][]*anchor.Table, cols int) *anchorMatrix {
	pool := newAnchorPool(name, 2+2*len(rows)*cols)
	offs := make([][]uint16, len(rows))
	for i, row := range rows {
		offs[i] = make([]uint16, len(row))
		for j, a := range row {
			offs[i][j] = pool.add(a)
		}
	}
	m := &anchorMatrix{rows: rows, cols: cols, offs: offs, pool: pool}
	checkSubtableOffset16(name, m.size())
	return m
}

// size returns the encoded length of the table.
func (m *anchorMatrix) size() int { return 2 + 2*len(m.rows)*m.cols + m.pool.len() }

// append appends the encoded table to buf.
func (m *anchorMatrix) append(buf []byte) []byte {
	buf = append(buf, byte(len(m.rows)>>8), byte(len(m.rows)))
	for _, row := range m.offs {
		for _, off := range row {
			buf = append(buf, byte(off>>8), byte(off))
		}
	}
	return append(buf, m.pool.bytes()...)
}
