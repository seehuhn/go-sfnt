package sfnt

import (
	"fmt"
	"testing"

	"seehuhn.de/go/sfnt/cff"
	"seehuhn.de/go/sfnt/funit"
	"seehuhn.de/go/sfnt/glyph"
	"seehuhn.de/go/sfnt/type1"
)

// TestFDSelect tests the FDSelect function in subsetted fonts is correct.
func TestFDSelect(t *testing.T) {
	// Construct a CID-keyed CFF font with several FDs.
	o1 := &cff.Outlines{}
	for i := 0; i < 10; i++ {
		g := cff.NewGlyph(fmt.Sprintf("orig%d", i), funit.Int16(100*i))
		g.MoveTo(0, 0)
		g.LineTo(100*float64(i), 0)
		g.LineTo(100*float64(i), 500)
		g.LineTo(0, 500)
		o1.Glyphs = append(o1.Glyphs, g)
		o1.Private = append(o1.Private, &type1.PrivateDict{
			StdHW: float64(10*i + 1),
		})
	}
	o1.FDSelect = func(gid glyph.ID) int {
		return int(gid) % 10
	}
	o1.ROS = &type1.CIDSystemInfo{
		Registry:   "Adobe",
		Ordering:   "Identity",
		Supplement: 0,
	}
	o1.Gid2Cid = make([]type1.CID, 10)
	for i := 0; i < 10; i++ {
		o1.Gid2Cid[i] = type1.CID(i)
	}
	i1 := &Info{
		FamilyName: "Test",
		UnitsPerEm: 1000,
		Outlines:   o1,
	}

	ss := []SubsetGlyph{
		{OrigGID: 0, CID: 0},
		{OrigGID: 3, CID: 3},
		{OrigGID: 5, CID: 5},
		{OrigGID: 4, CID: 4},
	}
	i2, err := i1.SubsetCID(ss, o1.ROS)
	if err != nil {
		t.Fatal(err)
	}
	o2 := i2.Outlines.(*cff.Outlines)

	if len(o2.Private) != len(ss) {
		t.Fatalf("expected %d FDs, got %d", len(ss), len(o2.Private))
	}
	if len(o2.Gid2Cid) != len(ss) {
		t.Fatalf("expected %d Gid2Cid entries, got %d", len(ss), len(o2.Gid2Cid))
	}
	for i, info := range []*Info{i1, i2} {
		o := info.Outlines.(*cff.Outlines)
		for gidInt, cid := range o.Gid2Cid {
			gid := glyph.ID(gidInt)
			w := info.GlyphWidth(gid)
			if w != funit.Int16(100*int(cid)) {
				t.Errorf("%d: wrong glyph %s@%d, expected width %d, got %d",
					i, o.Glyphs[gid].Name, gid, 100*int(cid), w)
				continue
			}
			fd := o.Private[o.FDSelect(gid)]
			if fd.StdHW != float64(10*int(cid)+1) {
				t.Errorf("%d: wrong FD for glyph %s@%d, expected StdHW %d, got %f",
					i, o.Glyphs[gid].Name, gid, 10*int(cid)+1, fd.StdHW)
			}
		}
	}
}
