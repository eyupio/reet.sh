// reet's colour tokens, as app.css sets them. Dark is the designed palette; light is derived from reet's
// earlier light tokens. theme.test.mjs checks app.css against these values and every text pair against WCAG AA.

export type Palette = 'dark' | 'light'

export const TOKENS: Record<Palette, Record<string, string>> = {
  dark: {
    '--bg': '#1d201c',
    '--card': '#262a24',
    '--card-raised': '#2a2e26',
    '--line-soft': '#333a2f',
    '--line': '#3b4237',
    '--line-strong': '#535f4b',
    '--ink': '#efeee5',
    '--muted': '#adb5a6',
    '--accent': '#b84216',
    '--accent-text': '#ed976e',
    '--accent-ink': '#ffffff',
    '--tint': '#3e2d24',
    '--accent-wash': '#34271f',
    '--shadow-dock': '0 -8px 30px rgb(0 0 0 / 0.25)',
    '--heart': '#ff7a9c',
    '--good': '#7fc79a',
    '--warn': '#e9b44c',
    '--info': '#7fb3d9',
    '--repo': '#5fc2a8',
    '--bad': '#f87171',
    '--status-working': '#22c55e',
    '--status-waiting': '#fbbf24',
    '--status-starting': '#f59e0b',
    '--status-finished': '#38bdf8',
    '--status-failed': '#f26969',
    '--status-ended': '#adb5a6',
    '--status-quiet': '#adb5a6',
    '--bg-sunk': '#191c18',
    '--dock-bg': '#191c18',
    '--faint': '#9aa492',
    '--ink-soft': '#d4d9cc',
    '--repo-line': '#2a5f52',
    '--line-deep': '#849175',
    // The rail's Add, machine tags and dashed foot (the mock-up's #424c3b).
    '--line-mid': '#424c3b',
    // Behind a modal dialog, and its shadow. Not a text colour.
    '--scrim': 'rgb(0 0 0 / 0.62)',
  },
  light: {
    '--bg': '#f6f5f0',
    '--card': '#fffefb',
    '--card-raised': '#fffefb',
    '--line-soft': '#e6e7de',
    '--line': '#dddfd5',
    '--line-strong': '#c9cdbf',
    '--ink': '#292b25',
    '--muted': '#60675a',
    '--accent': '#b84216',
    '--accent-text': '#b84216',
    '--accent-ink': '#ffffff',
    '--tint': '#f5e8dd',
    '--accent-wash': '#f5e8dd',
    '--shadow-dock': '0 -8px 30px rgb(29 28 26 / 0.08)',
    '--heart': '#be1f4a',
    '--good': '#15703b',
    '--warn': '#92400e',
    '--info': '#1d5f8a',
    '--repo': '#0f766e',
    '--bad': '#b91c1c',
    '--status-working': '#15803d',
    '--status-waiting': '#a16207',
    '--status-starting': '#b45309',
    '--status-finished': '#0369a1',
    '--status-failed': '#b91c1c',
    '--status-ended': '#52525b',
    '--status-quiet': '#52525b',
    '--bg-sunk': '#eeede6',
    '--dock-bg': '#f6f5f0',
    '--faint': '#656c5e',
    '--ink-soft': '#3a4233',
    '--repo-line': '#8fc7ba',
    '--line-deep': '#7c866f',
    '--line-mid': '#d2d8c8',
    '--scrim': 'rgb(20 18 15 / 0.45)',
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
