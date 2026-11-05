import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./fixDelivery.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const d = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

test('a terminal session has no composer, so its button copies and says so', () => {
  assert.equal(d.fixButtonText(false, 'Ask the agent to fix'), 'Copy message for the agent')
  assert.equal(d.fixRoute(false), 'clipboard')
})

test('a chat keeps the tracker wording and tries the composer first', () => {
  assert.equal(d.fixButtonText(true, 'Ask the agent to fix'), 'Ask the agent to fix')
  assert.equal(d.fixRoute(true), 'composer')
})

test('a copy from a full composer says why, and a terminal copy just says Copied', () => {
  assert.equal(d.copyNote(true, true), 'Copied. The composer is nearly full.')
  assert.equal(d.copyNote(false, true), 'Copied')
  assert.match(d.copyNote(true, false), /Could not copy/)
})
