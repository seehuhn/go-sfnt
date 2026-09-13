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
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/parser"
)

type cffDict map[dictOp][]any

func decodeDict(buf []byte, ss *cffStrings) (cffDict, error) {
	res := cffDict{}
	var stack []any

	flush := func(op dictOp) error {
		if op.isString() {
			l := len(stack)
			if op == opROS && l > 2 {
				l = 2
			}
			for i := 0; i < l; i++ {
				var idx int32
				switch x := stack[i].(type) {
				case int32:
					idx = x
				case float64:
					idx = int32(x)
					if float64(idx) != x {
						return errNoString
					}
				default:
					return errNoString
				}
				var err error
				stack[i], err = ss.get(idx)
				if err != nil {
					return err
				}
			}
		}
		res[op] = stack
		stack = nil
		return nil
	}

	for len(buf) > 0 {
		b0 := buf[0]
		var err error
		switch {
		case b0 == 12:
			if len(buf) < 2 {
				return nil, errCorruptDict
			}
			err = flush(dictOp(b0)<<8 | dictOp(buf[1]))
			buf = buf[2:]
		case b0 <= 21:
			err = flush(dictOp(b0))
			buf = buf[1:]
		case b0 <= 27: // values 22–27, 31, and 255 are reserved
			return nil, errCorruptDict
		case b0 == 28:
			if len(buf) < 3 {
				return nil, errCorruptDict
			}
			stack = append(stack, int32(int16(uint16(buf[1])<<8|uint16(buf[2]))))
			buf = buf[3:]
		case b0 == 29:
			if len(buf) < 5 {
				return nil, errCorruptDict
			}
			stack = append(stack,
				int32(uint32(buf[1])<<24|uint32(buf[2])<<16|uint32(buf[3])<<8|uint32(buf[4])))
			buf = buf[5:]
		case b0 == 30:
			tmp, x, err := decodeFloat(buf[1:])
			if err != nil {
				return nil, err
			}
			stack = append(stack, x)
			buf = tmp
		case b0 == 31: // values 22–27, 31, and 255 are reserved
			return nil, errCorruptDict
		case b0 <= 246:
			stack = append(stack, int32(b0)-139)
			buf = buf[1:]
		case b0 <= 250:
			if len(buf) < 2 {
				return nil, errCorruptDict
			}
			stack = append(stack, int32(b0)*256+int32(buf[1])+(108-247*256))
			buf = buf[2:]
		case b0 <= 254:
			if len(buf) < 2 {
				return nil, errCorruptDict
			}
			stack = append(stack, -int32(b0)*256-int32(buf[1])-(108-251*256))
			buf = buf[2:]
		default: // values 22–27, 31, and 255 are reserved
			err = errCorruptDict
		}
		if err != nil {
			return nil, err
		}
	}

	if len(stack) > 0 {
		return nil, errCorruptDict
	}

	return res, nil
}

func (d cffDict) encode(ss *cffStrings) []byte {
	keys := d.sortedKeys()

	res := &bytes.Buffer{}
	for _, op := range keys {
		var args []any
		for _, arg := range d[op] {
			if s, ok := arg.(string); ok {
				arg = ss.lookup(s)
			}
			args = append(args, arg)
		}

		for _, arg := range args {
			encodeDictNumber(res, arg)
		}
		if op > 255 {
			res.WriteByte(12)
		}
		res.WriteByte(byte(op))
	}
	return res.Bytes()
}

// registerStrings allocates a SID for every string operand of the dictionary,
// so that the number of strings a font needs can be determined without
// encoding it.
func (d cffDict) registerStrings(ss *cffStrings) {
	for _, args := range d {
		for _, arg := range args {
			if s, ok := arg.(string); ok {
				ss.lookup(s)
			}
		}
	}
}

// encodeDictNumber writes a single numeric DICT operand (int32 or float64).
func encodeDictNumber(res *bytes.Buffer, arg any) {
	switch a := arg.(type) {
	case int32:
		switch {
		case a >= -107 && a <= 107:
			res.WriteByte(byte(a + 139))
		case a >= 108 && a <= 1131:
			// a = (b0–247)*256+b1+108
			a -= 108
			b1 := byte(a)
			a >>= 8
			b0 := byte(a + 247)
			res.Write([]byte{b0, b1})
		case a >= -1131 && a <= -108:
			// a = -(b0–251)*256-b1-108
			a = -108 - a
			b1 := byte(a)
			a >>= 8
			b0 := byte(a + 251)
			res.Write([]byte{b0, b1})
		case a >= -32768 && a <= 32767:
			a16 := uint16(a)
			res.Write([]byte{28, byte(a16 >> 8), byte(a16)})
		default:
			a32 := uint32(a)
			res.Write([]byte{29, byte(a32 >> 24), byte(a32 >> 16), byte(a32 >> 8), byte(a32)})
		}
	case float64:
		buf := encodeFloat(a)
		res.WriteByte(0x1e)
		res.Write(buf)
	default:
		panic("unexpected type")
	}
}

// decodes a float (without the leading 0x1e)
func decodeFloat(buf []byte) ([]byte, float64, error) {
	var s []byte

	first := true
	var next byte
	for {
		var nibble byte
		if first {
			if len(buf) == 0 {
				return nil, 0, errors.New("incomplete float")
			}
			next, buf = buf[0], buf[1:]
			nibble = next >> 4
			next = next & 15
			first = false
		} else {
			nibble = next
			first = true
		}

		switch nibble {
		case 0x0a:
			s = append(s, '.')
		case 0xb:
			s = append(s, 'e')
		case 0xc:
			s = append(s, 'e', '-')
		case 0xd: // reserved
			return nil, 0, errors.New("unsupported float format")
		case 0xe:
			s = append(s, '-')
		case 0xf:
			x, err := strconv.ParseFloat(string(s), 64)
			switch {
			case x > 1e300:
				x = 1e300
			case x > -1e-300 && x < 1e-300:
				x = 0
			case x < -1e300:
				x = -1e300
			}
			return buf, x, err
		default:
			s = append(s, '0'+nibble)
		}
	}
}

// The range of real operands a font can carry.  The reader gives back zero
// below minReal and the bound above maxReal, so a value outside cannot be
// written and read again unchanged.
const (
	minReal = 1e-300
	maxReal = 1e300
)

// usableReal reports whether x can be written as a real operand and read back
// unchanged.  Writing the test in the positive also rejects a NaN or an
// infinity supplied through the API, neither of which the format can spell.
func usableReal(x float64) bool {
	if x == 0 {
		return true
	}
	a := math.Abs(x)
	return a >= minReal && a <= maxReal
}

// checkReals reports an error for an operand a font cannot carry.  Values off
// a file are inside the range already, since the reader bounds them, so one
// reaching here came from the caller.
func (d cffDict) checkReals() error {
	for op, args := range d {
		for _, arg := range args {
			if err := checkRealOperand(op, arg); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkRealOperand(op dictOp, arg any) error {
	switch arg := arg.(type) {
	case float64:
		if !usableReal(arg) {
			return fmt.Errorf("cff: %s operand %v is out of range", op, arg)
		}
	case dictBlendValue:
		if err := checkRealOperand(op, arg.Default); err != nil {
			return err
		}
		for _, d := range arg.Deltas {
			if err := checkRealOperand(op, d); err != nil {
				return err
			}
		}
	}
	return nil
}

// encodeFloat encodes x in the nibble form the format uses for real operands.
//
// The shortest digit string which parses back to x is written, so that a value
// read from a font is written out again unchanged, positioned either by a
// decimal point or by an exponent, whichever needs fewer nibbles.
func encodeFloat(x float64) []byte {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return []byte{0x0f}
	}

	digits, k, neg := shortestDigits(x)

	nibbles := positionalNibbles(digits, k)
	if alt := exponentNibbles(digits, k); len(alt) < len(nibbles) {
		nibbles = alt
	}
	if neg {
		nibbles = append([]byte{0xe}, nibbles...)
	}

	nibbles = append(nibbles, 0x0f)
	if len(nibbles)%2 != 0 {
		nibbles = append(nibbles, 0x0f)
	}
	out := make([]byte, len(nibbles)/2)
	for i := range out {
		out[i] = nibbles[2*i]<<4 | nibbles[2*i+1]
	}
	return out
}

// shortestDigits splits x into the shortest run of decimal digits which parses
// back to it, the power of ten the run is multiplied by, and the sign.
func shortestDigits(x float64) (digits []byte, k int, neg bool) {
	s := strconv.AppendFloat(nil, x, 'e', -1, 64)
	if s[0] == '-' {
		neg = true
		s = s[1:]
	}
	e := bytes.IndexByte(s, 'e')
	if s[1] == '.' {
		digits = append(digits, s[0])
		digits = append(digits, s[2:e]...)
	} else {
		digits = append(digits, s[:e]...)
	}
	exp, _ := strconv.Atoi(string(s[e+1:]))

	for i, c := range digits {
		digits[i] = c - '0'
	}
	return digits, exp - (len(digits) - 1), neg
}

// positionalNibbles writes the digits with a decimal point, without an
// exponent.
func positionalNibbles(digits []byte, k int) []byte {
	switch {
	case k >= 0:
		return append(slices.Clone(digits), make([]byte, k)...)
	case -k < len(digits):
		res := slices.Clone(digits[:len(digits)+k])
		res = append(res, 0xa)
		return append(res, digits[len(digits)+k:]...)
	default:
		res := append([]byte{0xa}, make([]byte, -k-len(digits))...)
		return append(res, digits...)
	}
}

// exponentNibbles writes the digits as a whole number followed by the power of
// ten it is multiplied by.
func exponentNibbles(digits []byte, k int) []byte {
	if k == 0 {
		return digits
	}
	marker := byte(0xb)
	if k < 0 {
		marker, k = 0xc, -k
	}
	res := append(slices.Clone(digits), marker)
	for _, c := range strconv.Itoa(k) {
		res = append(res, byte(c)-'0')
	}
	return res
}

func (d cffDict) getInt(op dictOp, defVal int32) int32 {
	if len(d[op]) != 1 {
		return defVal
	}
	x, ok := d[op][0].(int32)
	if !ok {
		return defVal
	}
	return x
}

func (d cffDict) getFloat(op dictOp, defVal float64) float64 {
	if len(d[op]) != 1 {
		return defVal
	}
	switch x := d[op][0].(type) {
	case int32:
		return float64(x)
	case float64:
		return x
	default:
		return defVal
	}
}

func (d cffDict) getString(op dictOp) string {
	if len(d[op]) != 1 {
		return ""
	}
	x, _ := d[op][0].(string)
	x = string([]rune(x)) // make sure we have valid utf-8 data
	return x
}

// getDelta reads a delta operand, a list of differences whose running sum
// gives the values.  The format declares these as numbers, so an operand may
// be real.  An operand of another kind ends the list, keeping the values ahead
// of it.
func (d cffDict) getDelta(op dictOp) []float64 {
	values := d[op]
	if len(values) == 0 {
		return nil
	}
	res := make([]float64, 0, len(values))
	var sum float64
	for _, v := range values {
		switch v := v.(type) {
		case int32:
			sum += float64(v)
		case float64:
			sum += v
		default:
			return trimmedDelta(res)
		}
		res = append(res, sum)
	}
	return trimmedDelta(res)
}

// trimmedDelta reports an empty delta list as nil, which is the shape an
// absent operand gives.
func trimmedDelta(res []float64) []float64 {
	if len(res) == 0 {
		return nil
	}
	return res
}

func (d cffDict) getPair(op dictOp) (int32, int32, bool) {
	xy := d[op]
	if len(xy) != 2 {
		return 0, 0, false
	}
	x, ok := xy[0].(int32)
	if !ok {
		return 0, 0, false
	}
	y, ok := xy[1].(int32)
	if !ok {
		return 0, 0, false
	}
	return x, y, true
}

// getFontMatrix reads the six FontMatrix operands from a CFF1 DICT.  See
// getFontMatrixCFF2 in cff2read.go for the CFF2 counterpart: CFF1 operands
// are always plain numbers, so this asserts float64 directly, while a CFF2
// DICT may carry blended operands (dictBlendValue) that need resolving to
// their default value; the two also pick their fallback matrix differently
// (isCIDKeyed here vs. an explicit def argument there).  Keeping the two
// separate avoids threading blend resolution through the CFF1 path.
func (d cffDict) getFontMatrix(op dictOp, isCIDKeyed bool) (res matrix.Matrix) {
	xx, ok := d[op]
	if !ok || len(xx) != 6 {
		if isCIDKeyed {
			return matrix.Identity
		}
		return defaultFontMatrix
	}

	for i, x := range xx {
		xi, ok := x.(float64)
		if !ok {
			if isCIDKeyed {
				return matrix.Identity
			}
			return defaultFontMatrix
		}
		res[i] = xi
	}

	return res
}

// setDelta stores a delta operand, using the more compact integer form for
// each difference which is integral.
func (d cffDict) setDelta(op dictOp, val []float64) {
	if len(val) == 0 {
		delete(d, op)
		return
	}
	res := make([]any, len(val))
	var prev float64
	for i, x := range val {
		res[i] = numberOperand(x - prev)
		prev = x
	}
	d[op] = res
}

// numberOperand returns x as a DICT operand, in the integer form where the
// value allows it.
func numberOperand(x float64) any {
	if x == math.Trunc(x) && x >= math.MinInt32 && x <= math.MaxInt32 {
		return int32(x)
	}
	return x
}

// setNumber stores a numeric operand, using the more compact integer form
// for integral values.
func (d cffDict) setNumber(op dictOp, x float64) {
	d[op] = []any{numberOperand(x)}
}

func (d cffDict) setFontMatrix(op dictOp, fm matrix.Matrix, isCIDKeyed bool) {
	needed := false
	for i, xi := range fm {
		var def float64
		if isCIDKeyed {
			def = matrix.Identity[i]
		} else {
			def = defaultFontMatrix[i]
		}
		// the negated comparison also counts a NaN as needing the entry,
		// so that the writer refuses it rather than omitting the matrix
		if !(math.Abs(xi-def) <= 1e-5) {
			needed = true
			break
		}
	}
	if !needed {
		return
	}

	val := make([]any, 6)
	for i, xi := range fm {
		val[i] = xi
	}
	d[op] = val
}

func (d cffDict) sortedKeys() []dictOp {
	keys := make([]dictOp, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	conv := func(op dictOp) int {
		switch op {
		case opROS:
			return -1
		case opSyntheticBase:
			return -2
		}
		return int(op)
	}
	sort.Slice(keys, func(i, j int) bool {
		return conv(keys[i]) < conv(keys[j])
	})
	return keys
}

func makeTopDict(info *type1.FontInfo) cffDict {
	topDict := cffDict{}
	if info.Version != "" {
		topDict[opVersion] = []any{info.Version}
	}
	if info.Notice != "" {
		topDict[opNotice] = []any{info.Notice}
	}
	if info.Copyright != "" {
		topDict[opCopyright] = []any{info.Copyright}
	}
	if info.FullName != "" {
		topDict[opFullName] = []any{info.FullName}
	}
	if info.FamilyName != "" {
		topDict[opFamilyName] = []any{info.FamilyName}
	}
	if info.Weight != "" {
		topDict[opWeight] = []any{info.Weight}
	}
	if info.IsFixedPitch {
		topDict[opIsFixedPitch] = []any{int32(1)}
	}
	if info.ItalicAngle != 0 {
		topDict[opItalicAngle] = []any{info.ItalicAngle}
	}
	if info.UnderlinePosition != defaultUnderlinePosition {
		topDict.setNumber(opUnderlinePosition, float64(info.UnderlinePosition))
	}
	if info.UnderlineThickness != defaultUnderlineThickness {
		topDict.setNumber(opUnderlineThickness, float64(info.UnderlineThickness))
	}
	// if info.IsOutlined {
	// 	topDict[opPaintType] = []any{int32(2)} // per font
	// }

	return topDict
}

type privateInfo struct {
	private      *type1.PrivateDict
	subrs        cffIndex
	defaultWidth float64
	nominalWidth float64
}

func (d cffDict) readPrivate(p *parser.Parser, strings *cffStrings) (*privateInfo, error) {
	// TODO(voss): handle the font matrix

	pdSize, pdOffs, ok := d.getPair(opPrivate)
	if !ok || pdOffs < 4 || pdSize < 0 || int64(pdSize) > p.Size()-int64(pdOffs) {
		return nil, errors.New("cff: invalid Private DICT")
	}

	err := p.SeekPos(int64(pdOffs))
	if err != nil {
		return nil, err
	}

	privateDictBlob := make([]byte, pdSize)
	_, err = p.Read(privateDictBlob)
	if err != nil {
		return nil, err
	}

	privateDict, err := decodeDict(privateDictBlob, strings)
	if err != nil {
		return nil, err
	}

	// TODO(voss): StemSnapH, StemSnapV

	private := &type1.PrivateDict{
		BlueValues: privateDict.getDelta(opBlueValues),
		OtherBlues: privateDict.getDelta(opOtherBlues),
		BlueScale:  privateDict.getFloat(opBlueScale, type1.DefaultBlueScale),
		BlueShift:  privateDict.getFloat(opBlueShift, type1.DefaultBlueShift),
		BlueFuzz:   privateDict.getFloat(opBlueFuzz, type1.DefaultBlueFuzz),
		StdHW:      privateDict.getFloat(opStdHW, 0),
		StdVW:      privateDict.getFloat(opStdVW, 0),
		ForceBold:  privateDict.getInt(opForceBold, 0) != 0,
	}
	private.Repair()

	var subrs cffIndex
	subrsIndexOffs := privateDict.getInt(opSubrs, 0)
	if subrsIndexOffs > 0 {
		subrs, err = readIndexAt(p, int64(pdOffs)+int64(subrsIndexOffs), "Subrs")
		if err != nil {
			return nil, err
		}
	}

	info := &privateInfo{
		private:      private,
		defaultWidth: privateDict.getFloat(opDefaultWidthX, 0),
		nominalWidth: privateDict.getFloat(opNominalWidthX, 0),
		subrs:        subrs,
	}

	return info, nil
}

var defaultFontMatrix = matrix.Matrix{0.001, 0, 0, 0.001, 0, 0}

type dictOp uint16

func (d dictOp) String() string {
	switch d {
	case opVersion:
		return "Version"
	case opNotice:
		return "Notice"
	case opFullName:
		return "FullName"
	case opFamilyName:
		return "FamilyName"
	case opWeight:
		return "Weight"
	case opFontBBox:
		return "FontBBox"
	case opCharset:
		return "Charset"
	case opEncoding:
		return "Encoding"
	case opCharStrings:
		return "CharStrings"
	case opPrivate:
		return "Private"
	case opCopyright:
		return "Copyright"
	case opUnderlinePosition:
		return "UnderlinePosition"
	case opCharstringType:
		return "CharstringType"
	case opSyntheticBase:
		return "SyntheticBase"
	case opROS:
		return "ROS"
	case opCIDFontVersion:
		return "CIDFontVersion"
	case opCIDFontRevision:
		return "CIDFontRevision"
	case opCIDFontType:
		return "CIDFontType"
	case opUIDBase:
		return "UIDBase"
	case opFontName:
		return "FontName"
	case opCIDCount:
		return "CIDCount"
	case opFDArray:
		return "FDArray"
	case opFDSelect:
		return "FDSelect"
	case opVStore:
		return "vstore"
	case opVSIndex:
		return "vsindex"
	case opBlend:
		return "blend"
	case opStemSnapH:
		return "StemSnapH"
	case opStemSnapV:
		return "StemSnapV"
	case opLanguageGroup:
		return "LanguageGroup"
	case opExpansionFactor:
		return "ExpansionFactor"

	case opBlueValues:
		return "BlueValues"
	case opOtherBlues:
		return "OtherBlues"
	case opFamilyBlues:
		return "FamilyBlues"
	case opFamilyOtherBlues:
		return "FamilyOtherBlues"
	case opStdHW:
		return "StdHW"
	case opStdVW:
		return "StdVW"
	case opSubrs:
		return "Subrs"
	case opDefaultWidthX:
		return "DefaultWidthX"
	case opNominalWidthX:
		return "NominalWidthX"
	case opBlueScale:
		return "BlueScale"
	case opBlueShift:
		return "BlueShift"
	case opBlueFuzz:
		return "BlueFuzz"
	case opForceBold:
		return "ForceBold"

	default:
		if d < 256 {
			return fmt.Sprintf("%d", d)
		}
		return fmt.Sprintf("%d %d", d>>8, d&0xff)
	}
}

const (
	// top DICT operators
	opVersion            dictOp = 0x0000
	opNotice             dictOp = 0x0001
	opFullName           dictOp = 0x0002
	opFamilyName         dictOp = 0x0003
	opWeight             dictOp = 0x0004
	opFontBBox           dictOp = 0x0005
	opCharset            dictOp = 0x000F
	opEncoding           dictOp = 0x0010
	opCharStrings        dictOp = 0x0011
	opPrivate            dictOp = 0x0012
	opVStore             dictOp = 0x0018 // CFF2 top DICT
	opCopyright          dictOp = 0x0C00
	opIsFixedPitch       dictOp = 0x0C01
	opItalicAngle        dictOp = 0x0C02
	opUnderlinePosition  dictOp = 0x0C03
	opUnderlineThickness dictOp = 0x0C04
	// opPaintType          dictOp = 0x0C05
	opCharstringType  dictOp = 0x0C06
	opFontMatrix      dictOp = 0x0C07
	opStemSnapH       dictOp = 0x0C0C
	opStemSnapV       dictOp = 0x0C0D
	opLanguageGroup   dictOp = 0x0C11
	opExpansionFactor dictOp = 0x0C12
	opSyntheticBase   dictOp = 0x0C14
	opPostScript      dictOp = 0x0C15
	opBaseFontName    dictOp = 0x0C16
	opROS             dictOp = 0x0C1E
	opCIDFontVersion  dictOp = 0x0C1F
	opCIDFontRevision dictOp = 0x0C20
	opCIDFontType     dictOp = 0x0C21
	opCIDCount        dictOp = 0x0C22
	opUIDBase         dictOp = 0x0C23
	opFDArray         dictOp = 0x0C24
	opFDSelect        dictOp = 0x0C25
	opFontName        dictOp = 0x0C26

	// private DICT operators
	opBlueValues       dictOp = 0x0006
	opOtherBlues       dictOp = 0x0007
	opFamilyBlues      dictOp = 0x0008
	opFamilyOtherBlues dictOp = 0x0009
	opStdHW            dictOp = 0x000A
	opStdVW            dictOp = 0x000B
	opSubrs            dictOp = 0x0013 // Offset (self) to local subrs
	opDefaultWidthX    dictOp = 0x0014
	opNominalWidthX    dictOp = 0x0015
	opVSIndex          dictOp = 0x0016 // CFF2 private DICT
	opBlend            dictOp = 0x0017 // CFF2 private DICT
	opBlueScale        dictOp = 0x0C09
	opBlueShift        dictOp = 0x0C0A
	opBlueFuzz         dictOp = 0x0C0B
	opForceBold        dictOp = 0x0C0E

	// used in local unit tests only
	opDebug dictOp = 0x0CFF
)

func (d dictOp) isString() bool {
	switch d {
	case opVersion, opNotice, opCopyright, opFullName, opFamilyName, opWeight,
		opPostScript, opBaseFontName, opROS, opFontName:
		return true
	default:
		return false
	}
}

const (
	defaultUnderlinePosition  = -100
	defaultUnderlineThickness = 50
	defaultExpansionFactor    = 0.06
)

var errCorruptDict = invalidSince("corrupt dict")

// dictBlendValue is a variable CFF2 DICT operand: a default value plus one
// delta per active variation region.  Default and each delta are int32 or
// float64, matching the plain DICT operand representation.
type dictBlendValue struct {
	Default any
	Deltas  []any
}

// resolvedBlend is a DICT operand resolved to float64 values.
type resolvedBlend struct {
	Default float64
	Deltas  []float64
}

// cff2BlendCap bounds the operands a single blend operator may consume:
// n*(k+1)+1 <= cff2BlendCap.
const cff2BlendCap = 513

// decodeDictCFF2 decodes a CFF2 top or private DICT.  Unlike a CFF1 DICT, a
// CFF2 DICT contains no string (SID) operands and may use the blend
// operator to make operands variable.  regionCount reports the number of
// variation regions k for a given vsindex; it is called with the DICT's
// current vsindex, which is 0 unless a vsindex operator has set it.
func decodeDictCFF2(buf []byte, regionCount func(vsindex int) (int, error)) (cffDict, error) {
	res := cffDict{}
	var stack []any
	vsindex := 0

	for len(buf) > 0 {
		b0 := buf[0]
		switch {
		case b0 == 12:
			if len(buf) < 2 {
				return nil, errCorruptDict
			}
			op := dictOp(b0)<<8 | dictOp(buf[1])
			res[op] = stack
			stack = nil
			buf = buf[2:]
		case b0 == 22: // vsindex
			if len(stack) == 0 {
				return nil, errCorruptDict
			}
			v, ok := stack[len(stack)-1].(int32)
			if !ok {
				return nil, errCorruptDict
			}
			vsindex = int(v)
			res[opVSIndex] = []any{int32(vsindex)}
			stack = nil
			buf = buf[1:]
		case b0 == 23: // blend
			var err error
			stack, err = applyDictBlend(stack, vsindex, regionCount)
			if err != nil {
				return nil, err
			}
			buf = buf[1:]
		case b0 <= 24: // single-byte operators (includes vstore, op 24)
			res[dictOp(b0)] = stack
			stack = nil
			buf = buf[1:]
		case b0 <= 27: // reserved
			return nil, errCorruptDict
		case b0 == 28:
			if len(buf) < 3 {
				return nil, errCorruptDict
			}
			stack = append(stack, int32(int16(uint16(buf[1])<<8|uint16(buf[2]))))
			buf = buf[3:]
		case b0 == 29:
			if len(buf) < 5 {
				return nil, errCorruptDict
			}
			stack = append(stack,
				int32(uint32(buf[1])<<24|uint32(buf[2])<<16|uint32(buf[3])<<8|uint32(buf[4])))
			buf = buf[5:]
		case b0 == 30:
			tmp, x, err := decodeFloat(buf[1:])
			if err != nil {
				return nil, err
			}
			stack = append(stack, x)
			buf = tmp
		case b0 == 31: // reserved
			return nil, errCorruptDict
		case b0 <= 246:
			stack = append(stack, int32(b0)-139)
			buf = buf[1:]
		case b0 <= 250:
			if len(buf) < 2 {
				return nil, errCorruptDict
			}
			stack = append(stack, int32(b0)*256+int32(buf[1])+(108-247*256))
			buf = buf[2:]
		case b0 <= 254:
			if len(buf) < 2 {
				return nil, errCorruptDict
			}
			stack = append(stack, -int32(b0)*256-int32(buf[1])-(108-251*256))
			buf = buf[2:]
		default: // 255 reserved
			return nil, errCorruptDict
		}
	}

	if len(stack) > 0 {
		return nil, errCorruptDict
	}

	return res, nil
}

// applyDictBlend consumes the blend operands from the top of stack and
// replaces them with the n blended values.  The stack layout before blend
// is [ ... n defaults, n*k deltas, n ], and afterwards the n blended values
// remain in place of the consumed operands.
func applyDictBlend(stack []any, vsindex int, regionCount func(int) (int, error)) ([]any, error) {
	if len(stack) == 0 {
		return nil, errCorruptDict
	}
	nVal, ok := stack[len(stack)-1].(int32)
	if !ok {
		return nil, errCorruptDict
	}
	n := int(nVal)
	stack = stack[:len(stack)-1]
	if n < 0 || n > cff2BlendCap {
		return nil, errCorruptDict
	}

	k, err := regionCount(vsindex)
	if err != nil {
		return nil, err
	}
	if k < 0 || (n > 0 && k >= cff2BlendCap) {
		return nil, errCorruptDict
	}
	if n*(k+1)+1 > cff2BlendCap {
		return nil, errCorruptDict
	}

	need := n + n*k
	if len(stack) < need {
		return nil, errCorruptDict
	}
	base := len(stack) - need
	defaults := stack[base : base+n]
	deltas := stack[base+n:]

	// blend operands must be plain numbers; a blended value cannot itself
	// be re-blended (this keeps dictBlendValue.Default/Deltas plain)
	for _, v := range stack[base:] {
		if _, ok := v.(dictBlendValue); ok {
			return nil, errCorruptDict
		}
	}

	blended := make([]any, n)
	for i := range n {
		bv := dictBlendValue{Default: defaults[i]}
		if k > 0 {
			ds := make([]any, k)
			copy(ds, deltas[i*k:i*k+k])
			bv.Deltas = ds
		}
		blended[i] = bv
	}
	stack = append(stack[:base], blended...)
	if len(stack) == 0 {
		return nil, nil
	}
	return stack, nil
}

// encodeCFF2 encodes a CFF2 top or private DICT.  A vsindex operator, if
// present, is emitted first so that it precedes any blend.  A blend
// operator is emitted for each run of consecutive blended operands (values
// carrying deltas); operands without deltas are emitted as plain numbers.
func (d cffDict) encodeCFF2() []byte {
	res := &bytes.Buffer{}

	if vs, ok := d[opVSIndex]; ok {
		for _, arg := range vs {
			encodeDictNumber(res, arg)
		}
		res.WriteByte(byte(opVSIndex))
	}

	for _, op := range d.sortedKeys() {
		if op == opVSIndex {
			continue
		}
		encodeCFF2Operands(res, d[op])
		if op > 255 {
			res.WriteByte(12)
		}
		res.WriteByte(byte(op))
	}
	return res.Bytes()
}

// encodeCFF2Operands writes the operands of a single CFF2 DICT key, emitting
// blend operators for runs of blended values.
func encodeCFF2Operands(res *bytes.Buffer, operands []any) {
	i := 0
	for i < len(operands) {
		bv, ok := operands[i].(dictBlendValue)
		if !ok || len(bv.Deltas) == 0 {
			encodeDictNumber(res, plainOperand(operands[i]))
			i++
			continue
		}

		// run of consecutive blended values sharing the same delta count
		k := len(bv.Deltas)
		j := i
		for j < len(operands) {
			b, ok := operands[j].(dictBlendValue)
			if !ok || len(b.Deltas) != k {
				break
			}
			j++
		}
		run := operands[i:j]
		for _, r := range run {
			encodeDictNumber(res, r.(dictBlendValue).Default)
		}
		for _, r := range run {
			for _, dl := range r.(dictBlendValue).Deltas {
				encodeDictNumber(res, dl)
			}
		}
		encodeDictNumber(res, int32(len(run)))
		res.WriteByte(byte(opBlend))
		i = j
	}
}

// plainOperand returns the numeric value of a plain operand, or the default
// of a blended operand that carries no deltas.
func plainOperand(v any) any {
	if bv, ok := v.(dictBlendValue); ok {
		return bv.Default
	}
	return v
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case int32:
		return float64(x)
	case float64:
		return x
	default:
		return 0
	}
}

func resolveBlend(v any) resolvedBlend {
	bv, ok := v.(dictBlendValue)
	if !ok {
		return resolvedBlend{Default: toFloat(v)}
	}
	res := resolvedBlend{Default: toFloat(bv.Default)}
	if len(bv.Deltas) > 0 {
		res.Deltas = make([]float64, len(bv.Deltas))
		for i, d := range bv.Deltas {
			res.Deltas[i] = toFloat(d)
		}
	}
	return res
}

// getBlend returns the single-operand value of op, plain or blended,
// resolved to float64.
func (d cffDict) getBlend(op dictOp) (resolvedBlend, bool) {
	v := d[op]
	if len(v) != 1 {
		return resolvedBlend{}, false
	}
	return resolveBlend(v[0]), true
}

// getBlendArray returns the delta-encoded array operands of op resolved to
// float64.  The default values and each region's deltas are decoded as
// running sums, so element i holds the absolute value for its region.
func (d cffDict) getBlendArray(op dictOp) []resolvedBlend {
	vals := d[op]
	if len(vals) == 0 {
		return nil
	}
	res := make([]resolvedBlend, len(vals))
	var prevDefault float64
	var prevDeltas []float64
	for i, v := range vals {
		rb := resolveBlend(v)
		prevDefault += rb.Default
		out := resolvedBlend{Default: prevDefault}
		if len(rb.Deltas) > 0 {
			if len(prevDeltas) < len(rb.Deltas) {
				grown := make([]float64, len(rb.Deltas))
				copy(grown, prevDeltas)
				prevDeltas = grown
			}
			acc := make([]float64, len(rb.Deltas))
			for j := range rb.Deltas {
				prevDeltas[j] += rb.Deltas[j]
				acc[j] = prevDeltas[j]
			}
			out.Deltas = acc
		}
		res[i] = out
	}
	return res
}
