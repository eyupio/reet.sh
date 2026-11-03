import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./reattachState.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { shouldAutoReattach } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

const args = extra => ({ canReattach: true, phase: 'idle', tried: new Set(), sessionId: 's1', ...extra })

test('the first view of a lost chat that can reattach tries once by itself', () => {
  assert.equal(shouldAutoReattach(args({})), true)
})

test('a chat already tried on this page load is not tried again by itself', () => {
  assert.equal(shouldAutoReattach(args({ tried: new Set(['s1']) })), false)
  assert.equal(shouldAutoReattach(args({ tried: new Set(['other']) })), true)
})

test('a pending or failed attempt is never restarted automatically', () => {
  assert.equal(shouldAutoReattach(args({ phase: 'pending' })), false)
  assert.equal(shouldAutoReattach(args({ phase: 'failed' })), false)
})

test('a chat the machine cannot reattach is left alone', () => {
  assert.equal(shouldAutoReattach(args({ canReattach: false })), false)
})
