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
	"cmp"
	"errors"
	"fmt"
	"slices"

	"seehuhn.de/go/postscript/type1"
)

// maxStemSnapEntries is the number of values the stem snap arrays may hold.
const maxStemSnapEntries = 12

// blueScaleForWriting returns the BlueScale to store for v.
//
// Zero is accepted as a shorthand for the default, so that a caller who leaves
// the field alone gets a usable font.  Any other value outside the range a
// font can hold is refused rather than repaired: values off a file have been
// put right on the read side already, so one reaching here came from the
// caller.
func blueScaleForWriting(v float64) (float64, error) {
	if v == 0 {
		return type1.DefaultBlueScale, nil
	}
	if type1.RepairBlueScale(v) != v {
		return 0, fmt.Errorf("cff: BlueScale %v outside (0, %v]", v, type1.MaxBlueScale)
	}
	return v, nil
}

// checkPrivateScalars reports an error for a non-blendable CFF2 scalar a font
// cannot hold.  The stored value is the whole value for these, since a blend
// on them is refused, so the rule which applies to a static CFF applies here
// too.  The blendable scalars are left out: there the constraint holds per
// instance rather than on the stored value.
func checkPrivateScalars(p *PrivateCFF2) error {
	for _, e := range []struct {
		name string
		val  float64
	}{{"BlueShift", p.BlueShift.Default}, {"BlueFuzz", p.BlueFuzz.Default}} {
		if !type1.UsableBlueDistance(e.val) {
			return fmt.Errorf("cff: %s %v is not a usable distance", e.name, e.val)
		}
	}
	return nil
}

// checkPrivateArrays reports an error for a CFF2 array entry a font cannot
// hold.  The zone arrays are judged on their stored values, which are the
// values at the default instance; the stem snap arrays only on their length,
// since the format says their order carries no meaning.
func checkPrivateArrays(p *PrivateCFF2) error {
	zones := []struct {
		name     string
		vals     []Blend
		maxPairs int
	}{
		{"BlueValues", p.BlueValues, type1.MaxBlueValuePairs},
		{"OtherBlues", p.OtherBlues, type1.MaxOtherBluePairs},
		{"FamilyBlues", p.FamilyBlues, type1.MaxBlueValuePairs},
		{"FamilyOtherBlues", p.FamilyOtherBlues, type1.MaxOtherBluePairs},
	}
	for _, z := range zones {
		if n := type1.UsableZones(blendDefaults(z.vals), z.maxPairs); n != len(z.vals) {
			return fmt.Errorf("cff: %s: %d of %d values usable", z.name, n, len(z.vals))
		}
	}

	for _, snap := range []struct {
		name string
		vals []Blend
	}{{"StemSnapH", p.StemSnapH}, {"StemSnapV", p.StemSnapV}} {
		if len(snap.vals) > maxStemSnapEntries {
			return fmt.Errorf("cff: %s has %d values, at most %d allowed",
				snap.name, len(snap.vals), maxStemSnapEntries)
		}
	}
	return nil
}

// errBlendNotAllowed reports a CFF2 Private DICT operand which carries
// variation deltas although its operator cannot be blended.
var errBlendNotAllowed = errors.New("cff: blend on a non-blendable Private DICT operand")

// The CFF2 Private DICT marks each operator as blendable or not.  BlueValues,
// OtherBlues, StdHW, StdVW, StemSnapH and StemSnapV may vary across the design
// space; the rest are plain values which must be the same for every instance.
func nonBlendableScalars(p *PrivateCFF2) []*Blend {
	return []*Blend{&p.BlueScale, &p.BlueShift, &p.BlueFuzz, &p.ExpansionFactor}
}

func nonBlendableArrays(p *PrivateCFF2) [][]Blend {
	return [][]Blend{p.FamilyBlues, p.FamilyOtherBlues}
}

// sanitizePrivateCFF2 repairs a CFF2 private dict on the read side.
//
// It removes variation deltas from the operands which cannot carry them --
// files in the wild may hold such deltas -- and then repairs BlueScale, whose
// stored value is its whole value once the deltas are gone.  Doing both here
// keeps writing the dict back out and reading it again a no-op.
//
// The stem widths may vary across the design space, but the stored value is
// the value at the default instance -- every region scores zero there, so the
// deltas vanish -- and the same rule applies to it.  When it turns out
// unusable the whole entry goes, deltas included: a "not given" marker with
// variation hanging off it says nothing coherent.  Whether some other instance
// is unusable takes evaluating the blend, which instancePrivateDict does.
//
// The remaining blendable operands are left alone.
func sanitizePrivateCFF2(p *PrivateCFF2) {
	for _, b := range nonBlendableScalars(p) {
		b.Deltas = nil
	}
	for _, arr := range nonBlendableArrays(p) {
		for i := range arr {
			arr[i].Deltas = nil
		}
	}
	p.BlueScale.Default = type1.RepairBlueScale(p.BlueScale.Default)
	if !type1.UsableBlueDistance(p.BlueShift.Default) {
		p.BlueShift = Blend{Default: type1.DefaultBlueShift}
	}
	if !type1.UsableBlueDistance(p.BlueFuzz.Default) {
		p.BlueFuzz = Blend{Default: type1.DefaultBlueFuzz}
	}

	if !type1.UsableStemWidth(p.StdHW.Default) {
		p.StdHW = Blend{}
	}
	if !type1.UsableStemWidth(p.StdVW.Default) {
		p.StdVW = Blend{}
	}

	// The zone arrays may vary too, so the ordering is judged on the stored
	// values, which are the values at the default instance.  Whether some
	// other instance breaks it takes evaluating the blends, which
	// instancePrivateDict does.
	p.BlueValues = trimZoneBlends(p.BlueValues, type1.MaxBlueValuePairs)
	p.OtherBlues = trimZoneBlends(p.OtherBlues, type1.MaxOtherBluePairs)
	p.FamilyBlues = trimZoneBlends(p.FamilyBlues, type1.MaxBlueValuePairs)
	p.FamilyOtherBlues = trimZoneBlends(p.FamilyOtherBlues, type1.MaxOtherBluePairs)

	// The stem snap widths are capped in number and wanted in increasing
	// order.  The order carries no meaning -- the format says so, and warns
	// that blending can undo it anyway -- so sorting is a repair which loses
	// nothing.
	p.StemSnapH = sortStemSnap(p.StemSnapH)
	p.StemSnapV = sortStemSnap(p.StemSnapV)
}

// blendDefaults returns the stored values of a blended array, which are the
// values at the default instance.
func blendDefaults(vals []Blend) []float64 {
	defaults := make([]float64, len(vals))
	for i, b := range vals {
		defaults[i] = b.Default
	}
	return defaults
}

// trimZoneBlends cuts a blended alignment zone array back to the part
// [type1.UsableZones] accepts, judged on the stored values.  An array with
// nothing left becomes nil rather than an empty slice, which is the shape a
// reader gives for an absent entry -- the writer omits either, so the two have
// to agree for a file to read back the same.
func trimZoneBlends(vals []Blend, maxPairs int) []Blend {
	n := type1.UsableZones(blendDefaults(vals), maxPairs)
	if n == 0 {
		return nil
	}
	return vals[:n]
}

// sortStemSnap caps a stem snap array and puts it in increasing order of its
// stored values, carrying each entry's deltas along with it.
func sortStemSnap(vals []Blend) []Blend {
	if len(vals) == 0 {
		return nil
	}
	if len(vals) > maxStemSnapEntries {
		vals = vals[:maxStemSnapEntries]
	}
	if !slices.IsSortedFunc(vals, compareBlendDefault) {
		vals = slices.Clone(vals)
		slices.SortStableFunc(vals, compareBlendDefault)
	}
	return vals
}

func compareBlendDefault(a, b Blend) int {
	return cmp.Compare(a.Default, b.Default)
}

// checkPrivateBlends reports an error if an operand which cannot be blended
// carries variation deltas.  Values read from a file are repaired by
// sanitizePrivateCFF2, so a failure here means the caller supplied them.
func checkPrivateBlends(p *PrivateCFF2) error {
	for _, b := range nonBlendableScalars(p) {
		if len(b.Deltas) > 0 {
			return errBlendNotAllowed
		}
	}
	for _, arr := range nonBlendableArrays(p) {
		for i := range arr {
			if len(arr[i].Deltas) > 0 {
				return errBlendNotAllowed
			}
		}
	}
	return nil
}
