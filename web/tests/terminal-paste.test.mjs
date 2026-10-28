import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('../src/lib/terminal/paste.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const p = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

const KIB = 1024

test('sanitise removes bracket markers wherever they sit, even split across the text', () => {
  assert.equal(p.sanitizePaste('\x1b[200~hello\x1b[201~'), 'hello')
  assert.equal(p.sanitizePaste('before\x1b[200~middle\x1b[201~after'), 'beforemiddleafter')
  assert.equal(p.sanitizePaste('\x1b[200~\x1b[200~two\x1b[201~'), 'two')
  assert.equal(p.sanitizePaste('a\x1b[201~b\x1b[200~c'), 'abc')
})

test('sanitise removes every remaining ESC byte and nothing else', () => {
  const clean = p.sanitizePaste('\x1b[31mred\x1b[0m\x1b]0;title\x07tail')
  assert.equal(clean, '[31mred[0m]0;title\x07tail')
  assert.equal(clean.includes('\x1b'), false)
  assert.equal(p.sanitizePaste('\x1b\x1b\x1b'), '')
})

test('newlines and tabs survive sanitising', () => {
  const text = 'one\ttwo\nthree\r\nfour'
  assert.equal(p.sanitizePaste(text), text)
})

test('sanitise is idempotent', () => {
  for (const text of ['\x1b[200~a\x1b[201~\x1b[1mb\n\t', 'plain', '', '\x1b\x1b[201~']) {
    const once = p.sanitizePaste(text)
    assert.equal(p.sanitizePaste(once), once)
  }
})

test('the 240 KiB cap counts UTF-8 bytes of the sanitised text', () => {
  assert.equal(p.MAX_PASTE_BYTES, 240 * KIB)
  assert.equal(p.utf8ByteLength('a'), 1)
  assert.equal(p.utf8ByteLength('😀'), 4)
  assert.equal(p.pasteTooLarge('a'.repeat(p.MAX_PASTE_BYTES)), false)
  assert.equal(p.pasteTooLarge('a'.repeat(p.MAX_PASTE_BYTES + 1)), true)
  assert.equal(p.pasteTooLarge('😀'.repeat(p.MAX_PASTE_BYTES / 4)), false)
  assert.equal(p.pasteTooLarge('😀'.repeat(p.MAX_PASTE_BYTES / 4) + 'a'), true)
})

test('execute and force always review, whatever else the text looks like', () => {
  assert.deepEqual(p.pasteNeedsReview('ok', { bracketed: true, execute: true }), { review: true, reason: 'execute' })
  assert.deepEqual(p.pasteNeedsReview('ok', { bracketed: true, force: true }), { review: true, reason: 'execute' })
})

test('a C0 or C1 control other than tab and newline always reviews', () => {
  for (const text of ['bell\x07', 'escape\x1b', 'carriage return\rmid', 'nul\x00', 'vertical\x0btab', 'delete\x7f', 'c1\x9b']) {
    assert.deepEqual(p.pasteNeedsReview(text, { bracketed: true }), { review: true, reason: 'control' })
  }
  assert.deepEqual(p.pasteNeedsReview('\tand\nnewline stay safe', { bracketed: true }), { review: false, reason: '' })
})

test('more than 64 KiB reviews even with bracketed paste on', () => {
  assert.equal(p.MAX_REVIEW_BYTES, 64 * KIB)
  assert.deepEqual(p.pasteNeedsReview('a'.repeat(p.MAX_REVIEW_BYTES), { bracketed: true }), { review: false, reason: '' })
  assert.deepEqual(p.pasteNeedsReview('a'.repeat(p.MAX_REVIEW_BYTES + 1), { bracketed: true }), { review: true, reason: 'large' })
})

test('without bracketed paste, newlines review even when short', () => {
  assert.deepEqual(p.pasteNeedsReview('one\ntwo', { bracketed: false }), { review: true, reason: 'newline' })
  assert.deepEqual(p.pasteNeedsReview('one\ntwo', {}), { review: true, reason: 'newline' })
})

test('without bracketed paste, more than 2,000 characters reviews', () => {
  assert.equal(p.MAX_REVIEW_CHARS, 2000)
  assert.deepEqual(p.pasteNeedsReview('a'.repeat(p.MAX_REVIEW_CHARS), { bracketed: false }), { review: false, reason: '' })
  assert.deepEqual(p.pasteNeedsReview('a'.repeat(p.MAX_REVIEW_CHARS + 1), { bracketed: false }), { review: true, reason: 'long' })
})

test('bracketed paste lets ordinary multi-line text through without review', () => {
  assert.deepEqual(p.pasteNeedsReview('one\ntwo', { bracketed: true }), { review: false, reason: '' })
  assert.deepEqual(p.pasteNeedsReview('a'.repeat(p.MAX_REVIEW_CHARS + 1), { bracketed: true }), { review: false, reason: '' })
})

test('empty text after sanitising needs no review', () => {
  const clean = p.sanitizePaste('\x1b[200~\x1b[201~\x1b')
  assert.equal(clean, '')
  assert.deepEqual(p.pasteNeedsReview(clean, { bracketed: true }), { review: false, reason: '' })
})
