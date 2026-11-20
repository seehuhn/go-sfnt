// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package variation

import (
	"bytes"
	"math"
	"math/rand"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/parser"
)

// roundTripStore encodes s and reads it back.
func roundTripStore(t *testing.T, s *ItemVariationStore) *ItemVariationStore {
	t.Helper()
	data := s.Encode()
	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	got, err := ReadItemVariationStore(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func expectedDelta(regions []Region, deltas []int32, coords []F2Dot14) float64 {
	var sum float64
	for i, r := range regions {
		sum += float64(deltas[i]) * r.Scalar(coords)
	}
	return sum
}

type testRow struct {
	regions      []Region
	deltas       []int32
	outer, inner uint16
}

func testRegions() []Region {
	return []Region{
		{{0, 0x4000, 0x4000}, {0, 0, 0}},
		{{-0x4000, -0x4000, 0}, {0, 0, 0}},
		{{0, 0, 0}, {0, 0x4000, 0x4000}},
		{{0, 0x2000, 0x4000}, {0, 0x4000, 0x4000}},
	}
}

func TestStoreBuilderRoundTrip(t *testing.T) {
	rs := testRegions()
	rows := []testRow{
		{regions: []Region{rs[0]}, deltas: []int32{5}},
		{regions: []Region{rs[0], rs[1]}, deltas: []int32{-100, 3}},
		{regions: []Region{rs[1], rs[0]}, deltas: []int32{70000, 2}},
		{regions: []Region{rs[2], rs[3], rs[0]}, deltas: []int32{1, 300, -2}},
		{regions: []Region{rs[3]}, deltas: []int32{-1000}},
		{regions: []Region{rs[0], rs[2]}, deltas: []int32{0, 7}},
		{regions: []Region{rs[0]}, deltas: []int32{0}},
		{regions: nil, deltas: nil},
		{regions: []Region{rs[1], rs[1]}, deltas: []int32{3, 4}},
	}

	b := NewStoreBuilder()
	for i := range rows {
		var err error
		rows[i].outer, rows[i].inner, err = b.Add(rows[i].regions, rows[i].deltas)
		if err != nil {
			t.Fatal(err)
		}
	}
	got := roundTripStore(t, b.Build())

	var points [][]F2Dot14
	for _, x := range []F2Dot14{-0x4000, -0x2000, 0, 0x1000, 0x2000, 0x4000} {
		for _, y := range []F2Dot14{0, 0x2000, 0x3000, 0x4000} {
			points = append(points, []F2Dot14{x, y})
		}
	}
	for i, row := range rows {
		for _, c := range points {
			want := expectedDelta(row.regions, row.deltas, c)
			have := got.Evaluate(row.outer, row.inner, c)
			if math.Abs(want-have) > 1e-6 {
				t.Errorf("row %d at %v: got %g, want %g", i, c, have, want)
			}
		}
	}
}

func TestStoreBuilderSharing(t *testing.T) {
	rs := testRegions()
	b := NewStoreBuilder()
	add := func(regions []Region, deltas []int32) [2]uint16 {
		o, i, err := b.Add(regions, deltas)
		if err != nil {
			t.Fatal(err)
		}
		return [2]uint16{o, i}
	}

	a1 := add([]Region{rs[0], rs[1]}, []int32{1, 2})
	a2 := add([]Region{rs[1], rs[0]}, []int32{2, 1})
	a3 := add([]Region{rs[0], rs[1], rs[2]}, []int32{1, 2, 0})
	if a1 != a2 || a1 != a3 {
		t.Errorf("equal rows got different indices: %v %v %v", a1, a2, a3)
	}

	seen := map[[2]uint16]bool{a1: true}
	for _, idx := range [][2]uint16{
		add([]Region{rs[0], rs[1]}, []int32{1, 3}),
		add([]Region{rs[0]}, []int32{1}),
		add([]Region{rs[0], rs[2]}, []int32{1, 2}),
		add(nil, nil),
	} {
		if seen[idx] {
			t.Errorf("distinct rows share index %v", idx)
		}
		seen[idx] = true
	}

	s := b.Build()
	if len(s.Regions) != 3 {
		t.Errorf("got %d regions, want 3", len(s.Regions))
	}
	for i, r := range s.Regions {
		for j := i + 1; j < len(s.Regions); j++ {
			if cmp.Equal(r, s.Regions[j]) {
				t.Errorf("regions %d and %d are equal", i, j)
			}
		}
	}
}

func TestStoreBuilderOverflow(t *testing.T) {
	rs := testRegions()
	const n = 0xFFFF + 10
	b := NewStoreBuilder()
	outers := make([]uint16, n)
	inners := make([]uint16, n)
	for i := range n {
		var err error
		outers[i], inners[i], err = b.Add([]Region{rs[0]}, []int32{int32(i + 1)})
		if err != nil {
			t.Fatal(err)
		}
	}
	got := roundTripStore(t, b.Build())
	if len(got.Data) != 2 {
		t.Errorf("got %d subtables, want 2", len(got.Data))
	}
	coords := []F2Dot14{0x4000, 0}
	for _, i := range []int{0, 1, 0xFFFD, 0xFFFE, 0xFFFF, 0xFFFF + 9, n - 1} {
		want := float64(i + 1)
		if have := got.Evaluate(outers[i], inners[i], coords); have != want {
			t.Errorf("row %d: got %g, want %g", i, have, want)
		}
	}
}

func TestStoreBuilderRandom(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	rs := testRegions()
	sizes := []int32{100, 30000, 3000000}
	b := NewStoreBuilder()
	var rows []testRow
	for range 300 {
		var row testRow
		for _, r := range rs {
			if rng.Intn(2) == 0 {
				continue
			}
			s := sizes[rng.Intn(3)]
			row.regions = append(row.regions, r)
			row.deltas = append(row.deltas, rng.Int31n(2*s)-s)
		}
		var err error
		row.outer, row.inner, err = b.Add(row.regions, row.deltas)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, row)
	}
	got := roundTripStore(t, b.Build())
	coords := []F2Dot14{-0x2000, 0x3000}
	for i, row := range rows {
		want := expectedDelta(row.regions, row.deltas, coords)
		if have := got.Evaluate(row.outer, row.inner, coords); math.Abs(have-want) > 1e-6 {
			t.Errorf("row %d: got %g, want %g", i, have, want)
		}
	}
}

// TestStoreBuilderTooManySubtables checks that Add fails once the store
// holds the maximal number of subtables, and that the failed call leaves
// the builder unchanged.
func TestStoreBuilderTooManySubtables(t *testing.T) {
	const nRegions = 16
	regions := make([]Region, nRegions+1)
	for i := range regions {
		regions[i] = Region{{Start: 0, Peak: F2Dot14FromFloat(float64(i+1) / 32), End: F2Dot14FromFloat(1)}}
	}

	// every subset of the first 16 regions gives a separate subtable;
	// together with the empty row this is one more than fits
	b := NewStoreBuilder()
	for set := range 1 << nRegions {
		var rr []Region
		var dd []int32
		for i := range nRegions {
			if set&(1<<i) != 0 {
				rr = append(rr, regions[i])
				dd = append(dd, 1)
			}
		}
		_, _, err := b.Add(rr, dd)
		if set < 0xFFFF && err != nil {
			t.Fatalf("subtable %d: %v", set, err)
		}
		if set == 0xFFFF && err == nil {
			t.Fatal("too many subtables accepted")
		}
	}
	before := b.Build()

	if _, _, err := b.Add(regions[nRegions:], []int32{1}); err == nil {
		t.Fatal("too many subtables accepted for a new region")
	}
	if diff := cmp.Diff(before, b.Build()); diff != "" {
		t.Errorf("failed Add changed the builder (-want +got):\n%s", diff)
	}
}

// TestStoreBuilderDeltaOverflow checks that Add rejects summed deltas which
// do not fit into an int32, and that the failed call leaves the builder
// unchanged.
func TestStoreBuilderDeltaOverflow(t *testing.T) {
	rs := testRegions()
	for _, deltas := range [][]int32{
		{math.MaxInt32, 1},
		{math.MinInt32, -1},
		{math.MaxInt32, math.MaxInt32, 2}, // sums to 2^32
	} {
		b := NewStoreBuilder()
		if _, _, err := b.Add([]Region{rs[0]}, []int32{3}); err != nil {
			t.Fatal(err)
		}
		before := b.Build()

		regions := []Region{rs[1], rs[1], rs[1]}[:len(deltas)]
		if _, _, err := b.Add(regions, deltas); err == nil {
			t.Fatalf("overflowing sum %v accepted", deltas)
		}
		if diff := cmp.Diff(before, b.Build()); diff != "" {
			t.Errorf("failed Add changed the builder (-want +got):\n%s", diff)
		}
		o, i, err := b.Add([]Region{rs[0]}, []int32{4})
		if err != nil {
			t.Fatal(err)
		}
		fresh := NewStoreBuilder()
		if _, _, err := fresh.Add([]Region{rs[0]}, []int32{3}); err != nil {
			t.Fatal(err)
		}
		fo, fi, err := fresh.Add([]Region{rs[0]}, []int32{4})
		if err != nil {
			t.Fatal(err)
		}
		if o != fo || i != fi {
			t.Errorf("indices after failed Add: got (%d,%d), want (%d,%d)", o, i, fo, fi)
		}
	}
}

// TestStoreBuilderEquivalentRegions checks that spellings of a region which
// differ only in axes which do not affect the region share one region entry.
func TestStoreBuilderEquivalentRegions(t *testing.T) {
	a := Region{{0, 0x4000, 0x4000}, {0, 0, 0}}
	c := Region{{0, 0x4000, 0x4000}, {-0x2000, 0, 0x2000}}      // zero peak
	d := Region{{0, 0x4000, 0x4000}, {0x1000, 0, 0x3000}}       // zero peak
	e := Region{{0, 0x4000, 0x4000}, {0x3000, 0x2000, 0x4000}}  // out of order
	g := Region{{0, 0x4000, 0x4000}, {-0x2000, 0x1000, 0x2000}} // crosses zero

	b := NewStoreBuilder()
	o1, i1, err := b.Add([]Region{c}, []int32{7})
	if err != nil {
		t.Fatal(err)
	}
	o2, i2, err := b.Add([]Region{a, d, e, g}, []int32{3, 1, 2, 1})
	if err != nil {
		t.Fatal(err)
	}
	s := b.Build()
	if len(s.Regions) != 1 {
		t.Errorf("got %d regions, want 1", len(s.Regions))
	}

	got := roundTripStore(t, s)
	coords := []F2Dot14{0x4000, 0x1000}
	if have := got.Evaluate(o1, i1, coords); have != 7 {
		t.Errorf("first row: got %g, want 7", have)
	}
	if have := got.Evaluate(o2, i2, coords); have != 7 {
		t.Errorf("second row: got %g, want 7", have)
	}
}

// TestStoreBuilderTooManyRegions checks that Add fails once the store holds
// the maximal number of regions, and that the failed call leaves the
// builder unchanged.
func TestStoreBuilderTooManyRegions(t *testing.T) {
	// 256*256 > 0xFFFF distinct two-axis regions
	region := func(i int) Region {
		return Region{
			{0, F2Dot14(i%256 + 1), 0x4000},
			{0, F2Dot14(i/256 + 1), 0x4000},
		}
	}
	b := NewStoreBuilder()
	for i := range 0xFFFF {
		if _, _, err := b.Add([]Region{region(i)}, []int32{1}); err != nil {
			t.Fatalf("region %d: %v", i, err)
		}
	}
	before := b.Build()

	if _, _, err := b.Add([]Region{region(0xFFFF)}, []int32{1}); err == nil {
		t.Fatal("too many regions accepted")
	}
	if diff := cmp.Diff(before, b.Build()); diff != "" {
		t.Errorf("failed Add changed the builder (-want +got):\n%s", diff)
	}
}

// TestStoreBuilderIndicesAfterBuild checks that indices returned by Add stay
// valid across Build and later calls to Add.
func TestStoreBuilderIndicesAfterBuild(t *testing.T) {
	rs := testRegions()
	b := NewStoreBuilder()
	rows := []testRow{
		{regions: []Region{rs[0]}, deltas: []int32{5}},
		{regions: []Region{rs[0], rs[1]}, deltas: []int32{-100, 3}},
	}
	more := []testRow{
		{regions: []Region{rs[0]}, deltas: []int32{6}},
		{regions: []Region{rs[2], rs[3]}, deltas: []int32{9, -70000}},
		{regions: []Region{rs[1], rs[0]}, deltas: []int32{8, 1}},
	}
	add := func(rows []testRow) {
		for i := range rows {
			var err error
			rows[i].outer, rows[i].inner, err = b.Add(rows[i].regions, rows[i].deltas)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	add(rows)
	b.Build()
	add(more)
	got := roundTripStore(t, b.Build())

	coords := []F2Dot14{-0x2000, 0x3000}
	for i, row := range append(rows, more...) {
		want := expectedDelta(row.regions, row.deltas, coords)
		if have := got.Evaluate(row.outer, row.inner, coords); math.Abs(have-want) > 1e-6 {
			t.Errorf("row %d: got %g, want %g", i, have, want)
		}
	}
}
