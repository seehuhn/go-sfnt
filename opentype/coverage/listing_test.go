// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package coverage

import (
	"bytes"
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
)

// be returns the big-endian encoding of a sequence of 16-bit values.
func be(vals ...uint16) []byte {
	res := make([]byte, 0, 2*len(vals))
	for _, v := range vals {
		res = append(res, byte(v>>8), byte(v))
	}
	return res
}

func readListing(t *testing.T, data []byte) *Listing {
	t.Helper()
	l, err := ReadListing(parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data)))), 0)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// sameMap reports whether a and b are the same map.
func sameMap(a, b Table) bool {
	return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
}

func TestAlign(t *testing.T) {
	type testCase struct {
		name     string
		cov      []byte
		data     []string
		wantCov  Table
		wantData []string
	}
	cases := []testCase{
		{
			name:     "format 1",
			cov:      be(1, 3, 5, 6, 9),
			data:     []string{"a", "b", "c"},
			wantCov:  Table{5: 0, 6: 1, 9: 2},
			wantData: []string{"a", "b", "c"},
		},
		{
			name:     "format 2",
			cov:      be(2, 2, 5, 6, 0, 9, 9, 2),
			data:     []string{"a", "b", "c"},
			wantCov:  Table{5: 0, 6: 1, 9: 2},
			wantData: []string{"a", "b", "c"},
		},
		{
			name:     "no glyphs",
			cov:      be(1, 0),
			data:     []string{},
			wantCov:  Table{},
			wantData: []string{},
		},
		{
			name:     "more glyphs than data",
			cov:      be(1, 3, 5, 6, 9),
			data:     []string{"a", "b"},
			wantCov:  Table{5: 0, 6: 1},
			wantData: []string{"a", "b"},
		},
		{
			name:     "more data than glyphs",
			cov:      be(2, 1, 5, 6, 0),
			data:     []string{"a", "b", "c"},
			wantCov:  Table{5: 0, 6: 1},
			wantData: []string{"a", "b"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := readListing(t, tc.cov)
			cov, data := Align(l, tc.data)
			if diff := cmp.Diff(tc.wantCov, cov); diff != "" {
				t.Errorf("wrong coverage (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tc.wantData, data); diff != "" {
				t.Errorf("wrong data (-want +got):\n%s", diff)
			}
		})
	}
}

// TestAlignDamaged checks, for coverage tables which a valid font cannot
// have, that the glyphs which are listed once get the data stored at their
// coverage index.  The glyphs in optional are listed more than once, and
// the test does not check their data.
func TestAlignDamaged(t *testing.T) {
	data := []string{"a", "b", "c", "d", "e", "f"}
	cases := []struct {
		name     string
		cov      []byte
		want     map[glyph.ID]string
		optional []glyph.ID
	}{
		{
			name:     "duplicate glyph",
			cov:      be(1, 3, 5, 5, 7),
			want:     map[glyph.ID]string{7: "c"},
			optional: []glyph.ID{5},
		},
		{
			name: "glyphs out of order",
			cov:  be(1, 3, 9, 5, 7),
			want: map[glyph.ID]string{9: "a", 5: "b", 7: "c"},
		},
		{
			name:     "overlapping ranges",
			cov:      be(2, 2, 5, 7, 0, 7, 9, 3),
			want:     map[glyph.ID]string{5: "a", 6: "b", 8: "e", 9: "f"},
			optional: []glyph.ID{7},
		},
		{
			name: "ranges out of order",
			cov:  be(2, 2, 8, 9, 0, 5, 6, 2),
			want: map[glyph.ID]string{8: "a", 9: "b", 5: "c", 6: "d"},
		},
		{
			name: "start index too large",
			cov:  be(2, 2, 5, 6, 1, 8, 9, 4),
			want: map[glyph.ID]string{5: "b", 6: "c", 8: "e", 9: "f"},
		},
		{
			name: "start index too small",
			cov:  be(2, 2, 5, 6, 0, 8, 9, 1),
			want: map[glyph.ID]string{5: "a", 6: "b", 8: "b", 9: "c"},
		},
		{
			name: "index beyond the data",
			cov:  be(2, 2, 5, 6, 0, 8, 9, 5),
			want: map[glyph.ID]string{5: "a", 6: "b", 8: "f"},
		},
		{
			name: "empty range",
			cov:  be(2, 2, 6, 5, 0, 8, 9, 0),
			want: map[glyph.ID]string{8: "a", 9: "b"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cov, items := Align(readListing(t, tc.cov), data)
			if !cov.Valid() {
				t.Fatalf("invalid coverage table %v", cov)
			}
			if len(items) != len(cov) {
				t.Fatalf("%d items for %d glyphs", len(items), len(cov))
			}
			for gid, want := range tc.want {
				idx, ok := cov[gid]
				if !ok {
					t.Errorf("glyph %d is not covered", gid)
				} else if items[idx] != want {
					t.Errorf("glyph %d: got %q, want %q", gid, items[idx], want)
				}
			}
			for gid := range cov {
				if _, ok := tc.want[gid]; !ok && !slices.Contains(tc.optional, gid) {
					t.Errorf("unexpected glyph %d", gid)
				}
			}

			// the encoded table is read back unchanged
			again, items2 := Align(readListing(t, cov.Encode()), items)
			if diff := cmp.Diff(cov, again); diff != "" {
				t.Errorf("round trip failed (-first +second):\n%s", diff)
			}
			if diff := cmp.Diff(items, items2); diff != "" {
				t.Errorf("round trip failed (-first +second):\n%s", diff)
			}
		})
	}
}

// TestAlignShares checks that a listing which needs no repair gives the same
// table to all its users, and that a user which needs a different table
// does not change the listing.
func TestAlignShares(t *testing.T) {
	l := readListing(t, be(1, 3, 5, 6, 9))
	data := []string{"a", "b", "c", "d"}

	cov1, data1 := Align(l, data)
	if !sameMap(cov1, l.Table()) {
		t.Error("table is not shared")
	}
	if len(data1) != 3 || &data1[0] != &data[0] {
		t.Error("data is not shared")
	}

	cov2, _ := Align(l, data[:2])
	if len(cov2) != 2 {
		t.Errorf("wrong coverage %v", cov2)
	}
	if diff := cmp.Diff(Table{5: 0, 6: 1, 9: 2}, l.Table()); diff != "" {
		t.Errorf("listing modified (-want +got):\n%s", diff)
	}
	if !sameMap(cov1, l.Table()) {
		t.Error("table replaced")
	}
}

func TestListing(t *testing.T) {
	l := readListing(t, be(2, 2, 5, 6, 0, 9, 9, 2))
	if diff := cmp.Diff(Table{5: 0, 6: 1, 9: 2}, l.Table()); diff != "" {
		t.Errorf("wrong table (-want +got):\n%s", diff)
	}
	if got := l.DataLen(); got != 3 {
		t.Errorf("got data length %d, want 3", got)
	}
	all := maps.Collect(l.All())
	if diff := cmp.Diff(map[glyph.ID]int{5: 0, 6: 1, 9: 2}, all); diff != "" {
		t.Errorf("wrong indices (-want +got):\n%s", diff)
	}
}

// FuzzAlign checks that every glyph of a listing is paired with the data
// item at its coverage index.
func FuzzAlign(f *testing.F) {
	f.Add(be(1, 3, 5, 6, 9), uint16(3))
	f.Add(be(1, 3, 5, 5, 7), uint16(3))
	f.Add(be(1, 3, 9, 5, 7), uint16(2))
	f.Add(be(2, 2, 5, 6, 0, 9, 9, 2), uint16(3))
	f.Add(be(2, 2, 5, 7, 0, 7, 9, 3), uint16(6))
	f.Add(be(2, 2, 5, 6, 0, 8, 9, 1), uint16(2))
	f.Add(be(2, 2, 6, 5, 0, 8, 9, 7), uint16(8))
	f.Fuzz(func(t *testing.T, data []byte, n uint16) {
		l, err := ReadListing(parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data)))), 0)
		if err != nil {
			return
		}
		items := make([]int, n)
		for i := range items {
			items[i] = i
		}

		cov, aligned := Align(l, items)
		if !cov.Valid() {
			t.Fatalf("invalid coverage table %v", cov)
		}
		if len(aligned) != len(cov) {
			t.Fatalf("%d items for %d glyphs", len(aligned), len(cov))
		}
		numGlyphs := 0
		for gid, idx := range l.All() {
			if idx >= l.DataLen() {
				t.Errorf("glyph %d: index %d beyond data length %d", gid, idx, l.DataLen())
			}
			if idx >= len(items) {
				if cov.Contains(gid) {
					t.Errorf("glyph %d has no data but is covered", gid)
				}
				continue
			}
			numGlyphs++
			if i, ok := cov[gid]; !ok {
				t.Errorf("glyph %d is not covered", gid)
			} else if aligned[i] != idx {
				t.Errorf("glyph %d: got item %d, want %d", gid, aligned[i], idx)
			}
		}
		if numGlyphs != len(cov) {
			t.Errorf("%d glyphs covered, want %d", len(cov), numGlyphs)
		}
		if !l.Table().Valid() {
			t.Errorf("invalid table %v", l.Table())
		}
	})
}
