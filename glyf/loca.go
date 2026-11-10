// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package glyf

import (
	"fmt"

	"seehuhn.de/go/sfnt/parser"
)

func decodeLoca(enc *Encoded) ([]int, error) {
	var offs []int
	switch enc.LocaFormat {
	case 0:
		n := len(enc.LocaData)
		if n < 4 || n%2 != 0 {
			return nil, &parser.InvalidFontError{
				SubSystem: "sfnt/loca",
				Reason:    "invalid table length",
			}
		}
		offs = make([]int, n/2)
		prev := 0
		for i := range offs {
			x := int(enc.LocaData[2*i])<<8 | int(enc.LocaData[2*i+1])
			pos := 2 * x
			if pos < prev || pos > len(enc.GlyfData) {
				return nil, &parser.InvalidFontError{
					SubSystem: "sfnt/loca",
					Reason:    fmt.Sprintf("invalid offset %d", pos),
				}
			}
			offs[i] = pos
			prev = pos
		}
	case 1:
		n := len(enc.LocaData)
		if n < 8 || n%4 != 0 {
			return nil, &parser.InvalidFontError{
				SubSystem: "sfnt/loca",
				Reason:    "invalid table length",
			}
		}
		offs = make([]int, len(enc.LocaData)/4)
		prev := 0
		for i := range offs {
			pos := int(enc.LocaData[4*i])<<24 | int(enc.LocaData[4*i+1])<<16 |
				int(enc.LocaData[4*i+2])<<8 | int(enc.LocaData[4*i+3])
			if pos < prev || pos > len(enc.GlyfData) {
				return nil, &parser.InvalidFontError{
					SubSystem: "sfnt/loca",
					Reason:    "invalid offset",
				}
			}
			offs[i] = pos
			prev = pos
		}
	default:
		return nil, &parser.NotSupportedError{
			SubSystem: "sfnt/loca",
			Feature:   fmt.Sprintf("loca table format %d", enc.LocaFormat),
		}
	}
	return offs, nil
}

func encodeLoca(offs []int) ([]byte, int16) {
	var locaData []byte
	var locaFormat int16
	if offs[len(offs)-1] <= 0xffff {
		locaFormat = 0
		locaData = make([]byte, 2*len(offs))
		for i, off := range offs {
			x := off / 2
			locaData[2*i] = byte(x >> 8)
			locaData[2*i+1] = byte(x)
		}
	} else {
		locaFormat = 1
		locaData = make([]byte, 4*len(offs))
		for i, off := range offs {
			locaData[4*i] = byte(off >> 24)
			locaData[4*i+1] = byte(off >> 16)
			locaData[4*i+2] = byte(off >> 8)
			locaData[4*i+3] = byte(off)
		}
	}
	return locaData, locaFormat
}
