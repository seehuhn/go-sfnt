# go-sfnt — known issues and follow-ups

## Metadata lost or altered when a font is read and written back

Observed on 2026-11-10 by reading Liberation Sans, Serif and Mono 2.1.5,
subsetting them and writing them back, then comparing the result with
the originals.  None of the rows has been traced to its cause yet.  For
each, find out whether the writer drops the value, recomputes it on
purpose, or computes it wrongly; fix what is wrong, and document what is
intended.  Fonts built with this package need rebuilding afterwards.

| Field | Original | Written | Note |
|---|---|---|---|
| OS/2 `yStrikeoutPosition`, `yStrikeoutSize` | 530, 102 | 0, 0 | strike-through cannot be drawn from the font |
| OS/2 `ySubscript*`, `ySuperscript*` | set | all 0 | |
| OS/2 `panose` | set | all 0 | |
| OS/2 `sFamilyClass` | Sans 2053, Serif 261 | Sans 0, Serif 768 | Serif gets a different class, not zero |
| OS/2 `usMaxContext` | 44 (Sans), 14 (Serif, Mono) | 0 | should cover the longest GSUB/GPOS context |
| OS/2 `usBreakChar` | 32 | 0 | |
| OS/2 `fsSelection` bit 7 (USE_TYPO_METRICS) | clear | set | changes which line metrics a consumer uses |
| `head` `flags` | 0x001F | 0x380B | clears bits 2 and 4, which describe the hinting, although `fpgm`, `prep` and `cvt` are kept; sets bit 13 (optimised for ClearType) |
| `head` `lowestRecPPEM` | 8 | 7 | |
| `hhea` `caretSlopeRise`, `caretSlopeRun` (Serif Italic) | 100, 29 | 30572, 8959 | slightly different angle |
| `hhea` `minRightSideBearing` | -1414 | -987 | equals `minLeftSideBearing` in Sans, and Mono Bold has the same value; check the computation |

The legacy `kern` table and the `FFTM` table are dropped as well.  The
kerning is still present in `GPOS`.

## Vertical metrics of variable fonts

`Font.Vertical` holds the vertical metrics of the default instance.
Nothing varies them yet, so `Instantiate` and `ConvertCFF2` carry the
default advance heights and vertical origins over to every instance,
and the top side bearings written for an instance are measured from its
varied glyph tops.  Closing this gap needs:

- the `VVAR` table (advance heights, top and bottom side bearings,
  vertical origins), read, written and applied by `Instantiate`;
- the `MVAR` tags `vasc`, `vdsc`, `vlgp`, `vcrs`, `vcrr` and `vcof`,
  applied to the `vhea` values;
- the vertical phantom points of `gvar`, which vary the advance height
  and top side bearing of glyf fonts without `VVAR`.

The `BASE` table, which holds baselines for vertical text, is not read
either.
