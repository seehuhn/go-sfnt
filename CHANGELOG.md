# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
