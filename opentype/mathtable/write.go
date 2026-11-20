// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package mathtable

import (
	"encoding/binary"
	"errors"
	"fmt"
	"maps"
	"slices"

	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/opentype/internal/offsetpool"
)

// Encode converts the MATH table to its binary form.
//
// An error is returned if the data cannot be represented in the file
// format: a kern table where the number of kerns is not one more than the
// number of heights, or where the heights are not in non-decreasing order; a
// glyph construction without variants and assembly; an assembly without
// parts; an invalid device table; a list with more than 65535 entries; or
// subtables which do not fit the 16-bit offsets.
func (info *Info) Encode() ([]byte, error) {
	body := make([]byte, headerSize)
	binary.BigEndian.PutUint16(body, 1) // major version
	pl := newPlacer("MATH", headerSize)

	enc, err := info.Constants.encode()
	if err != nil {
		return nil, err
	}
	err = pl.add(4, enc)
	if err != nil {
		return nil, err
	}
	enc, err = info.encodeGlyphInfo()
	if err != nil {
		return nil, err
	}
	err = pl.add(6, enc)
	if err != nil {
		return nil, err
	}
	enc, err = info.encodeVariants()
	if err != nil {
		return nil, err
	}
	err = pl.add(8, enc)
	if err != nil {
		return nil, err
	}
	return pl.finish(body), nil
}

// placer collects the subtables of a table and fills in the Offset16 fields
// pointing to them.
type placer struct {
	pool    *offsetpool.Deferred
	patches []patch
}

type patch struct {
	pos int // position of the offset field
	idx int // index of the subtable
}

// newPlacer returns a placer for a table where the subtables follow a body
// of length base.
func newPlacer(name string, base int) *placer {
	return &placer{pool: offsetpool.NewDeferred(name, base)}
}

// add adds the subtable enc, to be pointed to from the offset field at
// body position pos.
func (pl *placer) add(pos int, enc []byte) error {
	idx, err := pl.pool.Add(enc)
	if err != nil {
		return err
	}
	pl.patches = append(pl.patches, patch{pos, idx})
	return nil
}

// addFunc is like add, but the subtable is only encoded if no subtable with
// the same key was added before.
func (pl *placer) addFunc(pos int, key any, encode func() ([]byte, error)) error {
	idx, err := pl.pool.AddFunc(key, encode)
	if err != nil {
		return err
	}
	pl.patches = append(pl.patches, patch{pos, idx})
	return nil
}

// finish stores the subtables after body and fills in the offsets.
func (pl *placer) finish(body []byte) []byte {
	offs, data := pl.pool.Place()
	for _, pt := range pl.patches {
		binary.BigEndian.PutUint16(body[pt.pos:], offs[pt.idx])
	}
	return append(body, data...)
}

// value stores a MathValueRecord at body position pos.  Device tables are
// stored as subtables.
func (pl *placer) value(body []byte, pos int, v Value) error {
	binary.BigEndian.PutUint16(body[pos:], uint16(v.V))
	if v.Device == nil {
		return nil
	}
	return pl.addFunc(pos+2, v.Device, func() ([]byte, error) {
		return encodeDevice(v.Device)
	})
}

func encodeDevice(t *device.Table) ([]byte, error) {
	switch t.DeltaFormat {
	case device.VariationIndexFormat:
	case 1, 2, 3:
		if t.EndSize < t.StartSize || len(t.Deltas) != int(t.EndSize)-int(t.StartSize)+1 {
			return nil, errors.New("mathtable: wrong number of device deltas")
		}
		bits := 1 << t.DeltaFormat
		lo, hi := -(1 << (bits - 1)), 1<<(bits-1)-1
		for _, d := range t.Deltas {
			if int(d) < lo || int(d) > hi {
				return nil, errors.New("mathtable: device delta out of range")
			}
		}
	default:
		return nil, fmt.Errorf("mathtable: invalid device delta format 0x%04x", t.DeltaFormat)
	}
	return t.Encode(), nil
}

func checkCount(what string, n int) error {
	if n > 0xFFFF {
		return fmt.Errorf("mathtable: too many %s (%d)", what, n)
	}
	return nil
}

func (c *Constants) encode() ([]byte, error) {
	body := make([]byte, constantsSize)
	binary.BigEndian.PutUint16(body, uint16(c.ScriptPercentScaleDown))
	binary.BigEndian.PutUint16(body[2:], uint16(c.ScriptScriptPercentScaleDown))
	binary.BigEndian.PutUint16(body[4:], uint16(c.DelimitedSubFormulaMinHeight))
	binary.BigEndian.PutUint16(body[6:], uint16(c.DisplayOperatorMinHeight))
	binary.BigEndian.PutUint16(body[constantsSize-2:], uint16(c.RadicalDegreeBottomRaisePercent))

	pl := newPlacer("MathConstants", constantsSize)
	for i, v := range c.values() {
		err := pl.value(body, 8+4*i, *v)
		if err != nil {
			return nil, err
		}
	}
	return pl.finish(body), nil
}

// coverageOf returns the coverage table for the given glyphs, which must be
// in increasing order.
func coverageOf(gids []glyph.ID) coverage.Table {
	cov := make(coverage.Table, len(gids))
	for i, gid := range gids {
		cov[gid] = i
	}
	return cov
}

func (info *Info) encodeGlyphInfo() ([]byte, error) {
	body := make([]byte, 8)
	pl := newPlacer("MathGlyphInfo", len(body))

	enc, err := encodeValueMap("italics corrections", info.ItalicsCorrection)
	if err != nil {
		return nil, err
	}
	err = pl.add(0, enc)
	if err != nil {
		return nil, err
	}
	enc, err = encodeValueMap("top accent attachments", info.TopAccent)
	if err != nil {
		return nil, err
	}
	err = pl.add(2, enc)
	if err != nil {
		return nil, err
	}

	var shapes []glyph.ID
	for gid, ok := range info.ExtendedShapes {
		if ok {
			shapes = append(shapes, gid)
		}
	}
	slices.Sort(shapes)
	err = pl.add(4, coverageOf(shapes).Encode())
	if err != nil {
		return nil, err
	}

	enc, err = encodeKernInfo(info.Kerns)
	if err != nil {
		return nil, err
	}
	err = pl.add(6, enc)
	if err != nil {
		return nil, err
	}
	return pl.finish(body), nil
}

// encodeValueMap encodes a MathItalicsCorrectionInfo or
// MathTopAccentAttachment table.
func encodeValueMap(what string, m map[glyph.ID]Value) ([]byte, error) {
	if err := checkCount(what, len(m)); err != nil {
		return nil, err
	}
	gids := slices.Sorted(maps.Keys(m))
	body := make([]byte, 4+4*len(gids))
	binary.BigEndian.PutUint16(body[2:], uint16(len(gids)))
	pl := newPlacer("MathValueMap", len(body))
	err := pl.add(0, coverageOf(gids).Encode())
	if err != nil {
		return nil, err
	}
	for i, gid := range gids {
		err = pl.value(body, 4+4*i, m[gid])
		if err != nil {
			return nil, err
		}
	}
	return pl.finish(body), nil
}

// tbl is a table whose Offset16 fields point into the next layer.
type tbl struct {
	body []byte
	refs []ref
}

// ref is an Offset16 field at body position pos, pointing to item target of
// the next layer.
type ref struct {
	pos, target int
}

// content identifies the table by its bytes and the items it points to.
func (t *tbl) content() string {
	b := binary.BigEndian.AppendUint64(nil, uint64(len(t.body)))
	b = append(b, t.body...)
	for _, r := range t.refs {
		b = binary.BigEndian.AppendUint32(b, uint32(r.pos))
		b = binary.BigEndian.AppendUint32(b, uint32(r.target))
	}
	return string(b)
}

// layer is a list of tables which are stored one after the other.  Tables
// in a layer may be pointed to from several tables of the previous layer,
// which allows the children of different parents to be shared.
type layer struct {
	items     []*tbl
	byKey     map[any]int
	byContent map[string]int
}

// addFunc adds the table built by build, unless a table with the same key
// or the same content has been added before.  It returns the index of the
// table.  A nil key disables the lookup by key.
func (l *layer) addFunc(key any, build func() (*tbl, error)) (int, error) {
	if i, ok := l.byKey[key]; ok && key != nil {
		return i, nil
	}
	t, err := build()
	if err != nil {
		return 0, err
	}
	if l.byKey == nil {
		l.byKey = map[any]int{}
		l.byContent = map[string]int{}
	}
	c := t.content()
	i, ok := l.byContent[c]
	if !ok {
		i = len(l.items)
		l.items = append(l.items, t)
		l.byContent[c] = i
	}
	if key != nil {
		l.byKey[key] = i
	}
	return i, nil
}

// device adds a device table to the layer.
func (l *layer) device(d *device.Table) (int, error) {
	return l.addFunc(d, func() (*tbl, error) {
		enc, err := encodeDevice(d)
		return &tbl{body: enc}, err
	})
}

// putValue stores a MathValueRecord at body position pos.  Device tables
// are added to devs.
func (t *tbl) putValue(pos int, v Value, devs *layer) error {
	binary.BigEndian.PutUint16(t.body[pos:], uint16(v.V))
	if v.Device == nil {
		return nil
	}
	idx, err := devs.device(v.Device)
	if err != nil {
		return err
	}
	t.refs = append(t.refs, ref{pos + 2, idx})
	return nil
}

// layout stores the layers one after the other, starting with the single
// table of the first layer, and fills in the offsets.  Offsets only point
// forward, so the order inside a layer is free; below the first layer the
// tables are stored in order of increasing size, which keeps large tables
// from pushing the start of later ones beyond reach.  An error is returned
// if an offset exceeds 0xFFFF.
func layout(layers ...*layer) ([]byte, error) {
	order := make([][]int, len(layers)) // tables of each layer in storage order
	starts := make([][]int, len(layers))
	total := 0
	for i, l := range layers {
		order[i] = make([]int, len(l.items))
		for j := range order[i] {
			order[i][j] = j
		}
		if i > 0 {
			slices.SortStableFunc(order[i], func(a, b int) int {
				return len(l.items[a].body) - len(l.items[b].body)
			})
		}
		starts[i] = make([]int, len(l.items))
		for _, j := range order[i] {
			starts[i][j] = total
			total += len(l.items[j].body)
		}
	}
	data := make([]byte, 0, total)
	for i, l := range layers {
		for _, j := range order[i] {
			data = append(data, l.items[j].body...)
		}
	}
	for i, l := range layers {
		for j, t := range l.items {
			for _, r := range t.refs {
				off := starts[i+1][r.target] - starts[i][j]
				if off > 0xFFFF {
					return nil, fmt.Errorf("mathtable: offset %d out of range", off)
				}
				binary.BigEndian.PutUint16(data[starts[i][j]+r.pos:], uint16(off))
			}
		}
	}
	return data, nil
}

func encodeKernInfo(m map[glyph.ID]*KernInfo) ([]byte, error) {
	if err := checkCount("kern records", len(m)); err != nil {
		return nil, err
	}
	gids := slices.Sorted(maps.Keys(m))
	root := &tbl{body: make([]byte, 4+8*len(gids))}
	binary.BigEndian.PutUint16(root.body[2:], uint16(len(gids)))
	var kerns, devs layer
	cov, err := kerns.addFunc(nil, func() (*tbl, error) {
		return &tbl{body: coverageOf(gids).Encode()}, nil
	})
	if err != nil {
		return nil, err
	}
	root.refs = append(root.refs, ref{0, cov})
	for i, gid := range gids {
		for j, k := range m[gid].corners() {
			if k == nil {
				continue
			}
			idx, err := kerns.addFunc(k, func() (*tbl, error) {
				return k.build(&devs)
			})
			if err != nil {
				return nil, err
			}
			root.refs = append(root.refs, ref{4 + 8*i + 2*j, idx})
		}
	}
	return layout(&layer{items: []*tbl{root}}, &kerns, &devs)
}

func (k *Kern) build(devs *layer) (*tbl, error) {
	h := len(k.Heights)
	if err := checkCount("kern heights", h); err != nil {
		return nil, err
	}
	if len(k.Kerns) != h+1 {
		return nil, errors.New("mathtable: wrong number of kern values")
	}
	if !heightsSorted(k.Heights) {
		return nil, errors.New("mathtable: kern heights not in non-decreasing order")
	}

	t := &tbl{body: make([]byte, 2+4*h+4*(h+1))}
	binary.BigEndian.PutUint16(t.body, uint16(h))
	for i, v := range k.Heights {
		if err := t.putValue(2+4*i, v, devs); err != nil {
			return nil, err
		}
	}
	for i, v := range k.Kerns {
		if err := t.putValue(2+4*h+4*i, v, devs); err != nil {
			return nil, err
		}
	}
	return t, nil
}

func (info *Info) encodeVariants() ([]byte, error) {
	if err := checkCount("vertical constructions", len(info.Vertical)); err != nil {
		return nil, err
	}
	if err := checkCount("horizontal constructions", len(info.Horizontal)); err != nil {
		return nil, err
	}
	vert := slices.Sorted(maps.Keys(info.Vertical))
	horiz := slices.Sorted(maps.Keys(info.Horizontal))

	root := &tbl{body: make([]byte, 10+2*(len(vert)+len(horiz)))}
	binary.BigEndian.PutUint16(root.body, uint16(info.MinConnectorOverlap))
	binary.BigEndian.PutUint16(root.body[6:], uint16(len(vert)))
	binary.BigEndian.PutUint16(root.body[8:], uint16(len(horiz)))

	var constructions, assemblies, devs layer
	for i, gids := range [][]glyph.ID{vert, horiz} {
		cov, err := constructions.addFunc(nil, func() (*tbl, error) {
			return &tbl{body: coverageOf(gids).Encode()}, nil
		})
		if err != nil {
			return nil, err
		}
		root.refs = append(root.refs, ref{2 + 2*i, cov})
	}
	pos := 10
	for _, dir := range []struct {
		gids []glyph.ID
		m    map[glyph.ID]*Construction
	}{{vert, info.Vertical}, {horiz, info.Horizontal}} {
		for _, gid := range dir.gids {
			c := dir.m[gid]
			idx, err := constructions.addFunc(c, func() (*tbl, error) {
				return c.build(&assemblies, &devs)
			})
			if err != nil {
				return nil, err
			}
			root.refs = append(root.refs, ref{pos, idx})
			pos += 2
		}
	}
	return layout(&layer{items: []*tbl{root}}, &constructions, &assemblies, &devs)
}

func (c *Construction) build(assemblies, devs *layer) (*tbl, error) {
	if len(c.Variants) == 0 && c.Assembly == nil {
		return nil, errors.New("mathtable: empty glyph construction")
	}
	if err := checkCount("glyph variants", len(c.Variants)); err != nil {
		return nil, err
	}
	t := &tbl{body: make([]byte, 4+4*len(c.Variants))}
	binary.BigEndian.PutUint16(t.body[2:], uint16(len(c.Variants)))
	for i, v := range c.Variants {
		binary.BigEndian.PutUint16(t.body[4+4*i:], uint16(v.Glyph))
		binary.BigEndian.PutUint16(t.body[6+4*i:], uint16(v.Advance))
	}
	if a := c.Assembly; a != nil {
		idx, err := assemblies.addFunc(a, func() (*tbl, error) {
			return a.build(devs)
		})
		if err != nil {
			return nil, err
		}
		t.refs = append(t.refs, ref{0, idx})
	}
	return t, nil
}

func (a *Assembly) build(devs *layer) (*tbl, error) {
	if len(a.Parts) == 0 {
		return nil, errors.New("mathtable: glyph assembly without parts")
	}
	if err := checkCount("assembly parts", len(a.Parts)); err != nil {
		return nil, err
	}
	t := &tbl{body: make([]byte, 6+10*len(a.Parts))}
	binary.BigEndian.PutUint16(t.body[4:], uint16(len(a.Parts)))
	for i, part := range a.Parts {
		b := t.body[6+10*i:]
		binary.BigEndian.PutUint16(b, uint16(part.Glyph))
		binary.BigEndian.PutUint16(b[2:], uint16(part.StartConnector))
		binary.BigEndian.PutUint16(b[4:], uint16(part.EndConnector))
		binary.BigEndian.PutUint16(b[6:], uint16(part.FullAdvance))
		if part.Extender {
			b[9] = 1
		}
	}
	if err := t.putValue(0, a.ItalicsCorrection, devs); err != nil {
		return nil, err
	}
	return t, nil
}
