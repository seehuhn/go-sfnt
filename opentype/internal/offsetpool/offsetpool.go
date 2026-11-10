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

// Package offsetpool lays out the tables which an OpenType layout table
// reaches through Offset16 fields.
//
// Fonts often point many offsets at the same bytes, for example one
// coverage table used by several subtables, or one Anchor table used by
// many glyphs.  Without sharing on write, re-encoding such a table could
// need many times the space of the original and outgrow the 64 KiB an
// Offset16 can address.  A Pool therefore stores tables with identical
// encodings only once.
package offsetpool

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"iter"
	"reflect"
	"slices"
	"unsafe"
)

// Pool collects the encoded tables which follow the offset fields of an
// enclosing table.  Tables with identical bytes share a single copy.
type Pool struct {
	name    string
	base    int
	seen    map[string]uint16
	byKey   map[any]uint16
	encoded []byte
}

// New returns an empty pool whose first table will be stored at offset
// base from the start of the enclosing table.  The name identifies the
// enclosing table in error messages.
func New(name string, base int) *Pool {
	return &Pool{name: name, base: base, seen: map[string]uint16{}}
}

// Add registers the encoded table enc and returns its offset from the
// start of the enclosing table.  If a table with the same bytes was added
// before, its offset is returned and nothing is stored.
//
// An error is returned if the offset does not fit into an Offset16.  The
// table itself may extend past this limit.
func (p *Pool) Add(enc []byte) (uint16, error) {
	key := string(enc)
	if off, ok := p.seen[key]; ok {
		return off, nil
	}
	off := p.base + len(p.encoded)
	if off > 0xFFFF {
		return 0, fmt.Errorf("%s offset %d out of range", p.name, off)
	}
	p.seen[key] = uint16(off)
	p.encoded = append(p.encoded, enc...)
	return uint16(off), nil
}

// AddFunc is like [Pool.Add], but avoids encoding a Go value more than
// once.  The key identifies the Go value the table is encoded from, and
// encode returns its encoding.  If AddFunc was called with the same key
// before, the earlier offset is returned without calling encode.
//
// The readers share one Go value between all offsets which reach the same
// table.  Without the key, laying out such a table would encode the shared
// value once per reference, which takes time quadratic in the input size.
//
// The caller must make sure that equal keys imply equal encodings.  Use a
// pointer, or the result of [SliceKey] or [MapKey], and only for values
// whose encoding depends on nothing else.
func (p *Pool) AddFunc(key any, encode func() []byte) (uint16, error) {
	if off, ok := p.byKey[key]; ok {
		return off, nil
	}
	off, err := p.Add(encode())
	if err != nil {
		return 0, err
	}
	if p.byKey == nil {
		p.byKey = map[any]uint16{}
	}
	p.byKey[key] = off
	return off, nil
}

// Len returns the total length of the stored tables.
func (p *Pool) Len() int { return len(p.encoded) }

// Bytes returns the stored tables, in order of increasing offset.
func (p *Pool) Bytes() []byte { return p.encoded }

// SliceKey returns a key for [Pool.AddFunc] which identifies the slice s by
// its type, its first element and its length.  Empty slices of the same type
// share one key.
func SliceKey[T any](s []T) any {
	k := sliceKey[T]{n: len(s)}
	if len(s) > 0 {
		k.first = &s[0]
	}
	return k
}

// sliceKey is generic, so that slices of different types never share a key.
// All empty slices have the same address, whatever their element type.
type sliceKey[T any] struct {
	first *T
	n     int
}

// MapKey returns a key for [Pool.AddFunc] which identifies the map m by its
// type and identity.
func MapKey[K comparable, V any](m map[K]V) any {
	return mapKey[K, V]{reflect.ValueOf(m).UnsafePointer()}
}

type mapKey[K comparable, V any] struct {
	m unsafe.Pointer
}

// SeqKeys makes keys for [Pool.AddFunc] which identify a table by the
// values it refers to.  A table such as a rule set holds a list of
// references, and two tables with the same references have the same
// encoding.  The readers return a separate Go value for each such table
// when the tables lie at different positions in the file, even if they
// refer to shared values.  Keying the tables by their references avoids
// encoding each of them, which could take time far beyond what the input
// size justifies.
//
// The zero value is ready to use.  Keys from different SeqKeys values must
// not be used with the same Pool.
type SeqKeys[K comparable] struct {
	ids  map[K]uint64
	keys map[any]any
}

// Key returns a key for the table which refers to the values in refs, in
// order.  Two calls return equal keys if and only if the sequences are
// equal.  The caller must make sure that equal sequences imply equal
// encodings.  The length n of the sequence is used as a size hint.
//
// The value id identifies the Go value holding the references, as for the
// keys of [Pool.AddFunc].  If Key was called with the same id before, the
// earlier key is returned without consulting refs, so that a list shared
// between many tables is traversed only once.
func (s *SeqKeys[K]) Key(id any, n int, refs iter.Seq[K]) any {
	if key, ok := s.keys[id]; ok {
		return key
	}
	if s.ids == nil {
		s.ids = map[K]uint64{}
		s.keys = map[any]any{}
	}
	// uvarints are self-delimiting, so equal strings mean equal sequences
	buf := make([]byte, 0, n)
	for k := range refs {
		i, ok := s.ids[k]
		if !ok {
			i = uint64(len(s.ids))
			s.ids[k] = i
		}
		buf = binary.AppendUvarint(buf, i)
	}
	var key any = seqKey{unsafe.String(unsafe.SliceData(buf), len(buf))}
	s.keys[id] = key
	return key
}

type seqKey struct {
	ids string
}

// Place computes the offsets of tables which follow a header of length hdr
// and are reached through Offset16 fields of the header.  The sizes give
// the encoded lengths of the tables, and a negative size marks an absent
// table, which is given offset 0.
//
// The tables are stored in the given order if all offsets fit.  Otherwise
// they are stored in order of increasing size, which makes every offset as
// small as possible.  The result ok is false if the tables cannot be stored
// in any order.
func Place(hdr int, sizes []int) (offs []int, ok bool) {
	order := make([]int, len(sizes))
	for i := range order {
		order[i] = i
	}
	if offs, ok := placeInOrder(hdr, sizes, order); ok {
		return offs, true
	}
	slices.SortStableFunc(order, func(i, j int) int {
		return cmp.Compare(sizes[i], sizes[j])
	})
	return placeInOrder(hdr, sizes, order)
}

func placeInOrder(hdr int, sizes, order []int) ([]int, bool) {
	offs := make([]int, len(sizes))
	total := hdr
	for _, i := range order {
		if sizes[i] < 0 {
			continue
		}
		if total > 0xFFFF {
			return nil, false
		}
		offs[i] = total
		total += sizes[i]
	}
	return offs, true
}

// Deferred collects encoded tables like a [Pool], but assigns the offsets
// only once all tables are known.  This allows it to reorder the tables
// when they do not fit in the order they were added: each offset must
// reach the start of its table, so a table which extends past the 64 KiB
// an Offset16 can cover must come last.
type Deferred struct {
	name    string
	base    int
	seen    map[string]int
	byKey   map[any]int
	tables  []string
	total   int
	largest int
}

// NewDeferred returns an empty collection whose first table will be stored
// at offset base from the start of the enclosing table.  The name
// identifies the enclosing table in error messages.
func NewDeferred(name string, base int) *Deferred {
	return &Deferred{name: name, base: base, seen: map[string]int{}}
}

// Add registers the encoded table enc and returns its index, for use with
// the offsets returned by [Deferred.Place].  If a table with the same bytes
// was added before, its index is returned and nothing is stored.
//
// An error is returned once the tables cannot be stored in any order.
func (d *Deferred) Add(enc []byte) (int, error) {
	if i, ok := d.seen[string(enc)]; ok {
		return i, nil
	}
	d.total += len(enc)
	d.largest = max(d.largest, len(enc))
	// with the largest table last, every other table starts before it
	if off := d.base + d.total - d.largest; off > 0xFFFF {
		return 0, fmt.Errorf("%s offset %d out of range", d.name, off)
	}
	key := string(enc)
	i := len(d.tables)
	d.seen[key] = i
	d.tables = append(d.tables, key)
	return i, nil
}

// AddFunc is like [Deferred.Add], but avoids encoding a Go value more than
// once.  The key and encode arguments are as for [Pool.AddFunc].
func (d *Deferred) AddFunc(key any, encode func() []byte) (int, error) {
	if i, ok := d.byKey[key]; ok {
		return i, nil
	}
	i, err := d.Add(encode())
	if err != nil {
		return 0, err
	}
	if d.byKey == nil {
		d.byKey = map[any]int{}
	}
	d.byKey[key] = i
	return i, nil
}

// Len returns the total length of the stored tables.
func (d *Deferred) Len() int { return d.total }

// Place returns the offset of every table from the start of the enclosing
// table, indexed as returned by [Deferred.Add], and the tables themselves
// in order of increasing offset.
//
// The tables are stored in the order they were added if all offsets fit.
// Otherwise they are stored in order of increasing size.
func (d *Deferred) Place() (offs []uint16, data []byte) {
	sizes := make([]int, len(d.tables))
	for i, enc := range d.tables {
		sizes[i] = len(enc)
	}
	pos, ok := Place(d.base, sizes)
	if !ok {
		// Add has checked that the tables fit in order of increasing size
		panic("unreachable")
	}
	offs = make([]uint16, len(pos))
	data = make([]byte, d.total)
	for i, enc := range d.tables {
		offs[i] = uint16(pos[i])
		copy(data[pos[i]-d.base:], enc)
	}
	return offs, data
}
