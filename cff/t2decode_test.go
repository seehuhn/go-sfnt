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
	"fmt"
	"math"
	"strings"
	"testing"
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

func FuzzT2Decode(f *testing.F) {
	f.Add(t2endchar.Bytes())
	f.Fuzz(func(t *testing.T, data1 []byte) {
		info := &decodeInfo{
			subr:         cffIndex{},
			gsubr:        cffIndex{},
			defaultWidth: 500,
			nominalWidth: 666,
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
