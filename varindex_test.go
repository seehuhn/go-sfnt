// Copyright 2026 Jochen Voss <voss@seehuhn.de>
// SPDX-License-Identifier: BSD-3-Clause

package sfnt_test

import (
	"bytes"
	"testing"

	"github.com/google/go-cmp/cmp"
	"golang.org/x/text/language"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/internal/debug"
	"seehuhn.de/go/sfnt/opentype/coverage"
	"seehuhn.de/go/sfnt/opentype/device"
	"seehuhn.de/go/sfnt/opentype/gdef"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/opentype/mathtable"
	"seehuhn.de/go/sfnt/parser"
	"seehuhn.de/go/sfnt/variation"
)

// danglingIndex is a VariationIndex table without variation data in a font
// which has no GDEF item variation store.
func danglingIndex() *device.Table {
	return &device.Table{OuterIndex: 0, InnerIndex: 3, DeltaFormat: device.VariationIndexFormat}
}

// gposWithDevice returns a GPOS table whose single value record has dev as
// its x-advance device table.
func gposWithDevice(dev *device.Table) *gtab.Info {
	return &gtab.Info{
		ScriptList: gtab.ScriptListInfo{
			language.MustParse("und-Latn"): {Required: 0xFFFF, Optional: []gtab.FeatureIndex{0}},
		},
		FeatureList: gtab.FeatureListInfo{{Tag: "kern", Lookups: []gtab.LookupIndex{0}}},
		LookupList: gtab.LookupList{{
			Meta: &gtab.LookupMetaInfo{LookupType: 1},
			Subtables: []gtab.Subtable{&gtab.Gpos1_1{
				Cov:    coverage.Table{4: 0},
				Adjust: &gtab.GposValueRecord{XAdvance: -20, XAdvanceDev: dev},
			}},
		}},
	}
}

// gdefWithDevice returns a GDEF table whose single ligature caret has dev as
// its device table.
func gdefWithDevice(dev *device.Table) *gdef.Table {
	return &gdef.Table{
		LigCaretList: &gdef.LigCaretList{
			Cov:    coverage.Table{4: 0},
			Carets: [][]gdef.CaretValue{{{Coordinate: 300, Device: dev}}},
		},
	}
}

// TestWriteMissingVariation checks that a font is not written when a GPOS
// or GDEF VariationIndex table has no variation data to refer to.
func TestWriteMissingVariation(t *testing.T) {
	cases := []struct {
		name string
		set  func(f *sfnt.Font)
	}{
		{"GPOS", func(f *sfnt.Font) { f.Gpos = gposWithDevice(danglingIndex()) }},
		{"GDEF", func(f *sfnt.Font) { f.Gdef = gdefWithDevice(danglingIndex()) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := readGoRegular(t)
			f.Gdef = nil
			c.set(f)
			if _, err := f.Write(&bytes.Buffer{}); err == nil {
				t.Error("font with dangling VariationIndex written")
			}
		})
	}
}

// TestReadDropsMissingVariation checks that GPOS and GDEF VariationIndex
// tables which refer to missing variation data are dropped on read, that
// the values they adjust are kept, and that the result is stable.
func TestReadDropsMissingVariation(t *testing.T) {
	cases := []struct {
		tag   string
		table interface{ Encode() ([]byte, error) }
		check func(t *testing.T, f *sfnt.Font)
	}{
		{"GPOS", gposWithDevice(danglingIndex()), func(t *testing.T, f *sfnt.Font) {
			vr := f.Gpos.LookupList[0].Subtables[0].(*gtab.Gpos1_1).Adjust
			if vr.XAdvance != -20 || vr.XAdvanceDev != nil {
				t.Errorf("got %+v, want XAdvance -20 without device", vr)
			}
		}},
		{"GDEF", gdefWithDevice(danglingIndex()), func(t *testing.T, f *sfnt.Font) {
			cv := f.Gdef.LigCaretList.Carets[0][0]
			if cv.Coordinate != 300 || cv.Device != nil {
				t.Errorf("got %+v, want coordinate 300 without device", cv)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.tag, func(t *testing.T) {
			f := readGoRegular(t)
			f.Gdef = nil
			f.Gpos = nil
			raw, err := c.table.Encode()
			if err != nil {
				t.Fatal(err)
			}
			data := withRawTable(t, f, c.tag, raw)

			f2, err := sfnt.Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
			if err != nil {
				t.Fatal(err)
			}
			c.check(t, f2)

			f3 := writeAndRead(t, f2)
			if diff := cmp.Diff(f2.Gpos, f3.Gpos); diff != "" {
				t.Errorf("GPOS read-write-read not stable (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(f2.Gdef, f3.Gdef); diff != "" {
				t.Errorf("GDEF read-write-read not stable (-want +got):\n%s", diff)
			}
		})
	}
}

// TestVariationIndexKept checks that VariationIndex tables which refer to
// a row of the GDEF item variation store, or to no variation data, are
// written and survive a write and read.
func TestVariationIndexKept(t *testing.T) {
	vi := func(outer, inner uint16) *device.Table {
		return &device.Table{OuterIndex: outer, InnerIndex: inner, DeltaFormat: device.VariationIndexFormat}
	}
	noVar := vi(variation.NoVariationIndex, variation.NoVariationIndex)
	cases := []struct {
		name string
		font func(t *testing.T) *sfnt.Font
		dev  *device.Table
	}{
		{"existing row", func(*testing.T) *sfnt.Font { return debug.MakeVarFont() }, vi(0, 0)},
		{"no variation", func(*testing.T) *sfnt.Font { return debug.MakeVarFont() }, noVar},
		{"no variation without store", func(t *testing.T) *sfnt.Font {
			f := readGoRegular(t)
			f.Gdef = &gdef.Table{}
			return f
		}, noVar},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := c.font(t)
			f.Gpos = gposWithDevice(c.dev)
			f.Gdef.LigCaretList = gdefWithDevice(c.dev).LigCaretList
			f.Math = &mathtable.Info{}
			f.Math.Constants.AxisHeight = mathtable.Value{V: 250, Device: c.dev}

			f2 := writeAndRead(t, f)

			gposDev := f2.Gpos.LookupList[0].Subtables[0].(*gtab.Gpos1_1).Adjust.XAdvanceDev
			if diff := cmp.Diff(c.dev, gposDev); diff != "" {
				t.Errorf("GPOS device (-want +got):\n%s", diff)
			}
			gdefDev := f2.Gdef.LigCaretList.Carets[0][0].Device
			if diff := cmp.Diff(c.dev, gdefDev); diff != "" {
				t.Errorf("GDEF device (-want +got):\n%s", diff)
			}
			mathDev := f2.Math.Constants.AxisHeight.Device
			if diff := cmp.Diff(c.dev, mathDev); diff != "" {
				t.Errorf("MATH device (-want +got):\n%s", diff)
			}
		})
	}
}
