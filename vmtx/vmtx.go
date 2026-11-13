// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

// Package vmtx reads and writes "vhea" and "vmtx" tables.
// https://learn.microsoft.com/en-us/typography/opentype/spec/vhea
//
// The tables have the same binary layout as "hhea" and "hmtx", but describe
// the vertical text layout: advance heights and top side bearings.  The top
// side bearing of a glyph is the distance between the vertical origin and the
// top of the glyph bounding box.
package vmtx

import (
	"fmt"
	"math"

	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/sfnt/internal/lmetrics"
)

// Info contains information from the "vhea" and "vmtx" tables.
type Info struct {
	// Ascent, Descent and LineGap are the vertical typographic metrics.
	// The vertical line extends Ascent units to the right and -Descent
	// units to the left of the vertical centre line.
	Ascent  funit.Int16
	Descent funit.Int16
	LineGap funit.Int16

	// CaretSlopeRise and CaretSlopeRun give the slope of the text cursor.
	// The vertical caret has rise 0 and run 1.
	CaretSlopeRise int16
	CaretSlopeRun  int16

	// CaretOffset is the shift of the highlighted slanted caret
	// along the glyph's right-hand side.
	CaretOffset int16

	// Heights contains the advance height of each glyph.
	Heights []funit.Uint16

	// TSB contains the top side bearing of each glyph.  It must have the
	// same length as Heights.
	TSB []funit.Int16

	// GlyphExtents contains the glyph bounding boxes.  They are only used by
	// Encode, to compute the derived fields of the "vhea" table.  Decode
	// does not fill this field.
	GlyphExtents []funit.Rect16
}

// Decode extracts information from the "vhea" and "vmtx" tables.
// If vmtxData is nil, only the "vhea" fields are filled.
func Decode(vheaData, vmtxData []byte) (*Info, error) {
	vhea, err := lmetrics.ParseHeader(vheaData)
	if err != nil {
		return nil, err
	}
	if vhea.Version != 0x00010000 && vhea.Version != 0x00011000 {
		return nil, fmt.Errorf("unsupported vhea version %08x", vhea.Version)
	}
	if vhea.MetricDataFmt != 0 {
		return nil, fmt.Errorf("unsupported metric data format %d", vhea.MetricDataFmt)
	}

	info := &Info{
		Ascent:         vhea.Ascent,
		Descent:        vhea.Descent,
		LineGap:        vhea.LineGap,
		CaretSlopeRise: vhea.CaretSlopeRise,
		CaretSlopeRun:  vhea.CaretSlopeRun,
		CaretOffset:    int16(vhea.CaretOffset),
	}
	if info.CaretSlopeRise == 0 && info.CaretSlopeRun == 0 {
		info.CaretSlopeRun = 1
	}

	if vmtxData == nil {
		return info, nil
	}

	heights, tsbs, err := lmetrics.Decode(vmtxData, int(vhea.NumLong))
	if err != nil {
		return nil, err
	}
	info.Heights = heights
	info.TSB = tsbs

	return info, nil
}

// Encode creates the "vhea" and "vmtx" tables.  The "vhea" table is
// written in version 1.1.  If Heights is nil, the "vmtx" table is nil.
// Otherwise TSB must have one entry per glyph, and GlyphExtents, if present,
// must too; Encode panics otherwise.  A caret slope of (0, 0) is written
// as (0, 1).
func (info *Info) Encode() (vheaData []byte, vmtxData []byte) {
	rise, run := info.CaretSlopeRise, info.CaretSlopeRun
	if rise == 0 && run == 0 {
		run = 1
	}

	vhea := &lmetrics.Header{
		Version:        0x00011000, // 1.1
		Ascent:         info.Ascent,
		Descent:        info.Descent,
		LineGap:        info.LineGap,
		CaretSlopeRise: rise,
		CaretSlopeRun:  run,
		CaretOffset:    funit.Int16(info.CaretOffset),
	}

	for _, h := range info.Heights {
		if h > vhea.AdvanceMax {
			vhea.AdvanceMax = h
		}
	}

	if info.Heights != nil {
		if len(info.TSB) != len(info.Heights) {
			panic("len(info.TSB) != len(info.Heights)")
		}
		if info.GlyphExtents != nil && len(info.GlyphExtents) != len(info.Heights) {
			panic("len(info.GlyphExtents) != len(info.Heights)")
		}
	}

	// the summary fields only cover glyphs with a bounding box
	if info.Heights != nil && info.GlyphExtents != nil {
		minTSB, minBSB, maxExtent := math.MaxInt, math.MaxInt, math.MinInt
		for i, ext := range info.GlyphExtents {
			if ext.IsZero() {
				continue
			}
			tsb := int(info.TSB[i])
			extent := int(ext.URy) - int(ext.LLy)
			minTSB = min(minTSB, tsb)
			minBSB = min(minBSB, int(info.Heights[i])-tsb-extent)
			maxExtent = max(maxExtent, tsb+extent)
		}
		if minTSB != math.MaxInt {
			vhea.MinBearing1 = saturate(minTSB)
			vhea.MinBearing2 = saturate(minBSB)
			vhea.MaxExtent = saturate(maxExtent)
		}
	} else if info.Heights != nil {
		for i, tsb := range info.TSB {
			if i == 0 || tsb < vhea.MinBearing1 {
				vhea.MinBearing1 = tsb
			}
		}
	}

	if info.Heights == nil {
		return vhea.Bytes(), nil
	}

	vmtxData, numLong := lmetrics.Encode(info.Heights, info.TSB)
	vhea.NumLong = uint16(numLong)
	return vhea.Bytes(), vmtxData
}

// saturate converts v to funit.Int16, limiting it to the representable range.
func saturate(v int) funit.Int16 {
	return funit.Int16(max(math.MinInt16, min(math.MaxInt16, v)))
}
