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
	seacs []seacInfo
}

type seacInfo struct {
	name         string
	base, accent int
	dx, dy       float64
}

func (info *decodeInfo) decodeCharString(code []byte, name string) (*Glyph, error) {
	const maxStack = 24
	stack := make([]float64, 0, maxStack)
	clearStack := func() {
		stack = stack[:0]
	}

	var postscriptStack []float64
	var flexData []float64

	res := &Glyph{}

	var posX, posY float64
	var LsbX funit.Int16 // TODO(voss): use float64
	var LsbY funit.Int16
	isClosed := true
	rClosePath := func() {
		res.Cmds = append(res.Cmds, GlyphOp{Op: OpClosePath})
		isClosed = true
	}
	rMoveTo := func(dx, dy float64) {
		if !isClosed {
			rClosePath()
		}
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
		isClosed = false
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
glyphLoop:
	for len(cmdStack) > 0 {
		cmdStack, code = cmdStack[:len(cmdStack)-1], cmdStack[len(cmdStack)-1]

	opLoop:
		for len(code) > 0 {
			if len(stack) > maxStack {
				return nil, errStackOverflow
			}

			op := t1op(code[0])
			if op >= 32 && op <= 246 {
				stack = append(stack, float64(op)-139)
				code = code[1:]
				// fmt.Println("# push", stack[len(stack)-1])
				continue
			} else if op >= 247 && op <= 250 {
				if len(code) < 2 {
					return nil, errIncomplete
				}
				val := (float64(op)-247)*256 + float64(code[1]) + 108
				stack = append(stack, val)
				code = code[2:]
				// fmt.Println("# push", stack[len(stack)-1])
				continue
			} else if op >= 251 && op <= 254 {
				if len(code) < 2 {
					return nil, errIncomplete
				}
				val := (251-float64(op))*256 - float64(code[1]) - 108
				stack = append(stack, val)
				code = code[2:]
				// fmt.Println("# push", stack[len(stack)-1])
				continue
			} else if op == 255 {
				if len(code) < 5 {
					return nil, errIncomplete
				}
				val := int32(code[1])<<24 | int32(code[2])<<16 |
					int32(code[3])<<8 | int32(code[4])
				stack = append(stack, float64(val))
				code = code[5:]
				// fmt.Println("# push", stack[len(stack)-1])
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

		opSwitch:
			switch op {
			case t1endchar:
				// fmt.Println("endchar")
				if !isClosed {
					rClosePath()
				}
				break glyphLoop
			case t1hsbw:
				if len(stack) < 2 {
					return nil, errIncomplete
				}
				// fmt.Printf("hsbw(%g, %g)\n", stack[0], stack[1])
				posX = stack[0]
				posY = 0
				LsbX = funit.Int16(math.Round(stack[0]))
				LsbY = 0
				res.WidthX = funit.Int16(math.Round(stack[1]))
				res.WidthY = 0
				clearStack()
			case t1seac:
				if len(stack) < 5 {
					return nil, errIncomplete
				}
				// fmt.Printf("seac(%g, %g, %g, %g, %g)\n", stack[0], stack[1], stack[2], stack[3], stack[4])
				// asb := stack[0]
				adX := stack[1]
				adY := stack[2]
				bchar, err := getInt(stack[3])
				if err != nil {
					return nil, err
				}
				achar, err := getInt(stack[4])
				if err != nil {
					return nil, err
				}
				info.seacs = append(info.seacs, seacInfo{
					name:   name,
					base:   bchar,
					accent: achar,
					dx:     adX,
					dy:     adY,
				})
				// clearStack()
				break glyphLoop
			case t1sbw:
				if len(stack) < 4 {
					return nil, errIncomplete
				}
				// fmt.Printf("sbw(%g, %g, %g, %g)\n", stack[0], stack[1], stack[2], stack[3])
				posX = stack[0]
				posY = stack[1]
				LsbX = funit.Int16(math.Round(stack[0]))
				LsbY = funit.Int16(math.Round(stack[1]))
				res.WidthX = funit.Int16(math.Round(stack[2]))
				res.WidthY = funit.Int16(math.Round(stack[3]))
				clearStack()

			case t1closepath:
				// fmt.Println("closepath")
				rClosePath()
			case t1hlineto:
				if len(stack) < 1 {
					return nil, errIncomplete
				}
				// fmt.Printf("hlineto(%g)\n", stack[0])
				rLineTo(stack[0], 0)
				clearStack()
			case t1hmoveto:
				if len(stack) < 1 {
					return nil, errIncomplete
				}
				// fmt.Printf("hmoveto(%g)\n", stack[0])
				rMoveTo(stack[0], 0)
				clearStack()
			case t1hvcurveto:
				if len(stack) < 4 {
					return nil, errIncomplete
				}
				// fmt.Printf("hvcurveto(%g, %g, %g, %g)\n", stack[0], stack[1], stack[2], stack[3])
				rCurveTo(stack[0], 0, stack[1], stack[2], 0, stack[3])
				clearStack()
			case t1rlineto:
				if len(stack) < 2 {
					return nil, errIncomplete
				}
				// fmt.Printf("rlineto(%g, %g)\n", stack[0], stack[1])
				rLineTo(stack[0], stack[1])
				clearStack()
			case t1rmoveto:
				if len(stack) < 2 {
					return nil, errIncomplete
				}
				// fmt.Printf("rmoveto(%g, %g)\n", stack[0], stack[1])
				rMoveTo(stack[0], stack[1])
				clearStack()
			case t1rrcurveto:
				if len(stack) < 6 {
					return nil, errIncomplete
				}
				// fmt.Printf("rrcurveto(%g, %g, %g, %g, %g, %g)\n", stack[0], stack[1], stack[2], stack[3], stack[4], stack[5])
				rCurveTo(stack[0], stack[1], stack[2], stack[3], stack[4], stack[5])
				clearStack()
			case t1vhcurveto:
				if len(stack) < 4 {
					return nil, errIncomplete
				}
				// fmt.Printf("vhcurveto(%g, %g, %g, %g)\n", stack[0], stack[1], stack[2], stack[3])
				rCurveTo(0, stack[0], stack[1], stack[2], stack[3], 0)
				clearStack()
			case t1vlineto:
				if len(stack) < 1 {
					return nil, errIncomplete
				}
				// fmt.Printf("vlineto(%g)\n", stack[0])
				rLineTo(0, stack[0])
				clearStack()
			case t1vmoveto:
				if len(stack) < 1 {
					return nil, errIncomplete
				}
				// fmt.Printf("vmoveto(%g)\n", stack[0])
				rMoveTo(0, stack[0])
				clearStack()

			case t1dotsection:
				// fmt.Println("dotsection")
				clearStack()
			case t1hstem:
				if len(stack) < 2 {
					return nil, errIncomplete
				}
				// fmt.Printf("hstem(%g, %g)\n", stack[0], stack[1])
				a := LsbY + funit.Int16(math.Round(stack[0]))
				b := a + funit.Int16(math.Round(stack[1]))
				res.HStem = append(res.HStem, a, b)
				clearStack()
			case t1hstem3:
				if len(stack) < 6 {
					return nil, errIncomplete
				}
				// fmt.Printf("hstem3(%g, %g, %g, %g, %g, %g)\n", stack[0], stack[1], stack[2], stack[3], stack[4], stack[5])
				a := LsbY + funit.Int16(math.Round(stack[0]))
				b := a + funit.Int16(math.Round(stack[1]))
				c := LsbY + funit.Int16(math.Round(stack[2]))
				d := c + funit.Int16(math.Round(stack[3]))
				e := LsbY + funit.Int16(math.Round(stack[4]))
				f := e + funit.Int16(math.Round(stack[5]))
				res.HStem = append(res.HStem[:0], a, b, c, d, e, f)
				clearStack()
			case t1vstem:
				if len(stack) < 2 {
					return nil, errIncomplete
				}
				// fmt.Printf("vstem(%g, %g)\n", stack[0], stack[1])
				a := LsbX + funit.Int16(math.Round(stack[0]))
				b := a + funit.Int16(math.Round(stack[1]))
				res.VStem = append(res.VStem, a, b)
				clearStack()
			case t1vstem3:
				if len(stack) < 6 {
					return nil, errIncomplete
				}
				// fmt.Printf("vstem3(%g, %g, %g, %g, %g, %g)\n", stack[0], stack[1], stack[2], stack[3], stack[4], stack[5])
				a := LsbX + funit.Int16(math.Round(stack[0]))
				b := a + funit.Int16(math.Round(stack[1]))
				c := LsbX + funit.Int16(math.Round(stack[2]))
				d := c + funit.Int16(math.Round(stack[3]))
				e := LsbX + funit.Int16(math.Round(stack[4]))
				f := e + funit.Int16(math.Round(stack[5]))
				res.VStem = append(res.VStem[:0], a, b, c, d, e, f)
				clearStack()
			case t1div:
				if len(stack) < 2 {
					return nil, errIncomplete
				}
				// fmt.Printf("div(%g, %g)\n", stack[0], stack[1])
				stack = append(stack[:len(stack)-2], stack[len(stack)-2]/stack[len(stack)-1])

			case t1callsubr:
				if len(stack) < 1 {
					return nil, errIncomplete
				}
				idx, err := getInt(stack[len(stack)-1])
				if err != nil {
					return nil, err
				}
				stack = stack[:len(stack)-1]
				switch idx { // pre-defined subroutines
				case 3:
					// Entry 3 in the Subrs array is a charstring that does nothing.
					break opSwitch
				}

				if idx < 0 || idx >= len(info.subrs) {
					return nil, invalidSince("invalid subr index")
				}
				// fmt.Printf("callsubr(%d)\n", idx)

				cmdStack = append(cmdStack, code)
				if len(cmdStack) > 10 {
					return nil, invalidSince("maximum call stack size exceeded")
				}
				code = info.subrs[idx]
			case t1callothersubr:
				if len(stack) < 2 {
					return nil, errIncomplete
				}
				idx, err := getInt(stack[len(stack)-1])
				if err != nil {
					return nil, err
				}
				argN, err := getInt(stack[len(stack)-2])
				if err != nil {
					return nil, err
				}
				if len(stack) < argN+2 {
					return nil, errIncomplete
				}
				// fmt.Println("callothersubr", idx, args)
				stack = stack[:len(stack)-2]
				postscriptStack = postscriptStack[:0]
				for i := 0; i < argN; i++ {
					val := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					postscriptStack = append(postscriptStack, val)
				}
				// horizontal flex:
				// starting point: x0 A
				// reference point: x3 A
				//     x1 y1  x2  B  x3 B curveto
				//     x4  B  x5 y5  x6 A curveto
				// seven coordinate pairs:
				//     x3-x0 0     rmoveto   reference point relative to starting point
				//     x1-x3 y1-A  rmoveto   first rrcurveto pair relative to reference point
				//     x2-x1 B-y1  rmoveto
				//     x3-x2 0     rmoveto
				//     x4-x3 0     rmoveto   second rrcurveto
				//     x5-x4 y5-B  rmoveto
				//     x6-x5 A-y5  rmoveto

				switch idx {
				case 0: // flex end (3 args, 2 returns)
					if len(flexData) == 14 {
						res.Cmds = append(res.Cmds, GlyphOp{
							Op: OpCurveTo,
							Args: []float64{
								flexData[2], flexData[3],
								flexData[4], flexData[5],
								flexData[6], flexData[7],
							},
						}, GlyphOp{
							Op: OpCurveTo,
							Args: []float64{
								flexData[8], flexData[9],
								flexData[10], flexData[11],
								flexData[12], flexData[13],
							},
						})
					}
					postscriptStack = postscriptStack[:len(postscriptStack)-1]
				case 1: // flex start (0 args)
					flexData = flexData[:0]
				case 2: // flex coordinate pair (0 args)
					flexData = append(flexData, posX, posY)
					if len(res.Cmds) > 0 {
						// remove the rmoveTo command
						res.Cmds = res.Cmds[:len(res.Cmds)-1]
					}
				case 3: // hint replacement (1 arg)
					postscriptStack = append(postscriptStack[:0], 3)
				default:
					// can be ignored
				}
			case t1pop:
				if len(postscriptStack) < 1 {
					return nil, invalidSince("postscript interpreter operand stack underflow")
				}
				// fmt.Println("pop")
				val := postscriptStack[len(postscriptStack)-1]
				postscriptStack = postscriptStack[:len(postscriptStack)-1]
				stack = append(stack, val)

			case t1return:
				// fmt.Println("return")
				break opLoop
			case t1setcurrentpoint:
				if len(stack) < 2 {
					return nil, errIncomplete
				}
				// fmt.Printf("setcurrentpoint(%g, %g)\n", stack[0], stack[1])
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

func getInt(x float64) (int, error) {
	i := int(x)
	if float64(i) != x {
		return 0, invalidSince("invalid operand type")
	}
	return i, nil
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
		return "hstem"
	case t1vstem:
		return "vstem"
	case t1vmoveto:
		return "vmoveto"
	case t1rlineto:
		return "rlineto"
	case t1hlineto:
		return "hlineto"
	case t1vlineto:
		return "vlineto"
	case t1rrcurveto:
		return "rrcurveto"
	case t1callsubr:
		return "callsubr"
	case t1return:
		return "return"
	case t1hsbw:
		return "hsbw"
	case t1endchar:
		return "endchar"
	case t1rmoveto:
		return "rmoveto"
	case t1hmoveto:
		return "hmoveto"
	case t1vhcurveto:
		return "vhcurveto"
	case t1hvcurveto:
		return "hvcurveto"
	case t1dotsection:
		return "dotsection"
	case t1div:
		return "div"
	case 255:
		return "int32"
	}
	if 32 <= op && op <= 246 {
		return fmt.Sprintf("int1(%d)", op)
	}
	if 247 <= op && op <= 254 {
		return fmt.Sprintf("int2(%d)", op)
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
