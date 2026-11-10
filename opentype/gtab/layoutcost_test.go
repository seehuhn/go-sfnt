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

// Layout-cost regression tests.  The readers share one Go value between all
// offsets which reach the same table.  Laying out such a subtable must cost
// time and memory in proportion to the number of distinct tables, not the
// number of references.

import (
	"bytes"
	"runtime"
	"testing"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/anchor"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/opentype/markarray"
	"seehuhn.de/go/sfnt/parser"
)

// layoutAllocFactor is the allowed ratio of the bytes allocated by a layout
// to the size of the encoded input.  A layout which encodes every reference
// separately allocates thousands of times the input size.
const layoutAllocFactor = 16

// checkLayoutCost reads data and checks that computing the layout allocates
// at most layoutAllocFactor times len(data) bytes.
func checkLayoutCost(t *testing.T, read func(*parser.Parser, int64) (Subtable, error), data []byte) Subtable {
	t.Helper()
	sub := readSubtable(t, read, data)

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	err := sub.fits()
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}

	if used := after.TotalAlloc - before.TotalAlloc; used > layoutAllocFactor*uint64(len(data)) {
		t.Errorf("layout allocated %d bytes for a %d byte input", used, len(data))
	}
	return sub
}

func TestLayoutCostGsub2_1(t *testing.T) {
	const n = 16000
	sub := &bytes.Buffer{}
	writeU16(sub, 1) // format
	covOff := 6 + 2*n + 2 + 2*n
	writeU16(sub, uint16(covOff))
	writeU16(sub, n)
	for range n {
		writeU16(sub, 6+2*n)
	}
	writeU16(sub, n) // glyphCount
	for i := range n {
		writeU16(sub, uint16(i))
	}
	for _, v := range markBE16(2, 1, 0, n-1, 0) {
		sub.WriteByte(v)
	}

	got := checkLayoutCost(t, readGsub2_1, sub.Bytes()).(*Gsub2_1)
	if len(got.Repl) != n {
		t.Fatalf("got %d sequences, want %d", len(got.Repl), n)
	}
}

func TestLayoutCostGsub4_1(t *testing.T) {
	checkLayoutCost(t, readGsub4_1, makeAliasedGsub4_1(10000, 10000))
}

func TestLayoutCostSeqContext2(t *testing.T) {
	checkLayoutCost(t, readSeqContext2, makeAliasedSeqContext2(15000, 15000))
}

// TestLayoutCostMarkAttach checks that many base array cells which reach one
// anchor with a large Device table are encoded once.
func TestLayoutCostMarkAttach(t *testing.T) {
	const bases = 8000
	const sizes = 28000
	dev := &device.Table{StartSize: 1, EndSize: sizes, Deltas: make([]int8, sizes), DeltaFormat: 3}
	devEnc := dev.Encode()

	markCov := markBE16(1, 1, 7)
	baseCov := markBE16(2, 1, 1, bases, 0)
	markArray := markBE16(1, 0, 6, 1, 3, 4)

	anchorOff := 2 + 2*bases
	baseArray := markBE16(bases)
	for range bases {
		baseArray = append(baseArray, markBE16(anchorOff)...)
	}
	baseArray = append(baseArray, markBE16(3, 1, 2, 10, 10)...)
	baseArray = append(baseArray, devEnc...)

	markCovOff := 12
	baseCovOff := markCovOff + len(markCov)
	markArrayOff := baseCovOff + len(baseCov)
	baseArrayOff := markArrayOff + len(markArray)
	data := markBE16(1, markCovOff, baseCovOff, 1, markArrayOff, baseArrayOff)
	data = append(data, markCov...)
	data = append(data, baseCov...)
	data = append(data, markArray...)
	data = append(data, baseArray...)

	checkLayoutCost(t, readGpos4_1, data)
}

// TestLayoutCostFeatureList checks that many Feature records which reach
// one Feature table with a long lookup list are encoded once.
func TestLayoutCostFeatureList(t *testing.T) {
	const n = 5000
	const lookups = 15000

	data := markBE16(n)
	bodyOff := 2 + 6*n
	for range n {
		data = append(data, "liga"...)
		data = append(data, markBE16(bodyOff)...)
	}
	data = append(data, markBE16(0, lookups)...)
	for i := range lookups {
		data = append(data, markBE16(i)...)
	}

	p := parser.New(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	info, err := readFeatureList(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(info) != n {
		t.Fatalf("got %d features, want %d", len(info), n)
	}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err = info.layout()
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if used := after.TotalAlloc - before.TotalAlloc; used > layoutAllocFactor*uint64(len(data)) {
		t.Errorf("layout allocated %d bytes for a %d byte input", used, len(data))
	}
}

// TestLayoutCostValueRecordDevice checks that many value records which reach
// one large Device table encode it once.
func TestLayoutCostValueRecordDevice(t *testing.T) {
	const n = 8000
	const sizes = 28000
	dev := &device.Table{StartSize: 1, EndSize: sizes, Deltas: make([]int8, sizes), DeltaFormat: 3}
	devEnc := dev.Encode()

	devOff := 8 + 2*n
	covOff := devOff + len(devEnc)
	data := markBE16(2, covOff, 0x0040, n)
	for range n {
		data = append(data, markBE16(devOff)...)
	}
	data = append(data, devEnc...)
	data = append(data, markBE16(2, 1, 1, n, 0)...)

	checkLayoutCost(t, readGpos1_2, data)
}

// The following tests build subtables in which many distinct Go values, as
// the readers return them for tables at different positions in the file,
// refer to one large shared value.  Such a font is small, since each of the
// distinct tables needs only a few bytes, and the layout must not encode the
// shared value once per distinct table.

// sharedRefCount is the number of distinct tables which refer to one large
// shared value.
const sharedRefCount = 5000

// sharedAllocFactor is the allowed ratio of the bytes allocated by a layout
// to the input size, for the tests below.  The distinct tables take only a
// few bytes of input each, but each needs a few map entries during layout.
// Encoding the shared value once per distinct table would allocate about
// 2500 times the input size.
const sharedAllocFactor = 64

// checkFitsCost checks that sub.fits() succeeds and allocates at most
// sharedAllocFactor times inputSize bytes.  The value inputSize is the size
// of a font which reads as sub.
func checkFitsCost(t *testing.T, sub Subtable, inputSize int) {
	t.Helper()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	err := sub.fits()
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	if used := after.TotalAlloc - before.TotalAlloc; used > sharedAllocFactor*uint64(inputSize) {
		t.Errorf("layout allocated %d bytes for a %d byte input", used, inputSize)
	}
}

// bigDevice returns a Device table which encodes to about 28 kB.
func bigDevice() *device.Table {
	const sizes = 28000
	return &device.Table{StartSize: 1, EndSize: sizes, Deltas: make([]int8, sizes), DeltaFormat: 3}
}

func TestLayoutCostSharedRule(t *testing.T) {
	const m = 15000
	rule := &SeqRule{Input: make([]glyph.ID, m)}
	l := &SeqContext1{Cov: coverage.Table{}, Rules: make([][]*SeqRule, sharedRefCount)}
	for i := range l.Rules {
		l.Cov[glyph.ID(i)] = i
		l.Rules[i] = []*SeqRule{rule}
	}
	// rule sets of 4 bytes, and the shared rule
	checkFitsCost(t, l, 6+2*sharedRefCount+4*sharedRefCount+4+2*m+6)
}

func TestLayoutCostSharedLigature(t *testing.T) {
	const m = 15000
	in := make([]glyph.ID, m)
	l := &Gsub4_1{Cov: coverage.Table{}, Repl: make([][]Ligature, sharedRefCount)}
	for i := range l.Repl {
		l.Cov[glyph.ID(i)] = i
		l.Repl[i] = []Ligature{{In: in, Out: 1}}
	}
	// ligature sets of 4 bytes, and the shared ligature
	checkFitsCost(t, l, 6+2*sharedRefCount+4*sharedRefCount+4+2*m+6)
}

func TestLayoutCostSharedLigatureAnchor(t *testing.T) {
	a := &anchor.Table{XDev: bigDevice()}
	l := &Gpos5_1{
		MarkCov:   coverage.Table{0: 0},
		LigCov:    coverage.Table{},
		MarkArray: []markarray.Record{{Class: 0}},
		LigArray:  make([][][]*anchor.Table, sharedRefCount),
	}
	for i := range l.LigArray {
		l.LigCov[glyph.ID(i+1)] = i
		l.LigArray[i] = [][]*anchor.Table{{a}}
	}
	// LigatureAttach tables of 4 bytes, and the shared anchor
	checkFitsCost(t, l, 12+2*sharedRefCount+4*sharedRefCount+a.EncodeLen()+64)
}

func TestLayoutCostSharedLangSys(t *testing.T) {
	const m = 15000
	features := bigFeatures(m)
	info := ScriptListInfo{}
	for script := range scriptBcp47 {
		tag, err := otfToBCP47(script, "")
		if err != nil {
			continue
		}
		info[tag] = features
	}
	if len(info) < 100 {
		t.Fatalf("only %d scripts", len(info))
	}

	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err := info.layout()
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	// Script tables of 4 bytes, and the shared LangSys table
	inputSize := 2 + 10*len(info) + 6 + 2*m
	if used := after.TotalAlloc - before.TotalAlloc; used > layoutAllocFactor*uint64(inputSize) {
		t.Errorf("layout allocated %d bytes for a %d byte input", used, inputSize)
	}
}

func TestLayoutCostSharedPairDevice(t *testing.T) {
	dev := bigDevice()
	l := &Gpos2_1{Adjust: map[glyph.Pair]*PairAdjust{}}
	for i := range sharedRefCount {
		l.Adjust[glyph.Pair{Left: glyph.ID(i), Right: 0}] = &PairAdjust{
			First: &GposValueRecord{XAdvanceDev: dev},
		}
	}
	// pair sets of 6 bytes, the shared Device table and the coverage table
	checkFitsCost(t, l, 10+2*sharedRefCount+6*sharedRefCount+dev.EncodeLen()+10)
}

func TestLayoutCostSharedAnchorDevice(t *testing.T) {
	dev := bigDevice()
	l := &Gpos4_1{
		MarkCov:   coverage.Table{0: 0},
		BaseCov:   coverage.Table{},
		MarkArray: []markarray.Record{{Class: 0}},
		BaseArray: make([][]*anchor.Table, sharedRefCount),
	}
	for i := range l.BaseArray {
		l.BaseCov[glyph.ID(i+1)] = i
		l.BaseArray[i] = []*anchor.Table{{XDev: dev}}
	}
	// anchor tables of 10 bytes, and the shared Device table
	checkFitsCost(t, l, 12+2*sharedRefCount+10*sharedRefCount+dev.EncodeLen()+64)
}
