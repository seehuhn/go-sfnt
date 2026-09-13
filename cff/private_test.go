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
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/membudget"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/variation"
)

// writeReadCFF builds a one-glyph CFF font around private, writes it and reads
// it back.
func writeReadCFF(t *testing.T, private *type1.PrivateDict) *type1.PrivateDict {
	t.Helper()

	f := oneGlyphCFF(private)
	buf := &bytes.Buffer{}
	if err := f.Write(buf); err != nil {
		t.Fatalf("write: %v", err)
	}
	g, err := Read(bytes.NewReader(buf.Bytes()), membudget.New(1<<20))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return g.Private[0]
}

// blueScaleDict returns the Private DICT body storing v as BlueScale.
func blueScaleDict(v int32) []byte {
	var b bytes.Buffer
	encodeDictNumber(&b, v)
	b.Write([]byte{12, 9}) // BlueScale
	return b.Bytes()
}

// oneGlyphCFF builds a minimal CFF font around private.
func oneGlyphCFF(private *type1.PrivateDict) *Font {
	return &Font{
		FontInfo: &type1.FontInfo{
			FontName:   "Test",
			FontMatrix: [6]float64{0.001, 0, 0, 0.001, 0, 0},
		},
		Outlines: &Outlines{
			Glyphs:   []*Glyph{{Name: ".notdef", Width: 100}},
			Private:  []*type1.PrivateDict{private},
			FDSelect: func(glyph.ID) int { return 0 },
			Encoding: make([]glyph.ID, 256),
		},
	}
}

// oneGlyphCFF2 builds a minimal CFF2 font around private.
func oneGlyphCFF2(private *PrivateCFF2) *FontCFF2 {
	return &FontCFF2{
		FontMatrix: [6]float64{0.001, 0, 0, 0.001, 0, 0},
		OutlinesCFF2: &OutlinesCFF2{
			Glyphs: []*GlyphCFF2{
				{Cmds: []GlyphOpCFF2{{Op: OpMoveTo, Args: []Blend{{Default: 0}, {Default: 0}}}}},
			},
			Widths:   []float64{500},
			Private:  []*PrivateCFF2{private},
			FDSelect: func(glyph.ID) int { return 0 },
		},
	}
}

// TestBlueScaleRepairedOnRead checks that a BlueScale a font cannot hold is
// replaced when the font is read.  Only (0, type1.MaxBlueScale] is reachable for a
// conforming font, and a value outside carries no intent to preserve, so it
// falls back to the default rather than to the nearest bound.
//
// The CFF reader shares the repair but cannot be shown it, since the CFF
// writer now refuses to produce such a file; the two doors which can be fed
// one are covered here.
func TestBlueScaleRepairedOnRead(t *testing.T) {
	for _, v := range []int32{-91, 2, 1000} {
		t.Run(fmt.Sprintf("CFF2/%d", v), func(t *testing.T) {
			data := buildCFF2(&cff2Spec{
				fds:         []fdSpec{{privateBody: blueScaleDict(v)}},
				charStrings: cffIndex{cs(0, 0, t2rmoveto)},
			})
			font, err := ReadCFF2(bytes.NewReader(data), membudget.New(1<<20))
			if err != nil {
				t.Fatal(err)
			}
			if got := font.Private[0].BlueScale; got.Default != type1.DefaultBlueScale {
				t.Errorf("stored %d, read back %v, want the default", v, got.Default)
			}
			roundTripCFF2(t, data)
		})

		t.Run(fmt.Sprintf("Type1/%d", v), func(t *testing.T) {
			t1 := &type1.Font{
				FontInfo: &type1.FontInfo{
					FontName:   "Test",
					FontMatrix: [6]float64{0.001, 0, 0, 0.001, 0, 0},
				},
				Outlines: &type1.Outlines{
					Glyphs:  map[string]*type1.Glyph{".notdef": {}},
					Private: &type1.PrivateDict{BlueScale: float64(v)},
				},
			}
			f, err := FromType1(t1)
			if err != nil {
				t.Fatal(err)
			}
			if got := f.Private[0].BlueScale; got != type1.DefaultBlueScale {
				t.Errorf("stored %d, converted to %v, want the default", v, got)
			}
		})
	}
}

// stemWidthDict returns the Private DICT body storing v as StdHW, with one
// variation delta so that the entry has something to lose.
func stemWidthDict(v int32) []byte {
	var b bytes.Buffer
	encodeDictNumber(&b, v)        // default
	encodeDictNumber(&b, int32(5)) // one delta
	encodeDictNumber(&b, int32(1)) // operand count
	b.WriteByte(byte(opBlend))
	b.WriteByte(byte(opStdHW))
	return b.Bytes()
}

// TestStemWidthDroppedOnRead checks that a CFF2 stem width whose stored value
// no font could give is dropped whole, deltas and all.
//
// The stored value is the value at the default instance, so the rule which
// applies to a static CFF applies to it too.  Keeping the deltas after zeroing
// it would leave a "not given" marker with variation attached, which says
// nothing coherent.
func TestStemWidthDroppedOnRead(t *testing.T) {
	for _, v := range []int32{-1, 0, type1.MaxStemWidth + 1} {
		data := buildCFF2(&cff2Spec{
			vstore:      vsIndexStore(),
			fds:         []fdSpec{{privateBody: stemWidthDict(v)}},
			charStrings: cffIndex{cs(0, 0, t2rmoveto)},
		})
		font, err := ReadCFF2(bytes.NewReader(data), membudget.New(1<<20))
		if err != nil {
			t.Fatal(err)
		}
		if got := font.Private[0].StdHW; got.Default != 0 || len(got.Deltas) != 0 {
			t.Errorf("stored %d, read back %+v, want the entry dropped", v, got)
		}
		roundTripCFF2(t, data)
	}

	t.Run("a usable width is kept", func(t *testing.T) {
		data := buildCFF2(&cff2Spec{
			vstore:      vsIndexStore(),
			fds:         []fdSpec{{privateBody: stemWidthDict(60)}},
			charStrings: cffIndex{cs(0, 0, t2rmoveto)},
		})
		font, err := ReadCFF2(bytes.NewReader(data), membudget.New(1<<20))
		if err != nil {
			t.Fatal(err)
		}
		got := font.Private[0].StdHW
		if got.Default != 60 || len(got.Deltas) != 1 {
			t.Errorf("StdHW = %+v, want 60 with one delta", got)
		}
	})
}

// TestPrivateDictWriteRead checks what a Private DICT the writer accepts looks
// like after a write-read cycle: a usable value survives untouched, and zero
// is the writer's shorthand for the default.
func TestPrivateDictWriteRead(t *testing.T) {
	cases := []struct {
		name string
		in   type1.PrivateDict
		want type1.PrivateDict
	}{
		{"BlueScale zero means the default", type1.PrivateDict{BlueScale: 0},
			type1.PrivateDict{BlueScale: type1.DefaultBlueScale}},
		{"BlueScale in range", type1.PrivateDict{BlueScale: 0.5},
			type1.PrivateDict{BlueScale: 0.5}},
		{"BlueScale at the bound", type1.PrivateDict{BlueScale: type1.MaxBlueScale},
			type1.PrivateDict{BlueScale: type1.MaxBlueScale}},
		{"stem widths", type1.PrivateDict{StdHW: 60, StdVW: 80},
			type1.PrivateDict{BlueScale: type1.DefaultBlueScale, StdHW: 60, StdVW: 80}},
		{"stem width at the bound", type1.PrivateDict{StdHW: type1.MaxStemWidth},
			type1.PrivateDict{BlueScale: type1.DefaultBlueScale, StdHW: type1.MaxStemWidth}},
		// zero is how a stem width the font does not give is recorded, so it
		// makes the round trip untouched
		{"no stem width", type1.PrivateDict{StdHW: 0},
			type1.PrivateDict{BlueScale: type1.DefaultBlueScale, StdHW: 0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := c.in

			got := writeReadCFF(t, &in)
			if diff := cmp.Diff(&c.want, got); diff != "" {
				t.Errorf("after read (-want +got):\n%s", diff)
			}

			// what came back survives another cycle unchanged
			again := writeReadCFF(t, got)
			if diff := cmp.Diff(got, again); diff != "" {
				t.Errorf("second cycle changed it (-first +second):\n%s", diff)
			}
		})
	}
}

// TestPrivateRefusedOnWrite checks that the writers refuse a Private DICT
// value no font can hold rather than quietly repairing it.  Values off a file
// have been put right on the read side already, so one reaching a writer came
// from the caller.
func TestPrivateRefusedOnWrite(t *testing.T) {
	for _, v := range []float64{-91, 2, 1e6, math.NaN()} {
		t.Run(fmt.Sprintf("BlueScale/CFF/%v", v), func(t *testing.T) {
			f := oneGlyphCFF(&type1.PrivateDict{BlueScale: v})
			if err := f.Write(&bytes.Buffer{}); err == nil {
				t.Errorf("Write accepted BlueScale %v", v)
			}
		})
		t.Run(fmt.Sprintf("BlueScale/CFF2/%v", v), func(t *testing.T) {
			f := oneGlyphCFF2(&PrivateCFF2{BlueScale: Blend{Default: v}})
			if err := f.Write(&bytes.Buffer{}); err == nil {
				t.Errorf("Write accepted BlueScale %v", v)
			}
		})
	}
	// Zero is not refused -- it records a stem width the font does not give,
	// and the writer omits the entry for it.  The CFF2 writer is not asked:
	// there the operand may vary, and the constraint holds per instance
	// rather than on the stored value.
	for _, v := range []float64{-1, type1.MaxStemWidth + 1, math.NaN()} {
		t.Run(fmt.Sprintf("StdHW/%v", v), func(t *testing.T) {
			f := oneGlyphCFF(&type1.PrivateDict{StdHW: v})
			if err := f.Write(&bytes.Buffer{}); err == nil {
				t.Errorf("Write accepted StdHW %v", v)
			}
		})
		t.Run(fmt.Sprintf("StdVW/%v", v), func(t *testing.T) {
			f := oneGlyphCFF(&type1.PrivateDict{StdVW: v})
			if err := f.Write(&bytes.Buffer{}); err == nil {
				t.Errorf("Write accepted StdVW %v", v)
			}
		})
	}
}

// TestInstancePrivateDictSanitized checks that instancing a CFF2 font yields a
// private dict the CFF writer and reader agree on.
//
// The CFF2 reader is permissive about Private DICT values and the CFF reader
// repairs them, so without a repair at the point the blends become scalars an
// instanced font would lose the value on its way through a CFF file.
func TestInstancePrivateDictSanitized(t *testing.T) {
	o := &OutlinesCFF2{
		Glyphs: []*GlyphCFF2{
			{Cmds: []GlyphOpCFF2{{Op: OpMoveTo, Args: []Blend{{Default: 0}, {Default: 0}}}}},
		},
		Widths: []float64{500},
		Private: []*PrivateCFF2{{
			BlueScale: Blend{Default: -91},
			StdHW:     Blend{Default: -5},
		}},
		FDSelect: func(glyph.ID) int { return 0 },
	}

	static, err := o.Instance(nil, o.Widths)
	if err != nil {
		t.Fatal(err)
	}
	p := static.Private[0]
	if p.BlueScale < 0 || p.BlueScale > type1.MaxBlueScale {
		t.Errorf("instanced BlueScale = %v, outside the range the reader accepts", p.BlueScale)
	}
	// a negative stem width is dropped, not moved to a bound: zero is how
	// this package records a stem width the font does not give
	if p.StdHW != 0 {
		t.Errorf("instanced StdHW = %v, want it dropped to 0", p.StdHW)
	}

	// the whole point: the private dict survives a trip through a CFF file
	got := writeReadCFF(t, p)
	if diff := cmp.Diff(p, got); diff != "" {
		t.Errorf("instanced private dict changed on write-read (-want +got):\n%s", diff)
	}
}

// blendedBlueScale returns the Private DICT body for a BlueScale carrying one
// variation delta.  BlueScale is listed as non-blendable, so no conforming
// font holds this, but a reader has to cope with one that does.
func blendedBlueScale() []byte {
	var b bytes.Buffer
	encodeDictNumber(&b, int32(1)) // default
	encodeDictNumber(&b, int32(1)) // one delta
	encodeDictNumber(&b, int32(1)) // operand count
	b.WriteByte(byte(opBlend))
	b.Write([]byte{12, 9}) // BlueScale
	return b.Bytes()
}

// TestCFF2NonBlendableOperand covers an operand the CFF2 Private DICT does not
// allow to vary.  Reading is permissive and drops the deltas, so that the dict
// written back out reads the same way again; writing is strict, since deltas
// there can only have come from the caller.
func TestCFF2NonBlendableOperand(t *testing.T) {
	t.Run("read drops the deltas", func(t *testing.T) {
		data := buildCFF2(&cff2Spec{
			vstore:      vsIndexStore(),
			fds:         []fdSpec{{privateBody: blendedBlueScale()}},
			charStrings: cffIndex{cs(0, 0, t2rmoveto)},
		})
		font, err := ReadCFF2(bytes.NewReader(data), membudget.New(1<<20))
		if err != nil {
			t.Fatal(err)
		}
		if d := font.Private[0].BlueScale.Deltas; len(d) != 0 {
			t.Errorf("BlueScale kept %d deltas", len(d))
		}
		roundTripCFF2(t, data)
	})

	t.Run("write refuses the deltas", func(t *testing.T) {
		f2 := variation.F2Dot14FromFloat
		o := &OutlinesCFF2{
			Glyphs: []*GlyphCFF2{
				{Cmds: []GlyphOpCFF2{{Op: OpMoveTo, Args: []Blend{{Default: 0}, {Default: 0}}}}},
			},
			Widths:   []float64{500},
			Private:  []*PrivateCFF2{{BlueScale: Blend{Default: 0.04, Deltas: []float64{0.01}}}},
			FDSelect: func(glyph.ID) int { return 0 },
			VarStore: &variation.ItemVariationStore{
				Regions: []variation.Region{{{Start: 0, Peak: f2(1), End: f2(1)}}},
				Data: []*variation.ItemVariationData{
					{RegionIndexes: []uint16{0}, Deltas: [][]int32{}},
				},
			},
		}
		font := &FontCFF2{
			FontMatrix:   [6]float64{0.001, 0, 0, 0.001, 0, 0},
			OutlinesCFF2: o,
		}
		if err := font.Write(&bytes.Buffer{}); err == nil {
			t.Error("Write accepted a blend on a non-blendable operand")
		}
	})
}

// zoneDict returns the Private DICT body storing vals as BlueValues, in the
// delta-encoded form the operand uses.
func zoneDict(vals ...int32) []byte {
	var b bytes.Buffer
	prev := int32(0)
	for _, v := range vals {
		encodeDictNumber(&b, v-prev)
		prev = v
	}
	b.WriteByte(byte(opBlueValues))
	return b.Bytes()
}

// TestZonesTrimmedOnRead checks that an alignment-zone array is cut back to
// the part a font could hold: an even number of values, in pairs which do not
// run backwards, and no more pairs than the format allows.  The pairs ahead of
// a broken one are independent of it, so they are kept.
func TestZonesTrimmedOnRead(t *testing.T) {
	cases := []struct {
		name string
		in   []int32
		want []float64
	}{
		{"kept whole", []int32{-10, 0, 100, 110}, []float64{-10, 0, 100, 110}},
		{"odd trailing value", []int32{-10, 0, 100}, []float64{-10, 0}},
		{"pair runs backwards", []int32{-10, 0, 110, 100, 200, 210}, []float64{-10, 0}},
		{"first pair broken", []int32{10, 0}, nil},
		{"too many pairs",
			[]int32{0, 1, 10, 11, 20, 21, 30, 31, 40, 41, 50, 51, 60, 61, 70, 71},
			[]float64{0, 1, 10, 11, 20, 21, 30, 31, 40, 41, 50, 51, 60, 61}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			data := buildCFF2(&cff2Spec{
				fds:         []fdSpec{{privateBody: zoneDict(c.in...)}},
				charStrings: cffIndex{cs(0, 0, t2rmoveto)},
			})
			font, err := ReadCFF2(bytes.NewReader(data), membudget.New(1<<20))
			if err != nil {
				t.Fatal(err)
			}
			var got []float64
			for _, b := range font.Private[0].BlueValues {
				got = append(got, b.Default)
			}
			if diff := cmp.Diff(c.want, got); diff != "" {
				t.Errorf("BlueValues (-want +got):\n%s", diff)
			}
			roundTripCFF2(t, data)
		})
	}
}

// TestStemSnapSortedOnRead checks that a stem snap array is capped in length
// and put in increasing order.  The format says the order carries no meaning
// and warns that blending can undo it anyway, so sorting loses nothing.
func TestStemSnapSortedOnRead(t *testing.T) {
	p := &PrivateCFF2{StemSnapH: []Blend{{Default: 90}, {Default: 20}, {Default: 50}}}
	sanitizePrivateCFF2(p)
	got := []float64{p.StemSnapH[0].Default, p.StemSnapH[1].Default, p.StemSnapH[2].Default}
	if want := []float64{20, 50, 90}; !slices.Equal(got, want) {
		t.Errorf("StemSnapH = %v, want %v", got, want)
	}

	long := make([]Blend, maxStemSnapEntries+3)
	for i := range long {
		long[i] = Blend{Default: float64(i)}
	}
	p = &PrivateCFF2{StemSnapV: long}
	sanitizePrivateCFF2(p)
	if len(p.StemSnapV) != maxStemSnapEntries {
		t.Errorf("StemSnapV has %d values, want %d", len(p.StemSnapV), maxStemSnapEntries)
	}
}

// TestPrivateArraysRefusedOnWrite checks that the writers refuse an array
// entry no font can hold rather than quietly trimming it.  Entries off a file
// have been trimmed on the read side already, so one reaching a writer came
// from the caller.
func TestPrivateArraysRefusedOnWrite(t *testing.T) {
	bad := [][]float64{
		{-10, 0, 100}, // odd number of values
		{10, 0},       // pair runs backwards
		{0, 1, 10, 11, 20, 21, 30, 31, 40, 41, 50, 51, 60, 61, 70, 71}, // eight pairs
	}
	for _, vals := range bad {
		t.Run(fmt.Sprintf("CFF/%v", vals), func(t *testing.T) {
			f := oneGlyphCFF(&type1.PrivateDict{BlueValues: vals})
			if err := f.Write(&bytes.Buffer{}); err == nil {
				t.Errorf("Write accepted BlueValues %v", vals)
			}
		})
		t.Run(fmt.Sprintf("CFF2/%v", vals), func(t *testing.T) {
			blends := make([]Blend, len(vals))
			for i, v := range vals {
				blends[i] = Blend{Default: v}
			}
			f := oneGlyphCFF2(&PrivateCFF2{BlueValues: blends})
			if err := f.Write(&bytes.Buffer{}); err == nil {
				t.Errorf("Write accepted BlueValues %v", vals)
			}
		})
	}

	t.Run("StemSnapH too long", func(t *testing.T) {
		long := make([]Blend, maxStemSnapEntries+1)
		f := oneGlyphCFF2(&PrivateCFF2{StemSnapH: long})
		if err := f.Write(&bytes.Buffer{}); err == nil {
			t.Error("Write accepted an over-long StemSnapH")
		}
	})

	t.Run("StemSnapV too long", func(t *testing.T) {
		long := make([]Blend, maxStemSnapEntries+1)
		f := oneGlyphCFF2(&PrivateCFF2{StemSnapV: long})
		if err := f.Write(&bytes.Buffer{}); err == nil {
			t.Error("Write accepted an over-long StemSnapV")
		}
	})
}

// TestFractionalBlueDistance checks that a fractional BlueShift or BlueFuzz
// survives a CFF round trip.  The format spells these as numbers, so a font
// may give a fraction even though the Type 1 format cannot.
func TestFractionalBlueDistance(t *testing.T) {
	in := &type1.PrivateDict{
		BlueScale: type1.DefaultBlueScale,
		BlueShift: 7.5,
		BlueFuzz:  0.5,
	}
	got := writeReadCFF(t, in)

	if got.BlueShift != in.BlueShift || got.BlueFuzz != in.BlueFuzz {
		t.Errorf("got BlueShift %v, BlueFuzz %v; want %v and %v",
			got.BlueShift, got.BlueFuzz, in.BlueShift, in.BlueFuzz)
	}
}

// TestZoneEdgesRoundTrip checks that a fractional or large zone edge survives
// a CFF round trip.  The format spells a delta as a list of numbers, so a font
// may give either even though the Type 1 format cannot.
func TestZoneEdgesRoundTrip(t *testing.T) {
	in := &type1.PrivateDict{
		BlueScale:  type1.DefaultBlueScale,
		BlueShift:  type1.DefaultBlueShift,
		BlueFuzz:   type1.DefaultBlueFuzz,
		BlueValues: []float64{-10.5, 0, 40000, 40010.25},
		OtherBlues: []float64{-200.75, -190},
	}
	got := writeReadCFF(t, in)
	if diff := cmp.Diff(in, got); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

// TestGetDelta covers the delta reader.  The format declares a delta as a
// list of numbers, so real operands are kept as they are.
func TestGetDelta(t *testing.T) {
	cases := []struct {
		name string
		vals []any
		want []float64
	}{
		{"absent", nil, nil},
		{"integers", []any{int32(-15), int32(15), int32(685)}, []float64{-15, 0, 685}},
		{"reals", []any{-14.5, 15.0, 684.5}, []float64{-14.5, 0.5, 685}},
		{"mixed", []any{int32(-15), 15.5}, []float64{-15, 0.5}},
		{"beyond 16 bits", []any{int32(100), int32(40000)}, []float64{100, 40100}},
		{"operand of the wrong kind", []any{int32(10), "x", int32(20)}, []float64{10}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := cffDict{}
			if c.vals != nil {
				d[opBlueValues] = c.vals
			}
			if diff := cmp.Diff(c.want, d.getDelta(opBlueValues)); diff != "" {
				t.Errorf("wrong values (-want +got):\n%s", diff)
			}
		})
	}
}

// blueShiftDict returns the Private DICT body storing v as BlueShift.
func blueShiftDict(v int32) []byte {
	var b bytes.Buffer
	encodeDictNumber(&b, v)
	b.Write([]byte{12, 10}) // BlueShift
	return b.Bytes()
}

// TestBlueDistanceRepairedOnRead checks that a CFF2 BlueShift no font could
// give is replaced on read.  The operand cannot be blended, so its stored
// value is its whole value and the rule which applies to a static CFF applies
// to it too.
func TestBlueDistanceRepairedOnRead(t *testing.T) {
	data := buildCFF2(&cff2Spec{
		fds:         []fdSpec{{privateBody: blueShiftDict(-3)}},
		charStrings: cffIndex{cs(0, 0, t2rmoveto)},
	})
	font, err := ReadCFF2(bytes.NewReader(data), membudget.New(1<<20))
	if err != nil {
		t.Fatal(err)
	}
	if got := font.Private[0].BlueShift; got.Default != type1.DefaultBlueShift {
		t.Errorf("stored -3, read back %v, want the default", got.Default)
	}
	roundTripCFF2(t, data)
}

// TestBlueDistanceRefusedOnWrite checks that the writers refuse a BlueShift or
// BlueFuzz no font can hold.  Values off a file have been put right on the
// read side already, so one reaching a writer came from the caller.
func TestBlueDistanceRefusedOnWrite(t *testing.T) {
	for _, v := range []float64{-1, math.NaN(), math.Inf(1)} {
		t.Run(fmt.Sprintf("BlueShift/CFF/%v", v), func(t *testing.T) {
			f := oneGlyphCFF(&type1.PrivateDict{BlueShift: v})
			if err := f.Write(&bytes.Buffer{}); err == nil {
				t.Errorf("Write accepted BlueShift %v", v)
			}
		})
		t.Run(fmt.Sprintf("BlueShift/CFF2/%v", v), func(t *testing.T) {
			f := oneGlyphCFF2(&PrivateCFF2{BlueShift: Blend{Default: v}})
			if err := f.Write(&bytes.Buffer{}); err == nil {
				t.Errorf("Write accepted BlueShift %v", v)
			}
		})
		t.Run(fmt.Sprintf("BlueFuzz/CFF2/%v", v), func(t *testing.T) {
			f := oneGlyphCFF2(&PrivateCFF2{BlueFuzz: Blend{Default: v}})
			if err := f.Write(&bytes.Buffer{}); err == nil {
				t.Errorf("Write accepted BlueFuzz %v", v)
			}
		})
	}
}

// TestRealOperandRefusedOnWrite checks that the writers refuse a real operand
// they could not read back.  The reader bounds what it returns, so a font
// written with a value outside those bounds would not survive a round trip.
func TestRealOperandRefusedOnWrite(t *testing.T) {
	for _, v := range []float64{1e-310, 1e301, -1e301, math.Inf(1), math.NaN()} {
		t.Run(fmt.Sprintf("BlueScale/%v", v), func(t *testing.T) {
			f := oneGlyphCFF(&type1.PrivateDict{BlueScale: v})
			if err := f.Write(&bytes.Buffer{}); err == nil {
				t.Errorf("Write accepted BlueScale %v", v)
			}
		})
		t.Run(fmt.Sprintf("FontMatrix/%v", v), func(t *testing.T) {
			f := oneGlyphCFF(&type1.PrivateDict{BlueScale: type1.DefaultBlueScale})
			f.FontInfo.FontMatrix = matrix.Matrix{v, 0, 0, 0.001, 0, 0}
			if err := f.Write(&bytes.Buffer{}); err == nil {
				t.Errorf("Write accepted a FontMatrix holding %v", v)
			}
		})
		t.Run(fmt.Sprintf("FontMatrix/CFF2/%v", v), func(t *testing.T) {
			f := oneGlyphCFF2(&PrivateCFF2{})
			f.FontMatrix = matrix.Matrix{v, 0, 0, 0.001, 0, 0}
			if err := f.Write(&bytes.Buffer{}); err == nil {
				t.Errorf("Write accepted a FontMatrix holding %v", v)
			}
		})
		t.Run(fmt.Sprintf("FontMatrices/CFF2/%v", v), func(t *testing.T) {
			f := oneGlyphCFF2(&PrivateCFF2{})
			f.FontMatrices = []matrix.Matrix{{v, 0, 0, 1, 0, 0}}
			if err := f.Write(&bytes.Buffer{}); err == nil {
				t.Errorf("Write accepted a per-FD FontMatrix holding %v", v)
			}
		})
	}
}
