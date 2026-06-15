// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2021  Jochen Voss <voss@seehuhn.de>
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
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/sfnt/parser"
)

func TestRoll(t *testing.T) {
	in := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	out := []float64{1, 2, 4, 5, 6, 3, 7, 8}

	roll(in[2:6], 3)
	for i, x := range in {
		if out[i] != x {
			t.Error(in, out)
			break
		}
	}
}

// TestCharStringBudget verifies that a fan-out subroutine bomb is
// rejected by the per-charstring budget charge rather than executed
// to completion (which would take many CPU-hours).  The payload mirrors
// the proof-of-concept in the security report: ten global subroutines
// where gsubr 0..8 each call the next subroutine 47 times before
// returning, and gsubr 9 is a single return byte.
func TestCharStringBudget(t *testing.T) {
	const fanOut = 47

	gsubrs := buildSubrBomb(fanOut)
	info := &decodeInfo{
		gsubr:  gsubrs,
		budget: parser.NewBudget(0),
	}

	// glyph body: push gsubr 0; callgsubr; endchar
	glyphBody := []byte{pushGsubrIdx(0), byte(t2callgsubr), byte(t2endchar)}

	_, err := info.decodeCharString(glyphBody)
	if !errors.Is(err, membudget.ErrExceeded) {
		t.Errorf("expected ErrExceeded, got %v", err)
	}
}

// TestReadSubrBombBudgeted is the end-to-end counterpart of
// TestCharStringBudget: it constructs a complete (and otherwise valid)
// CFF byte stream whose Global Subr INDEX is the fan-out bomb, and
// asserts that Read returns ErrExceeded.  This protects the
// wiring in cff.Read that hands the parser budget to the decoders;
// without that wiring the decoders would run with a nil budget and
// the bomb would execute 47^9 ~= 10^15 calls.
func TestReadSubrBombBudgeted(t *testing.T) {
	blob := buildSubrBombCFF()
	_, err := Read(bytes.NewReader(blob), parser.NewBudget(int64(len(blob))))
	if !errors.Is(err, membudget.ErrExceeded) {
		t.Fatalf("err = %v, want ErrExceeded", err)
	}
}

// buildSubrBombCFF assembles a one-glyph CFF font whose Global Subr
// INDEX contains the same fan-out bomb as TestCharStringBudget: gsubr
// 0..8 each call the next subroutine 47 times before returning, and
// gsubr 9 is a single return byte.  The lone CharString invokes gsubr 0.
func buildSubrBombCFF() []byte {
	const fanOut = 47

	gsubrs := buildSubrBomb(fanOut)
	gsubrEnc := gsubrs.encode()

	// charstring: push gsubr 0; callgsubr; endchar
	charStringsEnc := cffIndex{{pushGsubrIdx(0), byte(t2callgsubr), byte(t2endchar)}}.encode()

	nameIdxEnc := cffIndex{[]byte("X")}.encode()
	stringIdxEnc := cffIndex{}.encode()

	// Top DICT, with all integer operands encoded as int32 (0x1D + 4
	// bytes) so the dict size is independent of the offset values:
	//   charStringsOffs(5) + opCharStrings(1)          =  6
	//   pdSize(5) + pdOffs(5) + opPrivate(1)           = 11
	const topDictSize = 17
	// Top DICT INDEX header: count(2) + offSize(1) + 2 offsets(2)    = 5
	const topDictIdxSize = 5 + topDictSize // = 22

	charStringsOffs := int32(4 + len(nameIdxEnc) + topDictIdxSize + len(stringIdxEnc) + len(gsubrEnc))
	pdSize := int32(0)
	pdOffs := int32(4) // any valid offset works when pdSize == 0

	topDict := &bytes.Buffer{}
	writeDictInt32(topDict, charStringsOffs)
	topDict.WriteByte(0x11) // opCharStrings
	writeDictInt32(topDict, pdSize)
	writeDictInt32(topDict, pdOffs)
	topDict.WriteByte(0x12) // opPrivate
	if topDict.Len() != topDictSize {
		panic(fmt.Sprintf("top dict size = %d, want %d", topDict.Len(), topDictSize))
	}

	out := &bytes.Buffer{}
	out.Write([]byte{0x01, 0x00, 0x04, 0x01}) // header: v1.0, hdrSize=4, offSize=1
	out.Write(nameIdxEnc)
	// Top DICT INDEX with a single entry of topDictSize bytes.
	out.Write([]byte{0x00, 0x01, 0x01, 0x01, byte(1 + topDictSize)})
	out.Write(topDict.Bytes())
	out.Write(stringIdxEnc)
	out.Write(gsubrEnc)
	out.Write(charStringsEnc)
	return out.Bytes()
}

func writeDictInt32(buf *bytes.Buffer, v int32) {
	buf.WriteByte(0x1D)
	binary.Write(buf, binary.BigEndian, v)
}

// buildSubrBomb returns a 10-entry global-subroutine INDEX where
// gsubr 0..8 each invoke the next subroutine fanOut times before
// returning, and gsubr 9 is a single return byte.  Executing the
// chain without budget enforcement would take fanOut^9 calls.
func buildSubrBomb(fanOut int) cffIndex {
	gsubrs := make(cffIndex, 10)
	for i := range 9 {
		body := make([]byte, 0, fanOut*2+1)
		for range fanOut {
			body = append(body, pushGsubrIdx(i+1), byte(t2callgsubr))
		}
		body = append(body, byte(t2return))
		gsubrs[i] = body
	}
	gsubrs[9] = []byte{byte(t2return)}
	return gsubrs
}

// pushGsubrIdx returns the one-byte type 2 integer-push opcode that
// pushes the biased value selecting global subroutine idx, assuming
// the small-fontset bias (107, used when nSubrs < 1240).  The encoding
// is byte(value + 139) for values in [-107, 107].
func pushGsubrIdx(idx int) byte {
	const gsubrBias = 107
	return byte(idx - gsubrBias + 139)
}

func FuzzT2Decode(f *testing.F) {
	f.Add(t2endchar.Bytes())
	f.Fuzz(func(t *testing.T, data1 []byte) {
		info := &decodeInfo{
			subr:         cffIndex{},
			gsubr:        cffIndex{},
			defaultWidth: 500,
			nominalWidth: 666,
			budget:       parser.NewBudget(0),
		}
		g1, err := info.decodeCharString(data1)
		if err != nil {
			return
		}

		// Skip glyphs with values that overflow int32 when scaled to 16.16
		// fixed-point. This can happen when arithmetic operators like div
		// produce values that can't be directly encoded.
		if hasOutOfRangeValues(g1, info.nominalWidth) {
			return
		}

		data2, err := g1.encodeCharString(info.defaultWidth, info.nominalWidth)
		if err != nil {
			t.Fatal(err)
		}

		g2, err := info.decodeCharString(data2)
		if err != nil {
			fmt.Printf("A % x\n", data1)
			fmt.Printf("A %s\n", g1)
			fmt.Printf("B % x\n", data2)
			fmt.Printf("B %s\n", g2)
			t.Fatal(err)
		}

		if !glyphsEqual(g1, g2) {
			fmt.Printf("A % x\n", data1)
			fmt.Println(formatT2CharString(data1))
			fmt.Printf("A %s\n", g1)
			fmt.Printf("B % x\n", data2)
			fmt.Println(formatT2CharString(data2))
			fmt.Printf("B %s\n", g2)
			t.Error("different")
		}
	})
}

func hasOutOfRangeValues(g *Glyph, nominalWidth float64) bool {
	// check width
	widthDiff := g.Width - nominalWidth
	if scaled := math.Round(widthDiff * 65536); scaled > math.MaxInt32 || scaled < math.MinInt32 {
		return true
	}

	// check stems
	for _, stems := range [][]float64{g.HStem, g.VStem} {
		prev := 0.0
		for _, x := range stems {
			diff := x - prev
			scaled := math.Round(diff * 65536)
			if scaled > math.MaxInt32 || scaled < math.MinInt32 {
				return true
			}
			prev = x
		}
	}
	return false
}

func formatT2CharString(code []byte) string {
	buf := &strings.Builder{}

	for len(code) > 0 {
		op := t2op(code[0])

		if op >= 32 && op <= 246 {
			fmt.Fprintf(buf, "%d ", int16(op)-139)
			code = code[1:]
			continue
		} else if op >= 247 && op <= 250 {
			if len(code) < 2 {
				buf.WriteString("{stack underflow}\n")
				continue
			}
			fmt.Fprintf(buf, "%d ", (int16(op)-247)*256+int16(code[1])+108)
			code = code[2:]
			continue
		} else if op >= 251 && op <= 254 {
			if len(code) < 2 {
				buf.WriteString("{stack underflow}\n")
				continue
			}
			fmt.Fprintf(buf, "%d ", (251-int16(op))*256-int16(code[1])-108)
			code = code[2:]
			continue
		} else if op == 28 {
			if len(code) < 3 {
				buf.WriteString("{stack underflow}\n")
				continue
			}
			fmt.Fprintf(buf, "%d ", int16(code[1])<<8|int16(code[2]))
			code = code[3:]
			continue
		} else if op == 255 {
			if len(code) < 5 {
				buf.WriteString("{stack underflow}\n")
				continue
			}
			val := int32(code[1])<<24 | int32(code[2])<<16 | int32(code[3])<<8 | int32(code[4])
			fmt.Fprintf(buf, "%g ", float64(val)/65536)
			code = code[5:]
			continue
		}

		if op == 12 {
			if len(code) < 2 {
				buf.WriteString("{incomplete opcode}\n")
				continue
			}
			op = op<<8 | t2op(code[1])
			code = code[2:]
		} else {
			code = code[1:]
		}

		fmt.Fprintf(buf, "%s\n", op)
	}
	return buf.String()
}

// floatEqual returns true if two float64 values are equal within CFF encoding tolerance.
// For values outside the 16-bit integer range, we don't check tolerance.
func floatEqual(a, b float64) bool {
	if math.Abs(a) >= 32767 || math.Abs(b) >= 32767 {
		return true
	}
	return math.Abs(a-b) <= 0.5/65536
}

func glyphsEqual(g1, g2 *Glyph) bool {
	if g1.Name != g2.Name {
		return false
	}
	if math.Abs(g1.Width-g2.Width) > 0.5/65536 {
		return false
	}

	if len(g1.HStem) != len(g2.HStem) || len(g1.VStem) != len(g2.VStem) {
		return false
	}
	for i, s1 := range g1.HStem {
		if !floatEqual(s1, g2.HStem[i]) {
			return false
		}
	}
	for i, s1 := range g1.VStem {
		if !floatEqual(s1, g2.VStem[i]) {
			return false
		}
	}

	if len(g1.Cmds) != len(g2.Cmds) {
		return false
	}
	for i, c1 := range g1.Cmds {
		c2 := g2.Cmds[i]
		if c1.Op != c2.Op || len(c1.Args) != len(c2.Args) {
			return false
		}
		for j, a1 := range c1.Args {
			a2 := c2.Args[j]
			if math.Abs(a1-a2) > 0.5/65536 && math.Abs(a1) < 32767 && math.Abs(a2) < 32767 {
				fmt.Println(a1, a2, math.Abs(a1-a2)*65536)
				return false
			}
		}
	}
	return true
}
