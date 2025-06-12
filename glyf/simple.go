// seehuhn.de/go/sfnt - a library for reading and writing font files
// Copyright (C) 2022  Jochen Voss <voss@seehuhn.de>
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

package glyf

import (
	"seehuhn.de/go/geom/path"

	"seehuhn.de/go/postscript/funit"

	"seehuhn.de/go/sfnt/parser"
)

// SimpleGlyph is a simple glyph.
type SimpleGlyph struct {
	NumContours int16
	Encoded     []byte
}

// Path returns a path.Path that iterates over all contours of the glyph outline.
// Each contour starts with a moveto, followed by all segments of that contour.
func (g SimpleGlyph) Path() path.Path {
	glyphInfo, err := g.Unpack()
	if err != nil {
		return func(yield func(path.Command, []path.Point) bool) {}
	}
	return glyphInfo.Path()
}

// A Point is a point in a glyph outline
type Point struct {
	X, Y    funit.Int16
	OnCurve bool
}

// A Contour describes a connected part of a glyph outline.
type Contour []Point

// SimpleUnpacked contains the contours of a SimpleGlyph.
type SimpleUnpacked struct {
	Contours     []Contour
	Instructions []byte
}

// Unpack returns the contours of a glyph.
func (sg SimpleGlyph) Unpack() (*SimpleUnpacked, error) {
	buf := sg.Encoded

	numContours := int(sg.NumContours)
	if len(buf) < 2*numContours+2 {
		return nil, errInvalidGlyphData
	}

	endPtsOfContours := make([]uint16, numContours)
	for i := range endPtsOfContours {
		endPtsOfContours[i] = uint16(buf[2*i])<<8 | uint16(buf[2*i+1])
	}
	buf = buf[2*numContours:]

	var numPoints int
	if numContours > 0 {
		numPoints = int(endPtsOfContours[numContours-1]) + 1
	}

	instructionLength := int(buf[0])<<8 | int(buf[1])
	if len(buf) < 2+instructionLength {
		return nil, errInvalidGlyphData
	}
	instructions := buf[2 : 2+instructionLength]
	buf = buf[2+instructionLength:]

	flags := make([]byte, numPoints)
	for i := 0; i < numPoints; {
		if len(buf) < 1 {
			return nil, errInvalidGlyphData
		}
		flag := buf[0]
		buf = buf[1:]
		flags[i] = flag
		i++
		if flag&flagRepeat != 0 {
			if len(buf) < 1 {
				return nil, errInvalidGlyphData
			}
			count := int(buf[0])
			buf = buf[1:]
			for count > 0 && i < numPoints {
				flags[i] = flag
				i++
				count--
			}
		}
	}

	// decode the x-coordinates
	xx := make([]funit.Int16, numPoints)
	var x funit.Int16
	for i, flag := range flags {
		if flag&flagXShortVec != 0 {
			if len(buf) < 1 {
				return nil, errInvalidGlyphData
			}
			dx := funit.Int16(buf[0])
			buf = buf[1:]
			if flag&flagXSameOrPos != 0 {
				x += dx
			} else {
				x -= dx
			}
		} else if flag&flagXSameOrPos == 0 {
			if len(buf) < 2 {
				return nil, errInvalidGlyphData
			}
			dx := funit.Int16(buf[0])<<8 | funit.Int16(buf[1])
			buf = buf[2:]
			x += dx
		}
		xx[i] = x
	}

	// decode the y-coordinates
	yy := make([]funit.Int16, numPoints)
	var y funit.Int16
	for i, flag := range flags {
		if flag&flagYShortVec != 0 {
			if len(buf) < 1 {
				return nil, errInvalidGlyphData
			}

			dy := funit.Int16(buf[0])
			buf = buf[1:]
			if flag&flagYSameOrPos != 0 {
				y += dy
			} else {
				y -= dy
			}
		} else if flag&flagYSameOrPos == 0 {
			if len(buf) < 2 {
				return nil, errInvalidGlyphData
			}
			dy := funit.Int16(buf[0])<<8 | funit.Int16(buf[1])
			buf = buf[2:]
			y += dy
		}
		yy[i] = y
	}

	// Build contours from decoded points
	var cc []Contour
	if numContours > 0 {
		cc = make([]Contour, numContours)
		start := 0
		for i := 0; i < numContours; i++ {
			end := int(endPtsOfContours[i]) + 1
			contour := make([]Point, end-start)
			for j := start; j < end; j++ {
				contour[j-start] = Point{xx[j], yy[j], flags[j]&flagOnCurve != 0}
			}
			cc[i] = contour
			start = end
		}
	}

	// Copy instructions if present
	var inst []byte
	if instructionLength > 0 {
		inst = make([]byte, len(instructions))
		copy(inst, instructions)
	}

	return &SimpleUnpacked{
		Contours:     cc,
		Instructions: inst,
	}, nil
}

func (sg *SimpleGlyph) removePadding() error {
	buf := sg.Encoded
	numContours := int(sg.NumContours)

	if len(buf) < 2*numContours+2 {
		return errInvalidGlyphData
	}
	pos := 2 * numContours

	var numPoints int
	if numContours > 0 {
		numPoints = (int(buf[pos-2])<<8 | int(buf[pos-1])) + 1
	}

	instructionLength := int(buf[pos])<<8 | int(buf[pos+1])
	pos += 2 + instructionLength

	coordBytes := 0
	for i := 0; i < numPoints; {
		if pos >= len(buf) {
			return errInvalidGlyphData
		}
		flag := buf[pos]
		pos++

		repeat := 1
		if flag&flagRepeat != 0 {
			if pos >= len(buf) {
				return errInvalidGlyphData
			}
			repeat = int(buf[pos]) + 1
			pos++
		}

		var xBytes, yBytes int
		if flag&flagXShortVec != 0 {
			xBytes = 1
		} else if flag&flagXSameOrPos == 0 {
			xBytes = 2
		}
		if flag&flagYShortVec != 0 {
			yBytes = 1
		} else if flag&flagYSameOrPos == 0 {
			yBytes = 2
		}

		coordBytes += (xBytes + yBytes) * repeat
		i += repeat
	}

	pos += coordBytes
	if pos > len(buf) {
		return errInvalidGlyphData
	}

	sg.Encoded = buf[:pos]
	return nil
}

// writeCoords writes coordinate deltas to buf based on flags
func writeCoords(buf []byte, flags []byte, deltas []funit.Int16, shortFlag, sameOrPosFlag byte) []byte {
	for i, flag := range flags {
		if flag&shortFlag != 0 {
			if flag&sameOrPosFlag != 0 {
				buf = append(buf, byte(deltas[i]))
			} else {
				buf = append(buf, byte(-deltas[i]))
			}
		} else if flag&sameOrPosFlag == 0 {
			buf = append(buf, byte(deltas[i]>>8), byte(deltas[i]))
		}
	}
	return buf
}

// Pack encodes the glyph info back into the binary format.
func (sd *SimpleUnpacked) Pack() SimpleGlyph {
	var numContours int
	var endPtsOfContours []uint16
	var totalPoints int

	if sd.Contours != nil {
		numContours = len(sd.Contours)
		endPtsOfContours = make([]uint16, numContours)
		for i, contour := range sd.Contours {
			totalPoints += len(contour)
			endPtsOfContours[i] = uint16(totalPoints - 1)
		}
	}

	points := make([]Point, 0, totalPoints)
	for _, contour := range sd.Contours {
		points = append(points, contour...)
	}

	flags := make([]byte, totalPoints)
	xDeltas := make([]funit.Int16, totalPoints)
	yDeltas := make([]funit.Int16, totalPoints)

	var prevX, prevY funit.Int16
	for i, pt := range points {
		xDeltas[i] = pt.X - prevX
		yDeltas[i] = pt.Y - prevY
		prevX = pt.X
		prevY = pt.Y

		if pt.OnCurve {
			flags[i] |= flagOnCurve
		}

		// Determine x-coordinate encoding
		if xDeltas[i] == 0 {
			flags[i] |= flagXSameOrPos
		} else if -255 <= xDeltas[i] && xDeltas[i] <= 255 {
			flags[i] |= flagXShortVec
			if xDeltas[i] > 0 {
				flags[i] |= flagXSameOrPos
			}
		}

		// Determine y-coordinate encoding
		if yDeltas[i] == 0 {
			flags[i] |= flagYSameOrPos
		} else if -255 <= yDeltas[i] && yDeltas[i] <= 255 {
			flags[i] |= flagYShortVec
			if yDeltas[i] > 0 {
				flags[i] |= flagYSameOrPos
			}
		}
	}

	// Build the encoded data
	var buf []byte

	// Write endPtsOfContours
	for _, endPt := range endPtsOfContours {
		buf = append(buf, byte(endPt>>8), byte(endPt))
	}

	// Write instruction length and instructions
	instructionLength := len(sd.Instructions)
	buf = append(buf, byte(instructionLength>>8), byte(instructionLength))
	buf = append(buf, sd.Instructions...)

	// Write flags with repetition compression
	i := 0
	for i < totalPoints {
		flag := flags[i]
		runLength := 1

		// Count consecutive identical flags
		for j := i + 1; j < totalPoints && flags[j] == flag && runLength < 256; j++ {
			runLength++
		}

		if runLength > 1 {
			buf = append(buf, flag|flagRepeat, byte(runLength-1))
		} else {
			buf = append(buf, flag)
		}

		i += runLength
	}

	// Write x-coordinates
	buf = writeCoords(buf, flags, xDeltas, flagXShortVec, flagXSameOrPos)

	// Write y-coordinates
	buf = writeCoords(buf, flags, yDeltas, flagYShortVec, flagYSameOrPos)

	return SimpleGlyph{
		NumContours: int16(numContours),
		Encoded:     buf,
	}
}

func (sd *SimpleUnpacked) AsGlyph() Glyph {
	var bbox funit.Rect16
	first := true
	for _, contour := range sd.Contours {
		for _, pt := range contour {
			if first || pt.X < bbox.LLx {
				bbox.LLx = pt.X
			}
			if first || pt.X > bbox.URx {
				bbox.URx = pt.X
			}
			if first || pt.Y < bbox.LLy {
				bbox.LLy = pt.Y
			}
			if first || pt.Y > bbox.URy {
				bbox.URy = pt.Y
			}
			first = false
		}
	}
	g := sd.Pack()
	return Glyph{
		Rect16: bbox,
		Data:   g,
	}
}

func (sd *SimpleUnpacked) Path() path.Path {
	return func(yield func(path.Command, []path.Point) bool) {
		var buf [3]path.Point

		for _, cc := range sd.Contours {
			if len(cc) < 2 { // no meaningful contour
				continue
			}

			toPoint := func(p Point) path.Point {
				return path.Point{X: float64(p.X), Y: float64(p.Y)}
			}

			midpoint := func(p1, p2 Point) path.Point {
				return path.Point{
					X: float64(p1.X+p2.X) / 2,
					Y: float64(p1.Y+p2.Y) / 2,
				}
			}

			// Find first on-curve point or compute midpoint if all off-curve
			start := 0
			for i, pt := range cc {
				if pt.OnCurve {
					start = i
					break
				}
			}

			// Move to start point
			if cc[start].OnCurve {
				buf[0] = toPoint(cc[start])
			} else {
				// if all points are off-curve, the TrueType spec says to
				// start at the midpoint of the first and last point.
				buf[0] = midpoint(cc[len(cc)-1], cc[0])
			}
			if !yield(path.CmdMoveTo, buf[:1]) {
				return
			}

			// makeExtendedPointIterator returns a stateful iterator function.
			// Each call to the iterator returns the next point in the "extended" sequence
			// (which includes implicit on-curve midpoints).
			makeExtendedPointIterator := func(
				cc []Point,
				toPointFunc func(Point) path.Point, // Renamed to avoid conflict
				midpointFunc func(Point, Point) path.Point, // Renamed to avoid conflict
			) func() (pt path.Point, onCurve bool, ok bool) {

				if len(cc) == 0 {
					return func() (path.Point, bool, bool) { return path.Point{}, false, false }
				}

				// State for the closure:
				i := 0 // Corresponds to the loop `for i := 0; i <= len(cc); i++`
				prevPtInCC := cc[len(cc)-1]
				prevOnCurveInCC := prevPtInCC.OnCurve
				pendingActualPoint := false // True if an implicit point was just yielded

				return func() (path.Point, bool, bool) {
					if pendingActualPoint {
						pendingActualPoint = false

						curPtOriginal := cc[i%len(cc)]

						prevPtInCC = curPtOriginal
						prevOnCurveInCC = curPtOriginal.OnCurve
						i++
						return toPointFunc(curPtOriginal), curPtOriginal.OnCurve, true
					}

					if i > len(cc) {
						return path.Point{}, false, false
					}

					curPtOriginal := cc[i%len(cc)]
					curOnCurveOriginal := curPtOriginal.OnCurve

					if !prevOnCurveInCC && !curOnCurveOriginal {
						pendingActualPoint = true
						// Note: prevPtInCC and i are NOT advanced here; they advance with the actual point.
						return midpointFunc(prevPtInCC, curPtOriginal), true, true
					}

					prevPtInCC = curPtOriginal
					prevOnCurveInCC = curPtOriginal.OnCurve
					i++
					return toPointFunc(curPtOriginal), curPtOriginal.OnCurve, true
				}
			}

			getNextExtendedPoint := makeExtendedPointIterator(cc, toPoint, midpoint)

			fillPoint := func(p *struct {
				pt      path.Point
				onCurve bool
				valid   bool
			}) {
				ptVal, onCurveVal, okVal := getNextExtendedPoint()
				if okVal {
					p.pt, p.onCurve, p.valid = ptVal, onCurveVal, true
				} else {
					p.valid = false
				}
			}

			var p0, p1, p2 struct {
				pt      path.Point
				onCurve bool
				valid   bool // false if we are at the end of the stream
			}

			// Prime the lookahead buffer
			fillPoint(&p0)
			fillPoint(&p1)
			fillPoint(&p2)

			for p0.valid {
				if !p0.onCurve {
					// This should not happen, as extendedPoints always yields on-curve points
					// or implicit midpoints which are on-curve.
					// If it does, it's an internal error or a misunderstanding of the spec.
					// As a fallback, treat as a line segment to the next available point.
					if p1.valid {
						buf[0] = p1.pt
						if !yield(path.CmdLineTo, buf[:1]) {
							return
						}
					} else if p0.pt != buf[0] { // Avoid empty line segment if p0 is the start point
						buf[0] = p0.pt
						if !yield(path.CmdLineTo, buf[:1]) {
							return
						}
					}
				} else if p1.valid && p1.onCurve {
					// On-curve to on-curve: Line segment
					buf[0] = p1.pt
					if !yield(path.CmdLineTo, buf[:1]) {
						return
					}
				} else if p1.valid && !p1.onCurve && p2.valid {
					// On-curve to off-curve to any: Quadratic curve
					buf[0] = p1.pt // control point
					buf[1] = p2.pt // end point
					if !yield(path.CmdQuadTo, buf[:2]) {
						return
					}
					// Advance p0 by two points (p0 becomes p2)
					p0 = p2
					fillPoint(&p1) // Get next point from stream for new p1
					if !p1.valid { // Reached end after advancing p0
						p2.valid = false
						break
					}
					fillPoint(&p2) // Get next point for new p2
					continue       // Restart loop with new p0, p1, p2
				} else {
					// This case should ideally not be reached if the contour is well-formed
					// and the extendedPoints generator works correctly.
					// It implies an on-curve point followed by an off-curve point with no subsequent point,
					// or some other unexpected sequence.
					// As a fallback, if p1 is valid (it must be off-curve here), draw a line to it.
					// This is not ideal as it might misinterpret the shape, but prevents crashing.
					if p1.valid && p1.pt != p0.pt { // p1 is off-curve
						buf[0] = p1.pt
						if !yield(path.CmdLineTo, buf[:1]) {
							return
						}
					}
					// If p1 is not valid, or p1.pt == p0.pt, we are at the end or have a degenerate segment.
					// The path will be closed by CmdClose outside the loop.
				}

				// Advance the lookahead buffer
				p0 = p1
				p1 = p2
				fillPoint(&p2) // Get next point from stream for new p2
			}

			if !yield(path.CmdClose, nil) {
				return
			}
		}
	}
}

// https://docs.microsoft.com/en-us/typography/opentype/spec/glyf#simpleGlyphFlags
const (
	flagOnCurve    = 0x01 // ON_CURVE_POINT
	flagXShortVec  = 0x02 // X_SHORT_VECTOR
	flagYShortVec  = 0x04 // Y_SHORT_VECTOR
	flagRepeat     = 0x08 // REPEAT_FLAG
	flagXSameOrPos = 0x10 // X_IS_SAME_OR_POSITIVE_X_SHORT_VECTOR
	flagYSameOrPos = 0x20 // Y_IS_SAME_OR_POSITIVE_Y_SHORT_VECTOR
)

var errInvalidGlyphData = &parser.InvalidFontError{
	SubSystem: "sfnt/glyf",
	Reason:    "invalid glyph data",
}
