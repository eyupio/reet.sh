# reet.sh logo concepts

Four directions for the reet.sh mark. All are plain SVG with the letters drawn as
paths, so they need no fonts and look the same in every browser, editor and
print pipeline. Open `contact-sheet.html` in a browser (or see
`contact-sheet.png`) to compare everything side by side.

The brief, as understood: reet.sh bookmarks AI skills from skills.sh, and
"reet" is the Yorkshire word for right / proper ("that's reet good"). Each
concept combines a bookmark with one of a tick, the Yorkshire white rose or
an AI sparkle.

Colours come straight from the web UI (`web/src/App.svelte`):

| Role | Light mode | Dark mode |
| --- | --- | --- |
| Ink | `#1d1c1a` | `#efece6` |
| Accent (burnt orange) | `#c2410c` | `#fb923c` |
| Background | `#f7f6f2` | `#151413` |

The wordmark keeps the header's existing treatment: `reet` in ink, `.sh` in the
accent. The dot of `.sh` is a filled circle in the accent, which is the only
place a solid shape meets the monoline letters, so it reads as a deliberate
marker rather than a typo.

## Concept 1: reet tick (recommended)

`concept-1-reet-tick/`

A bookmark ribbon with a tick knocked out of it. The bookmark says "saved", the
tick says "reet": it's right, it's the one you actually use. The ribbon is the
accent colour, so the mark carries the brand colour on its own and the
wordmark can stay in ink. Works at 16 px as a favicon because it's one solid
shape with one cut.

## Concept 2: Yorkshire rose

`concept-2-yorkshire-rose/`

The white rose of Yorkshire, with its five petals drawn as bookmark ribbons
(notch at the tip, narrow at the base) around an accent centre. The petals
and centre are separated by a ring of background, so the mono versions still
read as a rose rather than a blob. On the ink tile the petals are cream, which
is where the "white rose" reads most literally. The strongest Yorkshire
signal of the four, and the most distinctive, but it's also the busiest at
small sizes.

## Concept 3: skill sparkle

`concept-3-skill-sparkle/`

The same ribbon as concept 1, with the four-point "AI" sparkle knocked out
instead of a tick. Leans into "AI skills" rather than the name. Familiar
shorthand, which is both its strength (instantly placed as AI tooling) and
its risk (lots of AI products use a sparkle).

## Concept 4: tick wordmark

`concept-4-tick-wordmark/`

Wordmark only. The `t` of `reet` has a tick for a tail, in the accent, so the
word carries its own meaning without a separate mark. There's a
`wordmark-cursor-*` variant with a block cursor after `.sh` for a terminal
feel, and `icon-*` / `tile-*` files with the tick-t on its own for favicons.
No horizontal or vertical lockups, since the icon is already in the word.

## Files

Each concept folder holds:

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
- **Minimum size:** icons work down to 16 px. Keep horizontal lockups at 100 px
  wide or more; below that use the icon alone.
- **Backgrounds:** use `color-light` or `mono-dark` on light surfaces and
  `color-dark` or `mono-light` on dark surfaces. Mono versions for one-colour
  print, embroidery and engraving.
- **Don't** stretch, rotate, recolour outside the palette, add shadows or
  gradients, or set the wordmark in a different typeface.

## Export to PNG

```sh
# ImageMagick
convert -background none -density 384 brand/concept-1-reet-tick/icon-color-light.svg icon.png

# Headless Chromium (what contact-sheet.png was made with)
chromium --headless --screenshot=sheet.png --window-size=1300,3400 brand/contact-sheet.html
```

## Web use

```html
<img src="/brand/concept-1-reet-tick/horizontal-color-light.svg" alt="reet.sh" height="32">
```

For automatic light / dark switching, inline the SVG and replace the hard-coded
colours with `var(--ink)` and `var(--accent)`; the CSS variables already exist
in the app.

## Regenerating

```sh
python3 brand/generate.py        # rewrites every SVG
python3 brand/contact-sheet.py   # rebuilds contact-sheet.html
```

Letterforms, icon geometry and the colour table all live in `generate.py`.
Change there, not in the SVGs.
