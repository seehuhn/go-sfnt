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
	"bytes"
	"fmt"
	"io"
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"seehuhn.de/go/geom/matrix"
	"seehuhn.de/go/geom/path"
	"seehuhn.de/go/geom/vec"

	"seehuhn.de/go/membudget"
	"seehuhn.de/go/postscript/funit"
	"seehuhn.de/go/postscript/type1"

	"seehuhn.de/go/sfnt/glyph"
)

// makeTestType1 constructs a small Type 1 font which exercises all glyph
// drawing commands, several sub-paths, stem hints and a non-standard
// built-in encoding.
//
// The glyph order induced by [type1.Outlines.GlyphList] is
// ".notdef", "circle", "space", "A", "B", "zcaron".
func makeTestType1() *type1.Font {
	encoding := make([]string, 256)
	for i := range encoding {
		encoding[i] = ".notdef"
	}
	encoding[1] = "circle"
	encoding[' '] = "space"
	encoding['A'] = "A"
	encoding['B'] = "B"

	F := &type1.Font{
		FontInfo: &type1.FontInfo{
			FontName:           "Test-Regular",
			Version:            "1.000",
			Notice:             "Notice",
			Copyright:          "Copyright",
			FullName:           "Test Regular",
			FamilyName:         "Test",
			Weight:             "Regular",
			ItalicAngle:        -11.5,
			IsFixedPitch:       false,
			UnderlinePosition:  -100,
			UnderlineThickness: 50,
			FontMatrix:         matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
		},
		Outlines: &type1.Outlines{
			Glyphs: map[string]*type1.Glyph{},
			Private: &type1.PrivateDict{
				BlueValues: []funit.Int16{0, 10, 700, 710},
				OtherBlues: []funit.Int16{-210, -200},
				BlueScale:  0.05,
				BlueShift:  8,
				BlueFuzz:   2,
				StdHW:      23,
				StdVW:      85,
				ForceBold:  true,
			},
			Encoding: encoding,
		},
	}

	// a rectangle, drawn with lines only
	g := F.NewGlyph(".notdef", 250)
	g.MoveTo(50, 0)
	g.LineTo(200, 0)
	g.LineTo(200, 700)
	g.LineTo(50, 700)
	g.ClosePath()

	// a triangle with stem hints
	g = F.NewGlyph("A", 600)
	g.MoveTo(0, 0)
	g.LineTo(600, 0)
	g.LineTo(300, 700)
	g.ClosePath()
	g.HStem = []funit.Int16{0, 20, 680, 700}
	g.VStem = []funit.Int16{0, 30, 570, 600}

	// two sub-paths, mixing lines and curves
	g = F.NewGlyph("B", 550)
	g.MoveTo(50, 0)
	g.LineTo(300, 0)
	g.CurveTo(450, 0, 500, 100, 500, 200)
	g.CurveTo(500, 300, 450, 350, 300, 350)
	g.LineTo(50, 350)
	g.ClosePath()
	g.MoveTo(150, 80)
	g.LineTo(150, 270)
	g.LineTo(280, 270)
	g.CurveTo(370, 270, 400, 240, 400, 175)
	g.CurveTo(400, 110, 370, 80, 280, 80)
	g.ClosePath()

	// curves only
	g = F.NewGlyph("circle", 500)
	g.MoveTo(250, 0)
	g.CurveTo(112, 0, 0, 112, 0, 250)
	g.CurveTo(0, 388, 112, 500, 250, 500)
	g.CurveTo(388, 500, 500, 388, 500, 250)
	g.CurveTo(500, 112, 388, 0, 250, 0)
	g.ClosePath()

	// a blank glyph
	F.NewGlyph("space", 250)

	// an unencoded glyph
	g = F.NewGlyph("zcaron", 450)
	g.MoveTo(30, 0)
	g.LineTo(420, 0)
	g.LineTo(420, 90)
	g.LineTo(150, 90)
	g.LineTo(420, 610)
	g.LineTo(420, 700)
	g.LineTo(40, 700)
	g.LineTo(40, 610)
	g.LineTo(300, 610)
	g.LineTo(30, 90)
	g.ClosePath()

	return F
}

// glyphOrder is the expected glyph order of the font returned by
// makeTestType1.
var glyphOrder = []string{".notdef", "circle", "space", "A", "B", "zcaron"}

// comparePaths checks that two paths describe the same sequence of segments.
// Since FromType1 copies coordinates verbatim, the comparison is exact.
func comparePaths(t *testing.T, name string, want, got path.Path) {
	t.Helper()
	if d := cmp.Diff(path.DataFromPath(want), path.DataFromPath(got)); d != "" {
		t.Errorf("outline of %q differs (-want +got):\n%s", name, d)
	}
}

func TestFromType1Structure(t *testing.T) {
	F := makeTestType1()

	cffFont, err := FromType1(F)
	if err != nil {
		t.Fatal(err)
	}

	var names []string
	for _, g := range cffFont.Glyphs {
		names = append(names, g.Name)
	}
	if d := cmp.Diff(glyphOrder, names); d != "" {
		t.Errorf("glyph order differs (-want +got):\n%s", d)
	}
	if cffFont.Glyphs[0].Name != ".notdef" {
		t.Errorf("glyph 0 is %q, want \".notdef\"", cffFont.Glyphs[0].Name)
	}

	// the font must be name-keyed
	if cffFont.IsCIDKeyed() {
		t.Error("font is CID-keyed")
	}
	if cffFont.ROS != nil || cffFont.GIDToCID != nil || cffFont.FontMatrices != nil {
		t.Error("CID-keyed fields are set")
	}

	if len(cffFont.Private) != 1 {
		t.Fatalf("got %d private dicts, want 1", len(cffFont.Private))
	}
	if d := cmp.Diff(F.Private, cffFont.Private[0]); d != "" {
		t.Errorf("private dict differs (-want +got):\n%s", d)
	}

	if cffFont.FDSelect == nil {
		t.Fatal("missing FDSelect")
	}
	for gid := range cffFont.Glyphs {
		if fd := cffFont.FDSelect(glyph.ID(gid)); fd != 0 {
			t.Errorf("FDSelect(%d) = %d, want 0", gid, fd)
		}
	}

	if d := cmp.Diff(F.FontInfo, cffFont.FontInfo); d != "" {
		t.Errorf("font info differs (-want +got):\n%s", d)
	}
}

func TestFromType1Independent(t *testing.T) {
	F := makeTestType1()

	cffFont, err := FromType1(F)
	if err != nil {
		t.Fatal(err)
	}

	if cffFont.FontInfo == F.FontInfo {
		t.Error("font info is shared with the source font")
	}
	if cffFont.Private[0] == F.Private {
		t.Error("private dict is shared with the source font")
	}

	cffFont.FontInfo.FontName = "Changed"
	cffFont.Private[0].BlueValues[0] = 999
	cffFont.Private[0].StdHW = 999

	if F.FontInfo.FontName != "Test-Regular" {
		t.Error("source font name was modified")
	}
	if F.Private.BlueValues[0] != 0 {
		t.Error("source blue values were modified")
	}
	if F.Private.StdHW != 23 {
		t.Error("source private dict was modified")
	}
}

func TestFromType1Encoding(t *testing.T) {
	F := makeTestType1()

	cffFont, err := FromType1(F)
	if err != nil {
		t.Fatal(err)
	}

	if len(cffFont.Encoding) != 256 {
		t.Fatalf("encoding has length %d, want 256", len(cffFont.Encoding))
	}

	gidByName := make(map[string]glyph.ID)
	for gid, g := range cffFont.Glyphs {
		gidByName[g.Name] = glyph.ID(gid)
	}

	want := make([]glyph.ID, 256)
	want[1] = gidByName["circle"]
	want[' '] = gidByName["space"]
	want['A'] = gidByName["A"]
	want['B'] = gidByName["B"]
	if d := cmp.Diff(want, cffFont.Encoding); d != "" {
		t.Errorf("encoding differs (-want +got):\n%s", d)
	}

	// all encoded glyphs must have non-zero glyph IDs
	for _, code := range []int{1, ' ', 'A', 'B'} {
		if cffFont.Encoding[code] == 0 {
			t.Errorf("code %d is not encoded", code)
		}
	}
}

// TestFromType1EncodingLength checks that a source encoding which is nil,
// short or over-long still yields an encoding vector of length 256.
func TestFromType1EncodingLength(t *testing.T) {
	for _, n := range []int{0, 3, 256, 300} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			F := makeTestType1()

			var encoding []string
			if n > 0 {
				encoding = make([]string, n)
				for i := range encoding {
					encoding[i] = ".notdef"
				}
				encoding[min(n-1, 65)] = "A"
			}
			F.Encoding = encoding

			cffFont, err := FromType1(F)
			if err != nil {
				t.Fatal(err)
			}
			if len(cffFont.Encoding) != 256 {
				t.Fatalf("encoding has length %d, want 256", len(cffFont.Encoding))
			}

			numEncoded := 0
			for _, gid := range cffFont.Encoding {
				if gid != 0 {
					numEncoded++
				}
			}
			wantEncoded := 0
			if n > 0 && min(n-1, 65) < 256 {
				wantEncoded = 1
			}
			if numEncoded != wantEncoded {
				t.Errorf("got %d encoded codes, want %d", numEncoded, wantEncoded)
			}
		})
	}
}

func TestFromType1Glyphs(t *testing.T) {
	F := makeTestType1()

	cffFont, err := FromType1(F)
	if err != nil {
		t.Fatal(err)
	}

	for _, g := range cffFont.Glyphs {
		orig, ok := F.Glyphs[g.Name]
		if !ok {
			t.Errorf("unexpected glyph %q", g.Name)
			continue
		}

		if g.Width != orig.WidthX {
			t.Errorf("width of %q: got %v, want %v", g.Name, g.Width, orig.WidthX)
		}

		// the stems of makeTestType1 are already sorted and disjoint,
		// so conversion only widens the values to float64
		if d := cmp.Diff(convertStems(orig.HStem), g.HStem); d != "" {
			t.Errorf("HStem of %q differs (-want +got):\n%s", g.Name, d)
		}
		if d := cmp.Diff(convertStems(orig.VStem), g.VStem); d != "" {
			t.Errorf("VStem of %q differs (-want +got):\n%s", g.Name, d)
		}

		comparePaths(t, g.Name, orig.Path(), g.Path())
	}
}

// TestFromType1NoClosePath makes sure that the implicit closing of sub-paths
// in CFF charstrings does not add a spurious line segment.
func TestFromType1NoClosePath(t *testing.T) {
	F := makeTestType1()

	cffFont, err := FromType1(F)
	if err != nil {
		t.Fatal(err)
	}

	for _, g := range cffFont.Glyphs {
		for _, cmd := range g.Cmds {
			if cmd.Op != OpMoveTo && cmd.Op != OpLineTo && cmd.Op != OpCurveTo {
				t.Errorf("glyph %q uses unexpected command %s", g.Name, cmd.Op)
			}
		}
	}

	// The triangle "A" is drawn using three points, so the CFF glyph must
	// contain one MoveTo and two LineTo commands.
	for _, g := range cffFont.Glyphs {
		if g.Name != "A" {
			continue
		}
		if len(g.Cmds) != 3 {
			t.Errorf("glyph A has %d commands, want 3", len(g.Cmds))
		}
	}
}

// TestFromType1CloseThenDraw checks the case where a sub-path is closed and
// drawing continues without an intervening move.  Closing returns the current
// point to the start of the sub-path, so the following segment must begin
// there instead of extending the closed sub-path.
func TestFromType1CloseThenDraw(t *testing.T) {
	F := makeTestType1()
	g := F.Glyphs["A"]
	g.Outline = &path.Data{}
	g.MoveTo(0, 0)
	g.LineTo(600, 0)
	g.LineTo(300, 700)
	g.ClosePath()
	g.LineTo(100, 400)

	cffFont, err := FromType1(F)
	if err != nil {
		t.Fatal(err)
	}

	var out *Glyph
	for _, cand := range cffFont.Glyphs {
		if cand.Name == "A" {
			out = cand
		}
	}
	if out == nil {
		t.Fatal("glyph A is missing")
	}

	// The triangle, then a new sub-path from the triangle's start point.
	// Reading a CFF glyph makes the implicit closing of sub-paths explicit.
	want := &path.Data{}
	want.MoveTo(vec.Vec2{X: 0, Y: 0})
	want.LineTo(vec.Vec2{X: 600, Y: 0})
	want.LineTo(vec.Vec2{X: 300, Y: 700})
	want.Close()
	want.MoveTo(vec.Vec2{X: 0, Y: 0})
	want.LineTo(vec.Vec2{X: 100, Y: 400})
	want.Close()
	if d := cmp.Diff(want, path.DataFromPath(out.Path())); d != "" {
		t.Errorf("outline differs (-want +got):\n%s", d)
	}
}

// TestFromType1DecodedOutlines converts a font which has been through the
// Type 1 encoder and decoder, so that the outlines contain the closepath
// commands generated by the decoder.
func TestFromType1DecodedOutlines(t *testing.T) {
	F := makeTestType1()

	buf := &bytes.Buffer{}
	err := F.Write(buf, nil)
	if err != nil {
		t.Fatal(err)
	}
	G, err := type1.Read(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	// make sure the decoded outlines really use closepath
	numClose := 0
	for _, g := range G.Glyphs {
		for cmd := range g.Path() {
			if cmd == path.CmdClose {
				numClose++
			}
		}
	}
	if numClose == 0 {
		t.Fatal("decoded outlines contain no closepath")
	}

	a, err := FromType1(F)
	if err != nil {
		t.Fatal(err)
	}
	b, err := FromType1(G)
	if err != nil {
		t.Fatal(err)
	}

	opt := cmpopts.IgnoreFields(Outlines{}, "FDSelect")
	if d := cmp.Diff(a.Outlines, b.Outlines, opt); d != "" {
		t.Errorf("outlines differ (-want +got):\n%s", d)
	}
}

// TestFromType1MissingNotdef checks that a ".notdef" glyph is synthesised
// if the source font does not have one.
func TestFromType1MissingNotdef(t *testing.T) {
	F := makeTestType1()
	delete(F.Glyphs, ".notdef")

	cffFont, err := FromType1(F)
	if err != nil {
		t.Fatal(err)
	}

	g := cffFont.Glyphs[0]
	if g.Name != ".notdef" {
		t.Fatalf("glyph 0 is %q, want \".notdef\"", g.Name)
	}
	if len(g.Cmds) != 0 {
		t.Error("synthesised .notdef glyph is not blank")
	}
	if math.Abs(g.Width-500) > 1e-6 {
		t.Errorf("synthesised .notdef width is %v, want 500", g.Width)
	}

	// the font must still be writable
	err = cffFont.Write(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}

	// the width is half an em, so it follows the font matrix
	F.FontMatrix = matrix.Matrix{0.0005, 0, 0, 0.0005, 0, 0}
	cffFont, err = FromType1(F)
	if err != nil {
		t.Fatal(err)
	}
	if w := cffFont.Glyphs[0].Width; math.Abs(w-1000) > 1e-6 {
		t.Errorf("synthesised .notdef width is %v, want 1000", w)
	}

	// a rotated font matrix describes the same em size
	F.FontMatrix = matrix.Matrix{0, 0.0005, -0.0005, 0, 0, 0}
	cffFont, err = FromType1(F)
	if err != nil {
		t.Fatal(err)
	}
	if w := cffFont.Glyphs[0].Width; math.Abs(w-1000) > 1e-6 {
		t.Errorf("synthesised .notdef width is %v, want 1000", w)
	}

	// advance widths run along the x-axis, so only its image matters
	F.FontMatrix = matrix.Matrix{0.001, 0, 0, 0.002, 0, 0}
	cffFont, err = FromType1(F)
	if err != nil {
		t.Fatal(err)
	}
	if w := cffFont.Glyphs[0].Width; math.Abs(w-500) > 1e-6 {
		t.Errorf("synthesised .notdef width is %v, want 500", w)
	}
}

// makeManyGlyphType1 returns a Type 1 font with n glyphs.  The font
// information contributes no strings, so the ".notdef" glyph is the only
// glyph name which is a standard string.
func makeManyGlyphType1(n int) *type1.Font {
	F := &type1.Font{
		FontInfo: &type1.FontInfo{
			FontName:   "Test",
			FontMatrix: matrix.Matrix{0.001, 0, 0, 0.001, 0, 0},
		},
		Outlines: &type1.Outlines{
			Glyphs: make(map[string]*type1.Glyph, n),
		},
	}
	F.NewGlyph(".notdef", 500)
	for i := range n - 1 {
		F.NewGlyph(fmt.Sprintf("g%05d", i), 100)
	}
	return F
}

// TestFromType1GlyphLimit checks the boundary of the glyph count limit: a font
// which FromType1 accepts can always be written.
func TestFromType1GlyphLimit(t *testing.T) {
	// every glyph name except ".notdef" needs a SID of its own
	maxNames := int(maxSID+1-nStdString) + 1

	cffFont, err := FromType1(makeManyGlyphType1(maxNames))
	if err != nil {
		t.Fatalf("largest allowed font rejected: %v", err)
	}
	if err := cffFont.Write(io.Discard); err != nil {
		t.Errorf("largest allowed font cannot be written: %v", err)
	}

	if _, err := FromType1(makeManyGlyphType1(maxNames + 1)); err == nil {
		t.Error("expected an error")
	}
}

// findGlyph returns the glyph with the given name, or fails the test.
func findGlyph(t *testing.T, f *Font, name string) *Glyph {
	t.Helper()
	for _, g := range f.Glyphs {
		if g.Name == name {
			return g
		}
	}
	t.Fatalf("glyph %q is missing", name)
	return nil
}

// TestFromType1StemHints checks that stem hints are brought into the form
// Type 2 charstrings require: pairs sorted by increasing bottom edge, with
// pairs overlapping an earlier pair dropped.  Type 1 hint replacement leaves
// the accumulated hints of all replacement groups in the glyph, so unsorted
// and overlapping hints occur in real fonts.
func TestFromType1StemHints(t *testing.T) {
	F := makeTestType1()
	g := F.Glyphs["A"]
	// A ghost hint (edges in descending order) first, then a normal stem
	// below it, then a duplicate of that stem.
	g.HStem = []funit.Int16{700, 680, 0, 20, 0, 20}
	// the middle stem overlaps the first
	g.VStem = []funit.Int16{0, 30, 25, 60, 570, 600}

	cffFont, err := FromType1(F)
	if err != nil {
		t.Fatal(err)
	}
	out := findGlyph(t, cffFont, "A")

	wantH := []float64{0, 20, 700, 680}
	if d := cmp.Diff(wantH, out.HStem); d != "" {
		t.Errorf("HStem differs (-want +got):\n%s", d)
	}
	wantV := []float64{0, 30, 570, 600}
	if d := cmp.Diff(wantV, out.VStem); d != "" {
		t.Errorf("VStem differs (-want +got):\n%s", d)
	}
}

// TestFromType1StemLimit checks that the number of stem hints in a glyph is
// capped at the Type 2 limit of 96 stems per glyph.
func TestFromType1StemLimit(t *testing.T) {
	F := makeTestType1()
	g := F.Glyphs["A"]
	g.HStem = nil
	g.VStem = nil
	for i := range 60 {
		x := funit.Int16(100 * i)
		g.HStem = append(g.HStem, x, x+20)
		g.VStem = append(g.VStem, x, x+20)
	}

	cffFont, err := FromType1(F)
	if err != nil {
		t.Fatal(err)
	}
	out := findGlyph(t, cffFont, "A")

	nH := len(out.HStem) / 2
	nV := len(out.VStem) / 2
	if nH+nV != maxStemHints {
		t.Errorf("got %d+%d stems, want a total of %d", nH, nV, maxStemHints)
	}
}

// TestFromType1Quadratic checks that quadratic Bézier segments, which CFF
// cannot represent, are converted to cubic segments.
func TestFromType1Quadratic(t *testing.T) {
	F := makeTestType1()
	g := F.Glyphs["A"]
	g.Outline = &path.Data{}
	g.MoveTo(0, 0)
	g.Outline.QuadTo(vec.Vec2{X: 300, Y: 600}, vec.Vec2{X: 600, Y: 0})
	g.ClosePath()

	cffFont, err := FromType1(F)
	if err != nil {
		t.Fatal(err)
	}

	for _, out := range cffFont.Glyphs {
		if out.Name != "A" {
			continue
		}
		comparePaths(t, "A", g.Path().ToCubic(), out.Path())
	}
}

// TestFromType1NoPrivate checks that a missing private dictionary is
// replaced by one holding the CFF default values.
func TestFromType1NoPrivate(t *testing.T) {
	F := makeTestType1()
	F.Private = nil

	cffFont, err := FromType1(F)
	if err != nil {
		t.Fatal(err)
	}

	if len(cffFont.Private) != 1 {
		t.Fatalf("got %d private dicts, want 1", len(cffFont.Private))
	}
	want := &type1.PrivateDict{
		BlueScale: defaultBlueScale,
		BlueShift: defaultBlueShift,
		BlueFuzz:  defaultBlueFuzz,
	}
	if d := cmp.Diff(want, cffFont.Private[0]); d != "" {
		t.Errorf("private dict differs (-want +got):\n%s", d)
	}
}

// TestFromType1ZeroBlueScale checks that a private dictionary which leaves
// BlueScale unset gets the CFF default instead of a BlueScale of zero.
func TestFromType1ZeroBlueScale(t *testing.T) {
	F := makeTestType1()
	F.Private = &type1.PrivateDict{
		BlueValues: []funit.Int16{0, 10, 700, 710},
	}

	cffFont, err := FromType1(F)
	if err != nil {
		t.Fatal(err)
	}

	if got := cffFont.Private[0].BlueScale; got != defaultBlueScale {
		t.Errorf("got BlueScale %v, want %v", got, defaultBlueScale)
	}

	// the source font must not be modified
	if F.Private.BlueScale != 0 {
		t.Error("source private dict was modified")
	}
}

func TestFromType1WriteRead(t *testing.T) {
	F := makeTestType1()

	cffFont, err := FromType1(F)
	if err != nil {
		t.Fatal(err)
	}

	buf := &bytes.Buffer{}
	err = cffFont.Write(buf)
	if err != nil {
		t.Fatal(err)
	}

	out, err := Read(bytes.NewReader(buf.Bytes()), membudget.New(1<<26))
	if err != nil {
		t.Fatal(err)
	}

	if out.NumGlyphs() != cffFont.NumGlyphs() {
		t.Fatalf("got %d glyphs, want %d", out.NumGlyphs(), cffFont.NumGlyphs())
	}
	for gid, g := range out.Glyphs {
		want := cffFont.Glyphs[gid]
		if g.Name != want.Name {
			t.Errorf("glyph %d: got name %q, want %q", gid, g.Name, want.Name)
		}
		if g.Width != want.Width {
			t.Errorf("glyph %q: got width %v, want %v", want.Name, g.Width, want.Width)
		}
		if d := cmp.Diff(want.HStem, g.HStem); d != "" {
			t.Errorf("HStem of %q differs (-want +got):\n%s", want.Name, d)
		}
		if d := cmp.Diff(want.VStem, g.VStem); d != "" {
			t.Errorf("VStem of %q differs (-want +got):\n%s", want.Name, d)
		}
		comparePaths(t, want.Name, want.Path(), g.Path())

		// the outlines must also match the original Type 1 glyphs
		if orig, ok := F.Glyphs[want.Name]; ok {
			comparePaths(t, want.Name, orig.Path(), g.Path())
		}
	}

	if d := cmp.Diff(cffFont.Encoding, out.Encoding); d != "" {
		t.Errorf("encoding differs (-want +got):\n%s", d)
	}
	if d := cmp.Diff(cffFont.FontInfo, out.FontInfo); d != "" {
		t.Errorf("font info differs (-want +got):\n%s", d)
	}
	if d := cmp.Diff(cffFont.Private, out.Private); d != "" {
		t.Errorf("private dict differs (-want +got):\n%s", d)
	}
}

// FuzzFromType1 checks that every Type 1 font which FromType1 accepts
// converts to a CFF font which can be written, and that the written font
// reads back with the same content.
func FuzzFromType1(f *testing.F) {
	for _, format := range []type1.FileFormat{type1.FormatPFA, type1.FormatPFB} {
		buf := &bytes.Buffer{}
		err := makeTestType1().Write(buf, &type1.WriterOptions{Format: format})
		if err != nil {
			f.Fatal(err)
		}
		f.Add(buf.Bytes())
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		F, err := type1.Read(bytes.NewReader(data))
		if err != nil {
			return
		}
		c1, err := FromType1(F)
		if err != nil {
			return
		}

		// The Type 2 number encodings cover the range ±32767.99998, and
		// charstrings are limited to 65535 bytes.  Fonts with coordinates
		// or deltas outside this range, or with absurdly many segments,
		// are not usefully writable, so they are skipped here.
		const maxCoord = 16000
		const maxCmds = 2000
		for _, g := range c1.Glyphs {
			if math.Abs(g.Width) > maxCoord || len(g.Cmds) > maxCmds {
				return
			}
			for _, cmd := range g.Cmds {
				for _, x := range cmd.Args {
					if math.Abs(x) > maxCoord {
						return
					}
				}
			}
			for _, stems := range [][]float64{g.HStem, g.VStem} {
				for _, x := range stems {
					if math.Abs(x) > maxCoord {
						return
					}
				}
			}
		}

		buf := &bytes.Buffer{}
		if err := c1.Write(buf); err != nil {
			t.Fatal(err)
		}
		c2, err := Read(bytes.NewReader(buf.Bytes()), membudget.New(1<<26))
		if err != nil {
			t.Fatal(err)
		}

		cmpFDSelectFn := cmp.Comparer(func(fn1, fn2 FDSelectFn) bool {
			for gid := range c1.Glyphs {
				if fn1(glyph.ID(gid)) != fn2(glyph.ID(gid)) {
					return false
				}
			}
			return true
		})
		// Charstring numbers are quantised to 16.16 fixed point, and DICT
		// reals are written with nine significant digits.
		eqFloat := func(x, y float64) bool {
			maxVal := max(math.Abs(x), math.Abs(y))
			return math.Abs(x-y) <= max(1.0/65536, maxVal*1e-6)
		}
		cmpFloat := cmp.Comparer(eqFloat)
		cmpFunit := cmp.Comparer(func(x, y funit.Float64) bool {
			return eqFloat(float64(x), float64(y))
		})
		if d := cmp.Diff(c1, c2, cmpFDSelectFn, cmpFloat, cmpFunit); d != "" {
			t.Errorf("round trip failed (-want +got):\n%s", d)
		}
	})
}

func TestFromType1Errors(t *testing.T) {
	mm := makeTestType1()
	mm.MM = &type1.MMInfo{}

	// a font matrix which cannot be inverted does not describe a usable font
	zeroMatrix := makeTestType1()
	zeroMatrix.FontMatrix = matrix.Matrix{}

	collapsedMatrix := makeTestType1()
	collapsedMatrix.FontMatrix = matrix.Matrix{0.001, 0.001, 0.001, 0.001, 0, 0}

	nanMatrix := makeTestType1()
	nanMatrix.FontMatrix = matrix.Matrix{math.NaN(), 0, 0, math.NaN(), 0, 0}

	cases := []struct {
		name string
		font *type1.Font
	}{
		{"nil", nil},
		{"noFontInfo", &type1.Font{Outlines: &type1.Outlines{}}},
		{"noOutlines", &type1.Font{FontInfo: &type1.FontInfo{}}},
		{"multipleMaster", mm},
		{"zeroFontMatrix", zeroMatrix},
		{"collapsedFontMatrix", collapsedMatrix},
		{"nanFontMatrix", nanMatrix},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := FromType1(c.font)
			if err == nil {
				t.Error("expected an error")
			}
		})
	}
}
