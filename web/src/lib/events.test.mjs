import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
// Compile the pure label helper without a browser or an extra test dependency.
const { outputText } = ts.transpileModule(readFileSync(new URL('./events.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { eventLabel } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
const event = (action, detail) => ({ id: 1, at: 0, source: 'ui', action, ...(detail ? { detail: JSON.stringify(detail) } : {}) })
test('ownership receipt events read as plain sentences from their metadata', () => {
  assert.equal(eventLabel(event('skill-content-adopt', { name: 'demo', files: 3, receiptId: 'a'.repeat(48) })), 'Recorded ownership of 3 files of demo')
  assert.equal(eventLabel(event('skill-content-release', { name: 'demo', receiptId: 'a'.repeat(48) })), 'Released ownership of demo')
})
test('other activity labels are unchanged', () => {
  assert.equal(eventLabel(event('add', { name: 'demo' })), 'Added demo')
  assert.equal(eventLabel(event('merge-machines', { from: 'old', to: 'new' })), 'Consolidated machine old as new')
  assert.equal(eventLabel(event('frobnicate', { name: 'demo' })), 'frobnicate demo')
})
