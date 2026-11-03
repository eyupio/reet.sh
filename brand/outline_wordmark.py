#!/usr/bin/env python3
"""Outlines the "reet.sh" wordmark from IBM Plex Sans SemiBold into brand/generate.py.

Developer tool: run it only when the wordmark's typeface, weight or tracking changes.

    pip install fonttools brotli uharfbuzz
    python3 brand/outline_wordmark.py
    python3 brand/generate.py

It shapes the text with HarfBuzz (so the font's kerning is honoured), draws every glyph as an SVG path
in units where the font's x-height is 100, and rewrites the block between the BEGIN/END markers in
generate.py. generate.py itself needs no font and no third-party packages.
"""
from __future__ import annotations

import io
import re
from pathlib import Path

import uharfbuzz as hb
from fontTools.pens.boundsPen import BoundsPen
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.transformPen import TransformPen
from fontTools.ttLib import TTFont

HERE = Path(__file__).resolve().parent
FONT = HERE.parent / "web" / "public" / "fonts" / "ibm-plex-sans-latin-600-normal.woff2"
TARGET = HERE / "generate.py"

TEXT = "reet.sh"
INK_GLYPHS = 4          # "reet" is drawn in ink, ".sh" in the accent
TRACKING = -2.0         # extra space between letters, in x-height / 100 units
XHEIGHT = 100.0


def num(v: float) -> str:
    return ("%.2f" % v).rstrip("0").rstrip(".")


def main() -> None:
    font = TTFont(str(FONT))
    if font.flavor:  # woff2 -> plain sfnt, which is what HarfBuzz reads
        font.flavor = None
        buf = io.BytesIO()
        font.save(buf)
        data = buf.getvalue()
        font = TTFont(io.BytesIO(data))
    else:
        data = FONT.read_bytes()

    glyphs = font.getGlyphSet()
    order = font.getGlyphOrder()
    scale = XHEIGHT / font["OS/2"].sxHeight

    hbfont = hb.Font(hb.Face(data))
    shaped = hb.Buffer()
    shaped.add_str(TEXT)
    shaped.guess_segment_properties()
    hb.shape(hbfont, shaped, {"kern": True, "liga": False})

    # Pass 1: pen positions and the ink's bounding box.
    placed, x = [], 0.0
    xmin, xmax, top, bottom = 1e9, -1e9, 0.0, 0.0
    for info, pos in zip(shaped.glyph_infos, shaped.glyph_positions):
        name = order[info.codepoint]
        ox = x + pos.x_offset * scale
        placed.append((name, ox))
        bp = BoundsPen(glyphs)
        glyphs[name].draw(TransformPen(bp, (scale, 0, 0, scale, ox, 0)))
        if bp.bounds:
            xmin, xmax = min(xmin, bp.bounds[0]), max(xmax, bp.bounds[2])
            bottom, top = min(bottom, bp.bounds[1]), max(top, bp.bounds[3])
        x += pos.x_advance * scale + TRACKING

    # Pass 2: draw with the origin at the top-left of the ink, y pointing down.
    parts = []
    for name, ox in placed:
        pen = SVGPathPen(glyphs, ntos=num)
        glyphs[name].draw(TransformPen(pen, (scale, 0, 0, -scale, ox - xmin, top)))
        parts.append(pen.getCommands())

    block = (
        "# BEGIN wordmark paths (written by outline_wordmark.py; do not edit by hand)\n"
        f'WM_INK = "{" ".join(parts[:INK_GLYPHS])}"\n'
        f'WM_ACCENT = "{" ".join(parts[INK_GLYPHS:])}"\n'
        f"WM_W, WM_H, WM_BASE = {num(xmax - xmin)}, {num(top - bottom)}, {num(top)}\n"
        "# END wordmark paths"
    )
    src = TARGET.read_text()
    out, n = re.subn(r"# BEGIN wordmark paths.*?# END wordmark paths", lambda _: block, src, flags=re.S)
    if n != 1:
        raise SystemExit("could not find the BEGIN/END wordmark markers in generate.py")
    TARGET.write_text(out)
    print(f"wordmark: {num(xmax - xmin)} x {num(top - bottom)} units, baseline at y={num(top)}")


if __name__ == "__main__":
    main()
