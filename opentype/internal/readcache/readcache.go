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

// Package readcache lets the readers in sfnt/opentype read a table only
// once when several offsets reach it.
//
// A valid font may point many offsets at the same bytes.  Reading each
// reference separately would build one Go value per reference, and the
// memory used could grow far beyond what the input size justifies.  With a
// Cache, all references to one position share a single Go value.  Callers
// must therefore treat values read through a Cache as immutable.
package readcache

// Cache maps file positions to the values read there.
// The zero value is not usable; use [New].
type Cache[T any] struct {
	m map[int64]T
}

// New returns an empty cache.
func New[T any]() *Cache[T] {
	return &Cache[T]{m: map[int64]T{}}
}

// Get returns the value at pos.  If no value has been read at pos before,
// read is called and its result is stored.  Errors are not cached.
func (c *Cache[T]) Get(pos int64, read func(pos int64) (T, error)) (T, error) {
	if v, ok := c.m[pos]; ok {
		return v, nil
	}
	v, err := read(pos)
	if err != nil {
		return v, err
	}
	c.m[pos] = v
	return v, nil
}
