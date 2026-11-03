#!/usr/bin/env python3
"""Generates every reet.sh logo variant into brand/logo/.

Run `python3 brand/generate.py` from the repo root. Letters are drawn as
paths (no fonts), so the SVGs render identically everywhere.

The mark: the Yorkshire white rose ("reet" is Yorkshire for right / proper)
knocked out of a bookmark ribbon (the site bookmarks AI skills), with an
accent dot at the centre.
"""
from __future__ import annotations

import re
from pathlib import Path

OUT =Path(__file__).resolve().parent / "logo"
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
# "reet.sh" set in IBM Plex Sans SemiBold, the typeface the app's UI is set in (SIL OFL 1.1; the licence ships in
# web/public/fonts/OFL.txt), converted to outlines so no font is needed. One typeface gives every letter the same
# x-height, stroke weight and rhythm. Units: x-height 100, origin at the top-left of the ink, y pointing down; WM_BASE
# is the baseline's y. The block below is written by outline_wordmark.py.
# BEGIN wordmark paths (written by outline_wordmark.py; do not edit by hand)
WM_INK = "M24.52 141.76H0V41.76H24.52V62.45H25.48Q26.82 57.09 29.98 52.39Q33.14 47.7 38.6 44.73Q44.06 41.76 51.92 41.76H57.28V64.94H49.62Q41.38 64.94 35.73 66.57Q30.08 68.2 27.3 71.55Q24.52 74.9 24.52 80.46Z M112.75 144.06Q98 144.06 87.46 137.64Q76.93 131.23 71.28 119.44Q65.62 107.66 65.62 91.57Q65.62 75.86 71.18 64.08Q76.74 52.3 87.08 45.88Q97.43 39.46 111.98 39.46Q127.69 39.46 137.75 46.36Q147.81 53.26 152.69 64.66Q157.58 76.05 157.58 89.85V97.89H91.1V100.38Q91.1 111.11 97.23 117.82Q103.36 124.52 115.43 124.52Q124.44 124.52 130.38 120.59Q136.31 116.67 140.91 110.92L154.13 125.67Q148 133.91 137.37 138.98Q126.74 144.06 112.75 144.06ZM112.37 57.85Q105.85 57.85 101.07 60.82Q96.28 63.79 93.69 69.06Q91.1 74.33 91.1 81.23V82.76H132.1V81.03Q132.1 73.95 129.8 68.87Q127.5 63.79 123.1 60.82Q118.69 57.85 112.37 57.85Z M217.65 144.06Q202.9 144.06 192.36 137.64Q181.82 131.23 176.17 119.44Q170.52 107.66 170.52 91.57Q170.52 75.86 176.08 64.08Q181.63 52.3 191.98 45.88Q202.32 39.46 216.88 39.46Q232.59 39.46 242.65 46.36Q252.7 53.26 257.59 64.66Q262.48 76.05 262.48 89.85V97.89H196V100.38Q196 111.11 202.13 117.82Q208.26 124.52 220.33 124.52Q229.33 124.52 235.27 120.59Q241.21 116.67 245.81 110.92L259.03 125.67Q252.9 133.91 242.26 138.98Q231.63 144.06 217.65 144.06ZM217.26 57.85Q210.75 57.85 205.96 60.82Q201.17 63.79 198.59 69.06Q196 74.33 196 81.23V82.76H237V81.03Q237 73.95 234.7 68.87Q232.4 63.79 227.99 60.82Q223.59 57.85 217.26 57.85Z M330.97 141.76H313.35Q300.7 141.76 294.1 135.15Q287.49 128.54 287.49 116.28V61.11H272.74V41.76H280.4Q285.95 41.76 287.97 39.18Q289.98 36.59 289.98 31.42V14.37H312.01V41.76H332.51V61.11H312.01V122.22H330.97Z"
WM_ACCENT = "M366.14 143.87Q358.48 143.87 354.64 139.85Q350.81 135.82 350.81 129.89V126.44Q350.81 120.31 354.64 116.28Q358.48 112.26 366.14 112.26Q374.18 112.26 377.82 116.28Q381.46 120.31 381.46 126.44V129.89Q381.46 135.82 377.82 139.85Q374.18 143.87 366.14 143.87Z M439.81 144.06Q425.44 144.06 415.67 139.18Q405.9 134.29 398.43 125.67L413.37 111.11Q418.93 117.43 425.54 121.07Q432.15 124.71 440.77 124.71Q449.58 124.71 453.51 121.65Q457.43 118.58 457.43 113.22Q457.43 108.81 454.66 106.23Q451.88 103.64 445.17 102.68L435.21 101.34Q424.48 100 417.01 96.36Q409.54 92.72 405.71 86.4Q401.88 80.08 401.88 71.07Q401.88 56.51 412.32 47.99Q422.76 39.46 439.81 39.46Q449.58 39.46 456.57 41.28Q463.56 43.1 468.93 46.65Q474.29 50.19 478.89 55.17L464.52 69.54Q460.11 64.94 453.98 61.88Q447.85 58.81 440.57 58.81Q432.91 58.81 429.18 61.59Q425.44 64.37 425.44 69.16Q425.44 74.14 428.41 76.72Q431.38 79.31 438.47 80.46L448.62 81.8Q465.1 84.1 473.05 91.48Q481 98.85 481 111.3Q481 120.88 475.92 128.35Q470.84 135.82 461.55 139.94Q452.26 144.06 439.81 144.06Z M500.64 141.76V0H525.16V58.43H526.12Q529.19 50.38 535.8 44.92Q542.41 39.46 554.09 39.46Q569.42 39.46 577.75 49.71Q586.08 59.96 586.08 78.54V141.76H561.56V81.03Q561.56 70.5 557.73 65.04Q553.9 59.58 545.09 59.58Q540.11 59.58 535.51 61.4Q530.91 63.22 528.04 66.76Q525.16 70.31 525.16 75.67V141.76Z"
WM_W, WM_H, WM_BASE = 586.08, 144.06, 141.76
# END wordmark paths


def wordmark(ink, accent):
    """Returns (inner svg, width, height) for 'reet.sh' in native units."""
    g = f'<g id="wordmark"><path fill="{ink}" d="{WM_INK}"/><path fill="{accent}" d="{WM_ACCENT}"/></g>'
    return g, WM_W, WM_H


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
    s, x0 = 0.44, 112                      # x-height 44 against the 100-unit icon; the word starts at x=112
    ty = 80 - WM_BASE * s                  # baseline at y=80, near the foot of the ribbon
    tw = round(x0 + w * s) + 2
    inner = (f'<g id="icon">{icon(ink, accent, uid)}</g>\n'
             f'<g transform="translate({x0},{ty:.2f}) scale({s})">{wm}</g>')
    return svg(tw, 100, "Yorkshire rose in a bookmark, with the reet.sh wordmark to the right.", inner)


def layout_vertical(ink, accent, uid):
    wm, w, _ = wordmark(ink, accent)
    word_w, top = 205, 120                 # the word is 205 wide whatever its length; its ink starts at y=120
    s = word_w / w
    tw = word_w + 24
    inner = (f'<g id="icon" transform="translate({(tw - 100) / 2:.1f},0)">{icon(ink, accent, uid)}</g>\n'
             f'<g transform="translate({(tw - w * s) / 2:.2f},{top}) scale({s:.4f})">{wm}</g>')
    return svg(tw, round(top + WM_BASE * s + 16), "Yorkshire rose in a bookmark, with the reet.sh wordmark below.", inner)


def layout_icon(ink, accent, uid):
    return svg(100, 100, "Yorkshire rose in a bookmark.", f'<g id="icon">{icon(ink, accent, uid)}</g>')


def layout_tile(ink, accent, tile_bg, uid):
    inner = (f'<rect width="100" height="100" rx="22" fill="{tile_bg}"/>\n'
             f'<g transform="translate(14,14) scale(0.72)">{icon(ink, accent, uid)}</g>')
    return svg(100, 100, "Yorkshire rose in a bookmark, on a rounded tile.", inner)


def layout_wordmark(ink, accent):
    wm, w, h = wordmark(ink, accent)
    return svg(round(w + 24), round(h + 24), "The reet.sh wordmark.", f'<g transform="translate(12,12)">{wm}</g>')


def sync_boot_shell():
    """Rewrites the lockup inlined in the boot shell of web/index.html (shown before the app bundle loads).

    The boot shell is aria-hidden decoration, so its copy drops the role, title and description, and it is themed
    by the shell's own --boot-ink / --boot-accent variables.
    """
    path = OUT.parent.parent / "web" / "index.html"
    lockup = layout_horizontal("var(--boot-ink)", "var(--boot-accent)", "boot")
    lockup = re.sub(r' role="img" aria-labelledby="t d"|<title id="t">.*?</title>\n|<desc id="d">.*?</desc>\n', "", lockup).rstrip("\n")
    html, n = re.subn(r'(<span class="boot-brand">)<svg.*?</svg>(</span>)', lambda m: m.group(1) + lockup + m.group(2),
                      path.read_text(), flags=re.S)
    if n != 1:
        raise SystemExit(f"could not find the boot-brand lockup in {path}")
    path.write_text(html)
    return path


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
    files.append(sync_boot_shell())
    print(f"wrote {len(files)} logo assets")


if __name__ == "__main__":
    main()
