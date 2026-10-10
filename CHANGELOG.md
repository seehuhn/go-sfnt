# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.7.5] (2026-10-10)

### Added
- Variable fonts: `Font.Instantiate` turns a glyf or CFF2 variable font
  into a static instance at given design coordinates, and
  `Font.IsVariable`, `VariationAxes` and `NamedInstances` describe the
  design space.  New packages `fvar`, `avar`, `gvar`, `cvar`, `hvar`,
  `mvar` and `stat` read and write the variation tables, on top of the
  shared `variation` package.
- `Font.Instantiate` applies the `gvar`, `cvar`, `HVAR` and `MVAR`
  deltas, resolves GSUB/GPOS `FeatureVariations`, turns GDEF/GPOS
  `VariationIndex` device tables into static values and derives the
  instance's PostScript name per Adobe TN #5902.  `Font.ConvertCFF2`
  prepares the outlines of a static CFF2 font for embedding.
- CFF2 support: reading and writing the `CFF2` table, a CFF2
  charstring interpreter, the `OutlinesCFF2`/`FontCFF2`/
  `PrivateCFF2`/`GlyphCFF2` data model, and
  `OutlinesCFF2.Instance`, which collapses blends at given axis
  coordinates into static CID-keyed CFF outlines.
- `gtab`: GSUB/GPOS `FeatureVariations` (format 1.1) are modelled
  (`Info.Variations`, `VariationsRaw`,
  `FeatureVariationRecord.Matches`) instead of being dropped on
  read; `gdef.Table` reads and writes the GDEF 1.3 item variation
  store (`ItemVarStore`).
- `cff.FromType1` converts a Type 1 font into a name-keyed CFF font.
- `Outlines.SetGlyphName` and `Outlines.Clone` (also on
  `OutlinesCFF2`) replace or clone glyphs without disturbing other
  fonts that share them.
- `Font.MaxFontNameLen` and `Font.CheckFontName` report the
  PostScript-name rule that applies to a font's outlines.
- `Font` exposes the OS/2 `UnicodeRange` bitfield alongside
  `CodePageRange`.
- `glyf.Glyphs.RecomputeCompositeBBoxes` recomputes composite glyph
  bounding boxes bottom-up.

### Changed
- `Font.PostScriptName()`, `Font.Subfamily()` and `Font.FullName()`
  are replaced by stored `FontName`, `Subfamily` and `FullName`
  fields, repaired on read, instead of a name derived from the
  family name and style flags; a derived name could not be told
  apart from one the font actually carried.
- `Font.Subset` now returns `(*Font, error)` instead of `*Font`: it
  rejects a variable font (`ErrVariableFont`, instantiate it first)
  and a font with CFF2 outlines, instead of silently producing a
  broken subset.
- `Font.FontBBox` and `Font.GlyphBBox` return `rect.Rect` in font
  design units instead of `funit.Rect16`, composing the target scale
  into the font matrix rather than rescaling afterwards; this fixes
  bounding boxes that previously rounded outward incorrectly for
  some `UnitsPerEm` values.  `Font.GlyphBBoxes` is removed in favour
  of the per-glyph accessor, and `GlyphMatrix` joins the `Outlines`
  interface.
- CFF and CFF2 writers reject fonts that exceed format limits (more
  than 65535 glyphs or glyph names, charstrings over 65535 bytes,
  more than 256 Font DICTs, out-of-range `FDSelect` indices, a
  name-keyed font without exactly one Private DICT) instead of
  writing broken output; the CFF2 Private DICT is repaired on read
  and checked against the same rule on write.

### Removed
- `Outlines.IsBlank` is removed from the `sfnt.Outlines` interface
  and its CFF, CFF2 and glyf implementations; callers can test an
  empty path themselves.

### Fixed
- TrueType outlines: a contour no longer loses its closing segment or
  curve segments for some point layouts, and implied on-curve points
  no longer wrap near the `int16` range.  A composite glyph which uses
  the same component twice draws it twice, and a component reference
  outside the font gives a blank glyph instead of a panic.  The
  `MORE_COMPONENTS` and `WE_HAVE_INSTRUCTIONS` flags are derived on
  write, and `ComponentFlag.String` no longer recurses infinitely on
  an unknown flag bit.
- Composite glyph bounding boxes are recomputed in time linear in the
  glyph count, instead of exponential in the nesting depth; clamping a `float64` to a
  fixed-width integer now maps `NaN` to zero on every platform.
- Subsetting no longer leaves an empty `cmap` table, drops vertical
  metrics, or leaves feature and lookup indices (including those inside
  `FeatureVariations`) unremapped in the compacted GSUB/GPOS
  lookup list.
- GPOS anchors of lookup types 4, 5 and 6 are pooled like devices,
  so a large font's anchor arrays stay within reach of an
  `Offset16`; lookup list encoding also checks each subtable's
  distance from its own lookup table, not only from the list start,
  so a subtable placed too far away is no longer truncated silently.
- `head.UnitsPerEm` outside 16-16384 is repaired from the font
  matrix on read (falling back to 1000) and refused on write, since
  an unusable value spreads infinities through every glyph metric.
- hhea and Windows OS/2 metric fallbacks are shared between `Read`
  and `Write`, so a round trip no longer changes them, and
  `Font.Instantiate` carries the MVAR ascender/descender/line-gap
  deltas over to the hhea fields when they already match the OS/2
  typographic metrics.
- `OutlinesCFF2.Path`/`IsBlank` (sharing a bug with the CFF1
  `Outlines.IsBlank`) no longer panic on a font with no glyphs, and
  `OutlinesCFF2.GlyphMatrix` no longer discards a single Font DICT's
  matrix when `FDSelect` is nil.
- A font whose `hhea` table cannot be read now gets zero advance
  widths on read instead of `nil`, so a read-write-read cycle
  agrees with itself.
- The `post` table is padded on read when it lists fewer glyph
  names than the font has glyphs; `MakeGlyphNames` ignores
  out-of-range glyph IDs from `cmap`/GSUB instead of panicking; and
  a CFF stem hint whose junction no operand can express is rejected
  instead of being silently clamped.
- `parser.ReadBytes` returns an error instead of panicking when the
  requested size exceeds its internal buffer.

## [v0.7.4] (2026-06-25)

### Added
- GPOS lookup type 5 (mark-to-ligature) positioning.
- Anchor table formats 2 and 3, GDEF list tables, and GPOS feature
  parameters.

### Changed
- `parser.New` and `parser.Read` now require an explicit memory budget.
- The `Font` struct was reorganised to group the font-unit metrics
  together.
- Advance widths are stored as unsigned values.

### Fixed
- GPOS lookup type 4 (mark-to-base) attachment is now positioned
  absolutely, as the specification requires.
- `WriteTrueTypePDF` preserves glyph names.
- Per-FD font matrices are composed when emitting `hmtx` advance
  widths and glyph bounding boxes.
- Reject `cmap` format 12 segments with out-of-range glyph IDs and
  table headers whose offset plus length overflows; guard
  `SeqContext2` against an out-of-range class index.

## [0.7.3] (2026-05-19)

### Added
- GSUB/GPOS subsetting covers the subtable types that previously
  panicked (Gsub1_2/2_1/3_1/8_1, every contextual subtable,
  Gpos1_2/2_2/3_1/4_1/5_1/6_1, GDEF), with round-trip and fuzz tests.
- New `opentype/device` package for Device and VariationIndex tables;
  `GposValueRecord` stores `*device.Table` directly, with content
  deduplication at encode time.
- `glyph.Info.YAdvance` field carries vertical advance from GPOS.

### Changed
- Memory-budget primitives (`Budget`, `AllocSlice`, `ErrBudgetExceeded`)
  moved to the new sibling module `seehuhn.de/go/membudget`.  Only the
  sfnt-specific `NewBudget` sizing helper remains in the parser
  package; `Parser.Budget` is retyped to `*membudget.Budget`,
  initialised by `parser.New` to a sized default, and may no longer
  be nil.
- GSUB type 8 (Reverse Chaining Contextual Single Substitution) is
  now applied right-to-left as required by the OpenType spec; the
  driver dispatches forward vs reverse based on the first subtable
  type.

### Fixed
- CFF charstring decoder charges the parser budget for each
  charstring body and each `callsubr`/`callgsubr` invocation, so
  high-fan-out subroutine bombs trip `ErrExceeded` before they can
  amplify into ~10^15 calls.
- GSUB 4.1 ligature retries no longer alias the match and skip
  position slices to the same backing array; ligature loops with
  multiple alternatives now produce the correct output.
- Subset encoder no longer silently truncates offsets when a
  subtable exceeds 64 KiB; it panics with a clear message instead.
- `scriptlist`: `uint16` overflow in `defaultLangSysOffset` bounds.

## [0.7.2] (2026-05-11)

### Changed
- Minimum Go version raised to 1.25.

### Fixed
- GSUB/GPOS/GDEF readers now enforce a per-table memory budget, rejecting hostile fonts that alias offsets to amplify allocations from tiny inputs.
- CFF reader bounds the Private DICT size against the remaining file size and uses 64-bit arithmetic for INDEX offsets, preventing oversized allocations and overflow on malformed input.
- `glyf` simple glyphs with non-monotonic `endPtsOfContours` are rejected.
- GPOS 4/5/6 mark records with out-of-range class indices are rejected.
- GSUB 4 ligatures with zero components and ChainedSeqContext1/2 entries with zero input glyphs are rejected.
- GSUB 2/3 silently drop empty replacement and alternate entries on read, matching HarfBuzz and macOS behaviour; the GSUB 3 builder rejects empty alternates.

## [0.7.1] (2026-03-31)

### Changed
- `GlyphBBox` uses `vec.Vec2` for the updated `Matrix.Apply` signature.

### Fixed
- GPOS 5.1 reader corrected; malformed GDEF/GSUB/GPOS/kern tables are now skipped instead of causing errors.
- Malformed cmap subtables are skipped instead of rejecting the whole table.

## [0.7.0] (2025-01-25)

### Changed
- **API Change**: Path() methods in CFF and TrueType glyph outlines now use `vec.Vec2` instead of `path.Point` for point coordinates in the iterator function signature.
- Updated dependencies including seehuhn.de/go/geom and seehuhn.de/go/postscript packages.

### Fixed
- Stem hint encoding now uses actually encoded (rounded) values as base for subsequent deltas, ensuring round-trip consistency.
- Several fuzzing failures addressed.

## [0.6.0] (2025-06-30)

### Added
- New Path() and IsBlank() methods to font outline interfaces for glyph outline processing and content checking
- Comprehensive composite glyph support with ComponentUnpacked struct for programmatic TrueType glyph creation and manipulation
- Font subsetting capabilities with new cff.Outlines.Subset() method for creating font subsets
- GlyphBBox() method for computing glyph bounding boxes with arbitrary matrix transformations
- Font conversion utilities (MakeSimple, MakeCIDKeyed) for converting between simple and CID-keyed CFF fonts

### Changed
- **API Change**: Replaced dijkstra package with dag package for shortest path algorithms in CFF encoding and character mapping components
- **API Change**: Renamed GlyphInfo to SimpleUnpacked and Decode/Encode methods to Unpack/Pack for consistency
- **API Change**: Changed Glyphs.Path() return type from path.Compound to path.Path, aligning with geom package updates
- **API Change**: Font.GlyphWidthPDF() now returns values in PDF glyph space units (increased by factor of 1000)
- **API Change**: ComponentUnpacked.OurPoint and TheirPoint fields changed from int16 to uint16 for TrueType point indices
- **API Change**: Font.Subset() no longer returns an error result for consistency with cff.Font.Subset()
- Updated dependencies including seehuhn.de/go/geom and seehuhn.de/go/postscript packages

### Removed
- **API Change**: Deprecated BBox() method from Outlines type, replaced with improved GlyphBBox functionality
- **API Change**: Font.AsCFF() now returns nil instead of panicking when font doesn't contain CFF outlines

### Fixed
- Correctly map components when subsetting fonts with composite glyphs
- Improved composite glyph point matching and alignment calculations in glyf package
- Enhanced bounds checking in glyf package with fallback to .notdef glyph for invalid indices
- Minor bug in CFF width selection found by fuzzer testing

## [v0.5.0] (2024-05-17)

### Added
- GitHub Actions CI workflow for automated testing
- Contributing guidelines documentation

### Changed
- Updated dependency seehuhn.de/go/postscript to v0.5.0
- Updated Go version requirement to 1.22.2
- Updated golang.org/x/exp dependency
- Improved README documentation with badges and copyright information

### Removed
- Unused fuzz test data files
- OpenSSF Scorecard badge from README

### Fixed
- Integer overflow vulnerabilities in GTAB parser (closes #2, #3, #4, #5)
- Documentation typos in comments

### Security
- Fixed integer overflow vulnerabilities that could potentially be exploited in GTAB parsing
