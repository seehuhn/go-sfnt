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
	"errors"
	"fmt"
	"math"

	"seehuhn.de/go/sfnt/funit"
)

type decodeInfo struct {
	subr         cffIndex
	gsubr        cffIndex
	defaultWidth funit.Int16
	nominalWidth funit.Int16
}

type ccStage int

const (
	stageStart ccStage = iota
	stageStems
	stageHintMask
)

// Fixed16 is a 16.16-bit fixed number.
type Fixed16 int32 // 16.16 fixed point numbers

func f16FromByte(v byte) Fixed16 {
	return Fixed16(v) << 16
}

func f16FromInt16(v int16) Fixed16 {
	return Fixed16(v) << 16
}

func f16FromInt(v int) Fixed16 {
	return Fixed16(v) << 16
}

func f16(v float64) Fixed16 {
	return Fixed16(math.Round(v * 65536))
}

// Byte converts the operand to a byte (rounding towards zero).
func (x Fixed16) Byte() byte {
	return byte(x >> 16)
}

// Int16 converts the operand to an int16 (rounding towards zero).
func (x Fixed16) Int16() int16 {
	return int16(x >> 16)
}

// Int converts the operand to an int (rounding towards zero).
func (x Fixed16) Int() int {
	return int(x >> 16)
}

// Floor returns the largest int16 not greater than x.
func (x Fixed16) Floor() int16 {
	return int16(math.Floor(float64(x) / 65536))
}

// Ceil returns the smallest int16 not less than x.
func (x Fixed16) Ceil() int16 {
	return int16(math.Ceil(float64(x) / 65536))
}

// Float64 converts the operand to a float64.
func (x Fixed16) Float64() float64 {
	return float64(x) / 65536
}

// Abs returns the absolute value of the operand.
func (x Fixed16) Abs() Fixed16 {
	if x < 0 {
		return -x
	}
	return x
}

// decodeCharString returns the commands for the given charstring.
func (info *decodeInfo) decodeCharString(code []byte) (*Glyph, error) {
	res := &Glyph{
		Width: info.defaultWidth,
	}

	stack := make([]Fixed16, 0, maxStack)
	clearStack := func() {
		stack = stack[:0]
	}

	widthIsSet := false
	setGlyphWidth := func(isPresent bool) {
		if widthIsSet {
			return
		}
		if isPresent {
			res.Width = funit.Int16(stack[0].Int16()) + info.nominalWidth
			copy(stack, stack[1:])
			stack = stack[:len(stack)-1]
		}
		widthIsSet = true
	}

	var storage []Fixed16
	cmdStack := [][]byte{code}

	var posX, posY Fixed16
	hasMoved := false
	var moveError error
	rMoveTo := func(dx, dy Fixed16) {
		hasMoved = true
		posX += dx
		posY += dy
		res.Cmds = append(res.Cmds, GlyphOp{
			Op:   OpMoveTo,
			Args: []Fixed16{posX, posY},
		})
	}
	rLineTo := func(dx, dy Fixed16) {
		if !hasMoved {
			moveError = errors.New("lineTo before moveTo")
		}
		posX += dx
		posY += dy
		res.Cmds = append(res.Cmds, GlyphOp{
			Op:   OpLineTo,
			Args: []Fixed16{posX, posY},
		})
	}
	rCurveTo := func(dxa, dya, dxb, dyb, dxc, dyc Fixed16) {
		if !hasMoved {
			moveError = errors.New("curveTo before moveTo")
		}
		xa := posX + dxa
		ya := posY + dya
		xb := xa + dxb
		yb := ya + dyb
		posX = xb + dxc
		posY = yb + dyc
		res.Cmds = append(res.Cmds, GlyphOp{
			Op: OpCurveTo,
			Args: []Fixed16{
				xa, ya,
				xb, yb,
				posX, posY,
			},
		})
	}

	stage := stageStart

	for len(cmdStack) > 0 {
		cmdStack, code = cmdStack[:len(cmdStack)-1], cmdStack[len(cmdStack)-1]

	opLoop:
		for len(code) > 0 {
			if len(stack) > maxStack {
				return nil, errStackOverflow
			}

			op := t2op(code[0])

			if op >= 32 && op <= 246 {
				stack = append(stack, f16FromInt16(int16(op)-139))
				code = code[1:]
				continue
			} else if op >= 247 && op <= 250 {
				if len(code) < 2 {
					return nil, errIncomplete
				}
				val := (int16(op)-247)*256 + int16(code[1]) + 108
				stack = append(stack, f16FromInt16(val))
				code = code[2:]
				continue
			} else if op >= 251 && op <= 254 {
				if len(code) < 2 {
					return nil, errIncomplete
				}
				val := (251-int16(op))*256 - int16(code[1]) - 108
				stack = append(stack, f16FromInt16(val))
				code = code[2:]
				continue
			} else if op == 28 {
				if len(code) < 3 {
					return nil, errIncomplete
				}
				val := int16(code[1])<<8 | int16(code[2])
				stack = append(stack, f16FromInt16(val))
				code = code[3:]
				continue
			} else if op == 255 {
				if len(code) < 5 {
					return nil, errIncomplete
				}
				val := Fixed16(code[1])<<24 | Fixed16(code[2])<<16 |
					Fixed16(code[3])<<8 | Fixed16(code[4])
				stack = append(stack, val)
				code = code[5:]
				continue
			}

			if op == 12 {
				if len(code) < 2 {
					return nil, errIncomplete
				}
				op = op<<8 | t2op(code[1])
				code = code[2:]
			} else {
				code = code[1:]
			}

			switch op {
			case t2rmoveto:
				setGlyphWidth(len(stack) > 2)
				if len(stack) >= 2 {
					rMoveTo(stack[0], stack[1])
				}
				clearStack()

			case t2hmoveto:
				setGlyphWidth(len(stack) > 1)
				if len(stack) >= 1 {
					rMoveTo(stack[0], 0)
				}
				clearStack()

			case t2vmoveto:
				setGlyphWidth(len(stack) > 1)
				if len(stack) >= 1 {
					rMoveTo(0, stack[0])
				}
				clearStack()

			case t2rlineto:
				pos := 0
				for pos+1 < len(stack) {
					rLineTo(stack[pos], stack[pos+1])
					pos += 2
				}
				clearStack()

			case t2hlineto, t2vlineto:
				horizontal := op == t2hlineto
				for _, z := range stack {
					if horizontal {
						rLineTo(z, 0)
					} else {
						rLineTo(0, z)
					}
					horizontal = !horizontal
				}
				clearStack()

			case t2rrcurveto, t2rcurveline, t2rlinecurve:
				tmp := stack
				for op == t2rlinecurve && len(tmp) >= 8 {
					rLineTo(tmp[0], tmp[1])
					tmp = tmp[2:]
				}
				for len(tmp) >= 6 {
					rCurveTo(tmp[0], tmp[1],
						tmp[2], tmp[3],
						tmp[4], tmp[5])
					tmp = tmp[6:]
				}
				if op == t2rcurveline && len(tmp) >= 2 {
					rLineTo(tmp[0], tmp[1])
				}
				clearStack()

			case t2hhcurveto:
				tmp := stack
				var dy1 Fixed16
				if len(tmp)%4 != 0 {
					dy1, tmp = tmp[0], tmp[1:]
				}
				for len(tmp) >= 4 {
					rCurveTo(tmp[0], dy1,
						tmp[1], tmp[2],
						tmp[3], 0)
					tmp = tmp[4:]
					dy1 = 0
				}
				clearStack()

			case t2vvcurveto:
				tmp := stack
				var dx1 Fixed16
				if len(tmp)%4 != 0 {
					dx1, tmp = tmp[0], tmp[1:]
				}
				for len(tmp) >= 4 {
					rCurveTo(dx1, tmp[0],
						tmp[1], tmp[2],
						0, tmp[3])
					tmp = tmp[4:]
					dx1 = 0
				}
				clearStack()

			case t2hvcurveto, t2vhcurveto:
				tmp := stack
				horizontal := op == t2hvcurveto
				for len(tmp) >= 4 {
					var extra Fixed16
					if len(tmp) == 5 {
						extra = tmp[4]
					}
					if horizontal {
						rCurveTo(tmp[0], 0,
							tmp[1], tmp[2],
							extra, tmp[3])
					} else {
						rCurveTo(0, tmp[0],
							tmp[1], tmp[2],
							tmp[3], extra)
					}
					tmp = tmp[4:]
					horizontal = !horizontal
				}
				clearStack()

			case t2flex:
				if len(stack) >= 13 {
					rCurveTo(stack[0], stack[1],
						stack[2], stack[3],
						stack[4], stack[5])
					rCurveTo(stack[6], stack[7],
						stack[8], stack[9],
						stack[10], stack[11])
					// fd = stack[12] / 100
				}
				clearStack()
			case t2flex1:
				if len(stack) >= 11 {
					rCurveTo(stack[0], stack[1],
						stack[2], stack[3],
						stack[4], stack[5])
					extra := stack[10]
					dx := stack[0] + stack[2] + stack[4] + stack[6] + stack[8]
					dy := stack[1] + stack[3] + stack[5] + stack[7] + stack[9]
					if dx.Abs() > dy.Abs() {
						rCurveTo(stack[6], stack[7],
							stack[8], stack[9],
							extra, 0)
					} else {
						rCurveTo(stack[6], stack[7],
							stack[8], stack[9],
							0, extra)
					}
					// fd = 0.5
				}
				clearStack()
			case t2hflex:
				if len(stack) >= 7 {
					rCurveTo(stack[0], 0,
						stack[1], stack[2],
						stack[3], 0)
					rCurveTo(stack[4], 0,
						stack[5], -stack[2],
						stack[6], 0)
					// fd = 0.5
				}
				clearStack()
			case t2hflex1:
				if len(stack) >= 9 {
					rCurveTo(stack[0], stack[1],
						stack[2], stack[3],
						stack[4], 0)
					dy := stack[1] + stack[3] + stack[7]
					rCurveTo(stack[5], 0,
						stack[6], stack[7],
						stack[8], -dy)
					// fd = 0.5
				}
				clearStack()

			case t2dotsection: // deprecated
				clearStack()

			case t2hstem, t2hstemhm:
				if stage > stageStems {
					return nil, errors.New("too late for stem commands")
				} else if len(stack) < 2 {
					return nil, errStackUnderflow
				}
				stage = stageStems
				setGlyphWidth(len(stack)%2 == 1)
				var prev int16
				for k := 0; k+1 < len(stack); k += 2 {
					a := prev + stack[k].Int16()
					b := a + stack[k+1].Int16()
					res.HStem = append(res.HStem, a, b)
					prev = b
				}
				clearStack()

			case t2vstem, t2vstemhm:
				if stage > stageStems {
					return nil, errors.New("too late for stem commands")
				} else if len(stack) < 2 {
					return nil, errStackUnderflow
				}
				stage = stageStems
				setGlyphWidth(len(stack)%2 == 1)
				var prev int16
				for k := 0; k+1 < len(stack); k += 2 {
					a := prev + stack[k].Int16()
					b := a + stack[k+1].Int16()
					res.VStem = append(res.VStem, a, b)
					prev = b
				}
				clearStack()

			case t2hintmask, t2cntrmask:
				if len(stack) >= 2 {
					if stage > stageStems {
						return nil, errors.New("too late for stem commands")
					}
					stage = stageStems
				}
				setGlyphWidth(len(stack)%2 == 1)

				// "If hstem and vstem hints are both declared at the beginning
				// of a charstring, and this sequence is followed directly by
				// the hintmask or cntrmask operators, the vstem hint operator
				// need not be included."
				var prev int16
				for k := 0; k+1 < len(stack); k += 2 {
					a := prev + stack[k].Int16()
					b := a + stack[k+1].Int16()
					res.VStem = append(res.VStem, a, b)
					prev = b
				}

				if stage < stageStems {
					return nil, errors.New("too early for hintmask")
				}
				stage = stageHintMask

				nStems := (len(res.HStem) + len(res.VStem)) / 2
				if nStems == 0 {
					return nil, errIncomplete
				}
				k := (nStems + 7) / 8
				if k >= len(code) {
					return nil, errIncomplete
				}

				cmd := GlyphOp{
					Op: OpHintMask,
				}
				if op == t2cntrmask {
					cmd.Op = OpCntrMask
				}
				for _, b := range code[:k] {
					cmd.Args = append(cmd.Args, f16FromByte(b))
				}
				res.Cmds = append(res.Cmds, cmd)

				code = code[k:]
				clearStack()

			case t2abs:
				k := len(stack) - 1
				if k < 0 {
					return nil, errStackUnderflow
				}
				if stack[k] < 0 {
					stack[k] = -stack[k]
				}
			case t2add:
				k := len(stack) - 2
				if k < 0 {
					return nil, errStackUnderflow
				}
				stack[k] += stack[k+1]
				stack = stack[:k+1]
			case t2sub:
				k := len(stack) - 2
				if k < 0 {
					return nil, errStackUnderflow
				}
				stack[k] -= stack[k+1]
				stack = stack[:k+1]
			case t2div:
				k := len(stack) - 2
				if k < 0 {
					return nil, errStackUnderflow
				}
				var x Fixed16
				if stack[k+1] != 0 {
					x = f16(stack[k].Float64() / stack[k+1].Float64())
				}
				stack[k] = x
				stack = stack[:k+1]
			case t2neg:
				k := len(stack) - 1
				if k < 0 {
					return nil, errStackUnderflow
				}
				stack[k] = -stack[k]
			case t2random:
				stack = append(stack, 40501) // a random fixed16 in (0, 1]
			case t2mul:
				k := len(stack) - 2
				if k < 0 {
					return nil, errStackUnderflow
				}
				stack[k] = Fixed16(int64(stack[k]) * int64(stack[k+1]) >> 16)
				stack = stack[:k+1]
			case t2sqrt:
				k := len(stack) - 1
				if k < 0 {
					return nil, errStackUnderflow
				}
				var x Fixed16
				if stack[k] > 0 {
					x = f16(math.Sqrt(stack[k].Float64()))
				}
				stack[k] = x
			case t2drop:
				k := len(stack) - 1
				if k < 0 {
					return nil, errStackUnderflow
				}
				stack = stack[:k]
			case t2exch:
				k := len(stack) - 2
				if k < 0 {
					return nil, errStackUnderflow
				}
				stack[k], stack[k+1] = stack[k+1], stack[k]
			case t2index:
				k := len(stack) - 1
				if k < 0 {
					return nil, errStackUnderflow
				}
				idx := stack[k].Int()
				if idx < 0 {
					idx = 0
				}
				if k-idx-1 < 0 {
					return nil, errors.New("invalid index")
				}
				stack[k] = stack[k-idx-1]
			case t2roll:
				k := len(stack) - 2
				if k < 0 {
					return nil, errStackUnderflow
				}
				n := stack[k].Int()
				j := stack[k+1].Int()
				if n <= 0 || n > k {
					return nil, errors.New("invalid roll count")
				}
				roll(stack[k-n:k], j)
				stack = stack[:k]
			case t2dup:
				k := len(stack) - 1
				if k < 0 {
					return nil, errStackUnderflow
				}
				stack = append(stack, stack[k])

			case t2put:
				k := len(stack) - 2
				if k < 0 {
					return nil, errStackUnderflow
				}
				m := stack[k+1].Int()
				if m < 0 || m >= 32 {
					return nil, errors.New("cff: invalid store index")
				}
				if storage == nil {
					storage = make([]Fixed16, 32)
				}
				storage[m] = stack[k]
				stack = stack[:k]
			case t2get:
				k := len(stack) - 1
				if k < 0 {
					return nil, errStackUnderflow
				}
				m := stack[k].Int()
				if m < 0 || m >= len(storage) {
					return nil, errors.New("cff: invalid store index")
				}
				stack[k] = storage[m]

			case t2and:
				k := len(stack) - 2
				if k < 0 {
					return nil, errStackUnderflow
				}
				var val Fixed16
				if stack[k] != 0 && stack[k+1] != 0 {
					val = f16FromInt16(1)
				}
				stack = append(stack[:k], val)
			case t2or:
				k := len(stack) - 2
				if k < 0 {
					return nil, errStackUnderflow
				}
				var val Fixed16
				if stack[k] != 0 || stack[k+1] != 0 {
					val = f16FromInt16(1)
				}
				stack = append(stack[:k], val)
			case t2not:
				k := len(stack) - 1
				if k < 0 {
					return nil, errStackUnderflow
				}
				if stack[k] == 0 {
					stack[k] = f16FromInt16(1)
				} else {
					stack[k] = 0
				}
			case t2eq:
				k := len(stack) - 2
				if k < 0 {
					return nil, errStackUnderflow
				}
				if stack[k] == stack[k+1] {
					stack[k] = f16FromInt16(1)
				} else {
					stack[k] = 0
				}
				stack = stack[:k+1]
			case t2ifelse:
				k := len(stack) - 4
				if k < 0 {
					return nil, errStackUnderflow
				}
				var val Fixed16
				if stack[k+2] <= stack[k+3] {
					val = stack[k]
				} else {
					val = stack[k+1]
				}
				stack = append(stack[:k], val)

			case t2callsubr, t2callgsubr:
				k := len(stack) - 1
				if k < 0 {
					return nil, errStackUnderflow
				}
				biased := stack[k].Int()
				stack = stack[:k]

				cmdStack = append(cmdStack, code)
				if len(cmdStack) > 10 {
					return nil, errors.New("maximum call stack size exceeded")
				}

				var err error
				if op == t2callsubr {
					code, err = getSubr(info.subr, biased)
				} else {
					code, err = getSubr(info.gsubr, biased)
				}
				if err != nil {
					return nil, err
				}

			case t2return:
				break opLoop

			case t2endchar:
				setGlyphWidth(len(stack) == 1 || len(stack) > 4)
				return res, nil

			default:
				return nil, invalidSince(
					fmt.Sprintf("unsupported type 2 opcode %d", op))
			}

			if moveError != nil {
				return nil, moveError
			}
		} // end of opLoop
	}

	// The normal exit from this function is via the t2endchar case above.
	return nil, errIncomplete
}

func getSubr(subrs cffIndex, biased int) ([]byte, error) {
	var offset int
	nSubrs := len(subrs)
	if nSubrs < 1240 {
		offset = 107
	} else if nSubrs < 33900 {
		offset = 1131
	} else {
		offset = 32768
	}

	idx := biased + offset
	if idx < 0 || idx >= len(subrs) {
		return nil, errInvalidSubroutine
	}
	return subrs[idx], nil
}

func roll(data []Fixed16, j int) {
	n := len(data)

	j = j % n
	if j < 0 {
		j += n
	}

	tmp := make([]Fixed16, j)
	copy(tmp, data[n-j:])
	copy(data[j:], data[:n-j])
	copy(data[:j], tmp)
}

type t2op uint16

func (op t2op) Bytes() []byte {
	if op > 255 {
		return []byte{byte(op >> 8), byte(op)}
	}
	return []byte{byte(op)}
}

func (op t2op) String() string {
	switch op {
	case t2hstem:
		return "t2hstem"
	case t2vstem:
		return "t2vstem"
	case t2vmoveto:
		return "t2vmoveto"
	case t2rlineto:
		return "t2rlineto"
	case t2hlineto:
		return "t2hlineto"
	case t2vlineto:
		return "t2vlineto"
	case t2rrcurveto:
		return "t2rrcurveto"
	case t2callsubr:
		return "t2callsubr"
	case t2return:
		return "t2return"
	case t2endchar:
		return "t2endchar"
	case t2hstemhm:
		return "t2hstemhm"
	case t2hintmask:
		return "t2hintmask"
	case t2cntrmask:
		return "t2cntrmask"
	case t2rmoveto:
		return "t2rmoveto"
	case t2hmoveto:
		return "t2hmoveto"
	case t2vstemhm:
		return "t2vstemhm"
	case t2rcurveline:
		return "t2rcurveline"
	case t2rlinecurve:
		return "t2rlinecurve"
	case t2vvcurveto:
		return "t2vvcurveto"
	case t2hhcurveto:
		return "t2hhcurveto"
	case t2shortint:
		return "t2int3"
	case t2callgsubr:
		return "t2callgsubr"
	case t2vhcurveto:
		return "t2vhcurveto"
	case t2hvcurveto:
		return "t2hvcurveto"
	case t2dotsection:
		return "t2dotsection"
	case t2and:
		return "t2and"
	case t2or:
		return "t2or"
	case t2not:
		return "t2not"
	case t2abs:
		return "t2abs"
	case t2add:
		return "t2add"
	case t2sub:
		return "t2sub"
	case t2div:
		return "t2div"
	case t2neg:
		return "t2neg"
	case t2eq:
		return "t2eq"
	case t2drop:
		return "t2drop"
	case t2put:
		return "t2put"
	case t2get:
		return "t2get"
	case t2ifelse:
		return "t2ifelse"
	case t2random:
		return "t2random"
	case t2mul:
		return "t2mul"
	case t2sqrt:
		return "t2sqrt"
	case t2dup:
		return "t2dup"
	case t2exch:
		return "t2exch"
	case t2index:
		return "t2index"
	case t2roll:
		return "t2roll"
	case t2hflex:
		return "t2hflex"
	case t2flex:
		return "t2flex"
	case t2hflex1:
		return "t2hflex1"
	case t2flex1:
		return "t2flex1"
	case 255:
		return "t2float4"
	}
	if 32 <= op && op <= 246 {
		return fmt.Sprintf("t2int1(%d)", op)
	}
	if 247 <= op && op <= 254 {
		return fmt.Sprintf("t2int2(%d)", op)
	}
	return fmt.Sprintf("t2op(%d)", op)
}

const (
	t2hstem      t2op = 0x0001
	t2vstem      t2op = 0x0003
	t2vmoveto    t2op = 0x0004
	t2rlineto    t2op = 0x0005
	t2hlineto    t2op = 0x0006
	t2vlineto    t2op = 0x0007
	t2rrcurveto  t2op = 0x0008
	t2callsubr   t2op = 0x000a
	t2return     t2op = 0x000b
	t2endchar    t2op = 0x000e
	t2hstemhm    t2op = 0x0012
	t2hintmask   t2op = 0x0013
	t2cntrmask   t2op = 0x0014
	t2rmoveto    t2op = 0x0015
	t2hmoveto    t2op = 0x0016
	t2vstemhm    t2op = 0x0017
	t2rcurveline t2op = 0x0018
	t2rlinecurve t2op = 0x0019
	t2vvcurveto  t2op = 0x001a
	t2hhcurveto  t2op = 0x001b
	t2shortint   t2op = 0x001c
	t2callgsubr  t2op = 0x001d
	t2vhcurveto  t2op = 0x001e
	t2hvcurveto  t2op = 0x001f

	t2dotsection t2op = 0x0c00
	t2and        t2op = 0x0c03
	t2or         t2op = 0x0c04
	t2not        t2op = 0x0c05
	t2abs        t2op = 0x0c09
	t2add        t2op = 0x0c0a
	t2sub        t2op = 0x0c0b
	t2div        t2op = 0x0c0c
	t2neg        t2op = 0x0c0e
	t2eq         t2op = 0x0c0f
	t2drop       t2op = 0x0c12
	t2put        t2op = 0x0c14
	t2get        t2op = 0x0c15
	t2ifelse     t2op = 0x0c16
	t2random     t2op = 0x0c17
	t2mul        t2op = 0x0c18
	t2sqrt       t2op = 0x0c1a
	t2dup        t2op = 0x0c1b
	t2exch       t2op = 0x0c1c
	t2index      t2op = 0x0c1d
	t2roll       t2op = 0x0c1e
	t2hflex      t2op = 0x0c22
	t2flex       t2op = 0x0c23
	t2hflex1     t2op = 0x0c24
	t2flex1      t2op = 0x0c25
)

var (
	errStackOverflow     = invalidSince("type 2 stack overflow")
	errStackUnderflow    = invalidSince("type 2 stack underflow")
	errIncomplete        = invalidSince("incomplete tpye 2 charstring")
	errInvalidSubroutine = invalidSince("invalid tpye 2 subroutine index")
)
