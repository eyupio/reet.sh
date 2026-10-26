// reet's colour tokens, as app.css sets them. Dark is the designed palette; light is derived from reet's
// earlier light tokens. theme.test.mjs checks app.css against these values and every text pair against WCAG AA.

export type Palette = 'dark' | 'light'

export const TOKENS: Record<Palette, Record<string, string>> = {
  dark: {
    '--bg': '#14110f',
    '--card': '#1a1613',
    '--card-raised': '#1d1815',
    '--line-soft': '#2a241f',
    '--line': '#342d27',
    '--line-strong': '#4a4037',
    '--ink': '#f1ebe4',
    '--muted': '#a39a90',
    '--accent': '#c2410c',
    '--accent-text': '#ff8d63',
    '--accent-ink': '#ffffff',
    '--tint': '#3a2615',
    '--heart': '#ff7a9c',
    '--good': '#7fc79a',
    '--warn': '#e9b44c',
    '--info': '#7fb3d9',
    '--repo': '#5fc2a8',
    '--bad': '#f87171',
  },
  light: {
    '--bg': '#f7f6f2',
    '--card': '#fffdf9',
    '--card-raised': '#fffdf9',
    '--line-soft': '#e8e4db',
    '--line': '#dedbd2',
    '--line-strong': '#c9c3b6',
    '--ink': '#1d1c1a',
    '--muted': '#5f5c54',
    '--accent': '#c2410c',
    '--accent-text': '#c2410c',
    '--accent-ink': '#ffffff',
    '--tint': '#feefe3',
    '--heart': '#be1f4a',
    '--good': '#15703b',
    '--warn': '#92400e',
    '--info': '#1d5f8a',
    '--repo': '#0f766e',
    '--bad': '#b91c1c',
  },
}

const channels = (hex: string): number[] => {
  const m = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(hex)
  if (!m) throw new Error(`not a hex colour: ${hex}`)
  const h = m[1].length === 3 ? [...m[1]].map(c => c + c).join('') : m[1]
  return [0, 2, 4].map(i => parseInt(h.slice(i, i + 2), 16) / 255)
}

const luminance = (hex: string): number => {
  const [r, g, b] = channels(hex).map(c => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4))
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

/** WCAG 2 contrast ratio between two hex colours, from 1 to 21. */
export function contrast(fg: string, bg: string): number {
  const [a, b] = [luminance(fg), luminance(bg)].sort((x, y) => y - x)
  return (a + 0.05) / (b + 0.05)
}
