// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package sfnt

import (
	"fmt"
	"iter"

	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/opentype/gdef"
	"seehuhn.de/go/sfnt/variation"
)

// The GPOS, GDEF and MATH tables refer to the item variation store in the
// GDEF table through VariationIndex tables.  A font read from a file loses
// the references which have no row in the store, and a font is only written
// if every reference has one.

// dropMissingVariations removes the VariationIndex tables of f which refer to
// rows missing from the GDEF item variation store.  Tables without such
// references are left as they are.
func (f *Font) dropMissingVariations() {
	gdefTable := f.Gdef
	keep := func(d *device.Table) *device.Table {
		if isMissingVariation(gdefTable, d) {
			return nil
		}
		return d
	}
	if f.Gpos != nil && firstMissingVariation(gdefTable, f.Gpos.LookupList.Devices()) != nil {
		c := *f.Gpos
		c.LookupList = f.Gpos.LookupList.MapDevices(keep)
		f.Gpos = &c
	}
	if f.Gdef != nil && firstMissingVariation(gdefTable, f.Gdef.Devices()) != nil {
		f.Gdef = f.Gdef.MapDevices(keep)
	}
	if f.Math != nil && firstMissingVariation(gdefTable, f.Math.Devices()) != nil {
		f.Math = f.Math.MapDevices(keep)
	}
}

// checkVariations returns an error if a VariationIndex table of f refers to
// a row missing from the GDEF item variation store.
func (f *Font) checkVariations() error {
	missing := func(table string, devices iter.Seq[*device.Table]) error {
		d := firstMissingVariation(f.Gdef, devices)
		if d == nil {
			return nil
		}
		return fmt.Errorf("sfnt: %s refers to missing variation data (%d, %d)",
			table, d.OuterIndex, d.InnerIndex)
	}
	if f.Gpos != nil {
		if err := missing("GPOS", f.Gpos.LookupList.Devices()); err != nil {
			return err
		}
	}
	if f.Gdef != nil {
		if err := missing("GDEF", f.Gdef.Devices()); err != nil {
			return err
		}
	}
	if f.Math != nil {
		if err := missing("MATH", f.Math.Devices()); err != nil {
			return err
		}
	}
	return nil
}

// firstMissingVariation returns the first VariationIndex table in devices
// which refers to a row missing from the GDEF item variation store, or nil
// if there is none.
func firstMissingVariation(gdefTable *gdef.Table, devices iter.Seq[*device.Table]) *device.Table {
	for d := range devices {
		if isMissingVariation(gdefTable, d) {
			return d
		}
	}
	return nil
}

// isMissingVariation reports whether d is a VariationIndex table which
// refers to a row missing from the GDEF item variation store.  The index
// [variation.NoVariationIndex] refers to no variation data and needs no row.
func isMissingVariation(gdefTable *gdef.Table, d *device.Table) bool {
	if !d.IsVariationIndex() {
		return false
	}
	if d.OuterIndex == variation.NoVariationIndex && d.InnerIndex == variation.NoVariationIndex {
		return false
	}
	if gdefTable == nil || gdefTable.ItemVarStore == nil {
		return true
	}
	data := gdefTable.ItemVarStore.Data
	outer, inner := int(d.OuterIndex), int(d.InnerIndex)
	return outer >= len(data) || inner >= len(data[outer].Deltas)
}
