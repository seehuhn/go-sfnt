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

package cff

import (
	"bytes"
	"math"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/membudget"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/header"
	"seehuhn.de/go/sfnt/internal/testfonts"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/variation"
)

// cff2CmpOptions returns the cmp options used to compare two CFF2 fonts after a
// round trip: a float tolerance, a Blend normaliser (nil deltas are equivalent
// to all-zero deltas) and a per-glyph FDSelect comparer.
func cff2CmpOptions(nGlyphs int) []cmp.Option {
	floatClose := func(x, y float64) bool {
		diff := math.Abs(x - y)
		maxVal := math.Max(math.Abs(x), math.Abs(y))
		if maxVal == 0 {
			return diff < 1.0/65536
		}
		return diff < math.Max(1.0/65536, maxVal*1e-6)
	}
	return []cmp.Option{
		cmp.Comparer(func(a, b Blend) bool {
			if !floatClose(a.Default, b.Default) {
				return false
			}
			n := max(len(a.Deltas), len(b.Deltas))
			for i := range n {
				var av, bv float64
				if i < len(a.Deltas) {
					av = a.Deltas[i]
				}
				if i < len(b.Deltas) {
					bv = b.Deltas[i]
				}
				if !floatClose(av, bv) {
					return false
				}
			}
			return true
		}),
		cmp.Comparer(func(x, y float64) bool { return floatClose(x, y) }),
		cmp.Comparer(func(fn1, fn2 FDSelectFn) bool {
			for gid := range nGlyphs {
				a, b := 0, 0
				if fn1 != nil {
					a = fn1(glyph.ID(gid))
				}
				if fn2 != nil {
					b = fn2(glyph.ID(gid))
				}
				if a != b {
					return false
				}
			}
			return true
		}),
	}
}

// roundTripCFF2 reads font1 from data, writes it, reads font2, and checks that
// the two models agree and that a second write is byte-identical to the first.
func roundTripCFF2(t *testing.T, data []byte) {
	t.Helper()

	font1, err := ReadCFF2(bytes.NewReader(data), membudget.New(1<<26))
	if err != nil {
		t.Fatalf("read 1: %v", err)
	}

	var buf1 bytes.Buffer
	if err := font1.Write(&buf1); err != nil {
		t.Fatalf("write 1: %v", err)
	}

	font2, err := ReadCFF2(bytes.NewReader(buf1.Bytes()), membudget.New(1<<26))
	if err != nil {
		t.Fatalf("read 2: %v", err)
	}

	opts := cff2CmpOptions(font1.NumGlyphs())
	if diff := cmp.Diff(font1, font2, opts...); diff != "" {
		t.Errorf("model round trip (-first +second):\n%s", diff)
	}

	var buf2 bytes.Buffer
	if err := font2.Write(&buf2); err != nil {
		t.Fatalf("write 2: %v", err)
	}
	if !bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
		t.Errorf("write not a byte fixpoint: %d vs %d bytes", buf1.Len(), buf2.Len())
	}
}

// twoSubtableStore builds a store with two IVD subtables, each selecting two
// regions, so that vsindex values 0 and 1 both resolve to k=2.
func twoSubtableStore() []byte {
	f := variation.F2Dot14FromFloat
	store := &variation.ItemVariationStore{
		Regions: []variation.Region{
			{{Start: f(-1), Peak: f(-1), End: f(0)}},
			{{Start: f(0), Peak: f(1), End: f(1)}},
		},
		Data: []*variation.ItemVariationData{
			{RegionIndexes: []uint16{0, 1}, Deltas: [][]int32{}},
			{RegionIndexes: []uint16{0, 1}, Deltas: [][]int32{}},
		},
	}
	return store.Encode()
}

func TestWriteCFF2Minimal(t *testing.T) {
	roundTripCFF2(t, buildCFF2(&cff2Spec{
		charStrings: cffIndex{
			cs(0, 0, t2rmoveto, 100, 0, t2rlineto, 50, 100, t2rlineto),
		},
	}))
}

func TestWriteCFF2FontMatrix(t *testing.T) {
	roundTripCFF2(t, buildCFF2(&cff2Spec{
		fontMatrix:  &[6]float64{0.0005, 0, 0, 0.0005, 0, 0},
		charStrings: cffIndex{cs(0, 0, t2rmoveto, 100, 0, t2rlineto)},
	}))
}

func TestWriteCFF2Blended(t *testing.T) {
	roundTripCFF2(t, buildCFF2(&cff2Spec{
		vstore: twoRegionStore(),
		charStrings: cffIndex{
			cs(0, 0, t2rmoveto),
			cs(100, 200, 10, 20, 30, 40, 2, t2blend, t2rmoveto),
		},
	}))
}

// TestWriteCFF2AllFamilies exercises blends across moveto, lineto, curveto and
// stem-hint operators, plus a hintmask.
func TestWriteCFF2AllFamilies(t *testing.T) {
	roundTripCFF2(t, buildCFF2(&cff2Spec{
		vstore: twoRegionStore(),
		charStrings: cffIndex{
			// notdef
			cs(0, 0, t2rmoveto),
			// blended moveto
			cs(100, 200, 10, 20, 30, 40, 2, t2blend, t2rmoveto),
			// blended lineto mixed with plain operands
			cs(0, 0, t2rmoveto, 10, 10, 20, 30, 1, 2, 3, 4, 2, t2blend, t2rlineto),
			// all-six blended curveto
			cs(0, 0, t2rmoveto,
				10, 10, 10, 10, 10, 10,
				1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
				6, t2blend, t2rrcurveto),
			// stems + blend + hintmask
			cs(0, 100, 200, 40, 10, 20, 30, 40, 2, t2blend, t2hstemhm, t2hintmask, byte(0x80)),
			// various curve shapes (decode to OpCurveTo), no blend
			cs(0, 0, t2rmoveto, 10, 20, 30, 40, t2hvcurveto),
			// implicit vstem before hintmask
			cs(0, 10, t2hstem, 20, 30, t2hintmask, byte(0xc0), 5, 5, t2rmoveto),
		},
	}))
}

// TestWriteCFF2VSIndex covers a glyph selecting a non-default variation store
// subtable via the vsindex operator.
func TestWriteCFF2VSIndex(t *testing.T) {
	roundTripCFF2(t, buildCFF2(&cff2Spec{
		vstore: twoSubtableStore(),
		charStrings: cffIndex{
			cs(0, 0, t2rmoveto),
			cs(1, t2vsindex, 0, 0, t2rmoveto, 5, 7, 8, 1, t2blend, 0, t2rlineto),
		},
	}))
}

// TestWriteCFF2TwoFDs covers multiple Font DICTs, an FDSelect table and a
// per-FD font matrix.
func TestWriteCFF2TwoFDs(t *testing.T) {
	callSubr0 := []byte{byte(-107 + 139)}
	fdSelect3 := []byte{
		3,
		0, 2,
		0, 0, 0,
		0, 1, 1,
		0, 2,
	}
	roundTripCFF2(t, buildCFF2(&cff2Spec{
		fds: []fdSpec{
			{
				privateBody: append(dictNum(10), 10),
				subrs:       cffIndex{cs(50, 0, t2rlineto)},
			},
			{
				privateBody: append(dictNum(20), 10),
				subrs:       cffIndex{cs(0, 60, t2rlineto)},
				fontMatrix:  &[6]float64{0.002, 0, 0, 0.002, 0, 0},
			},
		},
		fdselect: fdSelect3,
		charStrings: cffIndex{
			append(cs(0, 0, t2rmoveto), append(callSubr0, byte(t2callsubr))...),
			append(cs(10, 10, t2rmoveto), append(callSubr0, byte(t2callsubr))...),
		},
	}))
}

// TestWriteCFF2PrivateBlends round-trips a blended private DICT (blue values
// and scalar hints that vary).
func TestWriteCFF2PrivateBlends(t *testing.T) {
	// Private DICT body: BlueValues (op 6) with two blended entries, and a
	// blended StdHW (op 10).
	var body bytes.Buffer
	// BlueValues: defaults 0, 100 (delta-encoded 0, 100); each with deltas.
	// element 0: default 0, deltas 1,2 ; element 1: rel default 100, deltas 3,4
	body.Write(cs(0, 100, 1, 2, 3, 4, 2, byte(23))) // 23 = blend
	body.WriteByte(6)                               // BlueValues
	// StdHW blended: default 50, deltas 5,6
	body.Write(cs(50, 5, 6, 1, byte(23)))
	body.WriteByte(10) // StdHW

	roundTripCFF2(t, buildCFF2(&cff2Spec{
		vstore: twoRegionStore(),
		fds:    []fdSpec{{privateBody: body.Bytes()}},
		charStrings: cffIndex{
			cs(0, 0, t2rmoveto, 100, 0, t2rlineto),
		},
	}))
}

// TestWriteCFF2FDSelectFormats checks that a font whose glyphs alternate
// between two Font DICTs round-trips regardless of the chosen FDSelect format.
func TestWriteCFF2FDSelectFormats(t *testing.T) {
	const n = 8
	var ranges bytes.Buffer
	ranges.Write([]byte{0, byte(n)}) // nRanges placeholder recomputed below
	nSeg := 0
	prevFD := -1
	var body []byte
	for i := range n {
		fd := i % 2
		if fd != prevFD {
			body = append(body, byte(i>>8), byte(i), byte(fd))
			nSeg++
			prevFD = fd
		}
	}
	fdSelect := []byte{3, byte(nSeg >> 8), byte(nSeg)}
	fdSelect = append(fdSelect, body...)
	fdSelect = append(fdSelect, byte(n>>8), byte(n))

	charStrings := make(cffIndex, n)
	for i := range n {
		charStrings[i] = cs(0, 0, t2rmoveto, 10, 0, t2rlineto)
	}
	roundTripCFF2(t, buildCFF2(&cff2Spec{
		fds: []fdSpec{
			{privateBody: append(dictNum(10), 10)},
			{privateBody: append(dictNum(20), 10)},
		},
		fdselect:    fdSelect,
		charStrings: charStrings,
	}))
}

// TestWriteCFF2AdobeVF round-trips the CFF2 table of the Adobe Variable Font
// Prototype, gated on the external test font.
func TestWriteCFF2AdobeVF(t *testing.T) {
	path := testfonts.Path(t, "AdobeVFPrototype.otf")
	fd, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer fd.Close()

	info, err := header.Read(fd)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Has("CFF2") {
		t.Fatal("font has no CFF2 table")
	}
	r, err := info.TableReader(fd, "CFF2")
	if err != nil {
		t.Fatal(err)
	}

	font1, err := ReadCFF2(r, membudget.New(1<<28))
	if err != nil {
		t.Fatal(err)
	}

	var buf1 bytes.Buffer
	if err := font1.Write(&buf1); err != nil {
		t.Fatalf("write 1: %v", err)
	}
	font2, err := ReadCFF2(bytes.NewReader(buf1.Bytes()), membudget.New(1<<28))
	if err != nil {
		t.Fatalf("read 2: %v", err)
	}

	opts := cff2CmpOptions(font1.NumGlyphs())
	if diff := cmp.Diff(font1, font2, opts...); diff != "" {
		t.Errorf("model round trip (-first +second):\n%s", diff)
	}

	var buf2 bytes.Buffer
	if err := font2.Write(&buf2); err != nil {
		t.Fatalf("write 2: %v", err)
	}
	if !bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
		t.Errorf("write not a byte fixpoint: %d vs %d bytes", buf1.Len(), buf2.Len())
	}
}

func FuzzCFF2(f *testing.F) {
	f.Add(buildCFF2(&cff2Spec{
		charStrings: cffIndex{cs(0, 0, t2rmoveto, 100, 0, t2rlineto)},
	}))
	f.Add(buildCFF2(&cff2Spec{
		vstore: twoRegionStore(),
		charStrings: cffIndex{
			cs(0, 0, t2rmoveto),
			cs(100, 200, 10, 20, 30, 40, 2, t2blend, t2rmoveto),
		},
	}))
	f.Add(buildCFF2(&cff2Spec{
		vstore: twoRegionStore(),
		charStrings: cffIndex{
			cs(0, 0, t2rmoveto),
			cs(0, 0, t2rmoveto, 10, 10, 20, 30, 1, 2, 3, 4, 2, t2blend, t2rlineto),
			cs(0, 100, 200, 40, 10, 20, 30, 40, 2, t2blend, t2hstemhm, t2hintmask, byte(0x80)),
		},
	}))
	f.Add(buildCFF2(&cff2Spec{
		vstore: twoSubtableStore(),
		charStrings: cffIndex{
			cs(0, 0, t2rmoveto),
			cs(1, t2vsindex, 0, 0, t2rmoveto, 5, 7, 8, 1, t2blend, 0, t2rlineto),
		},
	}))

	f.Fuzz(func(t *testing.T, data []byte) {
		font1, err := ReadCFF2(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		if err != nil {
			return
		}

		var buf1 bytes.Buffer
		if err := font1.Write(&buf1); err != nil {
			t.Fatalf("write: %v", err)
		}

		font2, err := ReadCFF2(bytes.NewReader(buf1.Bytes()), parser.NewBudget(int64(buf1.Len())+16))
		if err != nil {
			t.Fatalf("re-read: %v", err)
		}

		opts := cff2CmpOptions(font1.NumGlyphs())
		if diff := cmp.Diff(font1, font2, opts...); diff != "" {
			t.Errorf("model round trip (-first +second):\n%s", diff)
		}

		var buf2 bytes.Buffer
		if err := font2.Write(&buf2); err != nil {
			t.Fatalf("re-write: %v", err)
		}
		if !bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
			t.Errorf("write not a byte fixpoint")
		}
	})
}
