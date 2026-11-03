# reet.sh logo

The Yorkshire white rose knocked out of a bookmark ribbon, with a burnt-orange
dot at its centre. "Reet" is Yorkshire for right / proper; the site bookmarks
AI skills. Both halves of the name are in the mark.

All files are plain SVG with the letters drawn as paths, so they need no fonts
and look the same in every browser, editor and print pipeline. Open
`contact-sheet.html` in a browser (or see `contact-sheet.png`) for every
variant on light and dark panels.

Colours come straight from the web UI (`web/src/App.svelte`):

| Role | Light mode | Dark mode |
| --- | --- | --- |
| Ink | `#1d1c1a` | `#efece6` |
| Accent (burnt orange) | `#c2410c` | `#fb923c` |
| Background | `#f7f6f2` | `#151413` |

The ribbon is ink, so on a light page the rose is literally a white rose in a
dark bookmark; in dark mode the ribbon is cream and the rose shows the dark
background. The wordmark is IBM Plex Sans SemiBold, the typeface the web UI is
set in, drawn as outlines: `reet` in ink and `.sh`, dot included, in the accent.

## Files

Everything is in `logo/`.

| File | Use |
| --- | --- |
| `horizontal-*.svg` | Icon left, wordmark right. Site header, README, docs. |
| `vertical-*.svg` | Icon above wordmark. Social avatars, splash screens. |
| `icon-*.svg` | Mark alone, transparent. Favicon source, inline UI. |
| `wordmark-*.svg` | Word alone, transparent. |
| `tile-ink.svg` | Mark on a rounded ink tile. App icon, GitHub avatar. |
| `tile-accent.svg` | Mark in white on an accent tile. |
| `tile-mono.svg` | Mark in white on black. |

Colour suffixes:

| Suffix | Meaning |
| --- | --- |
| `color-light` | Ink and accent for light backgrounds (the web UI's light mode). |
| `color-dark` | Cream and light-orange for dark backgrounds (dark mode). |
| `mono-dark` | Single ink colour, for light backgrounds and print. |
| `mono-light` | Single white, for dark backgrounds. |

All files are transparent except the tiles. Each SVG has a `<title>` and
`<desc>` for screen readers.

## Usage guidelines

- **Clear space:** keep at least the height of the icon free on every side.
- **Minimum size:** the icon reads as a bookmark with a flower at 16 px and as
  a rose from about 24 px. Keep horizontal lockups at 100 px wide or more;
  below that use the icon alone.
- **Backgrounds:** use `color-light` or `mono-dark` on light surfaces and
  `color-dark` or `mono-light` on dark surfaces. Mono versions for one-colour
  print, embroidery and engraving.
- **Don't** stretch, rotate, recolour outside the palette, add shadows or
  gradients, or set the wordmark in a different typeface.

## Export to PNG

```sh
# ImageMagick
convert -background none -density 384 brand/logo/icon-color-light.svg icon.png

# Headless Chromium (what contact-sheet.png was made with)
chromium --headless --screenshot=sheet.png --window-size=1300,1400 brand/contact-sheet.html
```

## Web use

```html
<img src="/brand/logo/horizontal-color-light.svg" alt="reet.sh" height="32">
```

For automatic light / dark switching, inline the SVG and replace the hard-coded
colours with `var(--ink)` and `var(--accent)`; the CSS variables already exist
in the app.

## Regenerating

```sh
python3 brand/generate.py        # rewrites every SVG in brand/logo
python3 brand/contact-sheet.py   # rebuilds contact-sheet.html
```

Icon geometry, layouts and the colour table live in `generate.py`. Change them
there, not in the SVGs. The wordmark outlines are a generated block inside
`generate.py`, written by `outline_wordmark.py` from IBM Plex Sans SemiBold
(SIL Open Font License 1.1; the licence ships in `web/public/fonts/OFL.txt`).
To change the weight or tracking, edit that script and run both:

```sh
pip install fonttools brotli uharfbuzz   # needed by outline_wordmark.py only
python3 brand/outline_wordmark.py
python3 brand/generate.py
```

`generate.py` itself needs no font and no packages. Besides `brand/logo/`, it
writes the app's copies of the lockup, so the app always uses the same
geometry as the brand assets:

- `web/src/lib/logo.svg`, the themeable lockup imported by the Svelte header.
- The inline lockup in the boot shell of `web/index.html`, shown before the
  app bundle loads.

After regenerating, rebuild `contact-sheet.png` and `web/public/og-image.png`
(see "Export to PNG" and "Web icons and social image").

## Web icons and social image

The web UI's icons and link-preview image live in `web/public/` and are copied
to the site root on build.

| File | Source | Use |
| --- | --- | --- |
| `favicon.svg` | `icon-color-*.svg` | Browser tab icon; switches between the light and dark colours with the browser theme. |
| `favicon.ico` | `tile-ink.svg` (16, 32, 48 px) | Fallback for browsers without SVG favicons. |
| `apple-touch-icon.png` | `tile-ink.svg`, square corners, 180 px | iOS home screen (iOS rounds the corners itself). |
| `icon-192.png`, `icon-512.png` | `tile-ink.svg` | Installed-app icons. |
| `icon-maskable-512.png` | `tile-ink.svg`, square corners | Android adaptive icon (the mark sits inside the safe zone). |
| `og-image.png` | `og-image.html`, 1200 x 630 | `og:image` and `twitter:image` link previews. |
| `site.webmanifest` | | Names, colours and the icons above for "install app". |

The tile is used for the raster icons so they stay visible on both light and
dark tab bars. To rebuild the social image after editing `og-image.html`:

```sh
chromium --headless --screenshot=web/public/og-image.png --window-size=1200,630 brand/og-image.html
```

The `og:image` and `og:url` tags in `web/index.html` assume the public origin
`https://reet.sh`; change them there if the site is served elsewhere.
