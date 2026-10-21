#!/usr/bin/env python3
"""Generates every reet.sh logo variant in this folder.

Run `python3 brand/generate.py` from the repo root. Letters are drawn as
paths (no fonts), so the SVGs render identically everywhere.

"reet" is Yorkshire for right / proper. The site bookmarks AI skills, so
every concept combines a bookmark with a tick, the Yorkshire white rose or
an AI sparkle.
"""
from __future__ import annotations

import math
from pathlib import Path

OUT = Path(__file__).resolve().parent

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


def glyph_t_tick(pen):
    """A t whose tail is the long stroke of a tick: 'reet' as in right."""
    xs = pen + 38
    stem = f"M{xs-26},50H{xs+28} M{xs},22V112"
    tick = f"M{xs},112L{xs+15},134L{xs+52},66"
    return stem, tick, 100


def glyph_s(pen):
    r, cx = 21, pen + 33
    sx, sy = _pt(cx, 79, r, -25)
    ex, ey = _pt(cx, 121, r, 155)
    return f"M{sx:.1f},{sy:.1f}A{r},{r} 0 1 0 {cx},100A{r},{r} 0 1 1 {ex:.1f},{ey:.1f}", 66


def glyph_h(pen):
    x = pen + 12
    return f"M{x},0V150M{x},94A44,44 0 0 1 {x+88},94V150", 112


def wordmark(ink, accent, tick=False, cursor=False):
    """Returns (inner svg, width, height) for 'reet.sh' in native units."""
    stroke = f'fill="none" stroke-width="{W}" stroke-linecap="round" stroke-linejoin="round"'
    ink_paths, acc_paths, pen = [], [], 0
    d, adv = glyph_r(pen); ink_paths.append(d); pen += adv + GAP - 6
    d, adv = glyph_e(pen); ink_paths.append(d); pen += adv + GAP
    d, adv = glyph_e(pen); ink_paths.append(d); pen += adv + GAP
    if tick:
        d, t, adv = glyph_t_tick(pen); ink_paths.append(d); acc_paths.append(t); pen += adv + GAP - 4
    else:
        d, adv = glyph_t(pen); ink_paths.append(d); pen += adv + GAP - 4
    dot_x = pen + 12; pen += 24 + GAP - 2
    d, adv = glyph_s(pen); acc_paths.append(d); pen += adv + GAP
    d, adv = glyph_h(pen); acc_paths.append(d); pen += adv
    width = pen + 12
    g = [f'<g id="wordmark" {stroke}>',
         f'<path stroke="{ink}" d="{" ".join(ink_paths)}"/>',
         f'<circle fill="{accent}" cx="{dot_x}" cy="138" r="13"/>',
         f'<path stroke="{accent}" d="{" ".join(acc_paths)}"/>']
    if cursor:
        g.append(f'<rect fill="{accent}" x="{width+22}" y="50" width="52" height="100" rx="6"/>')
        width += 22 + 52 + 12
    g.append("</g>")
    return "\n".join(g), width, 162


# ------------------------------------------------------------------- icons
# Each icon lives in a 0..100 box and returns inner svg. `uid` keeps mask
# ids unique when several icons are inlined on one page.

RIBBON = "M24,10H76V90L50,70L24,90Z"


def _ribbon_with_cutout(colour, cutout, uid):
    return (
        f'<mask id="cut-{uid}"><rect width="100" height="100" fill="#fff"/>{cutout}</mask>'
        f'<path mask="url(#cut-{uid})" fill="{colour}" stroke="{colour}" stroke-width="6" '
        f'stroke-linejoin="round" d="{RIBBON}"/>'
    )


def icon_tick(ink, accent, uid):
    """Bookmark with a tick knocked out: saved, and reet (right)."""
    tick = ('<path d="M36,50L46,61L65,38" fill="none" stroke="#000" stroke-width="11" '
            'stroke-linecap="round" stroke-linejoin="round"/>')
    return _ribbon_with_cutout(accent, tick, uid)


def icon_sparkle(ink, accent, uid):
    """Bookmark with an AI sparkle knocked out: saved AI skills."""
    spark = ('<path fill="#000" d="M50,27C52.5,41 56,44.5 70,47C56,49.5 52.5,53 50,67'
             'C47.5,53 44,49.5 30,47C44,44.5 47.5,41 50,27Z"/>')
    return _ribbon_with_cutout(accent, spark, uid)


def icon_rose(ink, accent, uid):
    """The Yorkshire white rose, its five petals drawn as bookmark ribbons."""
    petal = "M-9,-12H9L19,-45L0,-34L-19,-45Z"
    petals = "".join(
        f'<path d="{petal}" transform="rotate({72*k})" fill="{ink}" stroke="{ink}" '
        f'stroke-width="5" stroke-linejoin="round"/>' for k in range(5))
    return (f'<mask id="cut-{uid}"><rect x="-50" y="-50" width="100" height="100" fill="#fff"/>'
            f'<circle r="13" fill="#000"/></mask>'
            f'<g transform="translate(50,50)"><g mask="url(#cut-{uid})">{petals}</g>'
            f'<circle r="8.5" fill="{accent}"/></g>')


ROSE_PETAL = "M-9,-12H9L19,-45L0,-34L-19,-45Z"


def _rose_cutout(cx, cy, scale):
    """Mask content: five petals and a ring, leaving a small centre dot."""
    petals = "".join(f'<path d="{ROSE_PETAL}" transform="rotate({72*k})" fill="#000" stroke="#000" '
                     f'stroke-width="5" stroke-linejoin="round"/>' for k in range(5))
    return (f'<g transform="translate({cx},{cy}) scale({scale})">{petals}'
            f'<circle r="13" fill="#000"/><circle r="7.5" fill="#fff"/></g>')


def icon_rose_bookmark(ink, accent, uid):
    """Yorkshire rose knocked out of an ink bookmark, with an accent centre."""
    cx, cy, sc = 50, 41, 0.44
    return (_ribbon_with_cutout(ink, _rose_cutout(cx, cy, sc), uid)
            + f'<circle cx="{cx}" cy="{cy}" r="{7.5*sc:.1f}" fill="{accent}"/>')


def icon_rose_bookmark_accent(ink, accent, uid):
    """Yorkshire rose knocked out of an accent bookmark."""
    return _ribbon_with_cutout(accent, _rose_cutout(50, 41, 0.44), uid)


def icon_tick_glyph(ink, accent, uid):
    """The tick-tailed t from the wordmark, on its own."""
    stem, tick, _ = glyph_t_tick(0)
    return (f'<g transform="translate(16,14) scale(0.48)" fill="none" stroke-width="{W}" '
            f'stroke-linecap="round" stroke-linejoin="round">'
            f'<path stroke="{ink}" d="{stem}"/><path stroke="{accent}" d="{tick}"/></g>')


ICONS = {"tick": icon_tick, "rose": icon_rose, "sparkle": icon_sparkle, "tickglyph": icon_tick_glyph,
         "rosebookmark": icon_rose_bookmark, "rosebookmark-accent": icon_rose_bookmark_accent}

CONCEPTS = {
    "concept-1-reet-tick": dict(icon="tick", tick=False, lockups=True,
                                title="reet.sh: bookmark with a tick"),
    "concept-2-yorkshire-rose": dict(icon="rose", tick=False, lockups=True,
                                     title="reet.sh: Yorkshire rose of bookmarks"),
    "concept-3-skill-sparkle": dict(icon="sparkle", tick=False, lockups=True,
                                    title="reet.sh: bookmark with an AI sparkle"),
    "concept-4-tick-wordmark": dict(icon="tickglyph", tick=True, lockups=False,
                                    title="reet.sh: wordmark with a tick for a t"),
    "concept-5-rose-bookmark": dict(icon="rosebookmark", tick=False, lockups=True,
                                    title="reet.sh: Yorkshire rose inside a bookmark"),
    "concept-5b-rose-bookmark-accent": dict(icon="rosebookmark-accent", tick=False, lockups=True,
                                            title="reet.sh: Yorkshire rose inside an accent bookmark"),
}


# ------------------------------------------------------------------ layouts
def svg(vb_w, vb_h, title, desc, inner):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {vb_w} {vb_h}" role="img" '
            f'aria-labelledby="t d">\n<title id="t">{title}</title>\n<desc id="d">{desc}</desc>\n'
            f'{inner}\n</svg>\n')


def layout_horizontal(c, ink, accent, uid):
    icon = ICONS[c["icon"]](ink, accent, uid)
    wm, w, _ = wordmark(ink, accent, c["tick"])
    s = 0.42
    tw = int(118 + w * s) + 2
    inner = (f'<g id="icon">{icon}</g>\n'
             f'<g transform="translate(118,{(100 - 150*s)/2 + 2:.1f}) scale({s})">{wm}</g>')
    return svg(tw, 100, c["title"], "Icon on the left, wordmark on the right.", inner)


def layout_vertical(c, ink, accent, uid):
    icon = ICONS[c["icon"]](ink, accent, uid)
    wm, w, _ = wordmark(ink, accent, c["tick"])
    s = 0.33
    tw = max(100, int(w * s)) + 24
    inner = (f'<g id="icon" transform="translate({(tw-100)/2:.1f},0)">{icon}</g>\n'
             f'<g transform="translate({(tw - w*s)/2:.1f},116) scale({s})">{wm}</g>')
    return svg(tw, 176, c["title"], "Icon above the wordmark.", inner)


def layout_icon(c, ink, accent, uid):
    return svg(100, 100, c["title"], "Icon only.", f'<g id="icon">{ICONS[c["icon"]](ink, accent, uid)}</g>')


def layout_tile(c, ink, accent, tile_bg, uid):
    icon = ICONS[c["icon"]](ink, accent, uid)
    inner = (f'<rect width="100" height="100" rx="22" fill="{tile_bg}"/>\n'
             f'<g transform="translate(14,14) scale(0.72)">{icon}</g>')
    return svg(100, 100, c["title"], "App icon tile.", inner)


def layout_wordmark(c, ink, accent, cursor=False):
    wm, w, _ = wordmark(ink, accent, c["tick"], cursor)
    return svg(w + 24, 186, c["title"], "Wordmark only.", f'<g transform="translate(12,24)">{wm}</g>')


def main():
    files = []
    for slug, c in CONCEPTS.items():
        d = OUT / slug
        d.mkdir(exist_ok=True)
        for old in d.glob("*.svg"):
            old.unlink()
        for scheme, col in SCHEMES.items():
            ink, accent = col["ink"], col["accent"]
            uid = f"{slug}-{scheme}"
            variants = {
                "icon": layout_icon(c, ink, accent, uid),
                "wordmark": layout_wordmark(c, ink, accent),
            }
            if c["lockups"]:
                variants["horizontal"] = layout_horizontal(c, ink, accent, uid)
                variants["vertical"] = layout_vertical(c, ink, accent, uid)
            else:
                variants["wordmark-cursor"] = layout_wordmark(c, ink, accent, cursor=True)
            for layout, body in variants.items():
                p = d / f"{layout}-{scheme}.svg"
                p.write_text(body)
                files.append(p)
        # Tiles: ink tile with cream marks, accent tile, and a black mono tile.
        tiles = {
            "tile-ink": (INK_D, ACCENT_D, INK_L),
            "tile-accent": ("#ffffff", "#ffffff", ACCENT_L),
            "tile-mono": ("#ffffff", "#ffffff", "#000000"),
        }
        for name, (ink, accent, bg) in tiles.items():
            p = d / f"{name}.svg"
            p.write_text(layout_tile(c, ink, accent, bg, f"{slug}-{name}"))
            files.append(p)
    print(f"wrote {len(files)} files")


if __name__ == "__main__":
    main()
