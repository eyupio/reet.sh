#!/usr/bin/env python3
"""Generates every reet.sh logo variant into brand/logo/.

Run `python3 brand/generate.py` from the repo root. Letters are drawn as
paths (no fonts), so the SVGs render identically everywhere.

The mark: the Yorkshire white rose ("reet" is Yorkshire for right / proper)
knocked out of a bookmark ribbon (the site bookmarks AI skills), with an
accent dot at the centre.
"""
from __future__ import annotations

import math
from pathlib import Path

OUT = Path(__file__).resolve().parent / "logo"
TITLE = "reet.sh"

# Palette: matches web/src/App.svelte CSS variables.
INK_L, ACCENT_L = "#1d1c1a", "#c2410c"
INK_D, ACCENT_D = "#efece6", "#fb923c"
SCHEMES = {
    "color-light": dict(ink=INK_L, accent=ACCENT_L),     # transparent, for light backgrounds
    "color-dark": dict(ink=INK_D, accent=ACCENT_D),      # transparent, for dark backgrounds
    "mono-dark": dict(ink=INK_L, accent=INK_L),          # one colour, for light backgrounds
    "mono-light": dict(ink="#ffffff", accent="#ffffff"),  # one colour, for dark backgrounds
}

# ---------------------------------------------------------------- wordmark
# Monoline geometric lowercase. Units: x-height 100, ascender 150, stroke 24.
W = 24
GAP = 16


def _pt(cx, cy, r, deg):
    return cx + r * math.cos(math.radians(deg)), cy + r * math.sin(math.radians(deg))


def glyph_r(pen):
    x = pen + 12
    return f"M{x},150V86A36,36 0 0 1 {x+36},50H{x+44}", 68


def glyph_e(pen):
    R, cx = 42, pen + 54
    ex, ey = _pt(cx, 100, R, 45)
    return f"M{cx-R},100H{cx+R}A{R},{R} 0 1 0 {ex:.1f},{ey:.1f}", 108


def glyph_t(pen):
    xs = pen + 38
    return f"M{xs},22V126A24,24 0 0 0 {xs+24},150H{xs+34} M{xs-26},50H{xs+28}", 84


def glyph_s(pen):
    # Open bowls and a continuous waist: the old large arcs nearly closed into
    # two circles once the round stroke caps were applied at small sizes.
    return (f"M{pen+54},62C{pen+42},49 {pen+14},47 {pen+14},74"
            f"C{pen+14},101 {pen+52},96 {pen+52},124"
            f"C{pen+52},151 {pen+26},156 {pen+12},141"), 66


def glyph_h(pen):
    x = pen + 12
    return f"M{x},0V150M{x},94A44,44 0 0 1 {x+88},94V150", 112


def wordmark(ink, accent):
    """Returns (inner svg, width, height) for 'reet.sh' in native units."""
    stroke = f'fill="none" stroke-width="{W}" stroke-linecap="round" stroke-linejoin="round"'
    ink_paths, acc_paths, pen = [], [], 0
    d, adv = glyph_r(pen); ink_paths.append(d); pen += adv + GAP - 6
    d, adv = glyph_e(pen); ink_paths.append(d); pen += adv + GAP
    d, adv = glyph_e(pen); ink_paths.append(d); pen += adv + GAP
    d, adv = glyph_t(pen); ink_paths.append(d); pen += adv + GAP - 4
    dot_x = pen + 12; pen += 24 + GAP - 2
    d, adv = glyph_s(pen); acc_paths.append(d); pen += adv + GAP
    d, adv = glyph_h(pen); acc_paths.append(d); pen += adv
    width = pen + 12
    g = (f'<g id="wordmark" {stroke}>'
         f'<path stroke="{ink}" d="{" ".join(ink_paths)}"/>'
         f'<circle fill="{accent}" cx="{dot_x}" cy="138" r="13"/>'
         f'<path stroke="{accent}" d="{" ".join(acc_paths)}"/></g>')
    return g, width, 162


# -------------------------------------------------------------------- icon
# The icon lives in a 0..100 box. `uid` keeps mask ids unique when several
# icons are inlined on one page.

RIBBON = "M24,10H76V90L50,70L24,90Z"
ROSE_PETAL = "M-9,-12H9L19,-45L0,-34L-19,-45Z"
ROSE_CX, ROSE_CY, ROSE_SCALE = 50, 41, 0.44


def icon(ink, accent, uid):
    petals = "".join(f'<path d="{ROSE_PETAL}" transform="rotate({72*k})" fill="#000" stroke="#000" '
                     f'stroke-width="5" stroke-linejoin="round"/>' for k in range(5))
    cutout = (f'<g transform="translate({ROSE_CX},{ROSE_CY}) scale({ROSE_SCALE})">{petals}'
              f'<circle r="13" fill="#000"/><circle r="7.5" fill="#fff"/></g>')
    return (
        f'<mask id="rose-{uid}"><rect width="100" height="100" fill="#fff"/>{cutout}</mask>'
        f'<path mask="url(#rose-{uid})" fill="{ink}" stroke="{ink}" stroke-width="6" '
        f'stroke-linejoin="round" d="{RIBBON}"/>'
        f'<circle cx="{ROSE_CX}" cy="{ROSE_CY}" r="{7.5*ROSE_SCALE:.1f}" fill="{accent}"/>'
    )


# ------------------------------------------------------------------ layouts
def svg(vb_w, vb_h, desc, inner):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {vb_w} {vb_h}" role="img" '
            f'aria-labelledby="t d">\n<title id="t">{TITLE}</title>\n<desc id="d">{desc}</desc>\n'
            f'{inner}\n</svg>\n')


def layout_horizontal(ink, accent, uid):
    wm, w, _ = wordmark(ink, accent)
    s = 0.42
    tw = int(118 + w * s) + 2
    inner = (f'<g id="icon">{icon(ink, accent, uid)}</g>\n'
             f'<g transform="translate(118,{(100 - 150*s)/2 + 2:.1f}) scale({s})">{wm}</g>')
    return svg(tw, 100, "Yorkshire rose in a bookmark, with the reet.sh wordmark to the right.", inner)


def layout_vertical(ink, accent, uid):
    wm, w, _ = wordmark(ink, accent)
    s = 0.33
    tw = max(100, int(w * s)) + 24
    inner = (f'<g id="icon" transform="translate({(tw-100)/2:.1f},0)">{icon(ink, accent, uid)}</g>\n'
             f'<g transform="translate({(tw - w*s)/2:.1f},116) scale({s})">{wm}</g>')
    return svg(tw, 176, "Yorkshire rose in a bookmark, with the reet.sh wordmark below.", inner)


def layout_icon(ink, accent, uid):
    return svg(100, 100, "Yorkshire rose in a bookmark.", f'<g id="icon">{icon(ink, accent, uid)}</g>')


def layout_tile(ink, accent, tile_bg, uid):
    inner = (f'<rect width="100" height="100" rx="22" fill="{tile_bg}"/>\n'
             f'<g transform="translate(14,14) scale(0.72)">{icon(ink, accent, uid)}</g>')
    return svg(100, 100, "Yorkshire rose in a bookmark, on a rounded tile.", inner)


def layout_wordmark(ink, accent):
    wm, w, _ = wordmark(ink, accent)
    return svg(w + 24, 186, "The reet.sh wordmark.", f'<g transform="translate(12,24)">{wm}</g>')


def main():
    OUT.mkdir(exist_ok=True)
    for old in OUT.glob("*.svg"):
        old.unlink()
    files = []
    for scheme, col in SCHEMES.items():
        ink, accent = col["ink"], col["accent"]
        for layout, body in {
            "horizontal": layout_horizontal(ink, accent, scheme),
            "vertical": layout_vertical(ink, accent, scheme),
            "icon": layout_icon(ink, accent, scheme),
            "wordmark": layout_wordmark(ink, accent),
        }.items():
            p = OUT / f"{layout}-{scheme}.svg"
            p.write_text(body)
            files.append(p)
    # Tiles: ink tile with cream marks, accent tile, and a black mono tile.
    for name, (ink, accent, bg) in {
        "tile-ink": (INK_D, ACCENT_D, INK_L),
        "tile-accent": ("#ffffff", "#ffffff", ACCENT_L),
        "tile-mono": ("#ffffff", "#ffffff", "#000000"),
    }.items():
        p = OUT / f"{name}.svg"
        p.write_text(layout_tile(ink, accent, bg, name))
        files.append(p)
    # Inline this generated asset in the app so geometry has one source of truth.
    themed = OUT.parent.parent / "web" / "src" / "lib" / "logo.svg"
    themed.write_text(layout_horizontal("var(--ink, #1d1c1a)", "var(--accent, #c2410c)", "theme"))
    files.append(themed)
    print(f"wrote {len(files)} logo assets")


if __name__ == "__main__":
    main()
