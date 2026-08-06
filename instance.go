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

package sfnt

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/postscript/funit"

	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/fvar"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/mvar"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/os2"
	"seehuhn.de/go/sfnt/variation"
)

// ErrNotVariable is returned by [Font.Instantiate] when the font defines no
// variation axes.
var ErrNotVariable = errors.New("sfnt: font is not variable")

// Instantiate pins every axis of a variable font and returns a new static
// font.  coords maps axis tags to user-scale values; axes omitted from coords
// keep their default value, and an unknown tag causes an error.
//
// The receiver is left unmodified.  The returned font carries no variation
// tables: the glyph outlines, advance widths, "cvt " values, font-wide metrics
// and GPOS/GDEF device adjustments are all resolved to the pinned instance, and
// its PostScript name follows Adobe Technical Note #5902.
func (f *Font) Instantiate(coords map[string]float64) (*Font, error) {
	// step 1: guards
	if !f.IsVariable() {
		return nil, ErrNotVariable
	}
	if f.Avar != nil && !f.Avar.IsSupported() {
		return nil, errors.New("sfnt: avar version 2 not supported")
	}
	if (f.Gsub != nil && f.Gsub.VariationsRaw != nil) ||
		(f.Gpos != nil && f.Gpos.VariationsRaw != nil) {
		return nil, errors.New("sfnt: unresolvable feature variations")
	}
	switch f.Outlines.(type) {
	case *glyf.Outlines, *cff.OutlinesCFF2:
		// supported outline flavors
	default:
		// plain CFF outlines are not variable
		return nil, errors.New("sfnt: outlines are not variable")
	}

	// step 2: normalize the axis coordinates
	norm, err := f.Fvar.Normalize(coords)
	if err != nil {
		return nil, err
	}
	if f.Avar != nil {
		norm = f.Avar.Map(norm)
	}

	// effective clamped user-scale value per axis, for OS/2 classes and naming
	userValues := make([]float64, len(f.Fvar.Axes))
	for i, ax := range f.Fvar.Axes {
		v := ax.Default
		if c, ok := coords[ax.Tag]; ok {
			v = c
		}
		userValues[i] = clampFloat(v, ax.Min, ax.Max)
	}

	// step 3: the result is a copy; every structure we mutate is deep-copied
	// below so the receiver stays untouched.
	out := *f

	// steps 4–6: resolve the glyph outlines, advance widths and cvt values to
	// the pinned instance.  The flavor-specific arm sets out.Outlines; the
	// shared tail below (steps 7–12) is flavor-agnostic.
	switch o := f.Outlines.(type) {
	case *glyf.Outlines:
		if err := instanceGlyf(f, &out, o, norm); err != nil {
			return nil, err
		}
	case *cff.OutlinesCFF2:
		if err := instanceCFF2(f, &out, o, norm); err != nil {
			return nil, err
		}
	}

	// step 7: MVAR font-wide metrics
	applyMVAR(&out, f.Mvar, norm)

	// step 8: OS/2 weight and width classes
	for i, ax := range f.Fvar.Axes {
		switch ax.Tag {
		case "wght":
			out.Weight = os2.Weight(clampInt(int(math.Round(userValues[i])), 1, 1000))
		case "wdth":
			out.Width = widthClass(userValues[i])
		}
	}

	// steps 9 & 10: GSUB/GPOS feature variations and GPOS/GDEF device baking
	resolve := func(outer, inner uint16) funit.Int16 {
		if f.Gdef == nil || f.Gdef.ItemVarStore == nil {
			return 0
		}
		return funit.Int16(variation.OTRound(f.Gdef.ItemVarStore.Evaluate(outer, inner, norm)))
	}
	if f.Gsub != nil {
		c := *f.Gsub
		c.FeatureList = applyFeatureVariations(f.Gsub.FeatureList, f.Gsub.Variations, norm)
		c.Variations = nil
		out.Gsub = &c
	}
	if f.Gpos != nil {
		c := *f.Gpos
		c.FeatureList = applyFeatureVariations(f.Gpos.FeatureList, f.Gpos.Variations, norm)
		c.Variations = nil
		c.LookupList = f.Gpos.LookupList.BakeVariationDevices(resolve)
		out.Gpos = &c
	}
	if f.Gdef != nil {
		g := f.Gdef.BakeVariationDevices(resolve)
		g.ItemVarStore = nil
		out.Gdef = g
	}

	// step 11: drop the variation tables
	out.Fvar = nil
	out.Avar = nil
	out.Stat = nil
	out.Gvar = nil
	out.Cvar = nil
	out.Hvar = nil
	out.Mvar = nil

	// step 12: the names of the pinned instance.  A named instance supplies
	// both its PostScript name (Adobe TN #5902) and its style name; anywhere
	// else in the design space the font has no name for its style, and the
	// names of the variable font it came from do not describe it.
	inst := namedInstance(f, userValues)
	out.FontName = instanceName(f, inst, userValues, out.MaxFontNameLen())
	out.VariationsPostScriptName = ""
	out.Subfamily = ""
	out.FullName = ""
	if inst != nil {
		out.Subfamily = inst.Name
		out.FullName = fullName(f.FamilyName, inst.Name)
	}

	return &out, nil
}

// ConvertCFF2 returns a copy of the font with its CFF2 glyph outlines
// converted to static CFF outlines.  For a variable font this pins all
// axes at their default values (like Instantiate with no coordinates);
// use Instantiate to select other coordinates.  ConvertCFF2 returns an
// error if the font does not use CFF2 outlines.
func (f *Font) ConvertCFF2() (*Font, error) {
	o, ok := f.Outlines.(*cff.OutlinesCFF2)
	if !ok {
		return nil, errors.New("sfnt: font does not use CFF2 outlines")
	}
	if f.IsVariable() {
		return f.Instantiate(nil)
	}

	// static font: no axes to pin, so metrics and naming stay untouched.
	out := *f
	if err := instanceCFF2(f, &out, o, nil); err != nil {
		return nil, err
	}
	return &out, nil
}

// instanceGlyf resolves the TrueType glyph outlines, advance widths and cvt
// values of out to the instance at norm (steps 4–6 of Instantiate).
func instanceGlyf(f *Font, out *Font, outlines *glyf.Outlines, norm []variation.F2Dot14) error {
	// step 4: instance the glyph outlines
	numGlyphs := len(outlines.Glyphs)
	newGlyphs := make(glyf.Glyphs, numGlyphs)
	phantomAdvances := make([]funit.Uint16, numGlyphs)

	if f.Gvar != nil {
		var totalGvar int64
		for _, gd := range f.Gvar.PerGlyph {
			totalGvar += int64(len(gd.Data))
		}
		workBudget := min(int64(64)*totalGvar+(1<<20), 1<<28)

		for gid := range numGlyphs {
			var gvarLen int64
			if gid < len(f.Gvar.PerGlyph) {
				gvarLen = int64(len(f.Gvar.PerGlyph[gid].Data))
			}
			// A fresh per-glyph budget bounds one glyph's scratch (proportional to
			// its point count and gvar block); the cumulative work budget bounds
			// total CPU across all glyphs.
			budget := membudget.New(int64(1<<24) + 64*gvarLen)
			res, err := f.Gvar.Apply(outlines.Glyphs, outlines.Widths, glyph.ID(gid), norm, budget, &workBudget)
			if err != nil {
				return err
			}
			newGlyphs[gid] = res.Glyph
			phantomAdvances[gid] = res.Advance
		}
		recomputeCompositeBBoxes(newGlyphs)
	} else {
		// no gvar table: glyph outlines and advances are unaffected by the
		// variation coordinates.
		copy(newGlyphs, outlines.Glyphs)
		copy(phantomAdvances, outlines.Widths)
	}

	newOutlines := *outlines
	newOutlines.Glyphs = newGlyphs

	// step 5: advance widths (HVAR takes precedence over phantom advances)
	if f.Hvar != nil || outlines.Widths != nil {
		newWidths := make([]funit.Uint16, numGlyphs)
		for gid := range numGlyphs {
			if f.Hvar != nil {
				var base float64
				if gid < len(outlines.Widths) {
					base = float64(outlines.Widths[gid])
				}
				newWidths[gid] = clampUint16(variation.OTRound(base + f.Hvar.AdvanceDelta(glyph.ID(gid), norm)))
			} else {
				newWidths[gid] = phantomAdvances[gid]
			}
		}
		newOutlines.Widths = newWidths
	}

	// step 6: cvt via cvar
	if f.Cvar != nil {
		if cvt, ok := outlines.Tables["cvt "]; ok {
			tables := maps.Clone(outlines.Tables)
			tables["cvt "] = f.Cvar.Apply(cvt, norm)
			newOutlines.Tables = tables
		}
	}
	out.Outlines = &newOutlines
	return nil
}

// instanceCFF2 resolves the CFF2 glyph outlines and advance widths of out to
// the instance at norm (steps 4–6 of Instantiate), producing static CID-keyed
// CFF outlines.  The top-level font matrix (out.FontMatrix, copied from the
// receiver) is unchanged, so the rendering transform matches the CFF2 font.
func instanceCFF2(f *Font, out *Font, outlines *cff.OutlinesCFF2, norm []variation.F2Dot14) error {
	// advance widths in design units; HVAR overrides the static widths when
	// present.  outlines.Widths is already in design units (read.go divides
	// the hmtx width by GlyphAdvanceScale(gid)*UnitsPerEm), but HVAR deltas
	// are in raw hmtx/UnitsPerEm units, so the delta needs the same
	// conversion before it can be added to the design-unit base.
	var widths []float64
	if f.Hvar != nil {
		widths = make([]float64, len(outlines.Glyphs))
		upm := float64(f.UnitsPerEm)
		for gid := range widths {
			var base float64
			if gid < len(outlines.Widths) {
				base = outlines.Widths[gid]
			}
			delta := f.Hvar.AdvanceDelta(glyph.ID(gid), norm)
			if d := outlines.GlyphAdvanceScale(f.FontMatrix, glyph.ID(gid)) * upm; d != 0 {
				delta /= d
			}
			widths[gid] = variation.OTRound(base + delta)
		}
	}

	static, err := outlines.Instance(norm, widths)
	if err != nil {
		return err
	}
	out.Outlines = static
	return nil
}

// applyFeatureVariations returns feats with the substitutions of the first
// matching variation record applied.  Features named by the winning record get
// their lookup lists replaced wholesale; every other feature is shared with
// feats.  If no record matches (or there are none), feats is returned as is.
func applyFeatureVariations(feats gtab.FeatureListInfo, vars []gtab.FeatureVariationRecord, norm []variation.F2Dot14) gtab.FeatureListInfo {
	if len(vars) == 0 {
		return feats
	}
	var rec *gtab.FeatureVariationRecord
	for i := range vars {
		if vars[i].Matches(norm) {
			rec = &vars[i]
			break
		}
	}
	if rec == nil {
		return feats
	}
	out := slices.Clone(feats)
	for _, sub := range rec.Substitutions {
		idx := int(sub.FeatureIndex)
		if idx < 0 || idx >= len(out) {
			continue
		}
		feat := *out[idx]
		feat.Lookups = slices.Clone(sub.Lookups)
		out[idx] = &feat
	}
	return out
}

// applyMVAR folds the supported MVAR deltas into the metric fields of out.
func applyMVAR(out *Font, mv *mvar.Table, norm []variation.F2Dot14) {
	if mv == nil {
		return
	}
	int16Field := func(field *funit.Int16, tag string) {
		if d, ok := mv.Delta(tag, norm); ok {
			*field = clampInt16(variation.OTRound(float64(*field) + d))
		}
	}
	floatField := func(field *funit.Float64, tag string) {
		if d, ok := mv.Delta(tag, norm); ok {
			*field = funit.Float64(float64(*field) + d)
		}
	}
	int16Field(&out.Ascent, "hasc")
	int16Field(&out.Descent, "hdsc")
	int16Field(&out.LineGap, "hlgp")
	int16Field(&out.CapHeight, "cpht")
	int16Field(&out.XHeight, "xhgt")
	floatField(&out.UnderlinePosition, "undo")
	floatField(&out.UnderlineThickness, "unds")
}

// widthClassTable maps the wdth axis percentage to an OS/2 usWidthClass value.
var widthClassTable = [...]struct {
	pct float64
	cls os2.Width
}{
	{50, 1}, {62.5, 2}, {75, 3}, {87.5, 4}, {100, 5},
	{112.5, 6}, {125, 7}, {150, 8}, {200, 9},
}

// widthClass returns the OS/2 usWidthClass nearest to the wdth percentage pct.
// Ties resolve toward the smaller (narrower) class.
func widthClass(pct float64) os2.Width {
	best := widthClassTable[0].cls
	bestDiff := math.Abs(pct - widthClassTable[0].pct)
	for _, e := range widthClassTable[1:] {
		if d := math.Abs(pct - e.pct); d < bestDiff {
			bestDiff = d
			best = e.cls
		}
	}
	return best
}

// namedInstance returns the named instance sitting at the given user
// coordinates, or nil if the font has none there.
func namedInstance(f *Font, userValues []float64) *fvar.Instance {
	for i := range f.Fvar.Instances {
		inst := &f.Fvar.Instances[i]
		if len(inst.Coordinates) != len(userValues) {
			continue
		}
		match := true
		for j, c := range inst.Coordinates {
			if math.Abs(c-userValues[j]) > 1.0/65536 {
				match = false
				break
			}
		}
		if match {
			return inst
		}
	}
	return nil
}

// instanceName derives the PostScript name of the instance following Adobe
// Technical Note #5902.  inst is the named instance at these coordinates, or
// nil if there is none, and maxLen is the greatest number of bytes the
// instanced font can carry.
//
// Every part of a generated name is confined to the characters the "name"
// table allows, so the result is always a name the instanced font can be
// written with.
func instanceName(f *Font, inst *fvar.Instance, userValues []float64, maxLen int) string {
	// a named instance whose coordinates match exactly wins
	if inst != nil && inst.PostScriptName != "" {
		return inst.PostScriptName
	}

	// generated name: prefix + one "_<value><tag>" group per axis in fvar order.
	// The prefix is filtered here rather than only as part of the assembled
	// name, because the length fallback below slices it on its own and a raw
	// prefix would put the removed characters back.
	prefix := repairVariationsName(f.VariationsPostScriptName)
	if prefix == "" {
		// no prefix of its own: fall back to the family name, stripped down to
		// the same character set
		prefix = keepAlnum(f.FamilyName)
	}
	tags := axisTagNames(f.Fvar.Axes)
	var b strings.Builder
	b.WriteString(prefix)
	for i := range f.Fvar.Axes {
		b.WriteByte('_')
		b.WriteString(renderFixed(userValues[i]))
		b.WriteString(tags[i])
	}
	name := b.String()

	// keep PostScript names within the length limit; over the limit we fall
	// back to a deterministic hash, which TN #5902 permits as an
	// implementation-defined last resort.
	if len(name) > maxLen {
		sum := sha256.Sum256([]byte(name))
		suffix := "-" + hex.EncodeToString(sum[:])[:8]
		// the prefix is ASCII letters and digits, so a byte is a character
		keep := min(max(maxLen-len(suffix), 0), len(prefix))
		name = prefix[:keep] + suffix
	}
	return name
}

// axisTagNames returns the strings standing for the axis tags in a generated
// instance name.
//
// A tag is used as it stands only where it already consists of ASCII letters
// and digits.  Removing the offending characters from the others would not do,
// because two different tags can reduce to the same string and the name would
// then no longer tell the axes apart; a substitute built from the position of
// the axis is used instead.  A substitute which clashes with a tag the font
// already uses is lengthened until it does not.
func axisTagNames(axes []fvar.Axis) []string {
	names := make([]string, len(axes))
	used := make(map[string]bool)
	for i, ax := range axes {
		if isAlnum(ax.Tag) {
			names[i] = ax.Tag
			used[ax.Tag] = true
		}
	}
	for i := range axes {
		if names[i] != "" {
			continue
		}
		name := "X" + strconv.Itoa(i)
		for used[name] {
			name = "X" + name
		}
		names[i] = name
		used[name] = true
	}
	return names
}

// fullName builds the full name of a font from its family and style names.  A
// style of "Regular" tells the reader nothing the family name does not already
// say, so it is left out.
func fullName(family, subfamily string) string {
	if family == "" {
		return subfamily
	}
	if subfamily == "" || subfamily == "Regular" {
		return family
	}
	return family + " " + subfamily
}

// renderFixed renders a user-scale axis value as a minimal decimal, rounding to
// the 16.16 grid, without trailing zeros and without a point for integers.
func renderFixed(v float64) string {
	fixed := int64(math.Round(v * 65536))
	if fixed%65536 == 0 {
		return strconv.FormatInt(fixed/65536, 10)
	}
	return strconv.FormatFloat(float64(fixed)/65536, 'f', -1, 64)
}

// recomputeCompositeBBoxes updates the header bounding box of every composite
// glyph from its instanced child outlines (gvar.Apply leaves them unchanged).
func recomputeCompositeBBoxes(glyphs glyf.Glyphs) {
	for gid := range glyphs {
		g := glyphs[gid]
		if g == nil {
			continue
		}
		if _, ok := g.Data.(glyf.CompositeGlyph); !ok {
			continue
		}
		if bbox, ok := compositeBBox(glyphs, glyph.ID(gid), make(map[glyph.ID]bool), 0); ok {
			g.Rect16 = bbox
		}
	}
}

const maxCompositeDepth = 64

// compositeBBox returns the bounding box of glyph gid, resolving composite
// components against their instanced children.  seen and depth guard against
// cyclic or over-deep composite references in malformed fonts.
func compositeBBox(glyphs glyf.Glyphs, gid glyph.ID, seen map[glyph.ID]bool, depth int) (funit.Rect16, bool) {
	if int(gid) >= len(glyphs) {
		return funit.Rect16{}, false
	}
	g := glyphs[gid]
	if g == nil {
		return funit.Rect16{}, false
	}
	cg, ok := g.Data.(glyf.CompositeGlyph)
	if !ok {
		return g.Rect16, true // simple glyph: bbox already recomputed
	}
	if depth >= maxCompositeDepth || seen[gid] {
		return g.Rect16, true // cycle or too deep: keep the existing box
	}
	seen[gid] = true
	defer delete(seen, gid)

	var res funit.Rect16
	first := true
	for _, comp := range cg.Components {
		cu, err := comp.Unpack()
		if err != nil {
			continue
		}
		childBBox, ok := compositeBBox(glyphs, cu.Child, seen, depth+1)
		if !ok {
			continue
		}
		b := transformRect(cu, childBBox)
		if first {
			res = b
			first = false
		} else {
			res.LLx = min(res.LLx, b.LLx)
			res.LLy = min(res.LLy, b.LLy)
			res.URx = max(res.URx, b.URx)
			res.URy = max(res.URy, b.URy)
		}
	}
	if first {
		return g.Rect16, true // no usable child: keep the existing box
	}
	return res, true
}

// transformRect applies a component's transform to the corners of a child
// bounding box and returns their axis-aligned bounding box.  Point-matching
// offsets are not resolved; the transform's translation is used as stored.
func transformRect(cu *glyf.ComponentUnpacked, r funit.Rect16) funit.Rect16 {
	m := cu.Trfm
	corners := [4][2]float64{
		{float64(r.LLx), float64(r.LLy)},
		{float64(r.URx), float64(r.LLy)},
		{float64(r.URx), float64(r.URy)},
		{float64(r.LLx), float64(r.URy)},
	}
	var minX, minY, maxX, maxY float64
	for i, c := range corners {
		x := m[0]*c[0] + m[2]*c[1] + m[4]
		y := m[1]*c[0] + m[3]*c[1] + m[5]
		if i == 0 {
			minX, maxX, minY, maxY = x, x, y, y
			continue
		}
		minX = math.Min(minX, x)
		maxX = math.Max(maxX, x)
		minY = math.Min(minY, y)
		maxY = math.Max(maxY, y)
	}
	return funit.Rect16{
		LLx: clampInt16(math.Round(minX)),
		LLy: clampInt16(math.Round(minY)),
		URx: clampInt16(math.Round(maxX)),
		URy: clampInt16(math.Round(maxY)),
	}
}

func clampFloat(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(v, hi))
}

func clampInt(v, lo, hi int) int {
	return max(lo, min(v, hi))
}

func clampInt16(v float64) funit.Int16 {
	if v > math.MaxInt16 {
		return math.MaxInt16
	}
	if v < math.MinInt16 {
		return math.MinInt16
	}
	return funit.Int16(v)
}

func clampUint16(v float64) funit.Uint16 {
	if v > math.MaxUint16 {
		return math.MaxUint16
	}
	if v < 0 {
		return 0
	}
	return funit.Uint16(v)
}
