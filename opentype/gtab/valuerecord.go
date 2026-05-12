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

package gtab

import (
	"fmt"
	"math/bits"
	"strings"

	"seehuhn.de/go/postscript/funit"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/parser"
)

// GposValueRecord describes an adjustment to the position of a glyph or set of glyphs.
// https://docs.microsoft.com/en-us/typography/opentype/spec/gpos#value-record
type GposValueRecord struct {
	XPlacement    funit.Int16   // Horizontal adjustment for placement
	YPlacement    funit.Int16   // Vertical adjustment for placement
	XAdvance      funit.Int16   // Horizontal adjustment for advance
	YAdvance      funit.Int16   // Vertical adjustment for advance
	XPlacementDev *device.Table // Device/VariationIndex for horizontal placement
	YPlacementDev *device.Table // Device/VariationIndex for vertical placement
	XAdvanceDev   *device.Table // Device/VariationIndex for horizontal advance
	YAdvanceDev   *device.Table // Device/VariationIndex for vertical advance
}

// readValueRecord reads the binary representation of a valueRecord.  The
// valueFormat determines which fields are present in the inline portion of
// the binary representation.  Device tables, if any, live elsewhere in the
// subtable; their offsets are relative to subtablePos.
func readValueRecord(p *parser.Parser, valueFormat uint16, subtablePos int64) (*GposValueRecord, error) {
	if valueFormat == 0 {
		return nil, nil
	}

	res := &GposValueRecord{}
	var devOffsets [4]uint16
	if valueFormat&0x0001 != 0 {
		tmp, err := p.ReadInt16()
		if err != nil {
			return nil, err
		}
		res.XPlacement = funit.Int16(tmp)
	}
	if valueFormat&0x0002 != 0 {
		tmp, err := p.ReadInt16()
		if err != nil {
			return nil, err
		}
		res.YPlacement = funit.Int16(tmp)
	}
	if valueFormat&0x0004 != 0 {
		tmp, err := p.ReadInt16()
		if err != nil {
			return nil, err
		}
		res.XAdvance = funit.Int16(tmp)
	}
	if valueFormat&0x0008 != 0 {
		tmp, err := p.ReadInt16()
		if err != nil {
			return nil, err
		}
		res.YAdvance = funit.Int16(tmp)
	}
	if valueFormat&0x0010 != 0 {
		offs, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		devOffsets[0] = offs
	}
	if valueFormat&0x0020 != 0 {
		offs, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		devOffsets[1] = offs
	}
	if valueFormat&0x0040 != 0 {
		offs, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		devOffsets[2] = offs
	}
	if valueFormat&0x0080 != 0 {
		offs, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		devOffsets[3] = offs
	}

	if devOffsets[0]|devOffsets[1]|devOffsets[2]|devOffsets[3] != 0 {
		resumePos := p.Pos()
		targets := [4]**device.Table{
			&res.XPlacementDev,
			&res.YPlacementDev,
			&res.XAdvanceDev,
			&res.YAdvanceDev,
		}
		for i, offs := range devOffsets {
			if offs == 0 {
				continue
			}
			t, err := device.Read(p, subtablePos+int64(offs))
			if err != nil {
				return nil, err
			}
			*targets[i] = t
		}
		if err := p.SeekPos(resumePos); err != nil {
			return nil, err
		}
	}

	return res, nil
}

func (vr *GposValueRecord) getFormat() uint16 {
	if vr == nil {
		return 0
	}

	var format uint16
	if vr.XPlacement != 0 {
		format |= 0x0001
	}
	if vr.YPlacement != 0 {
		format |= 0x0002
	}
	if vr.XAdvance != 0 {
		format |= 0x0004
	}
	if vr.YAdvance != 0 {
		format |= 0x0008
	}
	if vr.XPlacementDev != nil {
		format |= 0x0010
	}
	if vr.YPlacementDev != nil {
		format |= 0x0020
	}
	if vr.XAdvanceDev != nil {
		format |= 0x0040
	}
	if vr.YAdvanceDev != nil {
		format |= 0x0080
	}

	if format == 0 {
		// set one of the fields to mark the difference between
		// a nil *ValueRecord and a zero value.
		format = 0x0004
	}

	return format
}

// valueRecordEncodeLen returns the number of bytes that the inline
// portion of a GposValueRecord with the given valueFormat occupies in
// the encoded subtable.  The length is fully determined by the format
// bits (each set bit adds a 2-byte field), so this is a free function
// rather than a method — callers do not need a record to compute it.
func valueRecordEncodeLen(format uint16) int {
	return 2 * bits.OnesCount16(format)
}

// devicePool collects Device and VariationIndex tables referenced by
// the value records of a single GPOS subtable, deduplicating by
// encoded content.  Variable fonts typically share a single
// VariationIndex across many records; without dedup each record's
// reference would emit its own copy, multiplying the encoded GPOS
// size.
//
// The pool returns subtable-relative offsets directly: the caller
// supplies the pool's base offset (the byte position where the pool's
// concatenated tables begin) at construction time.  add returns 0 for
// a nil input — matching the OpenType convention that offset 0 means
// "no Device table".
type devicePool struct {
	base    int               // subtable-relative byte offset of the first pool entry
	seen    map[string]uint16 // encoded bytes → subtable-relative offset
	encoded []byte            // concatenated bytes of unique tables, in offset order
}

func newDevicePool(base int) *devicePool {
	return &devicePool{base: base, seen: map[string]uint16{}}
}

// add registers t and returns its subtable-relative offset.  Tables
// with byte-identical Encode output share a single offset.
func (p *devicePool) add(t *device.Table) uint16 {
	if t == nil {
		return 0
	}
	enc := t.Encode()
	key := string(enc)
	if off, ok := p.seen[key]; ok {
		return off
	}
	off := uint16(p.base + len(p.encoded))
	p.seen[key] = off
	p.encoded = append(p.encoded, enc...)
	return off
}

// addAll registers every non-nil Device table in vrs, in the canonical
// order returned by deviceTables.  It is shorthand for sizing passes
// that only need the pool's total length.
func (p *devicePool) addAll(vrs ...*GposValueRecord) {
	for _, vr := range vrs {
		for _, d := range vr.deviceTables() {
			p.add(d)
		}
	}
}

// len returns the total byte length of the pool's encoded data.
func (p *devicePool) len() int { return len(p.encoded) }

// bytes returns the concatenated bytes of the unique tables, in offset
// order.  Callers append this to the end of the subtable.
func (p *devicePool) bytes() []byte { return p.encoded }

// deviceTables returns the four Device pointers in canonical order
// (XPlacement, YPlacement, XAdvance, YAdvance).  A nil receiver
// returns four nil entries.
func (vr *GposValueRecord) deviceTables() [4]*device.Table {
	if vr == nil {
		return [4]*device.Table{}
	}
	return [4]*device.Table{vr.XPlacementDev, vr.YPlacementDev, vr.XAdvanceDev, vr.YAdvanceDev}
}

// encode emits the inline portion of the value record.  devOffsets gives the
// offset (from the enclosing subtable) of each Device table referenced by
// this record, in the canonical order returned by deviceTables.  The encoder
// writes 0 for any slot whose format bit is set but whose offset is 0.
func (vr *GposValueRecord) encode(format uint16, devOffsets [4]uint16) []byte {
	bufSize := valueRecordEncodeLen(format)
	buf := make([]byte, 0, bufSize)

	if vr == nil && format != 0 {
		vr = &GposValueRecord{}
	}

	if format&0x0001 != 0 {
		buf = append(buf, byte(vr.XPlacement>>8), byte(vr.XPlacement))
	}
	if format&0x0002 != 0 {
		buf = append(buf, byte(vr.YPlacement>>8), byte(vr.YPlacement))
	}
	if format&0x0004 != 0 {
		buf = append(buf, byte(vr.XAdvance>>8), byte(vr.XAdvance))
	}
	if format&0x0008 != 0 {
		buf = append(buf, byte(vr.YAdvance>>8), byte(vr.YAdvance))
	}
	if format&0x0010 != 0 {
		buf = append(buf, byte(devOffsets[0]>>8), byte(devOffsets[0]))
	}
	if format&0x0020 != 0 {
		buf = append(buf, byte(devOffsets[1]>>8), byte(devOffsets[1]))
	}
	if format&0x0040 != 0 {
		buf = append(buf, byte(devOffsets[2]>>8), byte(devOffsets[2]))
	}
	if format&0x0080 != 0 {
		buf = append(buf, byte(devOffsets[3]>>8), byte(devOffsets[3]))
	}
	return buf
}

func (vr *GposValueRecord) String() string {
	if vr == nil {
		return "<nil>"
	}

	var adjust []string
	if vr.XPlacement != 0 {
		adjust = append(adjust, fmt.Sprintf("x%+d", vr.XPlacement))
	}
	if vr.YPlacement != 0 {
		adjust = append(adjust, fmt.Sprintf("y%+d", vr.YPlacement))
	}
	if vr.XAdvance != 0 {
		adjust = append(adjust, fmt.Sprintf("dx%+d", vr.XAdvance))
	}
	if vr.YAdvance != 0 {
		adjust = append(adjust, fmt.Sprintf("dy%+d", vr.YAdvance))
	}
	if vr.XPlacementDev != nil {
		adjust = append(adjust, "xdev")
	}
	if vr.YPlacementDev != nil {
		adjust = append(adjust, "ydev")
	}
	if vr.XAdvanceDev != nil {
		adjust = append(adjust, "dxdev")
	}
	if vr.YAdvanceDev != nil {
		adjust = append(adjust, "dydev")
	}
	if len(adjust) == 0 {
		return "_"
	}
	return strings.Join(adjust, ",")
}

// Apply adjusts the position of a glyph according to the value record.
// Device-table adjustments are not applied here: ppem/variation context
// lives at the layout layer.
func (vr *GposValueRecord) Apply(glyph *glyph.Info) {
	if vr == nil {
		return
	}
	glyph.XOffset += vr.XPlacement
	glyph.YOffset += vr.YPlacement
	glyph.Advance += vr.XAdvance
	glyph.YAdvance += vr.YAdvance
}
