#!/usr/bin/env python3
# seehuhn.de/go/sfnt - a library for reading and writing font files
# Copyright (C) 2026  Jochen Voss <voss@seehuhn.de>
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <https://www.gnu.org/licenses/>.

"""instancer.py - fontTools ground truth for variable-font instancing.

Pins the axes of a variable font at one or more coordinate sets using
fontTools' varLib.instancer, and dumps per-glyph outline points, advance
widths, and selected font-wide metrics to a JSON file.  The output is
committed as testdata for TestVarExpect (see ../../varexpect_test.go),
which compares Font.Instantiate against these values without invoking
Python at test time.

This script is not run directly: "go generate ./..." runs the driver in
this directory, which builds the source fonts and invokes it.  It needs
fontTools (`python3 -m pip install --user fonttools`).

Usage:
    python3 instancer.py FONT.ttf -o OUTPUT.json \\
        --case "NAME:tag=val,tag=val,..." [--case ...] \\
        --glyph NAME [--glyph NAME ...]

A --case with no coordinates after the colon (e.g. "defaults:") pins every
axis at its fvar default.  Axes omitted from a case's coordinate list are
also pinned at their fvar default, matching the semantics of
Font.Instantiate.
"""

import argparse
import hashlib
import io
import json
import os
import re
import sys

try:
    import fontTools
    from fontTools.ttLib import TTFont
    from fontTools.varLib.instancer import instantiateVariableFont
    from fontTools.pens.recordingPen import RecordingPen
except ImportError:
    sys.exit(
        "fontTools not installed; run: python3 -m pip install --user fonttools"
    )


def parse_case(spec):
    """Parse "NAME:tag=val,tag=val,..." into (name, {tag: val})."""
    name, sep, coordspec = spec.partition(":")
    if not sep:
        sys.exit(f"--case {spec!r}: missing ':' separating name from coordinates")
    coords = {}
    if coordspec:
        for part in coordspec.split(","):
            tag, eq, val = part.partition("=")
            if not eq:
                sys.exit(f"--case {spec!r}: bad coordinate {part!r}, want tag=value")
            coords[tag] = float(val)
    return name, coords


def glyph_contours(glyf, glyph_name):
    """Return a glyph's raw outline points, one list of [x, y, on_curve] per
    contour.

    Composite glyphs are decomposed by recursively resolving their
    components (translation and 2x2 transform) via getCoordinates(), without
    inserting the on-curve points TrueType rasterizers imply between
    consecutive off-curve points.  This mirrors the raw points stored in the
    glyf table, structurally matching glyf.SimpleUnpacked.Contours on the Go
    side once a Go-side composite decomposes the same way.
    """
    coords, end_pts, flags = glyf[glyph_name].getCoordinates(glyf)
    contours = []
    start = 0
    for end in end_pts:
        contour = []
        for i in range(start, end + 1):
            x, y = coords[i]
            on_curve = bool(flags[i] & 0x1)
            contour.append([round(x, 2), round(y, 2), on_curve])
        contours.append(contour)
        start = end + 1
    return contours


def glyph_segments(glyphset, glyph_name):
    """Return a glyph's outline as a flat list of drawing segments, each of the
    form [op, x, y, ...].

    Used for CFF/CFF2 outlines, whose cubic charstrings do not decompose into
    the raw on/off-curve point lists that glyf tables expose.  The segments are
    what a pen is handed, so they line up one for one with the path commands
    seehuhn.de/go/sfnt's cff.Outlines.Path() yields, coordinate for
    coordinate.  A bounding box would not: it hides everything inside the
    outline, and fontTools computes the true extrema of a curve where the Go
    side reports the control-point box.
    """
    pen = RecordingPen()
    glyphset[glyph_name].draw(pen)
    segments = []
    for op, points in pen.value:
        seg = [op]
        for x, y in points:
            seg.append(round(x, 2))
            seg.append(round(y, 2))
        segments.append(seg)
    return segments


def instance_data(font_bytes, sparse_coords, glyph_names):
    """Pin every axis (sparse_coords, defaulted for the rest) and return
    per-glyph outline/advance data plus font-wide metrics for glyph_names."""
    f = TTFont(io.BytesIO(font_bytes))
    fvar = f["fvar"]

    unknown = set(sparse_coords) - {ax.axisTag for ax in fvar.axes}
    if unknown:
        sys.exit(f"unknown axis tag(s): {sorted(unknown)}")

    # every axis must be pinned explicitly, or fontTools leaves omitted axes
    # variable (partial instancing) instead of defaulting them the way
    # Font.Instantiate does.
    full_coords = {
        ax.axisTag: sparse_coords.get(ax.axisTag, ax.defaultValue) for ax in fvar.axes
    }

    inst = instantiateVariableFont(f, full_coords, inplace=False)

    # round-trip through the binary format: fontTools keeps glyf coordinates,
    # CFF charstrings and hmtx advances as un-rounded floats in memory until
    # compile() packs them onto the integer grid, applying the same
    # otRound treatment a real font file (and seehuhn.de/go/sfnt) would use.
    buf = io.BytesIO()
    inst.save(buf)
    buf.seek(0)
    inst = TTFont(buf)

    hmtx = inst["hmtx"]

    # glyf fonts report raw outline points; CFF/CFF2 fonts report the drawing
    # segments instead (charstrings do not expose a point-per-contour dump
    # comparable to glyf).
    glyf = inst.get("glyf")
    glyphset = None if glyf is not None else inst.getGlyphSet()

    glyph_data = {}
    for name in glyph_names:
        if name not in inst.getGlyphOrder():
            sys.exit(f"glyph {name!r} not found in font")
        width, _lsb = hmtx[name]
        entry = {
            "gid": inst.getGlyphID(name),
            "advance_width": width,
        }
        if glyf is not None:
            entry["contours"] = glyph_contours(glyf, name)
        else:
            entry["segments"] = glyph_segments(glyphset, name)
        glyph_data[name] = entry

    # Font.Ascent/Descent (seehuhn.de/go/sfnt) are read from the OS/2 typo
    # metrics, which is also where the MVAR "hasc"/"hdsc" tags land; hhea's
    # own ascent/descent are a separate (usually mirrored) pair of fields
    # that MVAR does not touch.
    os2 = inst.get("OS/2")
    metrics = {
        "ascent": getattr(os2, "sTypoAscender", None) if os2 else None,
        "descent": getattr(os2, "sTypoDescender", None) if os2 else None,
        "cap_height": getattr(os2, "sCapHeight", None) if os2 else None,
    }

    return glyph_data, metrics


# POINT_RE matches one [x, y, onCurve] outline point and SEGMENT_RE one
# ["op", x, y, ...] drawing segment, spread over several lines by json.dump's
# indentation.
POINT_RE = re.compile(r"\[\s*(-?\d+),\s*(-?\d+),\s*(true|false)\s*\]")
SEGMENT_RE = re.compile(r'\[\s*("[A-Za-z]+"(?:,\s*-?\d+)*)\s*\]')


def dumps_compact_points(doc):
    """Serialize doc with indent=2, keeping each point and segment on one line.

    The values are what a reviewer compares when the ground truth changes, and
    one line per point makes a moved point a one-line diff.
    """
    out = json.dumps(doc, indent=2, sort_keys=True)
    out = POINT_RE.sub(r"[\1, \2, \3]", out)
    return SEGMENT_RE.sub(lambda m: "[" + " ".join(m.group(1).split()) + "]", out)


def main():
    p = argparse.ArgumentParser(
        description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter
    )
    p.add_argument("font", help="path to a variable TrueType font")
    p.add_argument("-o", "--out", required=True, help="output JSON path")
    p.add_argument(
        "--case",
        action="append",
        required=True,
        metavar="NAME:TAG=VAL,...",
        help="a coordinate set to instance at; may be repeated",
    )
    p.add_argument(
        "--glyph",
        action="append",
        required=True,
        dest="glyphs",
        help="glyph name to record; may be repeated",
    )
    args = p.parse_args()

    with open(args.font, "rb") as fh:
        font_bytes = fh.read()
    source_sha256 = hashlib.sha256(font_bytes).hexdigest()

    cases = []
    for spec in args.case:
        name, coords = parse_case(spec)
        glyph_data, metrics = instance_data(font_bytes, coords, args.glyphs)
        cases.append(
            {
                "name": name,
                "coords": coords,
                "glyphs": glyph_data,
                "metrics": metrics,
            }
        )

    doc = {
        "fonttools_version": fontTools.version,
        "source_font": os.path.basename(args.font),
        "source_sha256": source_sha256,
        "cases": cases,
    }

    with open(args.out, "w") as fh:
        fh.write(dumps_compact_points(doc))
        fh.write("\n")


if __name__ == "__main__":
    main()
