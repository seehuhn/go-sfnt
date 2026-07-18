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
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/membudget"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/sfnt/header"
	"seehuhn.de/go/sfnt/internal/testfonts"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/variation"
)

// --- test-font builder ---------------------------------------------------

// fdSpec describes one Font DICT of a hand-built CFF2 font.
type fdSpec struct {
	privateBody []byte      // pre-encoded Private DICT operators (no Subrs op)
	subrs       cffIndex    // local subrs; nil means none
	fontMatrix  *[6]float64 // per-FD matrix; nil means absent
}

// cff2Spec describes a CFF2 font to assemble.
type cff2Spec struct {
	fontMatrix      *[6]float64
	gsubr           cffIndex
	vstore          []byte // ItemVariationStore.Encode() bytes; nil means none
	fds             []fdSpec
	fdselect        []byte // raw FDSelect table bytes; nil means none
	charStrings     cffIndex
	omitCharStrings bool
}

// dictOff encodes v as a fixed-width 5-byte DICT integer (op 29), so that a
// DICT's length does not depend on the offset value.
func dictOff(v int) []byte {
	return []byte{29, byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
}

func dictNum(v int32) []byte {
	var b bytes.Buffer
	encodeDictNumber(&b, v)
	return b.Bytes()
}

func writeUint16(b *bytes.Buffer, v int) {
	b.Write([]byte{byte(v >> 8), byte(v)})
}

func buildTopDict(spec *cff2Spec, cs, vs, fda, fds int) []byte {
	var b bytes.Buffer
	if spec.fontMatrix != nil {
		for _, v := range spec.fontMatrix {
			encodeDictNumber(&b, v)
		}
		b.Write([]byte{12, 7})
	}
	if !spec.omitCharStrings {
		b.Write(dictOff(cs))
		b.WriteByte(17)
	}
	if spec.vstore != nil {
		b.Write(dictOff(vs))
		b.WriteByte(24)
	}
	if len(spec.fds) > 0 {
		b.Write(dictOff(fda))
		b.Write([]byte{12, 36})
	}
	if spec.fdselect != nil {
		b.Write(dictOff(fds))
		b.Write([]byte{12, 37})
	}
	return b.Bytes()
}

func buildPrivateDict(body []byte, hasSubrs bool, subrsOff int) []byte {
	var b bytes.Buffer
	b.Write(body)
	if hasSubrs {
		b.Write(dictOff(subrsOff))
		b.WriteByte(19) // Subrs
	}
	return b.Bytes()
}

func buildFontDict(fm *[6]float64, privSize, privOff int) []byte {
	var b bytes.Buffer
	if fm != nil {
		for _, v := range fm {
			encodeDictNumber(&b, v)
		}
		b.Write([]byte{12, 7})
	}
	b.Write(dictOff(privSize))
	b.Write(dictOff(privOff))
	b.WriteByte(18) // Private
	return b.Bytes()
}

// buildCFF2 assembles a complete CFF2 font from spec.
func buildCFF2(spec *cff2Spec) []byte {
	L := len(buildTopDict(spec, 0, 0, 0, 0))
	base := 5 + L

	var trailer bytes.Buffer
	off := func() int { return base + trailer.Len() }

	// global subr INDEX, immediately after the top DICT
	trailer.Write(spec.gsubr.encode32())

	vstoreOff := 0
	if spec.vstore != nil {
		vstoreOff = off()
		writeUint16(&trailer, len(spec.vstore))
		trailer.Write(spec.vstore)
	}

	fdArrayOff := 0
	if n := len(spec.fds); n > 0 {
		privLen := make([]int, n)
		for i := range spec.fds {
			privLen[i] = len(buildPrivateDict(spec.fds[i].privateBody, spec.fds[i].subrs != nil, 0))
		}
		privOff := make([]int, n)
		cur := off()
		for i := range n {
			privOff[i] = cur
			cur += privLen[i]
		}
		subrOff := make([]int, n)
		for i := range n {
			if spec.fds[i].subrs != nil {
				subrOff[i] = cur
				cur += len(spec.fds[i].subrs.encode32())
			}
		}
		// private dicts
		for i := range n {
			so := 0
			if spec.fds[i].subrs != nil {
				so = subrOff[i] - privOff[i]
			}
			trailer.Write(buildPrivateDict(spec.fds[i].privateBody, spec.fds[i].subrs != nil, so))
		}
		// local subrs
		for i := range n {
			if spec.fds[i].subrs != nil {
				trailer.Write(spec.fds[i].subrs.encode32())
			}
		}
		// FDArray INDEX
		fdBlobs := make(cffIndex, n)
		for i := range n {
			fdBlobs[i] = buildFontDict(spec.fds[i].fontMatrix, privLen[i], privOff[i])
		}
		fdArrayOff = off()
		trailer.Write(fdBlobs.encode32())
	}

	fdSelectOff := 0
	if spec.fdselect != nil {
		fdSelectOff = off()
		trailer.Write(spec.fdselect)
	}

	charStringsOff := off()
	trailer.Write(spec.charStrings.encode32())

	top := buildTopDict(spec, charStringsOff, vstoreOff, fdArrayOff, fdSelectOff)
	if len(top) != L {
		panic("top dict length not stable")
	}

	var out bytes.Buffer
	out.Write([]byte{2, 0, 5, byte(L >> 8), byte(L)})
	out.Write(top)
	out.Write(trailer.Bytes())
	return out.Bytes()
}

func readCFF2Bytes(t *testing.T, data []byte) (*FontCFF2, error) {
	t.Helper()
	return ReadCFF2(bytes.NewReader(data), membudget.New(1<<20))
}

// twoRegionStore builds an item variation store with a single IVD subtable
// selecting two regions (k=2 for vsindex 0).
func twoRegionStore() []byte {
	f := variation.F2Dot14FromFloat
	store := &variation.ItemVariationStore{
		Regions: []variation.Region{
			{{Start: f(-1), Peak: f(-1), End: f(0)}},
			{{Start: f(0), Peak: f(1), End: f(1)}},
		},
		Data: []*variation.ItemVariationData{
			{RegionIndexes: []uint16{0, 1}, Deltas: [][]int32{}},
		},
	}
	return store.Encode()
}

// --- tests ---------------------------------------------------------------

// TestReadCFF2Minimal reads a one-glyph, non-variable font with no FDArray;
// the reader must synthesise a single default Font DICT.
func TestReadCFF2Minimal(t *testing.T) {
	spec := &cff2Spec{
		charStrings: cffIndex{
			cs(0, 0, t2rmoveto, 100, 0, t2rlineto, 50, 100, t2rlineto),
		},
	}
	font, err := readCFF2Bytes(t, buildCFF2(spec))
	if err != nil {
		t.Fatal(err)
	}

	if font.NumGlyphs() != 1 {
		t.Errorf("NumGlyphs = %d, want 1", font.NumGlyphs())
	}
	if font.FontMatrix != defaultFontMatrix {
		t.Errorf("FontMatrix = %v, want %v", font.FontMatrix, defaultFontMatrix)
	}
	if len(font.Private) != 1 {
		t.Fatalf("len(Private) = %d, want 1", len(font.Private))
	}
	if font.VarStore != nil {
		t.Error("VarStore should be nil")
	}
	if font.FDSelect(0) != 0 {
		t.Error("FDSelect(0) should be 0")
	}
	want := &GlyphCFF2{
		Cmds: []GlyphOpCFF2{
			{Op: OpMoveTo, Args: []Blend{{Default: 0}, {Default: 0}}},
			{Op: OpLineTo, Args: []Blend{{Default: 100}, {Default: 0}}},
			{Op: OpLineTo, Args: []Blend{{Default: 150}, {Default: 100}}},
		},
	}
	if diff := cmp.Diff(want, font.Glyphs[0]); diff != "" {
		t.Errorf("glyph 0 mismatch (-want +got):\n%s", diff)
	}
}

// TestReadCFF2Blended reads a two-glyph variable font whose second glyph uses
// a blended moveto, and checks the blend values against a hand computation.
func TestReadCFF2Blended(t *testing.T) {
	spec := &cff2Spec{
		vstore: twoRegionStore(),
		charStrings: cffIndex{
			cs(0, 0, t2rmoveto),
			// bases 100,200; deltas 10,20 / 30,40; n=2; blend; rmoveto
			cs(100, 200, 10, 20, 30, 40, 2, t2blend, t2rmoveto),
		},
	}
	font, err := readCFF2Bytes(t, buildCFF2(spec))
	if err != nil {
		t.Fatal(err)
	}
	if font.NumGlyphs() != 2 {
		t.Fatalf("NumGlyphs = %d, want 2", font.NumGlyphs())
	}
	if font.VarStore == nil {
		t.Fatal("VarStore should not be nil")
	}

	want := &GlyphCFF2{
		Cmds: []GlyphOpCFF2{
			{Op: OpMoveTo, Args: []Blend{
				{Default: 100, Deltas: []float64{10, 20}},
				{Default: 200, Deltas: []float64{30, 40}},
			}},
		},
	}
	if diff := cmp.Diff(want, font.Glyphs[1]); diff != "" {
		t.Errorf("glyph 1 mismatch (-want +got):\n%s", diff)
	}

	// hand-computed blend at scalars (0.5, 0.25)
	scalars := []float64{0.5, 0.25}
	gotX := font.Glyphs[1].Cmds[0].Args[0].At(scalars)
	wantX := 100.0 + 0.5*10 + 0.25*20
	if gotX != wantX {
		t.Errorf("blended X = %v, want %v", gotX, wantX)
	}
	gotY := font.Glyphs[1].Cmds[0].Args[1].At(scalars)
	wantY := 200.0 + 0.5*30 + 0.25*40
	if gotY != wantY {
		t.Errorf("blended Y = %v, want %v", gotY, wantY)
	}
}

// TestReadCFF2TwoFDs reads a font with two Font DICTs, FDSelect format 3, and
// per-FD private dicts and local subroutines.
func TestReadCFF2TwoFDs(t *testing.T) {
	// each FD's local subr 0 draws one line; callsubr 0 -> biased -107
	callSubr0 := []byte{byte(-107 + 139)}
	fdSelect3 := []byte{
		3,    // format
		0, 2, // nRanges
		0, 0, 0, // range 0: first=0, fd=0
		0, 1, 1, // range 1: first=1, fd=1
		0, 2, // sentinel = nGlyphs
	}
	spec := &cff2Spec{
		fds: []fdSpec{
			{
				privateBody: append(dictNum(10), 10), // StdHW = 10 (op 10)
				subrs:       cffIndex{cs(50, 0, t2rlineto)},
			},
			{
				privateBody: append(dictNum(20), 10), // StdHW = 20
				subrs:       cffIndex{cs(0, 60, t2rlineto)},
				fontMatrix:  &[6]float64{0.002, 0, 0, 0.002, 0, 0},
			},
		},
		fdselect: fdSelect3,
		charStrings: cffIndex{
			append(cs(0, 0, t2rmoveto), append(callSubr0, byte(t2callsubr))...),
			append(cs(10, 10, t2rmoveto), append(callSubr0, byte(t2callsubr))...),
		},
	}
	font, err := readCFF2Bytes(t, buildCFF2(spec))
	if err != nil {
		t.Fatal(err)
	}
	if len(font.Private) != 2 {
		t.Fatalf("len(Private) = %d, want 2", len(font.Private))
	}
	if font.Private[0].StdHW.Default != 10 || font.Private[1].StdHW.Default != 20 {
		t.Errorf("StdHW = %v/%v, want 10/20",
			font.Private[0].StdHW.Default, font.Private[1].StdHW.Default)
	}
	if font.FDSelect(0) != 0 || font.FDSelect(1) != 1 {
		t.Errorf("FDSelect = %d/%d, want 0/1", font.FDSelect(0), font.FDSelect(1))
	}
	wantFM := matrix.Matrix{0.002, 0, 0, 0.002, 0, 0}
	if font.FontMatrices[1] != wantFM {
		t.Errorf("FontMatrices[1] = %v, want %v", font.FontMatrices[1], wantFM)
	}
	if font.FontMatrices[0] != matrix.Identity {
		t.Errorf("FontMatrices[0] = %v, want identity", font.FontMatrices[0])
	}

	want0 := []GlyphOpCFF2{
		{Op: OpMoveTo, Args: []Blend{{Default: 0}, {Default: 0}}},
		{Op: OpLineTo, Args: []Blend{{Default: 50}, {Default: 0}}},
	}
	if diff := cmp.Diff(want0, font.Glyphs[0].Cmds); diff != "" {
		t.Errorf("glyph 0 mismatch (-want +got):\n%s", diff)
	}
	want1 := []GlyphOpCFF2{
		{Op: OpMoveTo, Args: []Blend{{Default: 10}, {Default: 10}}},
		{Op: OpLineTo, Args: []Blend{{Default: 10}, {Default: 70}}},
	}
	if diff := cmp.Diff(want1, font.Glyphs[1].Cmds); diff != "" {
		t.Errorf("glyph 1 mismatch (-want +got):\n%s", diff)
	}
}

func TestReadCFF2Malformed(t *testing.T) {
	// major != 2
	t.Run("wrong major", func(t *testing.T) {
		data := buildCFF2(&cff2Spec{charStrings: cffIndex{cs(0, 0, t2rmoveto)}})
		data[0] = 1
		if _, err := readCFF2Bytes(t, data); err == nil {
			t.Error("expected error")
		}
	})

	// missing CharStrings
	t.Run("missing CharStrings", func(t *testing.T) {
		spec := &cff2Spec{
			omitCharStrings: true,
			charStrings:     cffIndex{cs(0, 0, t2rmoveto)},
		}
		if _, err := readCFF2Bytes(t, buildCFF2(spec)); err == nil {
			t.Error("expected error")
		}
	})

	// blend without a variation store
	t.Run("blend without vstore", func(t *testing.T) {
		spec := &cff2Spec{
			charStrings: cffIndex{
				cs(100, 200, 10, 20, 30, 40, 2, t2blend, t2rmoveto),
			},
		}
		if _, err := readCFF2Bytes(t, buildCFF2(spec)); err == nil {
			t.Error("expected error")
		}
	})

	// bad FDSelect gid range (first range does not start at 0)
	t.Run("bad FDSelect range", func(t *testing.T) {
		bad := []byte{3, 0, 1, 0, 1, 0, 0, 2}
		spec := &cff2Spec{
			fds:      []fdSpec{{privateBody: append(dictNum(10), 10)}},
			fdselect: bad,
			charStrings: cffIndex{
				cs(0, 0, t2rmoveto),
				cs(0, 0, t2rmoveto),
			},
		}
		if _, err := readCFF2Bytes(t, buildCFF2(spec)); err == nil {
			t.Error("expected error")
		}
	})

	// too many Font DICTs (FDArray count 300)
	t.Run("FD count 300", func(t *testing.T) {
		data := buildTooManyFDs(300)
		if _, err := readCFF2Bytes(t, data); err == nil {
			t.Error("expected error")
		}
	})

	// truncated header
	t.Run("truncated", func(t *testing.T) {
		if _, err := readCFF2Bytes(t, []byte{2, 0}); err == nil {
			t.Error("expected error")
		}
	})
}

// buildTooManyFDs hand-crafts a CFF2 whose top DICT points at an FDArray
// INDEX claiming n (empty) Font DICTs, to exercise the FD-count cap before
// any Font DICT is decoded.
func buildTooManyFDs(n int) []byte {
	// FDArray INDEX with n empty objects: count(4) + offSize(1) + (n+1) offsets
	var fdArray bytes.Buffer
	fdArray.Write([]byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)})
	fdArray.WriteByte(1) // offSize
	for range n + 1 {
		fdArray.WriteByte(1) // all offsets = 1 -> empty objects
	}

	charStrings := cffIndex{cs(0, 0, t2rmoveto)}.encode32()
	gsubr := cffIndex(nil).encode32()

	// layout: header(5) + top + gsubr + fdArray + charStrings
	spec := &cff2Spec{
		fds:         []fdSpec{{}}, // presence flag so the top DICT has FDArray+CharStrings ops
		charStrings: cffIndex{cs(0, 0, t2rmoveto)},
	}
	L := len(buildTopDict(spec, 0, 0, 0, 0))
	base := 5 + L
	gsubrOff := base
	fdArrayOff := gsubrOff + len(gsubr)
	charStringsOff := fdArrayOff + fdArray.Len()

	top := buildTopDict(spec, charStringsOff, 0, fdArrayOff, 0)

	var out bytes.Buffer
	out.Write([]byte{2, 0, 5, byte(L >> 8), byte(L)})
	out.Write(top)
	out.Write(gsubr)
	out.Write(fdArray.Bytes())
	out.Write(charStrings)
	_ = charStrings
	return out.Bytes()
}

// TestReadCFF2MemoryBound verifies a tiny budget causes a bounded failure
// rather than an unbounded allocation.
func TestReadCFF2MemoryBound(t *testing.T) {
	data := buildCFF2(&cff2Spec{
		charStrings: cffIndex{cs(0, 0, t2rmoveto, 100, 0, t2rlineto)},
	})
	// a generous budget succeeds
	if _, err := ReadCFF2(bytes.NewReader(data), membudget.New(1<<20)); err != nil {
		t.Fatalf("generous budget: %v", err)
	}
	// a one-byte budget must fail
	if _, err := ReadCFF2(bytes.NewReader(data), membudget.New(1)); err == nil {
		t.Error("tiny budget: expected error")
	}
}

// TestReadCFF2AdobeVF reads the CFF2 table of the Adobe Variable Font
// Prototype, gated on the external test font being available.
func TestReadCFF2AdobeVF(t *testing.T) {
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

	font, err := ReadCFF2(r, membudget.New(1<<26))
	if err != nil {
		t.Fatal(err)
	}
	if font.NumGlyphs() == 0 {
		t.Fatal("no glyphs")
	}
	if font.VarStore == nil {
		t.Error("expected a variation store")
	}
	nonEmpty := 0
	for _, g := range font.Glyphs {
		if g != nil && len(g.Cmds) > 0 {
			nonEmpty++
		}
	}
	if nonEmpty == 0 {
		t.Error("no glyph decoded to a non-empty outline")
	}
	t.Logf("decoded %d glyphs, %d non-empty", font.NumGlyphs(), nonEmpty)
}

// largeTopDict builds a font whose top DICT exceeds the parser's internal
// buffer (1024 bytes).  Reading it must not panic; the padding leaves operands
// on the stack, so decoding returns an error.
func largeTopDict() []byte {
	const n = 1500
	out := []byte{2, 0, 5, byte(n >> 8), byte(n & 0xff)}
	for range n {
		out = append(out, 0x8b) // DICT operand: integer 0
	}
	return out
}

// TestReadCFF2LargeTopDict is a deterministic regression guard for a top DICT
// larger than the parser buffer.
func TestReadCFF2LargeTopDict(t *testing.T) {
	if _, err := readCFF2Bytes(t, largeTopDict()); err == nil {
		t.Error("expected error for oversized top DICT")
	}
}

func FuzzReadCFF2(f *testing.F) {
	f.Add(buildCFF2(&cff2Spec{charStrings: cffIndex{cs(0, 0, t2rmoveto, 100, 0, t2rlineto)}}))
	f.Add(buildCFF2(&cff2Spec{
		vstore:      twoRegionStore(),
		charStrings: cffIndex{cs(0, 0, t2rmoveto), cs(100, 200, 10, 20, 30, 40, 2, t2blend, t2rmoveto)},
	}))
	f.Add(buildTooManyFDs(300))
	f.Add(largeTopDict()) // regression: top DICT longer than the parser buffer

	f.Fuzz(func(t *testing.T, data []byte) {
		font, err := ReadCFF2(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		if err != nil {
			return
		}
		// on success the result must be internally consistent
		if len(font.Private) == 0 {
			t.Error("no private dicts")
		}
		for gid, g := range font.Glyphs {
			if g == nil {
				t.Errorf("glyph %d is nil", gid)
			}
		}
	})
}
