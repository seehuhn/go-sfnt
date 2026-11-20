// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package mathtable

import (
	"encoding/binary"
	"errors"
	"fmt"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/opentype/internal/readcache"
	"seehuhn.de/go/sfnt/parser"
)

const (
	headerSize    = 10
	constantsSize = 8 + 4*numConstantValues + 2
)

// Read reads a "MATH" table from r.  Allocations are charged against
// budget.
//
// The read is permissive.  A MathConstants table which cannot be read makes
// the read fail.  Other damaged parts are repaired: missing or unreadable
// subtables are treated as empty, a device table which cannot be read is
// dropped and the value kept, and records beyond the end of the data or
// beyond the extent of the coverage table are ignored.  A kern table with
// unsorted correction heights or which cannot be read is dropped, as are
// glyph constructions without variants and assembly, and assemblies without
// parts.
func Read(r parser.ReadSeekSizer, budget *membudget.Budget) (*Info, error) {
	p := parser.New(r, budget)
	buf, err := p.ReadBytes(headerSize)
	if err != nil {
		return nil, err
	}
	major := binary.BigEndian.Uint16(buf)
	if major != 1 {
		return nil, &parser.NotSupportedError{
			SubSystem: "sfnt/opentype/mathtable",
			Feature:   fmt.Sprintf("MATH table version %d", major),
		}
	}
	constantsOffset := binary.BigEndian.Uint16(buf[4:])
	glyphInfoOffset := binary.BigEndian.Uint16(buf[6:])
	variantsOffset := binary.BigEndian.Uint16(buf[8:])

	rd := &reader{
		p:             p,
		size:          p.Size(),
		devices:       readcache.New[*device.Table](),
		kerns:         readcache.New[*Kern](),
		assemblies:    readcache.New[*Assembly](),
		constructions: readcache.New[*Construction](),
	}

	if constantsOffset == 0 {
		return nil, &parser.InvalidFontError{
			SubSystem: "sfnt/opentype/mathtable",
			Reason:    "missing MathConstants table",
		}
	}
	info := &Info{}
	err = rd.constants(&info.Constants, int64(constantsOffset))
	if err != nil {
		return nil, err
	}
	if glyphInfoOffset != 0 {
		err = rd.glyphInfo(info, int64(glyphInfoOffset))
		if err != nil {
			return nil, err
		}
	}
	if variantsOffset != 0 {
		err = rd.variants(info, int64(variantsOffset))
		if err != nil {
			return nil, err
		}
	}
	return info, nil
}

// soft converts the errors which indicate damaged data into nil.  Only
// errors which must abort the read are returned.
func soft(err error) error {
	if errors.Is(err, membudget.ErrExceeded) {
		return err
	}
	return nil
}

// reader holds the state of a single Read call.  Tables which several
// offsets point to are read once and shared.  A nil entry in a cache marks a
// table which has been dropped.
type reader struct {
	p             *parser.Parser
	size          int64
	devices       *readcache.Cache[*device.Table]
	kerns         *readcache.Cache[*Kern]
	assemblies    *readcache.Cache[*Assembly]
	constructions *readcache.Cache[*Construction]
}

// value reads the MathValueRecord at pos.  Device offsets are relative to
// base.  A device table which cannot be read is dropped.
func (rd *reader) value(pos, base int64) (Value, error) {
	err := rd.p.SeekPos(pos)
	if err != nil {
		return Value{}, err
	}
	buf, err := rd.p.ReadBytes(4)
	if err != nil {
		return Value{}, err
	}
	v := Value{V: funit.Int16(int16(binary.BigEndian.Uint16(buf)))}
	if off := binary.BigEndian.Uint16(buf[2:]); off != 0 {
		v.Device, err = rd.devices.Get(base+int64(off), rd.device)
		if err != nil {
			return Value{}, err
		}
	}
	return v, nil
}

func (rd *reader) device(pos int64) (*device.Table, error) {
	t, err := device.Read(rd.p, pos)
	if err != nil {
		return nil, soft(err)
	}
	return t, nil
}

func (rd *reader) constants(c *Constants, pos int64) error {
	err := rd.p.SeekPos(pos)
	if err != nil {
		return err
	}
	buf, err := rd.p.ReadBytes(8)
	if err != nil {
		return err
	}
	c.ScriptPercentScaleDown = int16(binary.BigEndian.Uint16(buf))
	c.ScriptScriptPercentScaleDown = int16(binary.BigEndian.Uint16(buf[2:]))
	c.DelimitedSubFormulaMinHeight = funit.Uint16(binary.BigEndian.Uint16(buf[4:]))
	c.DisplayOperatorMinHeight = funit.Uint16(binary.BigEndian.Uint16(buf[6:]))

	for i, v := range c.values() {
		*v, err = rd.value(pos+8+4*int64(i), pos)
		if err != nil {
			return err
		}
	}

	err = rd.p.SeekPos(pos + constantsSize - 2)
	if err != nil {
		return err
	}
	x, err := rd.p.ReadInt16()
	if err != nil {
		return err
	}
	c.RadicalDegreeBottomRaisePercent = x
	return nil
}

// makeMap allocates a map for n entries, charging the budget.
func makeMap[K comparable, V any](b *membudget.Budget, n int) (map[K]V, error) {
	err := b.ChargeN(n, membudget.MapEntryOverhead+16)
	if err != nil {
		return nil, err
	}
	return make(map[K]V, n), nil
}

// alloc allocates a zero value, charging the budget.
func alloc[T any](b *membudget.Budget) (*T, error) {
	s, err := b.AllocSlice[T](1)
	if err != nil {
		return nil, err
	}
	return &s[0], nil
}

// readCoverage reads the coverage table at pos+off.  A missing or damaged
// table is reported as nil.
func (rd *reader) readCoverage(pos int64, off uint16) (coverage.Table, error) {
	if off == 0 {
		return nil, nil
	}
	cov, err := coverage.Read(rd.p, pos+int64(off))
	if err != nil {
		return nil, soft(err)
	}
	return cov, nil
}

// fit returns the number of records, out of count records of the given size
// starting at pos, which have a coverage entry and lie inside the data.
func (rd *reader) fit(count, covLen int, pos int64, recSize int) int {
	return max(min(count, covLen, int((rd.size-pos)/int64(recSize))), 0)
}

func (rd *reader) glyphInfo(info *Info, pos int64) error {
	err := rd.p.SeekPos(pos)
	if err != nil {
		return soft(err)
	}
	buf, err := rd.p.ReadBytes(8)
	if err != nil {
		return soft(err)
	}
	var offs [4]uint16
	for i := range offs {
		offs[i] = binary.BigEndian.Uint16(buf[2*i:])
	}

	if offs[0] != 0 {
		info.ItalicsCorrection, err = rd.valueMap(pos + int64(offs[0]))
		if err != nil {
			return err
		}
	}
	if offs[1] != 0 {
		info.TopAccent, err = rd.valueMap(pos + int64(offs[1]))
		if err != nil {
			return err
		}
	}
	if offs[2] != 0 {
		set, err := coverage.ReadSet(rd.p, pos+int64(offs[2]))
		if err != nil {
			if err = soft(err); err != nil {
				return err
			}
		} else if len(set) > 0 {
			info.ExtendedShapes = set
		}
	}
	if offs[3] != 0 {
		info.Kerns, err = rd.kernInfo(pos + int64(offs[3]))
		if err != nil {
			return err
		}
	}
	return nil
}

// valueMap reads a MathItalicsCorrectionInfo or MathTopAccentAttachment
// table.
func (rd *reader) valueMap(pos int64) (map[glyph.ID]Value, error) {
	err := rd.p.SeekPos(pos)
	if err != nil {
		return nil, soft(err)
	}
	buf, err := rd.p.ReadBytes(4)
	if err != nil {
		return nil, soft(err)
	}
	covOff := binary.BigEndian.Uint16(buf)
	count := int(binary.BigEndian.Uint16(buf[2:]))

	cov, err := rd.readCoverage(pos, covOff)
	if err != nil {
		return nil, err
	}
	n := rd.fit(count, len(cov), pos+4, 4)
	if n == 0 {
		return nil, nil
	}
	m, err := makeMap[glyph.ID, Value](rd.p.Budget, n)
	if err != nil {
		return nil, err
	}
	for gid, idx := range cov {
		if idx >= n {
			continue
		}
		v, err := rd.value(pos+4+4*int64(idx), pos)
		if err != nil {
			if err = soft(err); err != nil {
				return nil, err
			}
			continue
		}
		m[gid] = v
	}
	if len(m) == 0 {
		return nil, nil
	}
	return m, nil
}

func (rd *reader) kernInfo(pos int64) (map[glyph.ID]*KernInfo, error) {
	err := rd.p.SeekPos(pos)
	if err != nil {
		return nil, soft(err)
	}
	buf, err := rd.p.ReadBytes(4)
	if err != nil {
		return nil, soft(err)
	}
	covOff := binary.BigEndian.Uint16(buf)
	count := int(binary.BigEndian.Uint16(buf[2:]))

	cov, err := rd.readCoverage(pos, covOff)
	if err != nil {
		return nil, err
	}
	n := rd.fit(count, len(cov), pos+4, 8)
	if n == 0 {
		return nil, nil
	}
	m, err := makeMap[glyph.ID, *KernInfo](rd.p.Budget, n)
	if err != nil {
		return nil, err
	}
	for gid, idx := range cov {
		if idx >= n {
			continue
		}
		err := rd.p.SeekPos(pos + 4 + 8*int64(idx))
		if err != nil {
			if err = soft(err); err != nil {
				return nil, err
			}
			continue
		}
		buf, err := rd.p.ReadBytes(8)
		if err != nil {
			if err = soft(err); err != nil {
				return nil, err
			}
			continue
		}
		var offs [4]uint16
		for i := range offs {
			offs[i] = binary.BigEndian.Uint16(buf[2*i:])
		}

		ki, err := alloc[KernInfo](rd.p.Budget)
		if err != nil {
			return nil, err
		}
		for i, dst := range []**Kern{&ki.TopRight, &ki.TopLeft, &ki.BottomRight, &ki.BottomLeft} {
			if offs[i] == 0 {
				continue
			}
			*dst, err = rd.kerns.Get(pos+int64(offs[i]), rd.kern)
			if err != nil {
				return nil, err
			}
		}
		m[gid] = ki
	}
	if len(m) == 0 {
		return nil, nil
	}
	return m, nil
}

// kern reads a MathKern table.  A damaged table is dropped.
func (rd *reader) kern(pos int64) (*Kern, error) {
	err := rd.p.SeekPos(pos)
	if err != nil {
		return nil, soft(err)
	}
	buf, err := rd.p.ReadBytes(2)
	if err != nil {
		return nil, soft(err)
	}
	h := int(binary.BigEndian.Uint16(buf))
	if int64(8*h+4) > rd.size-pos-2 {
		return nil, nil
	}

	var heights []Value
	if h > 0 {
		heights, err = rd.p.Budget.AllocSlice[Value](h)
		if err != nil {
			return nil, err
		}
	}
	kerns, err := rd.p.Budget.AllocSlice[Value](h + 1)
	if err != nil {
		return nil, err
	}
	for i := range heights {
		heights[i], err = rd.value(pos+2+4*int64(i), pos)
		if err != nil {
			return nil, soft(err)
		}
	}
	for i := range kerns {
		kerns[i], err = rd.value(pos+2+4*int64(h)+4*int64(i), pos)
		if err != nil {
			return nil, soft(err)
		}
	}
	if !heightsSorted(heights) {
		return nil, nil
	}

	k, err := alloc[Kern](rd.p.Budget)
	if err != nil {
		return nil, err
	}
	k.Heights = heights
	k.Kerns = kerns
	return k, nil
}

func (rd *reader) variants(info *Info, pos int64) error {
	err := rd.p.SeekPos(pos)
	if err != nil {
		return soft(err)
	}
	buf, err := rd.p.ReadBytes(10)
	if err != nil {
		return soft(err)
	}
	info.MinConnectorOverlap = funit.Uint16(binary.BigEndian.Uint16(buf))
	vertCov := binary.BigEndian.Uint16(buf[2:])
	horizCov := binary.BigEndian.Uint16(buf[4:])
	vertCount := int(binary.BigEndian.Uint16(buf[6:]))
	horizCount := int(binary.BigEndian.Uint16(buf[8:]))

	info.Vertical, err = rd.constructionMap(pos, vertCov, vertCount, pos+10)
	if err != nil {
		return err
	}
	info.Horizontal, err = rd.constructionMap(pos, horizCov, horizCount, pos+10+2*int64(vertCount))
	return err
}

// constructionMap reads the glyph constructions of one direction.  The
// array of offsets starts at arr.
func (rd *reader) constructionMap(pos int64, covOff uint16, count int, arr int64) (map[glyph.ID]*Construction, error) {
	cov, err := rd.readCoverage(pos, covOff)
	if err != nil {
		return nil, err
	}
	n := rd.fit(count, len(cov), arr, 2)
	if n == 0 {
		return nil, nil
	}
	m, err := makeMap[glyph.ID, *Construction](rd.p.Budget, n)
	if err != nil {
		return nil, err
	}
	for gid, idx := range cov {
		if idx >= n {
			continue
		}
		err := rd.p.SeekPos(arr + 2*int64(idx))
		if err != nil {
			if err = soft(err); err != nil {
				return nil, err
			}
			continue
		}
		off, err := rd.p.ReadUint16()
		if err != nil {
			if err = soft(err); err != nil {
				return nil, err
			}
			continue
		}
		if off == 0 {
			continue
		}
		c, err := rd.constructions.Get(pos+int64(off), rd.construction)
		if err != nil {
			return nil, err
		}
		if c != nil {
			m[gid] = c
		}
	}
	if len(m) == 0 {
		return nil, nil
	}
	return m, nil
}

// construction reads a MathGlyphConstruction table.  A construction without
// variants and assembly is dropped.
func (rd *reader) construction(pos int64) (*Construction, error) {
	err := rd.p.SeekPos(pos)
	if err != nil {
		return nil, soft(err)
	}
	buf, err := rd.p.ReadBytes(4)
	if err != nil {
		return nil, soft(err)
	}
	assemblyOff := binary.BigEndian.Uint16(buf)
	n := min(int(binary.BigEndian.Uint16(buf[2:])), int((rd.size-pos-4)/4))

	var variants []Variant
	if n > 0 {
		variants, err = rd.p.Budget.AllocSlice[Variant](n)
		if err != nil {
			return nil, err
		}
		err = rd.p.SeekPos(pos + 4)
		if err != nil {
			return nil, soft(err)
		}
		for i := range variants {
			buf, err := rd.p.ReadBytes(4)
			if err != nil {
				return nil, soft(err)
			}
			variants[i] = Variant{
				Glyph:   glyph.ID(binary.BigEndian.Uint16(buf)),
				Advance: funit.Uint16(binary.BigEndian.Uint16(buf[2:])),
			}
		}
	}

	var assembly *Assembly
	if assemblyOff != 0 {
		assembly, err = rd.assemblies.Get(pos+int64(assemblyOff), rd.assembly)
		if err != nil {
			return nil, err
		}
	}
	if len(variants) == 0 && assembly == nil {
		return nil, nil
	}

	c, err := alloc[Construction](rd.p.Budget)
	if err != nil {
		return nil, err
	}
	c.Variants = variants
	c.Assembly = assembly
	return c, nil
}

// assembly reads a GlyphAssembly table.  An assembly without parts is
// dropped.
func (rd *reader) assembly(pos int64) (*Assembly, error) {
	italics, err := rd.value(pos, pos)
	if err != nil {
		return nil, soft(err)
	}
	err = rd.p.SeekPos(pos + 4)
	if err != nil {
		return nil, soft(err)
	}
	count, err := rd.p.ReadUint16()
	if err != nil {
		return nil, soft(err)
	}
	n := min(int(count), int((rd.size-pos-6)/10))
	if n <= 0 {
		return nil, nil
	}
	parts, err := rd.p.Budget.AllocSlice[Part](n)
	if err != nil {
		return nil, err
	}
	for i := range parts {
		buf, err := rd.p.ReadBytes(10)
		if err != nil {
			return nil, soft(err)
		}
		parts[i] = Part{
			Glyph:          glyph.ID(binary.BigEndian.Uint16(buf)),
			StartConnector: funit.Uint16(binary.BigEndian.Uint16(buf[2:])),
			EndConnector:   funit.Uint16(binary.BigEndian.Uint16(buf[4:])),
			FullAdvance:    funit.Uint16(binary.BigEndian.Uint16(buf[6:])),
			Extender:       binary.BigEndian.Uint16(buf[8:])&1 != 0,
		}
	}

	a, err := alloc[Assembly](rd.p.Budget)
	if err != nil {
		return nil, err
	}
	a.ItalicsCorrection = italics
	a.Parts = parts
	return a, nil
}
