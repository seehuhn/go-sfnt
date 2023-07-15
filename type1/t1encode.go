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

import "math"

func (g *Glyph) encodeCharString() []byte {
	var buf []byte

	if g.WidthY == 0 {
		buf = appendInt(buf, 0)
		buf = appendInt(buf, int32(g.WidthX))
		buf = appendOp(buf, t1hsbw)
	} else {
		buf = appendInt(buf, 0)
		buf = appendInt(buf, 0)
		buf = appendInt(buf, int32(g.WidthX))
		buf = appendInt(buf, int32(g.WidthY))
		buf = appendOp(buf, t1sbw)
	}

	// TODO(voss): emit hstem3 and vstem3 operators where possible.
	for i := 0; i+1 < len(g.HStem); i += 2 {
		buf = appendInt(buf, int32(g.HStem[i]))
		buf = appendInt(buf, int32(g.HStem[i+1])-int32(g.HStem[i]))
		buf = appendOp(buf, t1hstem)
	}
	for i := 0; i+1 < len(g.VStem); i += 2 {
		buf = appendInt(buf, int32(g.VStem[i]))
		buf = appendInt(buf, int32(g.VStem[i+1])-int32(g.VStem[i]))
		buf = appendOp(buf, t1vstem)
	}

	posX := 0.0
	posY := 0.0
	var dx, dy float64
	for _, cmd := range g.Cmds {
		switch cmd.Op {
		case OpMoveTo:
			if math.Abs(cmd.Args[1]-posY) < 1e-6 {
				buf, dx = appendNumber(buf, cmd.Args[0]-posX)
				buf = appendOp(buf, t1hmoveto)
				posX += dx
			} else if math.Abs(cmd.Args[0]-posX) < 1e-6 {
				buf, dy = appendNumber(buf, cmd.Args[1]-posY)
				buf = appendOp(buf, t1vmoveto)
				posY += dy
			} else {
				buf, dx = appendNumber(buf, cmd.Args[0]-posX)
				buf, dy = appendNumber(buf, cmd.Args[1]-posY)
				buf = appendOp(buf, t1rmoveto)
				posX += dx
				posY += dy
			}
		case OpLineTo:
			if math.Abs(cmd.Args[1]-posY) < 1e-6 {
				buf, dx = appendNumber(buf, cmd.Args[0]-posX)
				buf = appendOp(buf, t1hlineto)
				posX += dx
			} else if math.Abs(cmd.Args[0]-posX) < 1e-6 {
				buf, dy = appendNumber(buf, cmd.Args[1]-posY)
				buf = appendOp(buf, t1vlineto)
				posY += dy
			} else {
				buf, dx = appendNumber(buf, cmd.Args[0]-posX)
				buf, dy = appendNumber(buf, cmd.Args[1]-posY)
				buf = appendOp(buf, t1rlineto)
				posX += dx
				posY += dy
			}
		case OpCurveTo:
			if math.Abs(cmd.Args[1]-posY) < 1e-6 && math.Abs(cmd.Args[4]-cmd.Args[2]) < 1e-6 {
				var dxa, dxb, dyb, dyc float64
				buf, dxa = appendNumber(buf, cmd.Args[0]-posX)
				buf, dxb = appendNumber(buf, cmd.Args[2]-posX-dxa)
				buf, dyb = appendNumber(buf, cmd.Args[3]-posY)
				buf, dyc = appendNumber(buf, cmd.Args[5]-posY-dyb)
				buf = appendOp(buf, t1hvcurveto)
				posX += dxa + dxb
				posY += dyb + dyc
			} else if math.Abs(cmd.Args[0]-posX) < 1e-6 && math.Abs(cmd.Args[5]-cmd.Args[3]) < 1e-6 {
				var dya, dxb, dyb, dxc float64
				buf, dya = appendNumber(buf, cmd.Args[1]-posY)
				buf, dxb = appendNumber(buf, cmd.Args[2]-posX)
				buf, dyb = appendNumber(buf, cmd.Args[3]-posY-dya)
				buf, dxc = appendNumber(buf, cmd.Args[4]-posX-dxb)
				buf = appendOp(buf, t1vhcurveto)
				posX += dxb + dxc
				posY += dya + dyb
			} else {
				var dxa, dxb, dxc, dya, dyb, dyc float64
				buf, dxa = appendNumber(buf, cmd.Args[0]-posX)
				buf, dya = appendNumber(buf, cmd.Args[1]-posY)
				buf, dxb = appendNumber(buf, cmd.Args[2]-posX-dxa)
				buf, dyb = appendNumber(buf, cmd.Args[3]-posY-dya)
				buf, dxc = appendNumber(buf, cmd.Args[4]-posX-dxa-dxb)
				buf, dyc = appendNumber(buf, cmd.Args[5]-posY-dya-dyb)
				buf = appendOp(buf, t1rrcurveto)
				posX += dxa + dxb + dxc
				posY += dya + dyb + dyc
			}
		default:
			panic("unreachable")
		}
	}
	buf = appendOp(buf, t1endchar)
	return buf
}

func appendOp(buf []byte, op t1op) []byte {
	if op < 256 {
		return append(buf, byte(op))
	} else {
		return append(buf, byte(op>>8), byte(op))
	}
}

func appendInt(buf []byte, x int32) []byte {
	switch {
	case x >= -107 && x <= 107:
		return append(buf, byte(x+139))
	case x >= 108 && x <= 1131:
		x -= 108
		return append(buf, byte(x/256+247), byte(x%256))
	case x >= -1131 && x <= -108:
		x = -x - 108
		return append(buf, byte(x/256+251), byte(x%256))
	default:
		return append(buf, 255, byte(x>>24), byte(x>>16), byte(x>>8), byte(x))
	}
}

func appendNumber(buf []byte, x float64) ([]byte, float64) {
	xInt := int32(x)
	if float64(xInt) == x {
		return appendInt(buf, xInt), x
	}

	var bestP, bestQ int32
	bestDelta := math.Inf(1)
	for q := int32(1); q <= 107; q++ {
		pf := math.Round(x * float64(q))
		if pf > math.MaxInt32 {
			pf = math.MaxInt32
		} else if pf < math.MinInt32 {
			pf = math.MinInt32
		}
		p := int32(pf)
		delta := math.Abs(pf/float64(q) - x)
		if delta <= bestDelta {
			bestDelta = delta
			bestP = p
			bestQ = q
		}
	}
	buf = appendInt(buf, bestP)
	buf = appendInt(buf, bestQ)
	buf = appendOp(buf, t1div)
	return buf, float64(bestP) / float64(bestQ)
}
