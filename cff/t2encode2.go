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
	"math"

	"seehuhn.de/go/postscript/funit"
)

// encodeCharStringCFF2 encodes a decoded CFF2 glyph back into a charstring.
//
// It is the exact inverse of decodeCharStringCFF2: the absolute Blend
// positions stored in the glyph are re-differenced into relative operands,
// runs of varying operands are wrapped in blend operators, and stems are
// emitted before the drawing commands.  Local subroutines are not used, and
// the charstring has no width and no endchar; it ends at end-of-bytes.
func encodeCharStringCFF2(g *GlyphCFF2, private *PrivateCFF2) ([]byte, error) {
	var buf bytes.Buffer

	// a vsindex operator is needed only when the glyph selects a different
	// item variation data subtable than the Font DICT default
	if g.VSIndex != private.VSIndex {
		encodeCharInt(&buf, g.VSIndex)
		buf.WriteByte(byte(t2vsindex))
	}

	hintMaskUsed := false
	for _, cmd := range g.Cmds {
		if cmd.Op == OpHintMask || cmd.Op == OpCntrMask {
			hintMaskUsed = true
			break
		}
	}

	hOp, vOp := t2hstem, t2vstem
	if hintMaskUsed {
		hOp, vOp = t2hstemhm, t2vstemhm
	}
	if err := encodeStemsCFF2(&buf, g.HStem, hOp); err != nil {
		return nil, err
	}
	if err := encodeStemsCFF2(&buf, g.VStem, vOp); err != nil {
		return nil, err
	}

	if err := encodePathsCFF2(&buf, g.Cmds); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// encodePathsCFF2 emits the drawing commands of a CFF2 glyph.  Each command is
// written as its own operator (rmoveto, rlineto or rrcurveto), which keeps the
// operand count small and makes the blend-group split trivial.
func encodePathsCFF2(buf *bytes.Buffer, cmds []GlyphOpCFF2) error {
	var posX, posY Blend
	for _, cmd := range cmds {
		switch cmd.Op {
		case OpMoveTo:
			if len(cmd.Args) < 2 {
				return errBadGlyphCFF2
			}
			ops := []Blend{
				subBlend(cmd.Args[0], posX),
				subBlend(cmd.Args[1], posY),
			}
			posX, posY = cmd.Args[0], cmd.Args[1]
			if err := emitDrawOpCFF2(buf, ops, t2rmoveto); err != nil {
				return err
			}

		case OpLineTo:
			if len(cmd.Args) < 2 {
				return errBadGlyphCFF2
			}
			ops := []Blend{
				subBlend(cmd.Args[0], posX),
				subBlend(cmd.Args[1], posY),
			}
			posX, posY = cmd.Args[0], cmd.Args[1]
			if err := emitDrawOpCFF2(buf, ops, t2rlineto); err != nil {
				return err
			}

		case OpCurveTo:
			if len(cmd.Args) < 6 {
				return errBadGlyphCFF2
			}
			xa, ya := cmd.Args[0], cmd.Args[1]
			xb, yb := cmd.Args[2], cmd.Args[3]
			xc, yc := cmd.Args[4], cmd.Args[5]
			ops := []Blend{
				subBlend(xa, posX), subBlend(ya, posY),
				subBlend(xb, xa), subBlend(yb, ya),
				subBlend(xc, xb), subBlend(yc, yb),
			}
			posX, posY = xc, yc
			if err := emitDrawOpCFF2(buf, ops, t2rrcurveto); err != nil {
				return err
			}

		case OpHintMask, OpCntrMask:
			op := t2hintmask
			if cmd.Op == OpCntrMask {
				op = t2cntrmask
			}
			buf.Write(op.Bytes())
			// mask bytes are stored one per Blend, non-varying
			for _, a := range cmd.Args {
				buf.WriteByte(byte(a.Default))
			}

		default:
			return errBadGlyphCFF2
		}
	}
	return nil
}

// emitDrawOpCFF2 pushes the (already re-differenced) operands, wrapping varying
// runs in blend operators, and then writes the drawing operator.
func emitDrawOpCFF2(buf *bytes.Buffer, operands []Blend, op t2op) error {
	if err := pushOperandsCFF2(buf, operands); err != nil {
		return err
	}
	buf.Write(op.Bytes())
	return nil
}

// encodeStemsCFF2 emits stem-hint operators for the absolute stem edges.  The
// edges are re-differenced (the inverse of appendStems' prefix sum), which the
// decoder restarts from zero for every stem operator.  A single sequence of
// stem operators therefore stores its edges as one continuous prefix sum, so
// re-differencing consecutive edges recovers small relative operands.
//
// The edge list is the concatenation of one or more such operators, and the
// boundaries are lost.  When two operators are concatenated their junction
// shows up as a large jump; re-differencing it would exceed the operand range.
// The encoder therefore splits into a fresh operator whenever the relative
// operand is not encodable (the next operator restarts its prefix sum from
// zero, so its first operand is that edge's absolute value) or whenever the
// operand count reaches the stack limit.  Real fonts never trigger a split.
func encodeStemsCFF2(buf *bytes.Buffer, edges []Blend, op t2op) error {
	if len(edges) == 0 {
		return nil
	}
	const capEdges = 512 // even; bounds the operand stack depth
	start := 0
	for start < len(edges) {
		var prev Blend
		end := start
		lastSafe := start // last even index where the next segment can restart
		for end < len(edges) {
			r := subBlend(edges[end], prev)
			if end > start && !charOperandInRange(r) {
				break // junction between two concatenated operators
			}
			if end-start >= capEdges {
				break
			}
			prev = edges[end]
			end++
			if (end-start)%2 == 0 && end < len(edges) &&
				charOperandInRange(subBlend(edges[end], Blend{})) {
				lastSafe = end
			}
		}
		// when the split is forced by the size cap rather than a junction, back
		// up to an even edge that can start the next segment in range
		if end < len(edges) && end-start >= capEdges {
			if (end-start)%2 != 0 || !charOperandInRange(subBlend(edges[end], Blend{})) {
				if lastSafe > start {
					end = lastSafe
				}
			}
		}

		ops := make([]Blend, end-start)
		var p Blend
		for i := start; i < end; i++ {
			ops[i-start] = subBlend(edges[i], p)
			p = edges[i]
		}
		if err := pushOperandsCFF2(buf, ops); err != nil {
			return err
		}
		buf.Write(op.Bytes())
		start = end
	}
	return nil
}

// charNumberEncodable reports whether v fits a CFF2 charstring number operand
// (int16 or 16.16 fixed).
func charNumberEncodable(v float64) bool {
	return v >= -32768 && v < 32768 && !math.IsNaN(v)
}

// charOperandInRange reports whether every component of a re-differenced
// operand is encodable.
func charOperandInRange(b Blend) bool {
	if !charNumberEncodable(b.Default) {
		return false
	}
	for _, d := range b.Deltas {
		if !charNumberEncodable(d) {
			return false
		}
	}
	return true
}

// pushOperandsCFF2 writes the operand list, emitting a blend operator for each
// maximal run of varying operands.  A run is split into sub-groups so that the
// operand stack never exceeds maxStackCFF2: for a group of n operands with k
// deltas each, the peak stack depth is base+n*(k+1)+1, which must stay within
// the 513-slot limit.
func pushOperandsCFF2(buf *bytes.Buffer, operands []Blend) error {
	committed := 0 // operands already finalised on the stack
	i := 0
	for i < len(operands) {
		if !operandVaries(operands[i]) {
			if err := encodeCharNumber(buf, operands[i].Default); err != nil {
				return err
			}
			committed++
			i++
			continue
		}

		// maximal run of varying operands sharing the same delta count
		k := len(operands[i].Deltas)
		j := i
		for j < len(operands) && operandVaries(operands[j]) && len(operands[j].Deltas) == k {
			j++
		}
		run := operands[i:j]

		for pos := 0; pos < len(run); {
			maxN := (maxStackCFF2 - 1 - committed) / (k + 1)
			if maxN < 1 {
				return errBlendTooLargeCFF2
			}
			n := min(len(run)-pos, maxN)
			group := run[pos : pos+n]

			for _, b := range group {
				if err := encodeCharNumber(buf, b.Default); err != nil {
					return err
				}
			}
			// deltas, operand-major
			for _, b := range group {
				for _, d := range b.Deltas {
					if err := encodeCharNumber(buf, d); err != nil {
						return err
					}
				}
			}
			encodeCharInt(buf, n)
			buf.WriteByte(byte(t2blend))

			committed += n
			pos += n
		}
		i = j
	}
	return nil
}

// operandVaries reports whether a re-differenced operand needs a blend, i.e.
// carries at least one non-zero delta.  An all-zero (or nil) Deltas is treated
// as non-varying: emitting it as a plain number reproduces the same absolute
// value, because the decoder keeps the running position's existing deltas.
func operandVaries(b Blend) bool {
	for _, d := range b.Deltas {
		if d != 0 {
			return true
		}
	}
	return false
}

// subBlend returns a-b component-wise, treating missing deltas as zero.  When
// all delta differences are zero the result carries no deltas, so the operand
// encodes as a plain number.
func subBlend(a, b Blend) Blend {
	res := Blend{Default: a.Default - b.Default}
	n := max(len(a.Deltas), len(b.Deltas))
	if n == 0 {
		return res
	}
	deltas := make([]float64, n)
	nonZero := false
	for i := range n {
		var av, bv float64
		if i < len(a.Deltas) {
			av = a.Deltas[i]
		}
		if i < len(b.Deltas) {
			bv = b.Deltas[i]
		}
		deltas[i] = av - bv
		if deltas[i] != 0 {
			nonZero = true
		}
	}
	if nonZero {
		res.Deltas = deltas
	}
	return res
}

// encodeCharNumber writes a single charstring number operand.  Integral values
// in int16 range use the compact integer encodings; other values use the 16.16
// fixed encoding (operator 255).  A value outside the 16.16 range is rejected.
func encodeCharNumber(buf *bytes.Buffer, v float64) error {
	if v == math.Trunc(v) && v >= -32768 && v <= 32767 {
		buf.Write(encodeInt(funit.Int16(v)))
		return nil
	}
	if v < -32768 || v >= 32768 || math.IsNaN(v) {
		return errNumberRangeCFF2
	}
	x := int32(math.Round(v * 65536))
	buf.Write([]byte{255, byte(x >> 24), byte(x >> 16), byte(x >> 8), byte(x)})
	return nil
}

// encodeCharInt writes a small non-negative integer operand (a blend count or
// a vsindex).
func encodeCharInt(buf *bytes.Buffer, v int) {
	buf.Write(encodeInt(funit.Int16(v)))
}

var (
	errBadGlyphCFF2      = invalidSince("cff2: malformed glyph command")
	errNumberRangeCFF2   = invalidSince("cff2: number out of range")
	errBlendTooLargeCFF2 = invalidSince("cff2: blend group too large")
)
