import { readFileSync, readdirSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { join, relative } from 'node:path'
import { test } from 'node:test'
import assert from 'node:assert/strict'

// app.css sets the product's floor: no copy or chrome below 14px. A component may be quiet in colour and weight, never
// in size. The terminal canvas draws from --terminal-font-size, which a person chooses, but the chrome around it is
// product copy like anywhere else and is checked here too.
const FLOOR = 14
const LENGTH = /(\d*\.?\d+)(px|rem|em)\b/i
const src = fileURLToPath(new URL('../', import.meta.url))

// Only the style block is type; a script may hold any string. Scan for </script>, never parse HTML with a regexp.
const styleText = text => {
  let out = '', at = 0
  for (;;) {
    const open = text.indexOf('<script', at)
    if (open < 0) return out + text.slice(at)
    const close = text.indexOf('</script', open)
    const end = close < 0 ? text.length : text.indexOf('>', close) + 1 || text.length
    out += text.slice(at, open) + text.slice(open, end).replace(/[^\n]/g, '')
    at = end
  }
}

const sizeOf = value => {
  const found = LENGTH.exec(value)
  if (!found) return null
  const n = Number(found[1])
  return found[2].toLowerCase() === 'px' ? n : n * 16
}

export function sizesIn(text) {
  const sizes = []
  for (const [line, source] of styleText(text).split('\n').entries()) {
    for (const match of source.matchAll(/font-size\s*:\s*([^;}]+)/gi)) {
      const size = sizeOf(match[1])
      if (size !== null) sizes.push({ line: line + 1, size, declaration: match[0].trim() })
    }
    for (const match of source.matchAll(/(?<![-\w])font\s*:\s*([^;}]+)/gi)) {
      // The size is the first length before the optional /line-height, which is not a font size.
      const size = sizeOf(match[1].split('/')[0])
      if (size !== null) sizes.push({ line: line + 1, size, declaration: match[0].trim() })
    }
  }
  return sizes
}

const files = (dir, found = []) => {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name)
    if (entry.isDirectory()) files(path, found)
    else if (entry.name.endsWith('.svelte') || entry.name.endsWith('.css')) found.push(path)
  }
  return found
}

test('a font size reads px and rem, and ignores the line height of a font shorthand', () => {
  assert.deepEqual(sizesIn('.a { font-size: 13px; }').map(s => s.size), [13])
  assert.deepEqual(sizesIn('.a { font-size: .8125rem; }').map(s => s.size), [13])
  assert.deepEqual(sizesIn('.a { font: 500 14px/13px var(--sans); }').map(s => s.size), [14])
  assert.deepEqual(sizesIn('.a { font: inherit; }').map(s => s.size), [])
  assert.deepEqual(sizesIn('<script>const x = "font-size: 12px"</script><style>.a { font-size: 15px; }</style>').map(s => s.size), [15])
})

test('every product surface keeps the 14px type floor', () => {
  const offences = []
  for (const file of files(src)) {
    const name = relative(src, file)
    for (const { line, size, declaration } of sizesIn(readFileSync(file, 'utf8'))) {
      if (size < FLOOR) offences.push(`${name}:${line}: ${declaration} (${size}px)`)
    }
  }
  assert.deepEqual(offences, [])
})
