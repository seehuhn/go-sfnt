// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package offsetpool

import (
	"bytes"
	"slices"
	"testing"
)

func TestShare(t *testing.T) {
	p := New("test", 10)
	a, err := p.Add([]byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Add([]byte{4, 5})
	if err != nil {
		t.Fatal(err)
	}
	c, err := p.Add([]byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	if a != 10 || b != 13 || c != a {
		t.Errorf("offsets %d, %d, %d; want 10, 13, 10", a, b, c)
	}
	if p.Len() != 5 {
		t.Errorf("Len() = %d, want 5", p.Len())
	}
	if want := []byte{1, 2, 3, 4, 5}; !bytes.Equal(p.Bytes(), want) {
		t.Errorf("Bytes() = %v, want %v", p.Bytes(), want)
	}
}

func TestRange(t *testing.T) {
	p := New("test", 0xFFFE)
	// the last table may extend past the limit
	off, err := p.Add(make([]byte, 100))
	if err != nil || off != 0xFFFE {
		t.Fatalf("Add() = %d, %v; want %d, nil", off, err, 0xFFFE)
	}
	// a repeated table needs no new offset
	if _, err := p.Add(make([]byte, 100)); err != nil {
		t.Errorf("repeated table: %v", err)
	}
	if _, err := p.Add([]byte{1}); err == nil {
		t.Error("offset past the limit accepted")
	}
	if p.Len() != 100 {
		t.Errorf("Len() = %d after failed Add, want 100", p.Len())
	}
}

func TestAddFunc(t *testing.T) {
	p := New("test", 0)
	calls := 0
	enc := func(b ...byte) func() []byte {
		return func() []byte {
			calls++
			return b
		}
	}
	s := []int{1, 2, 3}
	m := map[int]bool{1: true}
	x := new(int)
	steps := []struct {
		key  any
		enc  func() []byte
		want uint16
	}{
		{SliceKey(s), enc(1, 2), 0},
		{SliceKey(s), enc(1, 2), 0},              // same slice: not encoded again
		{SliceKey(s[:2]), enc(7), 2},             // different length: a different key
		{SliceKey([]int{1, 2, 3}), enc(1, 2), 0}, // different value, same bytes
		{MapKey(m), enc(5), 3},
		{MapKey(m), enc(5), 3},
		{x, enc(6), 4},
		{x, enc(6), 4},
	}
	for i, st := range steps {
		off, err := p.AddFunc(st.key, st.enc)
		if err != nil || off != st.want {
			t.Errorf("step %d: AddFunc() = %d, %v; want %d, nil", i, off, err, st.want)
		}
	}
	if calls != 5 {
		t.Errorf("encode called %d times, want 5", calls)
	}

	// empty slices share an address, so the key must depend on the type
	if SliceKey([]int{}) == SliceKey([]string{}) {
		t.Error("empty slices of different types share a key")
	}

	full := New("test", 0x10000)
	if _, err := full.AddFunc(x, enc(1)); err == nil {
		t.Error("offset past the limit accepted")
	}
}

func TestPlace(t *testing.T) {
	cases := []struct {
		name  string
		hdr   int
		sizes []int
		want  []int // nil if the tables cannot be placed
	}{
		{"given order", 10, []int{5, 7, 3}, []int{10, 15, 22}},
		{"absent table", 10, []int{5, -1, 3}, []int{10, 0, 15}},
		{"large table last", 10, []int{70000, 5, 3}, []int{18, 13, 10}},
		{"last table may be large", 10, []int{5, 70000}, []int{10, 15}},
		{"three large tables", 10, []int{40000, 40000, 40000}, nil},
		{"limit", 0, []int{0xFFFF, 1}, []int{0, 0xFFFF}},
		{"beyond limit", 0, []int{0x10000, 1, 0xFFFF}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := Place(c.hdr, c.sizes)
			if c.want == nil {
				if ok {
					t.Errorf("expected failure, got %v", got)
				}
				return
			}
			if !ok {
				t.Fatal("unexpected failure")
			}
			if !slices.Equal(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestSeqKeys(t *testing.T) {
	a, b := new(int), new(int)
	s1 := []*int{a, b, a}
	s2 := []*int{a, b, a}
	s3 := []*int{b, a, b}
	s4 := []*int{a, b}

	var keys SeqKeys[*int]
	key := func(s []*int) any { return keys.Key(SliceKey(s), len(s), slices.Values(s)) }
	k1, k2, k3, k4 := key(s1), key(s2), key(s3), key(s4)
	if k1 != k2 {
		t.Error("equal sequences give different keys")
	}
	if k1 == k3 || k1 == k4 || k3 == k4 {
		t.Error("different sequences give equal keys")
	}
	if key(s1) != k1 {
		t.Error("repeated call gives a different key")
	}

	// a key which uses SliceKey(s1) as its id but differs from s1 is only
	// possible if s1 was modified, which callers do not do; check that the
	// earlier key is returned without looking at the sequence
	if got := keys.Key(SliceKey(s1), 0, func(func(*int) bool) { t.Error("refs consulted") }); got != k1 {
		t.Error("memoised key not returned")
	}
}

func TestSeqKeysManyIDs(t *testing.T) {
	// ids beyond 127 take more than one byte as uvarints
	vals := make([]*int, 300)
	for i := range vals {
		vals[i] = new(int)
	}
	var keys SeqKeys[*int]
	seen := map[any]int{}
	for i := range vals {
		for j := range vals {
			s := []*int{vals[i], vals[j]}
			k := keys.Key(SliceKey(s), len(s), slices.Values(s))
			if prev, dup := seen[k]; dup {
				t.Fatalf("sequences %d and %d share a key", prev, i*len(vals)+j)
			}
			seen[k] = i*len(vals) + j
		}
	}
}

func TestDeferred(t *testing.T) {
	d := NewDeferred("test", 10)
	a, err := d.Add([]byte{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	b, err := d.Add([]byte{3})
	if err != nil {
		t.Fatal(err)
	}
	a2, err := d.Add([]byte{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	if a2 != a {
		t.Errorf("identical tables not shared: %d != %d", a2, a)
	}

	calls := 0
	encode := func() []byte {
		calls++
		return []byte{3}
	}
	for range 2 {
		c, err := d.AddFunc("key", encode)
		if err != nil {
			t.Fatal(err)
		}
		if c != b {
			t.Errorf("identical tables not shared: %d != %d", c, b)
		}
	}
	if calls != 1 {
		t.Errorf("encode called %d times, want 1", calls)
	}

	if d.Len() != 3 {
		t.Errorf("Len() = %d, want 3", d.Len())
	}
	offs, data := d.Place()
	if !slices.Equal(offs, []uint16{10, 12}) {
		t.Errorf("offsets %d, want [10 12]", offs)
	}
	if !bytes.Equal(data, []byte{1, 2, 3}) {
		t.Errorf("data % x, want 01 02 03", data)
	}
}

// TestDeferredReorder checks that a table which extends past the range of
// an Offset16 is moved behind the tables added after it.
func TestDeferredReorder(t *testing.T) {
	d := NewDeferred("test", 10)
	large := make([]byte, 0x10000)
	large[0] = 1
	i, err := d.Add(large)
	if err != nil {
		t.Fatal(err)
	}
	j, err := d.Add([]byte{2, 3})
	if err != nil {
		t.Fatal(err)
	}
	offs, data := d.Place()
	if offs[j] != 10 || offs[i] != 12 {
		t.Errorf("wrong offsets: %d", offs)
	}
	if len(data) != d.Len() || data[0] != 2 || data[2] != 1 {
		t.Errorf("wrong data: % x ...", data[:3])
	}

	// a second large table cannot be reached in any order
	large2 := make([]byte, 0x10000)
	if _, err := d.Add(large2); err == nil {
		t.Error("expected error")
	}
	if _, err := d.AddFunc("key", func() []byte { return large2 }); err == nil {
		t.Error("expected error")
	}
}
