import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('../src/lib/terminal/upload.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const u = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

const MB = 1 << 20
const LIMIT = 5 << 20
const types = ['image/png', 'image/jpeg', 'image/gif', 'image/webp']

test('every accepted image type is accepted at the exact size limit', () => {
  assert.equal(u.MAX_IMAGE_BYTES, LIMIT)
  assert.deepEqual(u.ACCEPTED_IMAGE_TYPES, types)
  for (const type of types) assert.deepEqual(u.validateImage({ type, size: LIMIT }), { ok: true }, type)
})

test('an image one byte over the limit is refused for size', () => {
  assert.deepEqual(u.validateImage({ type: 'image/png', size: LIMIT + 1 }), { ok: false, reason: 'size' })
})

test('an empty image is refused before its type is considered', () => {
  assert.deepEqual(u.validateImage({ type: 'image/png', size: 0 }), { ok: false, reason: 'empty' })
  assert.deepEqual(u.validateImage({ type: 'text/plain', size: 0 }), { ok: false, reason: 'empty' })
})

test('SVG, HTML, uppercase and unknown types are refused', () => {
  for (const type of ['image/svg+xml', 'text/html', 'IMAGE/PNG', 'image/PNG', 'application/octet-stream', '']) {
    assert.deepEqual(u.validateImage({ type, size: 512 }), { ok: false, reason: 'type' }, type)
  }
})

test('sizes format as bytes, kilobytes and megabytes with one decimal', () => {
  assert.equal(u.formatImageSize(0), '0 B')
  assert.equal(u.formatImageSize(512), '512 B')
  assert.equal(u.formatImageSize(1023), '1023 B')
  assert.equal(u.formatImageSize(1024), '1.0 KB')
  assert.equal(u.formatImageSize(2150), '2.1 KB')
  assert.equal(u.formatImageSize(MB), '1.0 MB')
  assert.equal(u.formatImageSize(LIMIT), '5.0 MB')
  assert.equal(u.formatImageSize(Math.round(1.5 * MB)), '1.5 MB')
})

test('insertion is fenced to the generation captured when the upload began', () => {
  assert.equal(u.uploadInsertionAllowed(7, 7, true, true), true)
  assert.equal(u.uploadInsertionAllowed(7, 8, true, true), false)
  assert.equal(u.uploadInsertionAllowed(7, 7, false, true), false)
  assert.equal(u.uploadInsertionAllowed(7, 7, true, false), false)
})

test('references join with one leading and trailing space, and none gives an empty string', () => {
  assert.equal(u.joinReferences([]), '')
  assert.equal(u.joinReferences(['/tmp/a.png']), ' /tmp/a.png ')
  assert.equal(u.joinReferences(['/tmp/a.png', "'/tmp/b b.png'"]), " /tmp/a.png '/tmp/b b.png' ")
})

test('image items are extracted from clipboard and drop item lists', () => {
  const png = { name: 'shot.png', type: 'image/png', size: 4 }
  const jpeg = { name: 'photo.jpg', type: 'image/jpeg', size: 8 }
  const items = [
    { kind: 'string', type: 'text/plain' },
    { kind: 'file', type: 'image/png', getAsFile: () => png },
    { kind: 'file', type: 'text/plain', getAsFile: () => null },
    { kind: 'file', type: 'image/jpeg', getAsFile: () => jpeg },
  ]
  assert.deepEqual(u.imageFiles(items), [png, jpeg])
  assert.deepEqual(u.imageFiles([]), [])
  assert.deepEqual(u.imageFiles(null), [])
  assert.deepEqual(u.imageFiles(undefined), [])
})

test('an SVG file item is extracted so validation can refuse it rather than pasting text', () => {
  const svg = { name: 'drawing.svg', type: 'image/svg+xml', size: 12 }
  assert.deepEqual(u.imageFiles([{ kind: 'file', type: 'image/svg+xml', getAsFile: () => svg }]), [svg])
})
