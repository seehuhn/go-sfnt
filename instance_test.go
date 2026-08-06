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

package sfnt_test

import (
	"bytes"
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"seehuhn.de/go/postscript/funit"

	"seehuhn.de/go/sfnt"
	"seehuhn.de/go/sfnt/avar"
	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/glyf"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/internal/debug"
	"seehuhn.de/go/sfnt/opentype/gtab"
	"seehuhn.de/go/sfnt/os2"
	"seehuhn.de/go/sfnt/parser"
)

func simplePoints(t *testing.T, g *glyf.Glyph) []glyf.Point {
	t.Helper()
	sg, ok := g.Data.(glyf.SimpleGlyph)
	if !ok {
		t.Fatalf("glyph is not simple: %T", g.Data)
	}
	su, err := sg.Unpack()
	if err != nil {
		t.Fatal(err)
	}
	if len(su.Contours) != 1 {
		t.Fatalf("expected 1 contour, got %d", len(su.Contours))
	}
	return su.Contours[0]
}

func componentOffset(t *testing.T, g *glyf.Glyph, idx int) (float64, float64) {
	t.Helper()
	cg, ok := g.Data.(glyf.CompositeGlyph)
	if !ok {
		t.Fatalf("glyph is not composite: %T", g.Data)
	}
	cu, err := cg.Components[idx].Unpack()
	if err != nil {
		t.Fatal(err)
	}
	return cu.Trfm[4], cu.Trfm[5]
}

func cvtValues(data []byte) []int16 {
	out := make([]int16, len(data)/2)
	for i := range out {
		out[i] = int16(binary.BigEndian.Uint16(data[2*i:]))
	}
	return out
}

// TestInstantiateInstance pins the wght=900/wdth=75 instance against the
// hand-computed expectations in the fixture.
func TestInstantiateInstance(t *testing.T) {
	f := debug.MakeVarFont()
	exp := debug.MakeVarFontExpect()

	inst, err := f.Instantiate(map[string]float64{"wght": 900, "wdth": 75})
	if err != nil {
		t.Fatal(err)
	}

	if inst.IsVariable() {
		t.Error("instanced font is still variable")
	}

	o := inst.Outlines.(*glyf.Outlines)

	// IUP glyph outline
	if diff := cmp.Diff(exp.IUPPoints, simplePoints(t, o.Glyphs[debug.VarGidIUP])); diff != "" {
		t.Errorf("IUP glyph points (-want +got):\n%s", diff)
	}

	// composite component offset and recomputed bounding box
	dx, dy := componentOffset(t, o.Glyphs[debug.VarGidComp], 1)
	if dx != 500 || dy != -50 {
		t.Errorf("composite component offset = (%v, %v), want (500, -50)", dx, dy)
	}
	wantBBox := funit.Rect16{LLx: 100, LLy: -50, URx: 1140, URy: 900}
	if got := o.Glyphs[debug.VarGidComp].Rect16; got != wantBBox {
		t.Errorf("composite bbox = %+v, want %+v", got, wantBBox)
	}

	// advances: HVAR takes precedence over the (zero) phantom deltas
	if got, want := o.Widths[debug.VarGidRect], exp.RectAdvanceInstance; got != want {
		t.Errorf("Rect advance = %d, want %d (HVAR precedence)", got, want)
	}
	if got, want := o.Widths[debug.VarGidComp], exp.CompAdvanceInstance; got != want {
		t.Errorf("Comp advance = %d, want %d", got, want)
	}

	// MVAR: hasc adds +50 to Ascent, cpht adds -15 to CapHeight
	if got, want := inst.Ascent, funit.Int16(800+exp.HascDelta); got != want {
		t.Errorf("Ascent = %d, want %d", got, want)
	}
	if got, want := inst.CapHeight, funit.Int16(700+exp.CphtDelta); got != want {
		t.Errorf("CapHeight = %d, want %d", got, want)
	}

	// cvar
	if got := cvtValues(o.Tables["cvt "]); !slicesEqualInt16(got, exp.CVTInstance) {
		t.Errorf("cvt = %v, want %v", got, exp.CVTInstance)
	}

	// FeatureVariations substitution applied, then dropped
	if inst.Gsub.Variations != nil {
		t.Error("GSUB Variations not cleared")
	}
	if got := inst.Gsub.FeatureList[0].Lookups; len(got) != 1 || got[0] != 0 {
		t.Errorf("GSUB feature lookups = %v, want [0]", got)
	}

	// GPOS kern pair: static -40 folded with the +15 VariationIndex delta
	kern := inst.Gpos.LookupList[0].Subtables[0].(gtab.Gpos2_1)
	pair := kern[glyph.Pair{Left: debug.VarGidRect, Right: debug.VarGidIUP}]
	if pair.First.XAdvanceDev != nil {
		t.Error("GPOS device table not baked out")
	}
	wantKern := funit.Int16(exp.KernXAdvance) + funit.Int16(exp.KernDeltaInstance)
	if pair.First.XAdvance != wantKern {
		t.Errorf("kern XAdvance = %d, want %d", pair.First.XAdvance, wantKern)
	}
	if inst.Gdef.ItemVarStore != nil {
		t.Error("GDEF ItemVarStore not cleared")
	}

	// OS/2 classes
	if inst.Weight != 900 {
		t.Errorf("Weight = %d, want 900", inst.Weight)
	}
	if inst.Width != os2.Width(3) {
		t.Errorf("Width = %d, want 3", inst.Width)
	}
}

// TestInstantiateAvar pins the avar arithmetic at wght=650: normalized 0.5
// maps through avar to 0.75, so the all-points Rect glyph moves by 0.75x its
// tuple delta.
func TestInstantiateAvar(t *testing.T) {
	f := debug.MakeVarFont()

	inst, err := f.Instantiate(map[string]float64{"wght": 650})
	if err != nil {
		t.Fatal(err)
	}

	// Rect (gid 1) has an all-points wght tuple whose top corners rise by 200
	// at wght = +1.  The scalar at avar-mapped 0.75 is 0.75.
	const scalar = 0.75
	orig := [][2]funit.Int16{{100, 0}, {400, 0}, {400, 700}, {100, 700}}
	deltaY := []float64{0, 0, 200, 200}
	want := make([]glyf.Point, 4)
	for i, p := range orig {
		want[i] = glyf.Point{
			X:       p[0],
			Y:       p[1] + funit.Int16(math.Round(scalar*deltaY[i])),
			OnCurve: true,
		}
	}

	o := inst.Outlines.(*glyf.Outlines)
	if diff := cmp.Diff(want, simplePoints(t, o.Glyphs[debug.VarGidRect])); diff != "" {
		t.Errorf("Rect glyph points at wght=650 (-want +got):\n%s", diff)
	}
}

// TestInstantiateDefaults checks that pinning at the defaults reproduces the
// original outlines and drops the variation machinery.
func TestInstantiateDefaults(t *testing.T) {
	f := debug.MakeVarFont()
	orig := f.Outlines.(*glyf.Outlines)

	inst, err := f.Instantiate(nil)
	if err != nil {
		t.Fatal(err)
	}

	o := inst.Outlines.(*glyf.Outlines)
	for gid := range orig.Glyphs {
		og := orig.Glyphs[gid]
		ng := o.Glyphs[gid]
		if _, ok := og.Data.(glyf.SimpleGlyph); ok {
			if diff := cmp.Diff(simplePoints(t, og), simplePoints(t, ng)); diff != "" {
				t.Errorf("gid %d outline changed at defaults:\n%s", gid, diff)
			}
		}
	}
	for gid, w := range orig.Widths {
		if o.Widths[gid] != w {
			t.Errorf("gid %d width = %d, want %d", gid, o.Widths[gid], w)
		}
	}

	if inst.Fvar != nil || inst.Avar != nil || inst.Stat != nil || inst.Gvar != nil ||
		inst.Cvar != nil || inst.Hvar != nil || inst.Mvar != nil {
		t.Error("variation tables not fully dropped")
	}
	if inst.Ascent != f.Ascent || inst.CapHeight != f.CapHeight {
		t.Error("metrics changed at defaults")
	}
}

// TestInstantiateNoGvar checks that Instantiate tolerates a variable font
// whose gvar table is missing (Read sets Gvar to nil when it fails to
// decode, or on an axis-count mismatch, while leaving Fvar in place): glyph
// outlines and widths pass through unchanged instead of panicking.
func TestInstantiateNoGvar(t *testing.T) {
	f := debug.MakeVarFont()
	f.Gvar = nil
	f.Hvar = nil
	orig := f.Outlines.(*glyf.Outlines)

	inst, err := f.Instantiate(map[string]float64{"wght": 900, "wdth": 75})
	if err != nil {
		t.Fatal(err)
	}

	o := inst.Outlines.(*glyf.Outlines)
	for gid := range orig.Glyphs {
		og := orig.Glyphs[gid]
		ng := o.Glyphs[gid]
		if _, ok := og.Data.(glyf.SimpleGlyph); ok {
			if diff := cmp.Diff(simplePoints(t, og), simplePoints(t, ng)); diff != "" {
				t.Errorf("gid %d outline changed without gvar:\n%s", gid, diff)
			}
		}
	}
	for gid, w := range orig.Widths {
		if o.Widths[gid] != w {
			t.Errorf("gid %d width = %d, want %d", gid, o.Widths[gid], w)
		}
	}
}

// TestInstantiateReceiverUnmodified verifies that Instantiate does not mutate
// the receiver by comparing against an independent identical fixture.
func TestInstantiateReceiverUnmodified(t *testing.T) {
	f := debug.MakeVarFont()
	ref := debug.MakeVarFont()

	if _, err := f.Instantiate(map[string]float64{"wght": 900, "wdth": 75}); err != nil {
		t.Fatal(err)
	}

	opts := cmp.Options{cmpopts.IgnoreUnexported(sfnt.Font{})}
	if diff := cmp.Diff(ref, f, opts); diff != "" {
		t.Errorf("receiver mutated by Instantiate (-ref +f):\n%s", diff)
	}
}

func TestInstantiateNaming(t *testing.T) {
	t.Run("named-instance", func(t *testing.T) {
		f := debug.MakeVarFont()
		inst, err := f.Instantiate(map[string]float64{"wght": 700, "wdth": 75})
		if err != nil {
			t.Fatal(err)
		}
		if got := inst.FontName; got != "QuireVar-BoldNarrow" {
			t.Errorf("PostScriptName = %q, want QuireVar-BoldNarrow", got)
		}
		// the instance names the style it pins, and the full name follows from
		// the family and that style
		if got := inst.Subfamily; got != "Bold Narrow" {
			t.Errorf("Subfamily = %q, want %q", got, "Bold Narrow")
		}
		if got := inst.FullName; got != "QuireVar Bold Narrow" {
			t.Errorf("FullName = %q, want %q", got, "QuireVar Bold Narrow")
		}
	})

	// A style of "Regular" adds nothing to the family name, so the full name is
	// the family name on its own.
	t.Run("regular-instance", func(t *testing.T) {
		f := debug.MakeVarFont()
		f.Fvar.Instances[0].Name = "Regular"
		inst, err := f.Instantiate(map[string]float64{
			"wght": f.Fvar.Instances[0].Coordinates[0],
			"wdth": f.Fvar.Instances[0].Coordinates[1],
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := inst.Subfamily; got != "Regular" {
			t.Errorf("Subfamily = %q, want %q", got, "Regular")
		}
		if got := inst.FullName; got != "QuireVar" {
			t.Errorf("FullName = %q, want %q", got, "QuireVar")
		}
	})

	t.Run("generated", func(t *testing.T) {
		f := debug.MakeVarFont()
		inst, err := f.Instantiate(map[string]float64{"wght": 650})
		if err != nil {
			t.Fatal(err)
		}
		if got := inst.FontName; got != "QuireVar_650wght_100wdth" {
			t.Errorf("PostScriptName = %q, want QuireVar_650wght_100wdth", got)
		}
		// nowhere near a named instance, so the variable font's own style names
		// must not carry over
		if got := inst.Subfamily; got != "" {
			t.Errorf("Subfamily = %q, want %q", got, "")
		}
		if got := inst.FullName; got != "" {
			t.Errorf("FullName = %q, want %q", got, "")
		}
	})

	// an instance which matches the coordinates but carries no PostScript name
	// of its own cannot supply one, so the generated name is used
	t.Run("nameless-instance", func(t *testing.T) {
		f := debug.MakeVarFont()
		for i := range f.Fvar.Instances {
			f.Fvar.Instances[i].PostScriptName = ""
		}
		inst, err := f.Instantiate(map[string]float64{"wght": 700, "wdth": 75})
		if err != nil {
			t.Fatal(err)
		}
		if got := inst.FontName; got != "QuireVar_700wght_75wdth" {
			t.Errorf("PostScriptName = %q, want QuireVar_700wght_75wdth", got)
		}
	})

	t.Run("fractional", func(t *testing.T) {
		f := debug.MakeVarFont()
		inst, err := f.Instantiate(map[string]float64{"wdth": 87.5})
		if err != nil {
			t.Fatal(err)
		}
		if got := inst.FontName; got != "QuireVar_400wght_87.5wdth" {
			t.Errorf("PostScriptName = %q, want QuireVar_400wght_87.5wdth", got)
		}
	})

	t.Run("long-name-fallback", func(t *testing.T) {
		f := debug.MakeVarFont()
		f.VariationsPostScriptName = ""
		f.FamilyName = strings.Repeat("A", 200)
		inst, err := f.Instantiate(map[string]float64{"wght": 650})
		if err != nil {
			t.Fatal(err)
		}
		got := inst.FontName
		if len(got) > 127 {
			t.Errorf("PostScriptName length = %d, want <= 127", len(got))
		}
		if len(got) < 9 || got[len(got)-9] != '-' {
			t.Errorf("PostScriptName = %q, want an 8-hex-char hash suffix", got)
		}
	})

	// The hash fallback slices the prefix on its own, so the result must
	// still be a name the instance can be written with.
	t.Run("long-prefix-fallback", func(t *testing.T) {
		f := debug.MakeVarFont()
		f.Fvar.Instances = nil
		f.VariationsPostScriptName = strings.Repeat("x", 140)

		inst, err := f.Instantiate(nil)
		if err != nil {
			t.Fatal(err)
		}
		got := inst.FontName
		if len(got) < 9 || got[len(got)-9] != '-' {
			t.Errorf("PostScriptName = %q, want an 8-hex-char hash suffix", got)
		}
		if _, err := inst.Write(&bytes.Buffer{}); err != nil {
			t.Errorf("instance is not writable: %v", err)
		}
	})

	// A prefix outside the ASCII letters and digits cannot be reduced to one
	// without standing for a different family, so the family name is used.
	t.Run("non-ascii-prefix", func(t *testing.T) {
		f := debug.MakeVarFont()
		f.Fvar.Instances = nil
		f.FamilyName = "Quire Var"
		f.VariationsPostScriptName = "宋体"

		inst, err := f.Instantiate(nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := inst.FontName; got != "QuireVar_400wght_100wdth" {
			t.Errorf("PostScriptName = %q, want QuireVar_400wght_100wdth", got)
		}
	})

	// A prefix the caller supplied may hold anything.
	t.Run("invalid-prefix", func(t *testing.T) {
		f := debug.MakeVarFont()
		f.Fvar.Instances = nil
		f.VariationsPostScriptName = "Quire Var (draft)"

		inst, err := f.Instantiate(nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := inst.FontName; got != "QuireVardraft_400wght_100wdth" {
			t.Errorf("PostScriptName = %q, want QuireVardraft_400wght_100wdth", got)
		}
	})
}

func TestInstantiateRoundTrip(t *testing.T) {
	f := debug.MakeVarFont()
	inst, err := f.Instantiate(map[string]float64{"wght": 900, "wdth": 75})
	if err != nil {
		t.Fatal(err)
	}

	buf := &bytes.Buffer{}
	if _, err := inst.Write(buf); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()
	got, err := sfnt.Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
	if err != nil {
		t.Fatal(err)
	}
	if got.IsVariable() {
		t.Error("round-tripped instance is variable")
	}
}

func TestInstantiateErrors(t *testing.T) {
	t.Run("not-variable", func(t *testing.T) {
		f := debug.MakeVarFont()
		f.Fvar = nil
		if _, err := f.Instantiate(nil); err != sfnt.ErrNotVariable {
			t.Errorf("err = %v, want ErrNotVariable", err)
		}
	})

	t.Run("unknown-tag", func(t *testing.T) {
		f := debug.MakeVarFont()
		if _, err := f.Instantiate(map[string]float64{"zzzz": 1}); err == nil {
			t.Error("expected an error for an unknown axis tag")
		}
	})

	t.Run("feature-variations-raw", func(t *testing.T) {
		f := debug.MakeVarFont()
		f.Gsub.Variations = nil
		f.Gsub.VariationsRaw = []byte{0, 1}
		if _, err := f.Instantiate(nil); err == nil {
			t.Error("expected an error for unresolvable feature variations")
		}
	})

	t.Run("avar-v2", func(t *testing.T) {
		f := debug.MakeVarFont()
		f.Avar = &avar.Table{Raw: []byte{0, 2, 0, 0}}
		if _, err := f.Instantiate(nil); err == nil {
			t.Error("expected an error for an unsupported avar version")
		}
	})

	t.Run("cff2-outlines", func(t *testing.T) {
		f := debug.MakeVarFont()
		f.Outlines = &cff.Outlines{}
		if _, err := f.Instantiate(nil); err == nil {
			t.Error("expected an error for CFF outlines")
		}
	})
}

// FuzzInstantiate feeds arbitrary sfnt data through Read and, for variable
// fonts, Instantiate.  It checks that Instantiate never panics or hangs, and
// that a successful instantiation round-trips through Write/Read as a
// non-variable font.
func FuzzInstantiate(f *testing.F) {
	varBuf := &bytes.Buffer{}
	if _, err := debug.MakeVarFont().Write(varBuf); err != nil {
		f.Fatal(err)
	}
	f.Add(varBuf.Bytes(), uint16(0), uint16(0))
	f.Add(varBuf.Bytes(), uint16(0xFFFF), uint16(0xFFFF))
	f.Add(varBuf.Bytes(), uint16(0x8000), uint16(0x4000))

	otherBuf := &bytes.Buffer{}
	if _, err := makeVariableFont(f).Write(otherBuf); err != nil {
		f.Fatal(err)
	}
	f.Add(otherBuf.Bytes(), uint16(0), uint16(0xFFFF))

	cff2Buf := &bytes.Buffer{}
	if _, err := makeVarCFF2Font().Write(cff2Buf); err != nil {
		f.Fatal(err)
	}
	f.Add(cff2Buf.Bytes(), uint16(0), uint16(0))
	f.Add(cff2Buf.Bytes(), uint16(0xFFFF), uint16(0xFFFF))

	f.Fuzz(func(t *testing.T, data []byte, c1, c2 uint16) {
		font, err := sfnt.Read(bytes.NewReader(data), parser.NewBudget(int64(len(data))))
		if err != nil || !font.IsVariable() {
			return
		}

		// scale c1, c2 into the ranges of the first two axes; any further
		// axes are left at their default value.
		coords := make(map[string]float64)
		axes := font.VariationAxes()
		for i, u := range []uint16{c1, c2} {
			if i >= len(axes) {
				break
			}
			ax := axes[i]
			frac := float64(u) / 0xFFFF
			coords[ax.Tag] = ax.Min + frac*(ax.Max-ax.Min)
		}

		inst, err := font.Instantiate(coords)
		if err != nil {
			return
		}

		if inst.IsVariable() {
			t.Fatal("instantiated font is still variable")
		}

		buf := &bytes.Buffer{}
		if _, err := inst.Write(buf); err != nil {
			t.Fatalf("Write failed: %v", err)
		}
		out := buf.Bytes()
		got, err := sfnt.Read(bytes.NewReader(out), parser.NewBudget(int64(len(out))))
		if err != nil {
			t.Fatalf("re-read failed: %v", err)
		}
		if got.IsVariable() {
			t.Fatal("round-tripped instance is variable")
		}
	})
}

func slicesEqualInt16(a, b []int16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
