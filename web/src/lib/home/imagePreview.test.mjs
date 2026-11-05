import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = readFileSync(new URL('./imagePreview.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { placePreview } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)

const viewport = { width: 1200, height: 800 }
const anchor = (over = {}) => ({ left: 500, top: 300, bottom: 316, width: 26, height: 16, ...over })
const size = { width: 320, height: 260 }

test('a preview opens below its icon and is centred on it', () => {
  assert.deepEqual(placePreview(anchor(), size, viewport), { left: 353, top: 326, placement: 'below' })
})

test('a preview flips above when there is more room there', () => {
  assert.deepEqual(placePreview(anchor({ top: 700, bottom: 716 }), size, viewport), { left: 353, top: 430, placement: 'above' })
})

test('a preview stays inside the viewport on the left and right', () => {
  assert.equal(placePreview(anchor({ left: 0, width: 12 }), size, viewport).left, 8)
  assert.equal(placePreview(anchor({ left: 1180, width: 12 }), size, viewport).left, 1200 - 320 - 8)
})

test('a preview taller than the window is kept at the top margin', () => {
  const tall = { width: 320, height: 760 }
  const place = placePreview(anchor({ top: 60, bottom: 76 }), tall, { width: 1200, height: 700 })
  assert.equal(place.top, 8)
  assert.equal(place.left, 353)
})

test('custom gap and margin are honoured', () => {
  assert.deepEqual(placePreview(anchor(), size, viewport, 4, 2), { left: 353, top: 320, placement: 'below' })
})
