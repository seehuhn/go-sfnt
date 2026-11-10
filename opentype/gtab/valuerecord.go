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
	"unsafe"

	"seehuhn.de/go/postscript/funit"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/opentype/internal/offsetpool"
	"seehuhn.de/go/sfnt/opentype/internal/readcache"
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

// decodeValueFormat decodes a valueFormat field from its first two bytes.
// The reserved bits are cleared: they select no fields, and the spec
// requires them to be zero.  Without this, a format consisting only of
// reserved bits would give non-nil records which occupy no space in the
// file.
func decodeValueFormat(buf []byte) uint16 {
	return (uint16(buf[0])<<8 | uint16(buf[1])) & 0x00FF
}

// readValueRecord reads the binary representation of a valueRecord.  The
// valueFormat determines which fields are present in the inline portion of
// the binary representation.  Device tables, if any, live elsewhere in the
// subtable; their offsets are relative to subtablePos.  Device tables are
// read via devices, so that records which refer to the same bytes share one
// table.
func readValueRecord(p *parser.Parser, valueFormat uint16, subtablePos int64, devices *readcache.Cache[*device.Table]) (*GposValueRecord, error) {
	if valueFormat == 0 {
		return nil, nil
	}

	res := &GposValueRecord{}
	if err := p.Budget.Charge(int(unsafe.Sizeof(*res))); err != nil {
		return nil, err
	}
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
			t, err := devices.Get(subtablePos+int64(offs), func(pos int64) (*device.Table, error) {
				return device.Read(p, pos)
			})
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

// getFormat returns the valueFormat for encoding vr on its own.
func (vr *GposValueRecord) getFormat() uint16 {
	var b formatBuilder
	b.add(vr)
	return b.format()
}

// formatBuilder computes the valueFormat for a group of value records which
// share one format in the encoded subtable.  The format includes every field
// which is non-zero in at least one record.
//
// On read, a valueFormat of 0 gives nil records.  If the records are all
// non-nil but have no non-zero fields, the format includes the XAdvance
// field, so that the records read back as zero records rather than nil.
type formatBuilder struct {
	bits      uint16
	hasRecord bool
}

// add includes the fields of vr in the format.  A nil vr is ignored.
func (f *formatBuilder) add(vr *GposValueRecord) {
	if vr == nil {
		return
	}
	f.hasRecord = true

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

	f.bits |= format
}

// format returns the valueFormat for the records added so far.
func (f formatBuilder) format() uint16 {
	if f.bits == 0 && f.hasRecord {
		return 0x0004
	}
	return f.bits
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
// The caller supplies the pool's base offset (the byte position where the
// pool's tables begin) at construction time.  The offsets are assigned by
// the Place method, once all tables are known.
type devicePool struct{ *offsetpool.Deferred }

func newDevicePool(name string, base int) devicePool {
	return devicePool{offsetpool.NewDeferred(name, base)}
}

// add registers t and returns its pool index, or -1 if t is nil.  Tables
// with byte-identical Encode output share a single index.  An error is
// returned if the tables cannot be reached through Offset16 fields.
func (p devicePool) add(t *device.Table) (int, error) {
	if t == nil {
		return -1, nil
	}
	return p.AddFunc(t, t.Encode)
}

// addRecord registers the Device tables of vr, in the canonical order
// returned by deviceTables, and returns their pool indices.
func (p devicePool) addRecord(vr *GposValueRecord) ([4]int, error) {
	var indices [4]int
	for k, d := range vr.deviceTables() {
		idx, err := p.add(d)
		if err != nil {
			return indices, err
		}
		indices[k] = idx
	}
	return indices, nil
}

// deviceOffsets returns the offsets of the Device tables with the given pool
// indices.  A negative index stands for an absent table and gives offset 0.
func deviceOffsets(offs []uint16, indices [4]int) [4]uint16 {
	var res [4]uint16
	for k, idx := range indices {
		res[k] = poolOffset(offs, idx)
	}
	return res
}

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
