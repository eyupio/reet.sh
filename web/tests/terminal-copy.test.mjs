import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('../src/lib/terminal/copy.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const c = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

function buffer(lines, { baseY = 0, viewportY = 0, missing = [] } = {}) {
  const absent = new Set(missing)
  return {
    length: lines.length,
    baseY,
    viewportY,
    getLine(index) {
      if (absent.has(index) || index < 0 || index >= lines.length) return undefined
      return { translateToString: (trimRight = false) => trimRight ? lines[index].replace(/ +$/, '') : lines[index] }
    },
  }
}

test('an empty or alternate buffer copies nothing', () => {
  assert.equal(c.screenText(buffer([]), 24), '')
  assert.equal(c.lastLines(buffer([]), 50), '')
  assert.equal(c.screenText({ length: 0, baseY: 0, viewportY: 0, getLine: () => undefined }, 24), '')
  assert.equal(c.lastLines({ length: 0, baseY: 0, viewportY: 0, getLine: () => undefined }, 200), '')
})

test('screen text runs from viewportY for the given rows, not the terminal row count', () => {
  const b = buffer(['zero', 'one', 'two', 'three', 'four', 'five'])
  assert.equal(c.screenText(b, 3), 'zero\none\ntwo')
  b.viewportY = 2
  assert.equal(c.screenText(b, 4), 'two\nthree\nfour\nfive')
  b.viewportY = 4
  assert.equal(c.screenText(b, 1), 'four')
})

test('screen text clamps a viewport beyond the buffer and rows beyond its end', () => {
  const b = buffer(['zero', 'one', 'two'])
  b.viewportY = 9
  assert.equal(c.screenText(b, 10), '')
  b.viewportY = 1
  assert.equal(c.screenText(b, 99), 'one\ntwo')
  b.viewportY = -3
  assert.equal(c.screenText(b, 2), 'zero\none')
})

test('screen text tolerates undefined lines and keeps their blank line', () => {
  const b = buffer(['zero', 'one', 'two', 'three'], { missing: [1] })
  assert.equal(c.screenText(b, 4), 'zero\n\ntwo\nthree')
})

test('last lines counts back from the end of the whole buffer', () => {
  const b = buffer(Array.from({ length: 10 }, (_, i) => `line-${i + 1}`), { baseY: 2 })
  assert.equal(c.lastLines(b, 3), 'line-8\nline-9\nline-10')
  assert.equal(c.lastLines(b, 50), 'line-1\nline-2\nline-3\nline-4\nline-5\nline-6\nline-7\nline-8\nline-9\nline-10')
})

test('last lines crosses above the bottom page and never reads past the buffer', () => {
  const b = buffer(Array.from({ length: 10 }, (_, i) => `line-${i + 1}`), { baseY: 6 })
  assert.equal(c.lastLines(b, 8), 'line-3\nline-4\nline-5\nline-6\nline-7\nline-8\nline-9\nline-10')
  assert.equal(c.lastLines(b, 0), '')
})

test('last lines keeps blank lines and trims only trailing spaces', () => {
  const b = buffer(['alpha   ', '', '  indented  ', 'omega\t '])
  assert.equal(c.lastLines(b, 4), 'alpha\n\n  indented\nomega\t')
})

test('zero or negative counts copy nothing', () => {
  const b = buffer(['zero', 'one'])
  assert.equal(c.lastLines(b, 0), '')
  assert.equal(c.lastLines(b, -5), '')
  assert.equal(c.screenText(b, 0), '')
  assert.equal(c.screenText(b, -2), '')
})

test('a single line copies without a trailing newline', () => {
  const b = buffer(['only'])
  assert.equal(c.screenText(b, 1), 'only')
  assert.equal(c.lastLines(b, 1), 'only')
})

test('all text keeps the scrollback that a last-N copy also reads', () => {
  const b = buffer(['zero', 'one', 'two', 'three'], { baseY: 2 })
  assert.equal(c.lastLines(b, 9), 'zero\none\ntwo\nthree')
  assert.equal(c.allText(b), 'zero\none\ntwo\nthree')
  assert.equal(c.allText(buffer([])), '')
})

test('select text is empty for an empty buffer', () => {
  assert.equal(c.selectionText(buffer([])), '')
  assert.equal(c.selectionText({ length: 0, baseY: 0, viewportY: 0, getLine: () => undefined }), '')
})

test('select text keeps the whole last-N block under both caps', () => {
  const b = buffer(['zero', 'one', 'two'])
  assert.equal(c.selectionText(b), 'zero\none\ntwo')
  assert.equal(c.selectionText(b, 2), 'one\ntwo')
  assert.equal(c.selectionText(b, 3, 13), 'zero\none\ntwo')
})

test('select text keeps the longest suffix of complete lines that fits the byte cap', () => {
  const b = buffer(['aaaa', 'bbbb', 'cccc'])
  assert.equal(c.selectionText(b, 2000, 14), 'aaaa\nbbbb\ncccc')
  assert.equal(c.selectionText(b, 2000, 13), 'bbbb\ncccc')
  assert.equal(c.selectionText(b, 2000, 9), 'bbbb\ncccc')
  assert.equal(c.selectionText(b, 2000, 8), 'cccc')
})

test('select text truncates one over-cap line at a code-point boundary', () => {
  const b = buffer(['abcdefghij'])
  assert.equal(c.selectionText(b, 2000, 10), 'abcdefghij')
  assert.equal(c.selectionText(b, 2000, 4), 'abcd')
  const emoji = buffer(['a😀b'])
  assert.equal(c.selectionText(emoji, 2000, 6), 'a😀b')
  assert.equal(c.selectionText(emoji, 2000, 5), 'a😀')
  assert.equal(c.selectionText(emoji, 2000, 3), 'a')
  assert.equal(c.selectionText(emoji, 2000, 1), 'a')
  assert.equal(c.selectionText(buffer(['😀']), 2000, 3), '')
  assert.equal(c.selectionText(buffer(['😀']), 2000, 4), '😀')
})

test('select text counts bytes, not characters, and never splits a UTF-8 sequence', () => {
  const b = buffer(['éé', 'üü'])
  assert.equal(c.selectionText(b, 2000, 9), 'éé\nüü')
  assert.equal(c.selectionText(b, 2000, 8), 'üü')
  assert.equal(c.selectionText(b, 2000, 4), 'üü')
  const line = 'aé𝄞'
  const clipped = c.selectionText(buffer([line]), 2000, 5)
  assert.equal(clipped, 'aé')
  const encoded = new TextEncoder().encode(clipped)
  assert.equal(encoded.length, 3)
  assert.equal(new TextDecoder('utf-8', { fatal: true }).decode(encoded), 'aé')
  assert.equal(c.selectionText(buffer([line]), 2000, 7), line)
})

test('select text defaults to the last 2,000 lines and 256 KiB', () => {
  const many = buffer(Array.from({ length: 2005 }, (_, i) => `line-${i + 1}`))
  const kept = c.selectionText(many)
  assert.ok(kept.startsWith('line-6\n'))
  assert.ok(kept.endsWith('\nline-2005'))
  assert.equal(kept.split('\n').length, 2000)
  const huge = buffer([`${'x'.repeat(300 * 1024)}`])
  assert.equal(c.selectionText(huge).length, 256 * 1024)
  assert.equal(c.selectionText(huge), 'x'.repeat(256 * 1024))
})
