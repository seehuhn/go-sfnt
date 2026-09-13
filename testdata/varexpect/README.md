# varexpect fixtures

This directory holds the inputs and the recorded answers for `TestVarExpect`
(`../../varexpect_test.go`), which checks `Font.Instantiate` against
fontTools' `varLib.instancer`.

Each `*.json` file records, for one source font and several pinned coordinate
sets, per-glyph advance widths, per-glyph outlines, and selected OS/2 metrics,
as computed by fontTools.  Outlines are recorded per glyph flavour: a glyf font
stores raw (un-flattened, un-implied-point) outline points, a CFF/CFF2 font the
drawing segments a pen is handed, since charstrings do not decompose into a
comparable per-contour point dump.  Both are compared coordinate by
coordinate.

The two fonts are built to reach the parts of their formats where instancing
can go wrong.  `glyf.ttf` carries a subset tuple needing IUP, a composite
glyph, an intermediate region, a two-tuple glyph, and phantom points which
repeat what HVAR says.  `cff2.otf` carries two axes, two item variation data
subtables of different width, an intermediate region which stays at zero over
the lower half of the wght axis, two Font DICTs, blended stem hints behind a
hintmask, cubic curves, and a glyph of two subpaths.

One thing the CFF2 spec allows is deliberately left out of `cff2.otf`, since
fontTools cannot read it: a charstring which relies on the `vsindex` of its
Font DICT's Private DICT instead of naming a subtable itself.  fontTools reads
charstrings assuming subtable 0 until an explicit operator says otherwise, so
such a glyph would leave the oracle unable to read this font at all.  That path
is covered by the `cff` package instead, in `TestReadCFF2PrivateVSIndex` and
`TestWriteCFF2InheritedVSIndex`.

The source fonts (`glyf.ttf`, `cff2.otf`) are committed alongside.  They are
built by `internal/debug`, but they are also the input fontTools computed its
answers from, so leaving them out would let a change to the font writer
silently redefine what the recorded answers describe.  `TestVarExpectSourceFonts`
checks that the builders still produce these exact bytes, and `source_sha256`
pins the file each JSON was generated from.

Regenerate with (requires `fontTools` installed; the tests themselves do not
need it):

```sh
go generate ./...
```

Regeneration is byte-stable: with unchanged builders, libraries and fontTools,
it leaves the working tree alone, and any diff it produces reflects a real
change.  The recorded `fonttools_version` says which fontTools produced the
current answers.
