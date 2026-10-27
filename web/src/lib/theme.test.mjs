import { readFileSync, existsSync, statSync } from 'node:fs'
import { execFileSync } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import { join } from 'node:path'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { TOKENS, contrast } = await import(`data:text/javascript;base64,${Buffer.from(compile('./theme.ts')).toString('base64')}`)

const web = fileURLToPath(new URL('../../', import.meta.url))
const css = readFileSync(join(web, 'src/app.css'), 'utf8')
const PALETTES = ['dark', 'light']
const NAMES = ['--bg', '--card', '--card-raised', '--line-soft', '--line', '--line-strong', '--ink', '--muted', '--accent', '--accent-text', '--accent-ink', '--good', '--warn', '--info', '--repo', '--bad', '--bg-sunk', '--faint', '--ink-soft', '--repo-line', '--line-deep', '--line-mid', '--dock-bg']

test('contrast follows WCAG relative luminance', () => {
  assert.equal(Math.round(contrast('#000000', '#ffffff') * 100) / 100, 21)
  assert.equal(contrast('#fff', '#fff'), 1)
  assert.equal(Math.round(contrast('#c2410c', '#ffffff') * 100) / 100, Math.round(contrast('#ffffff', '#c2410c') * 100) / 100)
  assert.throws(() => contrast('red', '#fff'))
})

test('both palettes define every token, with the designed dark values', () => {
  for (const p of PALETTES) for (const name of NAMES) assert.match(TOKENS[p][name] ?? '', /^#[0-9a-f]{6}$/, `${p} ${name}`)
  const dark = { '--bg': '#14110f', '--card': '#1a1613', '--card-raised': '#1d1815', '--line-soft': '#2a241f', '--line': '#342d27', '--line-strong': '#4a4037', '--ink': '#f1ebe4', '--muted': '#a39a90', '--accent': '#c2410c', '--accent-text': '#ff8d63', '--accent-ink': '#ffffff', '--good': '#7fc79a', '--warn': '#e9b44c', '--info': '#7fb3d9', '--repo': '#5fc2a8', '--bg-sunk': '#110e0c', '--faint': '#8c837a', '--ink-soft': '#cfc6bc', '--repo-line': '#2a5f52', '--line-deep': '#5a5047', '--line-mid': '#3d342d', '--dock-bg': '#110e0c' }
  for (const [k, v] of Object.entries(dark)) assert.equal(TOKENS.dark[k], v, k)
  const light = { '--bg': '#f7f6f2', '--card': '#fffdf9', '--ink': '#1d1c1a', '--muted': '#5f5c54', '--accent': '#c2410c', '--accent-text': '#c2410c', '--accent-ink': '#ffffff' }
  for (const [k, v] of Object.entries(light)) assert.equal(TOKENS.light[k], v, k)
})

test('avatar text stays legible on the designed account disc', () => {
  for (const p of PALETTES) {
    assert.ok(TOKENS[p]['--accent-wash'], `${p}: account disc surface exists`)
    assert.ok(contrast(TOKENS[p]['--accent-text'], TOKENS[p]['--accent-wash']) >= 4.5, `${p}: avatar initials meet AA`)
  }
})

test('every text pair passes AA', () => {
  for (const p of PALETTES) {
    const t = TOKENS[p]
    for (const fg of ['--ink', '--ink-soft', '--muted', '--faint', '--accent-text', '--good', '--warn', '--info', '--repo', '--bad'])
      for (const bg of ['--bg', '--card', '--card-raised'])
        assert.ok(contrast(t[fg], t[bg]) >= 4.5, `${p}: ${fg} ${t[fg]} on ${bg} ${t[bg]} is ${contrast(t[fg], t[bg]).toFixed(2)}`)
    assert.ok(contrast(t['--accent-ink'], t['--accent']) >= 4.5, `${p}: --accent-ink on --accent`)
    // The sunk surface (the phone sheet's pinned footer) carries plain text and the repository word.
    for (const fg of ['--ink', '--ink-soft', '--muted', '--faint', '--repo']) assert.ok(contrast(t[fg], t['--bg-sunk']) >= 4.5, `${p}: ${fg} ${t[fg]} on --bg-sunk ${t['--bg-sunk']} is ${contrast(t[fg], t['--bg-sunk']).toFixed(2)}`)
    // Selected and hover states put text on --tint; the pressed Liked filter puts --bg on --heart (App.svelte).
    for (const fg of ['--ink', '--accent-text']) assert.ok(contrast(t[fg], t['--tint']) >= 4.5, `${p}: ${fg} ${t[fg]} on --tint ${t['--tint']} is ${contrast(t[fg], t['--tint']).toFixed(2)}`)
    // The phone tab bar draws its current label in --accent-text on --dock-bg (BottomTabs.svelte).
    for (const fg of ['--ink', '--muted', '--accent-text']) assert.ok(contrast(t[fg], t['--dock-bg']) >= 4.5, `${p}: ${fg} ${t[fg]} on --dock-bg ${t['--dock-bg']} is ${contrast(t[fg], t['--dock-bg']).toFixed(2)}`)
    assert.ok(contrast(t['--bg'], t['--heart']) >= 4.5, `${p}: --bg on --heart is ${contrast(t['--bg'], t['--heart']).toFixed(2)}`)
  }
})

// Non-text contrast (WCAG 1.4.11): --line-deep is the only edge of an unchecked checkbox, so it needs 3:1 against the
// surfaces it sits on. Light passes. Dark keeps the mock-up's #5a5047 (2.39:1 on --bg), an owner call recorded beside the
// --line-strong gap; this pins it so a change is deliberate.
test('the checkbox edge (--line-deep) is 3:1 in light; dark is the recorded mock-up value', () => {
  for (const bg of ['--bg', '--card', '--card-raised', '--bg-sunk'])
    assert.ok(contrast(TOKENS.light['--line-deep'], TOKENS.light[bg]) >= 3, `light --line-deep ${TOKENS.light['--line-deep']} on ${bg} is ${contrast(TOKENS.light['--line-deep'], TOKENS.light[bg]).toFixed(2)}`)
  assert.equal(TOKENS.dark['--line-deep'], '#5a5047')
})

test('the pressed Liked filter draws its text in --bg, not a literal white', () => {
  const app = readFileSync(join(web, 'src/App.svelte'), 'utf8')
  assert.match(app, /\.filters \.liked\[aria-pressed='true'\] \{ background: var\(--heart\); color: var\(--bg\); \}/)
})

// Every block in app.css that sets the palette must set it exactly as TOKENS says, so the stylesheet and the checked values cannot drift.
const blocks = (source, selector) => {
  const out = []
  const re = /([^{}]+)\{([^{}]*)\}/g
  for (const m of source.replace(/\/\*[\s\S]*?\*\//g, '').matchAll(re)) if (m[1].trim() === selector) out.push(Object.fromEntries([...m[2].matchAll(/(--[\w-]+)\s*:\s*([^;]+);?/g)].map(d => [d[1], d[2].trim()])))
  return out
}

const COLOUR = /^(#[0-9a-f]{3,8}|(rgba?|hsla?|oklch|oklab|lab|lch|color|color-mix)\(.*\)|white|black|transparent|currentcolor)$/i

test('app.css matches TOKENS', () => {
  const want = { ":root": 'dark', ":root[data-theme='dark']": 'dark', ":root:not([data-theme='dark'])": 'light', ":root[data-theme='light']": 'light' }
  for (const [selector, palette] of Object.entries(want)) {
    const found = blocks(css, selector)
    assert.ok(found.length > 0, `app.css has a ${selector} block`)
    const merged = Object.assign({}, ...found)
    for (const [name, value] of Object.entries(TOKENS[palette])) assert.equal(merged[name], value, `${selector} ${name}`)
    // And the other way: a colour set in a palette block that TOKENS does not list is a stray token the checks never see.
    // :root also carries the fonts, sizes and easing, so only colour values count there; the other blocks set the palette alone.
    for (const [name, value] of Object.entries(merged)) {
      if (selector === ':root' && !COLOUR.test(value)) continue
      assert.ok(name in TOKENS[palette], `${selector} sets ${name}: ${value}, which TOKENS.${palette} does not list`)
    }
  }
  assert.match(css, /@media \(prefers-color-scheme: light\)\s*\{\s*:root:not\(\[data-theme='dark'\]\)/, 'a light system preference picks the light palette unless the account chose dark')
  assert.match(css, /(^|\n)a \{ color: var\(--accent-text\);/, 'links use the accent text colour')
})

test('fonts are self-hosted WOFF2 with swap, under their licence', () => {
  assert.match(css, /--serif: 'Fraunces', /)
  assert.match(css, /--sans: 'IBM Plex Sans', /)
  assert.match(css, /--mono: 'IBM Plex Mono', /)
  const faces = [...css.matchAll(/@font-face\s*\{([^}]*)\}/g)].map(m => m[1])
  const want = [['Fraunces', 600], ['IBM Plex Sans', 400], ['IBM Plex Sans', 500], ['IBM Plex Sans', 600], ['IBM Plex Mono', 400]]
  assert.equal(faces.length, want.length)
  let total = 0
  for (const [family, weight] of want) {
    const face = faces.find(f => f.includes(`font-family: '${family}'`) && f.includes(`font-weight: ${weight};`))
    assert.ok(face, `${family} ${weight}`)
    assert.match(face, /font-display: swap;/)
    const url = face.match(/src: url\('(\/fonts\/[\w-]+\.woff2)'\) format\('woff2'\)/)?.[1]
    assert.ok(url, `${family} ${weight} is served from /fonts as WOFF2`)
    const file = join(web, 'public', url)
    assert.ok(existsSync(file), file)
    total += statSync(file).size
    assert.equal(readFileSync(file).subarray(0, 4).toString('latin1'), 'wOF2')
  }
  assert.ok(total < 160_000, `fonts total ${total} bytes`)
  const ofl = readFileSync(join(web, 'public/fonts/OFL.txt'), 'utf8')
  for (const who of ['The Fraunces Project Authors', 'IBM Corp.', 'SIL OPEN FONT LICENSE Version 1.1']) assert.ok(ofl.includes(who), who)
})

test('the logo draws the real logo.svg with the ink and accent tokens', () => {
  const logo = readFileSync(join(web, 'src/lib/Logo.svelte'), 'utf8')
  assert.match(logo, /from '\.\/logo\.svg\?raw'/)
  const svg = readFileSync(join(web, 'src/lib/logo.svg'), 'utf8')
  assert.match(svg, /var\(--ink[,)]/)
  assert.match(svg, /var\(--accent[,)]/)
})

// Components use tokens only. Files listed here are always checked whole; with a git base (REET_THEME_BASE, or the merge
// base with origin/main) every line this branch adds to a component is checked too.
const LITERAL = /#[0-9a-fA-F]{3,8}\b|\b(rgba?|hsla?|oklch|lab|lch)\(|\b(white|black)\b(?=\s*[;}"'])/
const ALWAYS = ['src/lib/Logo.svelte']
const git = args => execFileSync('git', args, { cwd: web, encoding: 'utf8', stdio: ['ignore', 'pipe', 'ignore'] })
const base = () => { try { return process.env.REET_THEME_BASE || git(['merge-base', 'HEAD', 'origin/main']).trim() } catch { return '' } }
// Blanks <script> blocks (keeping line numbers) by scanning, not by pattern: no HTML is parsed with a regexp.
const strip = text => {
  const lower = text.toLowerCase()
  let out = '', at = 0
  for (;;) {
    const open = lower.indexOf('<script', at)
    if (open < 0) return out + text.slice(at)
    const close = lower.indexOf('</script', open)
    const end = close < 0 ? text.length : lower.indexOf('>', close) + 1 || text.length
    out += text.slice(at, open) + text.slice(open, end).replace(/[^\n]/g, '')
    at = end
  }
}

test('no literal colour in a component', () => {
  const offences = []
  for (const file of ALWAYS) strip(readFileSync(join(web, file), 'utf8')).split('\n').forEach((line, i) => { if (LITERAL.test(line)) offences.push(`${file}:${i + 1}: ${line.trim()}`) })
  const from = base()
  // A shallow checkout (or a fork with no origin/main) has no base: say so, so a passing run is not read as a full check.
  if (!from) console.log('# no literal colour: no git base (set REET_THEME_BASE or fetch origin/main), so only Logo.svelte was checked')
  if (from) {
    // A line this branch rewrites may carry a literal it already had; only literals the branch adds count.
    const added = new Map(), removed = new Map()
    let current = ''
    for (const line of git(['diff', '-U0', from, '--', 'src/**/*.svelte']).split('\n')) {
      if (line.startsWith('+++ ')) { current = line.slice(6); continue }
      if (line.startsWith('--- ')) continue
      const into = line.startsWith('+') ? added : line.startsWith('-') ? removed : null
      if (!into) continue
      for (const m of line.slice(1).matchAll(new RegExp(LITERAL.source, 'g'))) into.set(`${current}\t${m[0]}`, (into.get(`${current}\t${m[0]}`) ?? 0) + 1)
    }
    for (const [key, n] of added) if (n > (removed.get(key) ?? 0)) offences.push(key.replaceAll('\t', ': adds '))
    for (const file of git(['ls-files', '--others', '--exclude-standard', '--', 'src']).split('\n').filter(f => f.endsWith('.svelte')))
      strip(readFileSync(join(web, file), 'utf8')).split('\n').forEach((line, i) => { if (LITERAL.test(line)) offences.push(`${file}:${i + 1}: ${line.trim()}`) })
  }
  assert.deepEqual(offences, [])
})

// applyTheme leaves the choice where index.html's pre-paint script can read it,
// so a dark preference is painted dark before the profile arrives.
const { applyTheme } = await import(`data:text/javascript;base64,${Buffer.from(compile('./api.ts')).toString('base64')}`)

function page() {
  const store = new Map()
  const root = { dataset: {} }
  globalThis.document = { documentElement: root }
  globalThis.window = { dispatchEvent: () => true }
  globalThis.CustomEvent = class { constructor(type, init) { this.type = type; this.detail = init?.detail } }
  globalThis.localStorage = { getItem: key => store.get(key) ?? null, setItem: (key, value) => store.set(key, value) }
  return { root, store }
}

test('a chosen theme is cached for the next first paint', () => {
  const { root, store } = page()
  applyTheme('dark')
  assert.equal(root.dataset.theme, 'dark')
  assert.equal(store.get('reet-theme'), 'dark')
  applyTheme('light')
  assert.equal(root.dataset.theme, 'light')
  assert.equal(store.get('reet-theme'), 'light')
})

test('following the system clears the attribute and caches that choice', () => {
  const { root, store } = page()
  applyTheme('dark')
  applyTheme('system')
  assert.equal('theme' in root.dataset, false)
  assert.equal(store.get('reet-theme'), 'system')
})

test('a blocked store still applies the theme to this visit', () => {
  const { root } = page()
  globalThis.localStorage.setItem = () => { throw new Error('storage disabled') }
  applyTheme('dark')
  assert.equal(root.dataset.theme, 'dark')
})

test('the boot script reads the key applyTheme writes', () => {
  const html = readFileSync(join(web, 'index.html'), 'utf8')
  assert.match(html, /localStorage\.getItem\('reet-theme'\)/)
})
