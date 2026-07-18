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
	"math"

	"seehuhn.de/go/geom/matrix"

	"seehuhn.de/go/postscript/cid"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/variation"
)

// Instance collapses all blends at the given normalized coordinates and
// returns static CID-keyed CFF outlines.  The result uses the Adobe-Identity-0
// character collection with an identity GIDToCID map, so a composite embedder
// can consume it natively as CIDFontType0C.
//
// widths supplies per-glyph advance widths in design units; a nil slice falls
// back to o.Widths.  Negative widths are clamped to zero, mirroring the CFF
// read convention.
//
// The receiver is left unmodified.
func (o *OutlinesCFF2) Instance(coords []variation.F2Dot14, widths []float64) (*Outlines, error) {
	// per-vsindex region-scalar vectors, computed lazily.  scalarsFor(vsi)
	// returns one scalar per region column of IVD subtable vsi, aligned to the
	// Blend.Deltas of every value that selects that subtable.
	var scalarCache map[int][]float64
	scalarsFor := func(vsi int) []float64 {
		if o.VarStore == nil {
			return nil
		}
		if s, ok := scalarCache[vsi]; ok {
			return s
		}
		var s []float64
		if vsi >= 0 && vsi < len(o.VarStore.Data) {
			ivd := o.VarStore.Data[vsi]
			s = make([]float64, len(ivd.RegionIndexes))
			for i, ri := range ivd.RegionIndexes {
				if int(ri) < len(o.VarStore.Regions) {
					s[i] = o.VarStore.Regions[ri].Scalar(coords)
				}
			}
		}
		if scalarCache == nil {
			scalarCache = make(map[int][]float64)
		}
		scalarCache[vsi] = s
		return s
	}

	n := len(o.Glyphs)
	glyphs := make([]*Glyph, n)
	for gid := range o.Glyphs {
		g := o.Glyphs[gid]
		ng := &Glyph{}
		if g != nil {
			sc := scalarsFor(g.VSIndex)
			ng.Cmds = instanceCmds(g.Cmds, sc)
			ng.HStem = instanceStems(g.HStem, sc)
			ng.VStem = instanceStems(g.VStem, sc)
		}

		var w float64
		if widths != nil {
			if gid < len(widths) {
				w = widths[gid]
			}
		} else if gid < len(o.Widths) {
			w = o.Widths[gid]
		}
		if w < 0 {
			w = 0
		}
		ng.Width = w

		glyphs[gid] = ng
	}

	private := make([]*type1.PrivateDict, len(o.Private))
	for i, p := range o.Private {
		private[i] = instancePrivateDict(p, scalarsFor(privateVSIndex(p)))
	}

	fdSelect := o.FDSelect
	if fdSelect == nil {
		fdSelect = func(glyph.ID) int { return 0 }
	}

	gidToCID := make([]cid.CID, n)
	for i := range gidToCID {
		gidToCID[i] = cid.CID(i)
	}

	fontMatrices := make([]matrix.Matrix, len(private))
	for i := range fontMatrices {
		if i < len(o.FontMatrices) {
			fontMatrices[i] = o.FontMatrices[i]
		} else {
			fontMatrices[i] = matrix.Identity
		}
	}

	return &Outlines{
		Glyphs:       glyphs,
		Private:      private,
		FDSelect:     fdSelect,
		ROS:          &cid.SystemInfo{Registry: "Adobe", Ordering: "Identity", Supplement: 0},
		GIDToCID:     gidToCID,
		FontMatrices: fontMatrices,
	}, nil
}

// privateVSIndex returns the dict-level vsindex of p, or 0 when p is nil.
func privateVSIndex(p *PrivateCFF2) int {
	if p == nil {
		return 0
	}
	return p.VSIndex
}

// instanceCmds evaluates the drawing commands of a CFF2 glyph at scalars,
// producing static CFF1 commands.  Hintmask/cntrmask operands are raw mask
// bytes and are copied verbatim; all other operands are blended and clamped to
// the 16.16 value domain.
func instanceCmds(cmds []GlyphOpCFF2, scalars []float64) []GlyphOp {
	if len(cmds) == 0 {
		return nil
	}
	res := make([]GlyphOp, len(cmds))
	for i, cmd := range cmds {
		out := GlyphOp{Op: cmd.Op, Args: make([]float64, len(cmd.Args))}
		switch cmd.Op {
		case OpHintMask, OpCntrMask:
			for j, a := range cmd.Args {
				out.Args[j] = a.Default
			}
		default:
			for j, a := range cmd.Args {
				out.Args[j] = fix(a.At(scalars))
			}
		}
		res[i] = out
	}
	return res
}

// instanceStems evaluates blended stem edges at scalars.  Stems are stored as
// edge pairs; a pair whose second edge precedes the first after evaluation is
// swapped so the instanced stem stays valid.  The list length and order are
// otherwise preserved, so the hintmask bit numbering (which depends on the
// stem count) survives unchanged.
func instanceStems(stems []Blend, scalars []float64) []float64 {
	if len(stems) == 0 {
		return nil
	}
	res := make([]float64, len(stems))
	for i, s := range stems {
		res[i] = fix(s.At(scalars))
	}
	for i := 0; i+1 < len(res); i += 2 {
		if res[i+1] < res[i] {
			res[i], res[i+1] = res[i+1], res[i]
		}
	}
	return res
}

// instancePrivateDict evaluates a CFF2 private dict at scalars into a static
// Type 1 private dict.
//
// type1.PrivateDict has no fields for the CFF2 FamilyBlues, FamilyOtherBlues,
// StemSnapH/V, LanguageGroup or ExpansionFactor entries, so those values are
// dropped, mirroring the Type 1 multiple-master conversion.  ForceBold is
// absent in CFF2 and left false.
func instancePrivateDict(p *PrivateCFF2, scalars []float64) *type1.PrivateDict {
	if p == nil {
		return &type1.PrivateDict{}
	}
	return &type1.PrivateDict{
		BlueValues: instanceBlueArray(p.BlueValues, scalars),
		OtherBlues: instanceBlueArray(p.OtherBlues, scalars),
		BlueScale:  p.BlueScale.At(scalars),
		BlueShift:  int32(variation.OTRound(p.BlueShift.At(scalars))),
		BlueFuzz:   int32(variation.OTRound(p.BlueFuzz.At(scalars))),
		StdHW:      p.StdHW.At(scalars),
		StdVW:      p.StdVW.At(scalars),
	}
}

// instanceBlueArray evaluates a list of blended blue-zone edges at scalars,
// rounding each to the int16 grid.  The order is preserved.
func instanceBlueArray(vals []Blend, scalars []float64) []funit.Int16 {
	if len(vals) == 0 {
		return nil
	}
	res := make([]funit.Int16, len(vals))
	for i, v := range vals {
		res[i] = toInt16(variation.OTRound(v.At(scalars)))
	}
	return res
}

// toInt16 clamps v to the range of funit.Int16.
func toInt16(v float64) funit.Int16 {
	if v > math.MaxInt16 {
		return math.MaxInt16
	}
	if v < math.MinInt16 {
		return math.MinInt16
	}
	return funit.Int16(v)
}
