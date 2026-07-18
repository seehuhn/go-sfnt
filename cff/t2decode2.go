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
	"math"

	"seehuhn.de/go/membudget"
)

// maxStackCFF2 is the CFF2 interpreter operand stack depth limit.
// CFF2 raises the CFF1 limit of 48 to 513.
const maxStackCFF2 = 513

// CFF2 charstring operators that do not exist in CFF1.
const (
	t2vsindex t2op = 0x000f
	t2blend   t2op = 0x0010
)

// decodeInfoCFF2 carries the per-font-DICT context needed to decode a CFF2
// charstring.  regionCount must be non-nil: it returns the number of variation
// regions of the item variation data subtable selected by a vsindex, and the
// error it returns for an out-of-range index is propagated.  budget must be
// non-nil.
type decodeInfoCFF2 struct {
	subr, gsubr    cffIndex
	defaultVSIndex int                            // from the FD's PrivateCFF2.VSIndex
	regionCount    func(vsindex int) (int, error) // region count of an IVD subtable
	budget         *membudget.Budget
}

// decodeCharStringCFF2 decodes a CFF2 charstring into a GlyphCFF2.
//
// The interpreter differs from the CFF1 one (decodeCharString) in three ways:
// the operand stack holds Blend values rather than plain numbers; the operator
// set is the CFF2 subset (no endchar, return, arithmetic or storage ops, plus
// the new vsindex and blend operators); and there is no explicit terminator –
// the top-level charstring ends at end-of-bytes and a subroutine returns at its
// own end-of-bytes, resuming the caller.  There is no charstring width.
func (info *decodeInfoCFF2) decodeCharStringCFF2(code []byte) (*GlyphCFF2, error) {
	if err := info.budget.Charge(len(code)); err != nil {
		return nil, err
	}

	res := &GlyphCFF2{}

	// currentVSIndex selects the active item variation data subtable.  It is
	// initialised from the FD default and can be changed by a vsindex operator,
	// but only before the first blend.
	currentVSIndex := info.defaultVSIndex
	blendSeen := false
	vsindexSeen := false

	stack := make([]Blend, 0, maxStackCFF2)
	clearStack := func() {
		stack = stack[:0]
	}

	var posX, posY Blend
	hasMoved := false
	var moveError error
	rMoveTo := func(dx, dy Blend) {
		hasMoved = true
		posX = addBlend(posX, fixBlend(dx))
		posY = addBlend(posY, fixBlend(dy))
		res.Cmds = append(res.Cmds, GlyphOpCFF2{
			Op:   OpMoveTo,
			Args: []Blend{posX, posY},
		})
	}
	rLineTo := func(dx, dy Blend) {
		if !hasMoved {
			moveError = errMoveMissing
		}
		posX = addBlend(posX, fixBlend(dx))
		posY = addBlend(posY, fixBlend(dy))
		res.Cmds = append(res.Cmds, GlyphOpCFF2{
			Op:   OpLineTo,
			Args: []Blend{posX, posY},
		})
	}
	rCurveTo := func(dxa, dya, dxb, dyb, dxc, dyc Blend) {
		if !hasMoved {
			moveError = errMoveMissing
		}
		xa := addBlend(posX, fixBlend(dxa))
		ya := addBlend(posY, fixBlend(dya))
		xb := addBlend(xa, fixBlend(dxb))
		yb := addBlend(ya, fixBlend(dyb))
		posX = addBlend(xb, fixBlend(dxc))
		posY = addBlend(yb, fixBlend(dyc))
		res.Cmds = append(res.Cmds, GlyphOpCFF2{
			Op:   OpCurveTo,
			Args: []Blend{xa, ya, xb, yb, posX, posY},
		})
	}

	stage := stageStart

	cmdStack := [][]byte{code}
	for len(cmdStack) > 0 {
		cmdStack, code = cmdStack[:len(cmdStack)-1], cmdStack[len(cmdStack)-1]

		for len(code) > 0 {
			if len(stack) > maxStackCFF2 {
				return nil, errStackOverflow
			}

			op := t2op(code[0])

			// number operands push Blend{Default: v}
			switch {
			case op >= 32 && op <= 246:
				stack = append(stack, Blend{Default: float64(int16(op) - 139)})
				code = code[1:]
				continue
			case op >= 247 && op <= 250:
				if len(code) < 2 {
					return nil, errIncomplete
				}
				val := (int16(op)-247)*256 + int16(code[1]) + 108
				stack = append(stack, Blend{Default: float64(val)})
				code = code[2:]
				continue
			case op >= 251 && op <= 254:
				if len(code) < 2 {
					return nil, errIncomplete
				}
				val := (251-int16(op))*256 - int16(code[1]) - 108
				stack = append(stack, Blend{Default: float64(val)})
				code = code[2:]
				continue
			case op == 28:
				if len(code) < 3 {
					return nil, errIncomplete
				}
				val := int16(code[1])<<8 | int16(code[2])
				stack = append(stack, Blend{Default: float64(val)})
				code = code[3:]
				continue
			case op == 255:
				if len(code) < 5 {
					return nil, errIncomplete
				}
				val := int32(code[1])<<24 | int32(code[2])<<16 | int32(code[3])<<8 | int32(code[4])
				stack = append(stack, Blend{Default: float64(val) / 65536})
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
				if len(stack) >= 2 {
					rMoveTo(stack[0], stack[1])
				}
				clearStack()

			case t2hmoveto:
				if len(stack) >= 1 {
					rMoveTo(stack[0], Blend{})
				}
				clearStack()

			case t2vmoveto:
				if len(stack) >= 1 {
					rMoveTo(Blend{}, stack[0])
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
						rLineTo(z, Blend{})
					} else {
						rLineTo(Blend{}, z)
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
				var dy1 Blend
				if len(tmp)%4 != 0 {
					dy1, tmp = tmp[0], tmp[1:]
				}
				for len(tmp) >= 4 {
					rCurveTo(tmp[0], dy1,
						tmp[1], tmp[2],
						tmp[3], Blend{})
					tmp = tmp[4:]
					dy1 = Blend{}
				}
				clearStack()

			case t2vvcurveto:
				tmp := stack
				var dx1 Blend
				if len(tmp)%4 != 0 {
					dx1, tmp = tmp[0], tmp[1:]
				}
				for len(tmp) >= 4 {
					rCurveTo(dx1, tmp[0],
						tmp[1], tmp[2],
						Blend{}, tmp[3])
					tmp = tmp[4:]
					dx1 = Blend{}
				}
				clearStack()

			case t2hvcurveto, t2vhcurveto:
				tmp := stack
				horizontal := op == t2hvcurveto
				for len(tmp) >= 4 {
					var extra Blend
					if len(tmp) == 5 {
						extra = tmp[4]
					}
					if horizontal {
						rCurveTo(tmp[0], Blend{},
							tmp[1], tmp[2],
							extra, tmp[3])
					} else {
						rCurveTo(Blend{}, tmp[0],
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
				}
				clearStack()
			case t2flex1:
				if len(stack) >= 11 {
					rCurveTo(stack[0], stack[1],
						stack[2], stack[3],
						stack[4], stack[5])
					extra := stack[10]
					// direction of the closing point is chosen from the
					// default instance; the outline topology is fixed.
					dx := stack[0].Default + stack[2].Default + stack[4].Default +
						stack[6].Default + stack[8].Default
					dy := stack[1].Default + stack[3].Default + stack[5].Default +
						stack[7].Default + stack[9].Default
					if math.Abs(dx) > math.Abs(dy) {
						rCurveTo(stack[6], stack[7],
							stack[8], stack[9],
							extra, Blend{})
					} else {
						rCurveTo(stack[6], stack[7],
							stack[8], stack[9],
							Blend{}, extra)
					}
				}
				clearStack()
			case t2hflex:
				if len(stack) >= 7 {
					rCurveTo(stack[0], Blend{},
						stack[1], stack[2],
						stack[3], Blend{})
					rCurveTo(stack[4], Blend{},
						stack[5], negBlend(stack[2]),
						stack[6], Blend{})
				}
				clearStack()
			case t2hflex1:
				if len(stack) >= 9 {
					rCurveTo(stack[0], stack[1],
						stack[2], stack[3],
						stack[4], Blend{})
					dy := addBlend(addBlend(stack[1], stack[3]), stack[7])
					rCurveTo(stack[5], Blend{},
						stack[6], stack[7],
						stack[8], negBlend(dy))
				}
				clearStack()

			case t2hstem, t2hstemhm:
				if stage > stageStems {
					return nil, errTooLateStem
				} else if len(stack) < 2 {
					return nil, errStackUnderflow
				}
				stage = stageStems
				appendStems(&res.HStem, stack)
				clearStack()

			case t2vstem, t2vstemhm:
				if stage > stageStems {
					return nil, errTooLateStem
				} else if len(stack) < 2 {
					return nil, errStackUnderflow
				}
				stage = stageStems
				appendStems(&res.VStem, stack)
				clearStack()

			case t2hintmask, t2cntrmask:
				if len(stack) >= 2 {
					if stage > stageStems {
						return nil, errTooLateStem
					}
					stage = stageStems
				}

				// implicit vstem operands before hintmask (as in CFF1: a
				// leading hstem/vstem pair sequence may omit the vstem op).
				appendStems(&res.VStem, stack)
				clearStack()

				nStems := (len(res.HStem) + len(res.VStem)) / 2
				if nStems == 0 {
					// permissive read: a hintmask with no declared stems
					// consumes zero mask bytes and is dropped; a strict
					// writer never emits such an operator.
					continue
				}
				stage = stageHintMask

				nMask := (nStems + 7) / 8
				if nMask > len(code) {
					return nil, errIncomplete
				}

				// mask bytes are stored as non-varying Blend values, one per
				// byte (mirroring CFF1's one-float64-per-byte convention), so
				// the CFF2 encoder can round-trip them.
				cmd := GlyphOpCFF2{Op: OpHintMask}
				if op == t2cntrmask {
					cmd.Op = OpCntrMask
				}
				for _, b := range code[:nMask] {
					cmd.Args = append(cmd.Args, Blend{Default: float64(b)})
				}
				res.Cmds = append(res.Cmds, cmd)
				code = code[nMask:]

			case t2vsindex:
				if len(stack) < 1 {
					return nil, errStackUnderflow
				}
				v := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if blendSeen || vsindexSeen {
					// vsindex after a blend, or a second vsindex, is ignored
					// permissively (the operand is dropped above).
					continue
				}
				vsindexSeen = true
				idx := int(v.Default)
				if v.Deltas != nil || v.Default != float64(idx) || idx < 0 {
					return nil, errBadVSIndex
				}
				currentVSIndex = idx

			case t2blend:
				blendSeen = true
				if len(stack) < 1 {
					return nil, errStackUnderflow
				}
				nB := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				n := int(nB.Default)
				if nB.Deltas != nil || nB.Default != float64(n) || n < 1 {
					return nil, errBadBlend
				}
				k, err := info.regionCount(currentVSIndex)
				if err != nil {
					return nil, err
				}
				// charge delta storage so region-heavy fonts cannot amplify
				// memory beyond the budget
				if err := info.budget.Charge(n * k * 8); err != nil {
					return nil, err
				}
				need := n * (k + 1)
				if len(stack) < need {
					return nil, errStackUnderflow
				}
				base := len(stack) - need
				for i := range n {
					b := stack[base+i]
					if b.Deltas != nil {
						// a base must be a raw number, not a previous blend
						return nil, errBadBlend
					}
					if k > 0 {
						deltas := make([]float64, k)
						for j := range k {
							d := stack[base+n+i*k+j]
							if d.Deltas != nil {
								return nil, errBadBlend
							}
							deltas[j] = d.Default
						}
						b.Deltas = deltas
					}
					stack[base+i] = b
				}
				stack = stack[:base+n]

			case t2callsubr, t2callgsubr:
				k := len(stack) - 1
				if k < 0 {
					return nil, errStackUnderflow
				}
				biased := int(stack[k].Default)
				stack = stack[:k]

				cmdStack = append(cmdStack, code)
				if len(cmdStack) > 10 {
					return nil, invalidSince("maximum call stack size exceeded")
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
				if err := info.budget.Charge(len(code)); err != nil {
					return nil, err
				}

			default:
				return nil, invalidSince("cff2: unsupported charstring operator")
			}

			if moveError != nil {
				return nil, moveError
			}
		}
	}

	res.VSIndex = currentVSIndex
	return res, nil
}

// appendStems prefix-sums the operand pairs and appends the resulting absolute
// stem edges (as Blend pairs) to *out.  A trailing odd operand is ignored.
func appendStems(out *[]Blend, stack []Blend) {
	var prev Blend
	for i := 0; i+1 < len(stack); i += 2 {
		a := addBlend(prev, stack[i])
		b := addBlend(a, stack[i+1])
		*out = append(*out, a, b)
		prev = b
	}
}

// addBlend returns the componentwise sum a+b.  Missing deltas are treated as
// zero, so a non-varying value adds cleanly to a varying one.
func addBlend(a, b Blend) Blend {
	res := Blend{Default: a.Default + b.Default}
	n := max(len(a.Deltas), len(b.Deltas))
	if n > 0 {
		res.Deltas = make([]float64, n)
		for i := range n {
			var av, bv float64
			if i < len(a.Deltas) {
				av = a.Deltas[i]
			}
			if i < len(b.Deltas) {
				bv = b.Deltas[i]
			}
			res.Deltas[i] = av + bv
		}
	}
	return res
}

// negBlend returns -b.
func negBlend(b Blend) Blend {
	res := Blend{Default: -b.Default}
	if b.Deltas != nil {
		res.Deltas = make([]float64, len(b.Deltas))
		for i, d := range b.Deltas {
			res.Deltas[i] = -d
		}
	}
	return res
}

// fixBlend clamps and rounds the default to a 16.16 fixed-point number, leaving
// the deltas unrounded.
func fixBlend(b Blend) Blend {
	b.Default = fix(b.Default)
	return b
}

var (
	errMoveMissing = invalidSince("cff2: drawing command before moveto")
	errTooLateStem = invalidSince("cff2: stem command too late")
	errBadVSIndex  = invalidSince("cff2: invalid vsindex operand")
	errBadBlend    = invalidSince("cff2: invalid blend operands")
)
