// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

// Package lmetrics implements the binary layout shared by the "hhea"/"hmtx"
// and "vhea"/"vmtx" table pairs.
//
// The header tables have a fixed length of 36 bytes.  The metrics tables
// consist of an array of long metrics (an unsigned advance and a signed
// bearing), followed by bare signed bearings for the remaining glyphs, which
// reuse the advance of the last long metric.
package lmetrics

import (
	"bytes"
	"encoding/binary"
	"errors"

	"seehuhn.de/go/postscript/funit"
)

// HeaderLength is the length of the "hhea" and "vhea" tables in bytes.
const HeaderLength = 36

var errTooShort = errors.New("metrics table too short")

// Header is the binary form of the "hhea" and "vhea" tables.  The field
// names use the "hhea" interpretation, except that MinBearing1 and
// MinBearing2 are the minimum leading and trailing side bearings,
// respectively.
type Header struct {
	Version        uint32
	Ascent         funit.Int16
	Descent        funit.Int16
	LineGap        funit.Int16
	AdvanceMax     funit.Uint16
	MinBearing1    funit.Int16
	MinBearing2    funit.Int16
	MaxExtent      funit.Int16
	CaretSlopeRise int16
	CaretSlopeRun  int16
	CaretOffset    funit.Int16
	_              int16 // reserved
	_              int16 // reserved
	_              int16 // reserved
	_              int16 // reserved
	MetricDataFmt  int16
	NumLong        uint16
}

// ParseHeader decodes a "hhea" or "vhea" table.
func ParseHeader(data []byte) (*Header, error) {
	h := &Header{}
	err := binary.Read(bytes.NewReader(data), binary.BigEndian, h)
	if err != nil {
		return nil, err
	}
	return h, nil
}

// Bytes returns the binary form of the header.
func (h *Header) Bytes() []byte {
	buf := bytes.NewBuffer(make([]byte, 0, HeaderLength))
	_ = binary.Write(buf, binary.BigEndian, h)
	return buf.Bytes()
}

// Decode decodes the array of long metrics and trailing bearings in the
// "hmtx" or "vmtx" table.  The number of glyphs is determined by the length
// of data.  The function returns one advance and one bearing per glyph.
func Decode(data []byte, numLong int) ([]funit.Uint16, []funit.Int16, error) {
	var prev funit.Uint16
	var advances []funit.Uint16
	var bearings []funit.Int16
	for i := 0; len(data) > 0; i++ {
		adv := prev
		if i < numLong {
			if len(data) < 2 {
				return nil, nil, errTooShort
			}
			adv = funit.Uint16(data[0])<<8 | funit.Uint16(data[1])
			data = data[2:]
			prev = adv
		}
		advances = append(advances, adv)

		if len(data) < 2 {
			return nil, nil, errTooShort
		}
		bearing := funit.Int16(data[0])<<8 | funit.Int16(data[1])
		data = data[2:]
		bearings = append(bearings, bearing)
	}
	if len(advances) < numLong {
		return nil, nil, errTooShort
	}
	return advances, bearings, nil
}

// Encode builds the "hmtx" or "vmtx" table.  The slices must have the same
// length.  The function returns the table and the number of long metrics,
// which must be stored in the corresponding header.  Trailing glyphs with
// the same advance as their predecessor are stored as bare bearings.
func Encode(advances []funit.Uint16, bearings []funit.Int16) ([]byte, int) {
	numGlyphs := len(advances)
	if len(bearings) != numGlyphs {
		panic("len(bearings) != len(advances)")
	}

	numLong := numGlyphs
	for numLong > 1 && advances[numLong-1] == advances[numLong-2] {
		numLong--
	}

	buf := make([]byte, 0, 4*numLong+2*(numGlyphs-numLong))
	for i := range numGlyphs {
		if i < numLong {
			buf = append(buf, byte(advances[i]>>8), byte(advances[i]))
		}
		buf = append(buf, byte(bearings[i]>>8), byte(bearings[i]))
	}
	return buf, numLong
}
