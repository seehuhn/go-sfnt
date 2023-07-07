// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2023  Jochen Voss <voss@seehuhn.de>
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

package type1

import (
	"fmt"
	"math"

	"seehuhn.de/go/sfnt/funit"
	"seehuhn.de/go/sfnt/parser"
)

type decodeInfo struct {
	subrs [][]byte
}

func (info *decodeInfo) decodeCharString(code []byte) (*Glyph, error) {
	const maxStack = 24
	stack := make([]float64, 0, maxStack)
	clearStack := func() {
		stack = stack[:0]
	}

	res := &Glyph{}

	var posX, posY float64
	rMoveTo := func(dx, dy float64) {
		posX += dx
		posY += dy
		res.Cmds = append(res.Cmds, GlyphOp{
			Op:   OpMoveTo,
			Args: []float64{posX, posY},
		})
	}
	rLineTo := func(dx, dy float64) {
		posX += dx
		posY += dy
		res.Cmds = append(res.Cmds, GlyphOp{
			Op:   OpLineTo,
			Args: []float64{posX, posY},
		})
	}
	rCurveTo := func(dxa, dya, dxb, dyb, dxc, dyc float64) {
		xa := posX + dxa
		ya := posY + dya
		xb := xa + dxb
		yb := ya + dyb
		posX = xb + dxc
		posY = yb + dyc
		res.Cmds = append(res.Cmds, GlyphOp{
			Op: OpCurveTo,
			Args: []float64{
				xa, ya,
				xb, yb,
				posX, posY,
			},
		})
	}

	cmdStack := [][]byte{code}
	for len(cmdStack) > 0 {
		cmdStack, code = cmdStack[:len(cmdStack)-1], cmdStack[len(cmdStack)-1]

		for len(code) > 0 {
			if len(stack) > maxStack {
				return nil, errStackOverflow
			}

			op := t1op(code[0])
			if op >= 32 && op <= 246 {
				stack = append(stack, float64(op)-139)
				code = code[1:]
				fmt.Println("# push", stack[len(stack)-1])
				continue
			} else if op >= 247 && op <= 250 {
				if len(code) < 2 {
					return nil, errIncomplete
				}
				val := (float64(op)-247)*256 + float64(code[1]) + 108
				stack = append(stack, val)
				code = code[2:]
				fmt.Println("# push", stack[len(stack)-1])
				continue
			} else if op >= 251 && op <= 254 {
				if len(code) < 2 {
					return nil, errIncomplete
				}
				val := (251-float64(op))*256 - float64(code[1]) - 108
				stack = append(stack, val)
				code = code[2:]
				fmt.Println("# push", stack[len(stack)-1])
				continue
			} else if op == 255 {
				if len(code) < 5 {
					return nil, errIncomplete
				}
				val := int32(code[1])<<24 | int32(code[2])<<16 |
					int32(code[3])<<8 | int32(code[4])
				stack = append(stack, float64(val))
				code = code[5:]
				fmt.Println("# push", stack[len(stack)-1])
				continue
			}

			if op == 12 {
				if len(code) < 2 {
					return nil, errIncomplete
				}
				op = op<<8 | t1op(code[1])
				code = code[2:]
			} else {
				code = code[1:]
			}

			switch op {
			case t1endchar:
				fmt.Println("endchar")
			case t1hsbw:
				if len(stack) < 2 {
					return nil, errIncomplete
				}
				fmt.Printf("hsbw(%g, %g)\n", stack[0], stack[1])
				posX = stack[0]
				posY = 0
				res.LsbX = funit.Int16(math.Round(stack[0]))
				res.LsbY = 0
				res.WidthX = funit.Int16(math.Round(stack[1]))
				res.WidthY = 0
				clearStack()
			case t1seac:
				if len(stack) < 5 {
					return nil, errIncomplete
				}
				fmt.Printf("seac(%g, %g, %g, %g, %g)\n", stack[0], stack[1], stack[2], stack[3], stack[4])
				panic("not implemented") // TODO(voss): implement
			case t1sbw:
				if len(stack) < 4 {
					return nil, errIncomplete
				}
				fmt.Printf("sbw(%g, %g, %g, %g)\n", stack[0], stack[1], stack[2], stack[3])
				posX = stack[0]
				posY = stack[1]
				res.LsbX = funit.Int16(math.Round(stack[0]))
				res.LsbY = funit.Int16(math.Round(stack[1]))
				res.WidthX = funit.Int16(math.Round(stack[2]))
				res.WidthY = funit.Int16(math.Round(stack[3]))
				clearStack()

			case t1closepath:
				fmt.Println("closepath")
			// TODO(voss): what to do here?
			case t1hlineto:
				if len(stack) < 1 {
					return nil, errIncomplete
				}
				fmt.Printf("hlineto(%g)\n", stack[0])
				rLineTo(stack[0], 0)
				clearStack()
			case t1hmoveto:
				if len(stack) < 1 {
					return nil, errIncomplete
				}
				fmt.Printf("hmoveto(%g)\n", stack[0])
				rMoveTo(stack[0], 0)
				clearStack()
			case t1hvcurveto:
				if len(stack) < 4 {
					return nil, errIncomplete
				}
				fmt.Printf("hvcurveto(%g, %g, %g, %g)\n", stack[0], stack[1], stack[2], stack[3])
				rCurveTo(stack[0], 0, stack[1], stack[2], 0, stack[3])
				clearStack()
			case t1rlineto:
				if len(stack) < 2 {
					return nil, errIncomplete
				}
				fmt.Printf("rlineto(%g, %g)\n", stack[0], stack[1])
				rLineTo(stack[0], stack[1])
				clearStack()
			case t1rmoveto:
				if len(stack) < 2 {
					return nil, errIncomplete
				}
				fmt.Printf("rmoveto(%g, %g)\n", stack[0], stack[1])
				rMoveTo(stack[0], stack[1])
				clearStack()
			case t1rrcurveto:
				if len(stack) < 6 {
					return nil, errIncomplete
				}
				fmt.Printf("rrcurveto(%g, %g, %g, %g, %g, %g)\n", stack[0], stack[1], stack[2], stack[3], stack[4], stack[5])
				rCurveTo(stack[0], stack[1], stack[2], stack[3], stack[4], stack[5])
				clearStack()
			case t1vhcurveto:
				if len(stack) < 4 {
					return nil, errIncomplete
				}
				fmt.Printf("vhcurveto(%g, %g, %g, %g)\n", stack[0], stack[1], stack[2], stack[3])
				rCurveTo(0, stack[0], stack[1], stack[2], stack[3], 0)
				clearStack()
			case t1vlineto:
				if len(stack) < 1 {
					return nil, errIncomplete
				}
				fmt.Printf("vlineto(%g)\n", stack[0])
				rLineTo(0, stack[0])
				clearStack()
			case t1vmoveto:
				if len(stack) < 1 {
					return nil, errIncomplete
				}
				fmt.Printf("vmoveto(%g)\n", stack[0])
				rMoveTo(0, stack[0])
				clearStack()

			case t1dotsection:
				fmt.Println("dotsection")
				clearStack()
			case t1hstem:
				if len(stack) < 2 {
					return nil, errIncomplete
				}
				fmt.Printf("hstem(%g, %g)\n", stack[0], stack[1])
				a := res.LsbY + funit.Int16(math.Round(stack[0]))
				b := a + funit.Int16(math.Round(stack[1]))
				res.HStem = append(res.HStem, a, b)
				clearStack()
			case t1hstem3:
				if len(stack) < 6 {
					return nil, errIncomplete
				}
				fmt.Printf("hstem3(%g, %g, %g, %g, %g, %g)\n", stack[0], stack[1], stack[2], stack[3], stack[4], stack[5])
				a := res.LsbY + funit.Int16(math.Round(stack[0]))
				b := a + funit.Int16(math.Round(stack[1]))
				c := res.LsbY + funit.Int16(math.Round(stack[2]))
				d := c + funit.Int16(math.Round(stack[3]))
				e := res.LsbY + funit.Int16(math.Round(stack[4]))
				f := e + funit.Int16(math.Round(stack[5]))
				res.HStem = append(res.HStem[:0], a, b, c, d, e, f)
				clearStack()
			case t1vstem:
				if len(stack) < 2 {
					return nil, errIncomplete
				}
				fmt.Printf("vstem(%g, %g)\n", stack[0], stack[1])
				a := res.LsbX + funit.Int16(math.Round(stack[0]))
				b := a + funit.Int16(math.Round(stack[1]))
				res.VStem = append(res.VStem, a, b)
				clearStack()
			case t1vstem3:
				if len(stack) < 6 {
					return nil, errIncomplete
				}
				fmt.Printf("vstem3(%g, %g, %g, %g, %g, %g)\n", stack[0], stack[1], stack[2], stack[3], stack[4], stack[5])
				a := res.LsbX + funit.Int16(math.Round(stack[0]))
				b := a + funit.Int16(math.Round(stack[1]))
				c := res.LsbX + funit.Int16(math.Round(stack[2]))
				d := c + funit.Int16(math.Round(stack[3]))
				e := res.LsbX + funit.Int16(math.Round(stack[4]))
				f := e + funit.Int16(math.Round(stack[5]))
				res.VStem = append(res.VStem[:0], a, b, c, d, e, f)
				clearStack()

			case t1div:
				if len(stack) < 2 {
					return nil, errIncomplete
				}
				fmt.Printf("div(%g, %g)\n", stack[0], stack[1])
				stack = append(stack, stack[len(stack)-2]/stack[len(stack)-1])
			case t1callothersubr:
				fmt.Println("callothersubr", stack)
				panic("not implemented") // TODO
			case t1callsubr:
				if len(stack) < 1 {
					return nil, errIncomplete
				}
				idx := int(stack[len(stack)-1])
				if idx < 0 || idx >= len(info.subrs) || float64(idx) != stack[len(stack)-1] {
					return nil, invalidSince("invalid subr index")
				}
				stack = stack[:len(stack)-1]
				fmt.Printf("callsubr(%d)\n", idx)

				cmdStack = append(cmdStack, code)
				if len(cmdStack) > 10 {
					return nil, invalidSince("maximum call stack size exceeded")
				}
				code = info.subrs[idx]

			case t1pop:
				fmt.Println("pop")
				panic("not implemented") // TODO
			case t1return:
				fmt.Println("return")
				panic("not implemented") // TODO
			case t1setcurrentpoint:
				if len(stack) < 2 {
					return nil, errIncomplete
				}
				fmt.Printf("setcurrentpoint(%g, %g)\n", stack[0], stack[1])
				posX = stack[0]
				posY = stack[1]
				clearStack()

			default:
				return nil, invalidSince(
					fmt.Sprintf("unsupported type 1 opcode %d", op))
			}
		}
	}

	return res, nil
}

type t1op uint16

func (op t1op) Bytes() []byte {
	if op > 255 {
		return []byte{byte(op >> 8), byte(op)}
	}
	return []byte{byte(op)}
}

func (op t1op) String() string {
	switch op {
	case t1hstem:
		return "t1hstem"
	case t1vstem:
		return "t1vstem"
	case t1vmoveto:
		return "t1vmoveto"
	case t1rlineto:
		return "t1rlineto"
	case t1hlineto:
		return "t1hlineto"
	case t1vlineto:
		return "t1vlineto"
	case t1rrcurveto:
		return "t1rrcurveto"
	case t1callsubr:
		return "t1callsubr"
	case t1return:
		return "t1return"
	case t1hsbw:
		return "t1hsbw"
	case t1endchar:
		return "t1endchar"
	case t1rmoveto:
		return "t1rmoveto"
	case t1hmoveto:
		return "t1hmoveto"
	case t1vhcurveto:
		return "t1vhcurveto"
	case t1hvcurveto:
		return "t1hvcurveto"
	case t1dotsection:
		return "t1dotsection"
	case t1div:
		return "t1div"
	case 255:
		return "t1int32"
	}
	if 32 <= op && op <= 246 {
		return fmt.Sprintf("t1int1(%d)", op)
	}
	if 247 <= op && op <= 254 {
		return fmt.Sprintf("t1int2(%d)", op)
	}
	return fmt.Sprintf("t1op(%d)", op)
}

const (
	t1hstem     t1op = 0x0001
	t1vstem     t1op = 0x0003
	t1vmoveto   t1op = 0x0004
	t1rlineto   t1op = 0x0005
	t1hlineto   t1op = 0x0006
	t1vlineto   t1op = 0x0007
	t1rrcurveto t1op = 0x0008
	t1closepath t1op = 0x0009
	t1callsubr  t1op = 0x000a
	t1return    t1op = 0x000b
	t1hsbw      t1op = 0x000d
	t1endchar   t1op = 0x000e
	t1rmoveto   t1op = 0x0015
	t1hmoveto   t1op = 0x0016
	t1vhcurveto t1op = 0x001e
	t1hvcurveto t1op = 0x001f

	t1dotsection      t1op = 0x0c00
	t1vstem3          t1op = 0x0c01
	t1hstem3          t1op = 0x0c02
	t1seac            t1op = 0x0c06
	t1sbw             t1op = 0x0c07
	t1div             t1op = 0x0c0c
	t1callothersubr   t1op = 0x0c10
	t1pop             t1op = 0x0c11
	t1setcurrentpoint t1op = 0x0c21
)

func invalidSince(reason string) error {
	return &parser.InvalidFontError{
		SubSystem: "type1",
		Reason:    reason,
	}
}

var (
	errStackOverflow = invalidSince("type 1 buildchar stack overflow")
	errIncomplete    = invalidSince("incomplete type 1 charstring")
)
