// Copyright 2022 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package gtab

import (
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/parser"
)

// readGIDSlice reads a length followed by a sequence of GlyphID values.
func readGIDSlice(p *parser.Parser) ([]glyph.ID, error) {
	n, err := p.ReadUint16()
	if err != nil {
		return nil, err
	}
	res, err := p.Budget.AllocSlice[glyph.ID](int(n))
	if err != nil {
		return nil, err
	}
	for i := range res {
		val, err := p.ReadUint16()
		if err != nil {
			return nil, err
		}
		res[i] = glyph.ID(val)
	}
	return res, nil
}

// encodeGIDSlice encodes gids in the format read by [readGIDSlice].
func encodeGIDSlice(gids []glyph.ID) []byte {
	buf := make([]byte, 0, 2+2*len(gids))
	buf = append(buf, byte(len(gids)>>8), byte(len(gids)))
	for _, gid := range gids {
		buf = append(buf, byte(gid>>8), byte(gid))
	}
	return buf
}
