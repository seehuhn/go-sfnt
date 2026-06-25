# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
