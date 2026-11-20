// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package mathtable

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/device"
)

func TestEncodeErrors(t *testing.T) {
	okAssembly := &Assembly{Parts: []Part{{Glyph: 1}}}
	cases := map[string]*Info{
		"too few kerns": {Kerns: map[glyph.ID]*KernInfo{
			1: {TopRight: &Kern{Heights: []Value{{V: 1}}, Kerns: []Value{{V: 1}}}},
		}},
		"too many kerns": {Kerns: map[glyph.ID]*KernInfo{
			1: {TopRight: &Kern{Kerns: []Value{{V: 1}, {V: 2}}}},
		}},
		"unsorted heights": {Kerns: map[glyph.ID]*KernInfo{
			1: {TopRight: &Kern{
				Heights: []Value{{V: 5}, {V: 1}},
				Kerns:   []Value{{}, {}, {}},
			}},
		}},
		"empty construction": {Vertical: map[glyph.ID]*Construction{1: {}}},
		"empty horizontal construction": {
			Horizontal: map[glyph.ID]*Construction{1: {Assembly: okAssembly}, 2: {}},
		},
		"assembly without parts": {Vertical: map[glyph.ID]*Construction{
			1: {Assembly: &Assembly{}},
		}},
		"bad device format": {ItalicsCorrection: map[glyph.ID]Value{
			1: {Device: &device.Table{DeltaFormat: 4}},
		}},
		"wrong delta count": {TopAccent: map[glyph.ID]Value{
			1: {Device: &device.Table{StartSize: 1, EndSize: 3, Deltas: []int8{1}, DeltaFormat: 1}},
		}},
		"end before start": {TopAccent: map[glyph.ID]Value{
			1: {Device: &device.Table{StartSize: 3, EndSize: 2, DeltaFormat: 1}},
		}},
		"delta out of range": {Constants: Constants{
			AxisHeight: Value{Device: &device.Table{EndSize: 0, Deltas: []int8{2}, DeltaFormat: 1}},
		}},
		"bad device in kern": {Kerns: map[glyph.ID]*KernInfo{
			1: {TopLeft: &Kern{Kerns: []Value{{Device: &device.Table{DeltaFormat: 7}}}}},
		}},
		"bad device in assembly": {Vertical: map[glyph.ID]*Construction{
			1: {Assembly: &Assembly{
				ItalicsCorrection: Value{Device: &device.Table{DeltaFormat: 0}},
				Parts:             []Part{{Glyph: 1}},
			}},
		}},
		"too many variants": {Vertical: map[glyph.ID]*Construction{
			1: {Variants: make([]Variant, 0x10000)},
		}},
		"too many parts": {Vertical: map[glyph.ID]*Construction{
			1: {Assembly: &Assembly{Parts: make([]Part, 0x10000)}},
		}},
		"too many heights": {Kerns: map[glyph.ID]*KernInfo{
			1: {TopRight: &Kern{
				Heights: make([]Value, 0x10000),
				Kerns:   make([]Value, 0x10001),
			}},
		}},
		"too many glyphs": {ItalicsCorrection: func() map[glyph.ID]Value {
			m := map[glyph.ID]Value{}
			for i := range 0x10000 {
				m[glyph.ID(i)] = Value{}
			}
			return m
		}()},
	}
	for name, info := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := info.Encode(); err == nil {
				t.Error("Encode succeeded")
			}
		})
	}
}

// distinctDevices returns n device tables which differ in their encoding.
func distinctDevices(n int) []*device.Table {
	res := make([]*device.Table, n)
	for i := range res {
		deltas := make([]int8, 40)
		deltas[0] = int8(i % 100)
		deltas[1] = int8(i / 100)
		res[i] = &device.Table{StartSize: 1, EndSize: 40, Deltas: deltas, DeltaFormat: 3}
	}
	return res
}

// bigGlyphInfo returns glyph info with more than 64 kB of device tables,
// where all subtables start below the 64 kB limit.
func bigGlyphInfo(info *Info) *Info {
	devs := distinctDevices(2000)
	info.ItalicsCorrection = map[glyph.ID]Value{}
	info.TopAccent = map[glyph.ID]Value{}
	for i, dev := range devs[:1000] {
		info.ItalicsCorrection[glyph.ID(i+1)] = Value{V: 1, Device: dev}
		info.TopAccent[glyph.ID(i+1)] = Value{V: 1, Device: devs[1000+i]}
	}
	return info
}

// bigVariants returns an Info with glyph constructions which need almost
// 64 kB.
func bigVariants(info *Info) *Info {
	info.Vertical = map[glyph.ID]*Construction{}
	for i := range 2180 {
		info.Vertical[glyph.ID(i+1)] = &Construction{
			Variants: []Variant{{Glyph: glyph.ID(i), Advance: 1}, {Glyph: 2, Advance: 3}},
			Assembly: &Assembly{Parts: []Part{{Glyph: glyph.ID(i)}}},
		}
	}
	return info
}

func TestEncodeOffsetRange(t *testing.T) {
	small := map[glyph.ID]*Construction{1: {Variants: []Variant{{Glyph: 1}}}}

	// large subtables are stored after small ones
	fitting := map[string]*Info{
		"glyph info":    bigGlyphInfo(&Info{Vertical: small}),
		"constructions": bigVariants(&Info{}),
	}
	for name, info := range fitting {
		t.Run(name, func(t *testing.T) {
			got, _ := roundTrip(t, info)
			if diff := cmp.Diff(info, got); diff != "" {
				t.Errorf("round trip failed (-want +got):\n%s", diff)
			}
		})
	}

	huge := func(i int8) *device.Table {
		deltas := make([]int8, 0x10000)
		deltas[0] = i
		return &device.Table{EndSize: 0xFFFF, Deltas: deltas, DeltaFormat: 3}
	}
	tooLarge := map[string]*Info{
		"two large tables": bigGlyphInfo(bigVariants(&Info{})),
		"large devices": {Constants: Constants{
			AxisHeight:  Value{Device: huge(0)},
			MathLeading: Value{Device: huge(1)},
		}},
		"many devices": {ItalicsCorrection: func() map[glyph.ID]Value {
			m := map[glyph.ID]Value{}
			for i, dev := range distinctDevices(3000) {
				m[glyph.ID(i+1)] = Value{Device: dev}
			}
			return m
		}()},
	}
	for name, info := range tooLarge {
		t.Run(name, func(t *testing.T) {
			if _, err := info.Encode(); err == nil {
				t.Error("Encode succeeded")
			}
		})
	}
}

func TestCrossParentSharing(t *testing.T) {
	dev := distinctDevices(1)[0]
	parts := make([]Part, 100)
	for i := range parts {
		parts[i] = Part{Glyph: glyph.ID(i), FullAdvance: 5}
	}
	mk := func(n int) *Info {
		asm := &Assembly{ItalicsCorrection: Value{V: 1, Device: dev}, Parts: parts}
		info := &Info{Vertical: map[glyph.ID]*Construction{}, Kerns: map[glyph.ID]*KernInfo{}}
		for i := range n {
			info.Vertical[glyph.ID(i+1)] = &Construction{
				Variants: []Variant{{Glyph: glyph.ID(i), Advance: 1}},
				Assembly: asm,
			}
			info.Kerns[glyph.ID(i+1)] = &KernInfo{TopRight: &Kern{
				Kerns: []Value{{V: funit16(i), Device: dev}},
			}}
		}
		return info
	}

	small, big := mk(2), mk(20)
	got, dataBig := roundTrip(t, big)
	if diff := cmp.Diff(big, got); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
	dataSmall, err := small.Encode()
	if err != nil {
		t.Fatal(err)
	}
	// each further sharer costs a few bytes, not a copy of the shared table
	if extra := len(dataBig) - len(dataSmall); extra > 18*60 {
		t.Errorf("shared tables duplicated: %d extra bytes", extra)
	}

	asm := got.Vertical[1].Assembly
	for _, c := range got.Vertical {
		if c.Assembly != asm {
			t.Fatal("assembly not shared")
		}
	}
	d := got.Kerns[1].TopRight.Kerns[0].Device
	for _, ki := range got.Kerns {
		if ki.TopRight.Kerns[0].Device != d {
			t.Fatal("device not shared")
		}
	}
}

func funit16(i int) funit.Int16 { return funit.Int16(i) }

func TestReadManyParentsOneAssembly(t *testing.T) {
	const n, nParts = 300, 3000
	be := func(vals ...int) []byte {
		var res []byte
		for _, v := range vals {
			res = append(res, byte(v>>8), byte(v))
		}
		return res
	}
	// MathVariants: header, offsets, coverage, constructions, assembly
	const covPos = 10 + 2*n
	consPos := covPos + 10
	asmPos := consPos + 4*n
	v := be(0, covPos, 0, n, 0)
	for i := range n {
		v = append(v, be(consPos+4*i)...)
	}
	v = append(v, be(2, 1, 1, n, 0)...) // coverage format 2, glyphs 1..n
	for i := range n {
		v = append(v, be(asmPos-consPos-4*i, 0)...)
	}
	v = append(v, be(0, 0, nParts)...)
	for i := range nParts {
		v = append(v, be(i, 0, 0, 7, 0)...)
	}

	data := be(1, 0, headerSize, 0, 0)
	data = append(data, make([]byte, headerSize+constantsSize-len(data))...)
	data[8], data[9] = byte(len(data)>>8), byte(len(data))
	data = append(data, v...)

	info, err := readBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Vertical) != n {
		t.Fatalf("read %d constructions, want %d", len(info.Vertical), n)
	}
	got, _ := roundTrip(t, info)
	if diff := cmp.Diff(info, got); diff != "" {
		t.Errorf("round trip failed (-want +got):\n%s", diff)
	}
}

func TestEncodeLargeSharedTable(t *testing.T) {
	deltas := make([]int8, 0x10000)
	deltas[0] = 1
	huge := &device.Table{EndSize: 0xFFFF, Deltas: deltas, DeltaFormat: 3}
	small := &device.Table{EndSize: 0, Deltas: []int8{1}, DeltaFormat: 3}

	// the order of the glyphs must not matter
	for _, hugeGid := range []glyph.ID{1, 2} {
		info := &Info{Kerns: map[glyph.ID]*KernInfo{}}
		for _, gid := range []glyph.ID{1, 2} {
			dev := small
			if gid == hugeGid {
				dev = huge
			}
			info.Kerns[gid] = &KernInfo{TopRight: &Kern{Kerns: []Value{{V: 1, Device: dev}}}}
		}
		got, _ := roundTrip(t, info)
		if diff := cmp.Diff(info, got); diff != "" {
			t.Errorf("round trip failed (-want +got):\n%s", diff)
		}
	}
}

// TestEncodeGolden checks the encoding against a hand-computed layout.
func TestEncodeGolden(t *testing.T) {
	info := &Info{
		MinConnectorOverlap: 0x0010,
		Kerns: map[glyph.ID]*KernInfo{
			5: {TopRight: &Kern{Kerns: []Value{{V: 7}}}},
		},
		Vertical: map[glyph.ID]*Construction{
			9: {
				Variants: []Variant{{Glyph: 0x0A, Advance: 0x0123}},
				Assembly: &Assembly{Parts: []Part{{Glyph: 0x0B, StartConnector: 1, EndConnector: 2, FullAdvance: 3, Extender: true}}},
			},
		},
	}
	info.Constants.AxisHeight = Value{V: 0x0100, Device: &device.Table{
		StartSize: 9, EndSize: 10, Deltas: []int8{1, -1}, DeltaFormat: 1,
	}}

	var want []byte
	u16 := func(vals ...int) {
		for _, v := range vals {
			want = append(want, byte(v>>8), byte(v))
		}
	}
	zeros := func(n int) { want = append(want, make([]byte, n)...) }

	// MATH header, at 0
	u16(1, 0) // majorVersion, minorVersion
	u16(10)   // mathConstantsOffset
	u16(232)  // mathGlyphInfoOffset
	u16(276)  // mathVariantsOffset

	// MathConstants, at 10, length 214+8
	zeros(8)        // four uint16 percentages and heights, all zero
	zeros(4)        // mathLeading
	u16(0x0100)     // axisHeight.value
	u16(214)        // axisHeight.deviceOffset, relative to MathConstants
	zeros(214 - 16) // remaining value records, radicalDegreeBottomRaisePercent
	// device table, at 224
	u16(9, 10, 1) // startSize, endSize, deltaFormat
	u16(0x7000)   // deltas +1 and -1 as 2-bit values

	// MathGlyphInfo, at 232
	u16(8)  // mathItalicsCorrectionInfoOffset
	u16(8)  // mathTopAccentAttachmentOffset, same empty table
	u16(16) // extendedShapeCoverageOffset
	u16(20) // mathKernInfoOffset
	// empty MathItalicsCorrectionInfo, at 240
	u16(4, 0) // coverageOffset, count
	u16(1, 0) // coverage format 1, no glyphs
	// empty extended shape coverage, at 248
	u16(1, 0)
	// MathKernInfo, at 252
	u16(12, 1)       // mathKernCoverageOffset, mathKernCount
	u16(18, 0, 0, 0) // topRight, topLeft, bottomRight, bottomLeft offsets
	u16(1, 1, 5)     // coverage: format 1, one glyph, glyph 5
	u16(0)           // MathKern at 270: heightCount
	u16(7, 0)        // kernValues[0]: value, no device table

	// MathVariants, at 276
	u16(0x0010)  // minConnectorOverlap
	u16(16)      // vertGlyphCoverageOffset
	u16(12)      // horizGlyphCoverageOffset
	u16(1, 0)    // vertGlyphCount, horizGlyphCount
	u16(22)      // vertGlyphConstructionOffsets[0]
	u16(1, 0)    // horizontal coverage at 288: format 1, no glyphs
	u16(1, 1, 9) // vertical coverage at 292: format 1, one glyph, glyph 9
	// MathGlyphConstruction, at 298
	u16(8)              // glyphAssemblyOffset
	u16(1)              // variantCount
	u16(0x000A, 0x0123) // variantGlyph, advanceMeasurement
	// GlyphAssembly, at 306
	u16(0, 0)            // italicsCorrection: value, no device table
	u16(1)               // partCount
	u16(0x000B, 1, 2, 3) // glyph, startConnectorLength, endConnectorLength, fullAdvance
	u16(1)               // partFlags: extender

	got, err := info.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("unexpected encoding\n got %x\nwant %x", got, want)
	}
}
