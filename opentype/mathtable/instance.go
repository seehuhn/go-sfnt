// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package mathtable

import (
	"iter"
	"maps"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/internal/vardelta"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/device"
)

// BakeVariationDevices returns a copy of info in which every value with a
// VariationIndex table has been folded into the value and the table
// removed.  For each such table t, resolve(t.OuterIndex, t.InnerIndex)
// supplies the delta which is added to the value.  Plain Device tables are
// left in place.  A kern table whose correction heights are out of order
// after the deltas have been added is removed.  If a value with its delta
// added does not fit into 16 bits, BakeVariationDevices returns an error.
//
// The receiver is not modified.  Kern tables, glyph constructions and
// assemblies which are shared in info are shared in the result.
func (info *Info) BakeVariationDevices(resolve func(outer, inner uint16) int64) (*Info, error) {
	var err error
	res := info.MapValues(func(v Value) Value {
		if v.Device != nil && v.Device.IsVariationIndex() {
			x, e := vardelta.AddInt16(v.V, resolve(v.Device.OuterIndex, v.Device.InnerIndex))
			if e != nil && err == nil {
				err = e
			}
			v.V = x
			v.Device = nil
		}
		return v
	})
	// the first error is reported; the partial result is discarded
	if err != nil {
		return nil, err
	}
	return res, nil
}

// MapValues returns a copy of info in which fn has been applied to every
// Value: the constants, italics corrections, top accent attachments, kern
// heights and kerns, and the italics corrections of assemblies.  A kern
// table whose correction heights are no longer in non-decreasing order
// after the change is removed from the result.
//
// The receiver is not modified.  Kern tables, glyph constructions and
// assemblies which are shared in info are shared in the result.
func (info *Info) MapValues(fn func(Value) Value) *Info {
	b := &mapper{
		fn:            fn,
		kerns:         map[*Kern]*Kern{},
		assemblies:    map[*Assembly]*Assembly{},
		constructions: map[*Construction]*Construction{},
	}

	res := *info
	for _, v := range res.Constants.values() {
		*v = fn(*v)
	}
	res.ItalicsCorrection = b.valueMap(info.ItalicsCorrection)
	res.TopAccent = b.valueMap(info.TopAccent)
	if info.Kerns != nil {
		res.Kerns = make(map[glyph.ID]*KernInfo, len(info.Kerns))
		for gid, ki := range info.Kerns {
			res.Kerns[gid] = &KernInfo{
				TopRight:    b.kern(ki.TopRight),
				TopLeft:     b.kern(ki.TopLeft),
				BottomRight: b.kern(ki.BottomRight),
				BottomLeft:  b.kern(ki.BottomLeft),
			}
		}
	}
	res.Vertical = b.constructionMap(info.Vertical)
	res.Horizontal = b.constructionMap(info.Horizontal)
	res.ExtendedShapes = maps.Clone(info.ExtendedShapes)
	return &res
}

// Values yields every Value in info: the constants, italics corrections,
// top accent attachments, kern heights and kerns, and the italics
// corrections of assemblies, in no particular order.  The values of a kern
// table or assembly which is shared within info are yielded once, so that
// the work done is proportional to the memory used by info.
func (info *Info) Values() iter.Seq[Value] {
	return func(yield func(Value) bool) {
		for _, v := range info.Constants.values() {
			if !yield(*v) {
				return
			}
		}
		for _, m := range [...]map[glyph.ID]Value{info.ItalicsCorrection, info.TopAccent} {
			for _, v := range m {
				if !yield(v) {
					return
				}
			}
		}
		seenKerns := map[*Kern]bool{}
		for _, ki := range info.Kerns {
			for _, k := range ki.corners() {
				if k == nil || seenKerns[k] {
					continue
				}
				seenKerns[k] = true
				for _, vv := range [...][]Value{k.Heights, k.Kerns} {
					for _, v := range vv {
						if !yield(v) {
							return
						}
					}
				}
			}
		}
		seenAssemblies := map[*Assembly]bool{}
		for _, m := range [...]map[glyph.ID]*Construction{info.Vertical, info.Horizontal} {
			for _, c := range m {
				a := c.Assembly
				if a == nil || seenAssemblies[a] {
					continue
				}
				seenAssemblies[a] = true
				if !yield(a.ItalicsCorrection) {
					return
				}
			}
		}
	}
}

// MapDevices returns a copy of info in which the device table t of every
// Value has been replaced by fn(t).  fn is called only for device tables
// which are present.  If fn returns nil, the device table is removed and
// the value is kept.
//
// The receiver is not modified.  Kern tables, glyph constructions and
// assemblies which are shared in info are shared in the result.
func (info *Info) MapDevices(fn func(*device.Table) *device.Table) *Info {
	return info.MapValues(func(v Value) Value {
		if v.Device != nil {
			v.Device = fn(v.Device)
		}
		return v
	})
}

// Devices yields the device tables of the values in info, in no particular
// order.  The device tables of a kern table or assembly which is shared
// within info are yielded once, so that the work done is proportional to
// the memory used by info.
func (info *Info) Devices() iter.Seq[*device.Table] {
	return func(yield func(*device.Table) bool) {
		for v := range info.Values() {
			if v.Device != nil && !yield(v.Device) {
				return
			}
		}
	}
}

type mapper struct {
	fn            func(Value) Value
	kerns         map[*Kern]*Kern
	assemblies    map[*Assembly]*Assembly
	constructions map[*Construction]*Construction
}

func (b *mapper) values(vv []Value) []Value {
	if vv == nil {
		return nil
	}
	res := make([]Value, len(vv))
	for i, v := range vv {
		res[i] = b.fn(v)
	}
	return res
}

func (b *mapper) valueMap(m map[glyph.ID]Value) map[glyph.ID]Value {
	if m == nil {
		return nil
	}
	res := make(map[glyph.ID]Value, len(m))
	for gid, v := range m {
		res[gid] = b.fn(v)
	}
	return res
}

func (b *mapper) kern(k *Kern) *Kern {
	if k == nil {
		return nil
	}
	if res, ok := b.kerns[k]; ok {
		return res
	}
	res := &Kern{Heights: b.values(k.Heights), Kerns: b.values(k.Kerns)}
	if !heightsSorted(res.Heights) {
		res = nil
	}
	b.kerns[k] = res
	return res
}

// heightsSorted reports whether the correction heights are in
// non-decreasing order, as required for writing.
func heightsSorted(heights []Value) bool {
	for i := 1; i < len(heights); i++ {
		if heights[i].V < heights[i-1].V {
			return false
		}
	}
	return true
}

func (b *mapper) assembly(a *Assembly) *Assembly {
	if a == nil {
		return nil
	}
	if res, ok := b.assemblies[a]; ok {
		return res
	}
	res := &Assembly{ItalicsCorrection: b.fn(a.ItalicsCorrection), Parts: a.Parts}
	b.assemblies[a] = res
	return res
}

func (b *mapper) constructionMap(m map[glyph.ID]*Construction) map[glyph.ID]*Construction {
	if m == nil {
		return nil
	}
	res := make(map[glyph.ID]*Construction, len(m))
	for gid, c := range m {
		cc, ok := b.constructions[c]
		if !ok {
			cc = &Construction{Variants: c.Variants, Assembly: b.assembly(c.Assembly)}
			b.constructions[c] = cc
		}
		res[gid] = cc
	}
	return res
}

// MapGlyphs returns a copy of info in which every glyph ID has been
// replaced by the result of m.  The function returns false for glyphs which
// are to be removed.  It must map distinct glyphs to distinct IDs.
//
// Entries whose glyph is removed are dropped.  Variants with a removed
// glyph are dropped from their list, an assembly with a removed part is
// dropped, and constructions left with neither variants nor assembly are
// dropped.  Empty maps and lists in the result are nil.
//
// The receiver is not modified.  Kern tables, glyph constructions and
// assemblies which are shared in info are shared in the result.
func (info *Info) MapGlyphs(m func(glyph.ID) (glyph.ID, bool)) *Info {
	res := *info
	res.ItalicsCorrection = mapKeys(info.ItalicsCorrection, m)
	res.TopAccent = mapKeys(info.TopAccent, m)
	res.Kerns = mapKeys(info.Kerns, m)

	res.ExtendedShapes = nil
	for gid, ok := range info.ExtendedShapes {
		if !ok {
			continue
		}
		if g, ok := m(gid); ok {
			if res.ExtendedShapes == nil {
				res.ExtendedShapes = coverage.Set{}
			}
			res.ExtendedShapes[g] = true
		}
	}

	asms := map[*Assembly]*Assembly{}
	cons := map[*Construction]*Construction{}
	mapCons := func(src map[glyph.ID]*Construction) map[glyph.ID]*Construction {
		var out map[glyph.ID]*Construction
		for gid, c := range src {
			g, ok := m(gid)
			if !ok || c == nil {
				continue
			}
			nc, seen := cons[c]
			if !seen {
				nc = mapConstruction(c, m, asms)
				cons[c] = nc
			}
			if nc == nil {
				continue
			}
			if out == nil {
				out = map[glyph.ID]*Construction{}
			}
			out[g] = nc
		}
		return out
	}
	res.Vertical = mapCons(info.Vertical)
	res.Horizontal = mapCons(info.Horizontal)
	return &res
}

// mapKeys renumbers the keys of m, dropping unmapped keys.  The result is
// nil if it would be empty.
func mapKeys[V any](src map[glyph.ID]V, m func(glyph.ID) (glyph.ID, bool)) map[glyph.ID]V {
	var out map[glyph.ID]V
	for gid, v := range src {
		g, ok := m(gid)
		if !ok {
			continue
		}
		if out == nil {
			out = map[glyph.ID]V{}
		}
		out[g] = v
	}
	return out
}

func mapConstruction(c *Construction, m func(glyph.ID) (glyph.ID, bool), asms map[*Assembly]*Assembly) *Construction {
	var variants []Variant
	for _, v := range c.Variants {
		if g, ok := m(v.Glyph); ok {
			variants = append(variants, Variant{Glyph: g, Advance: v.Advance})
		}
	}
	var asm *Assembly
	if a := c.Assembly; a != nil {
		var seen bool
		if asm, seen = asms[a]; !seen {
			asm = mapAssembly(a, m)
			asms[a] = asm
		}
	}
	if len(variants) == 0 && asm == nil {
		return nil
	}
	return &Construction{Variants: variants, Assembly: asm}
}

func mapAssembly(a *Assembly, m func(glyph.ID) (glyph.ID, bool)) *Assembly {
	parts := make([]Part, len(a.Parts))
	for i, p := range a.Parts {
		g, ok := m(p.Glyph)
		if !ok {
			return nil
		}
		p.Glyph = g
		parts[i] = p
	}
	return &Assembly{ItalicsCorrection: a.ItalicsCorrection, Parts: parts}
}
