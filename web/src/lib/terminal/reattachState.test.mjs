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

const { recoveryInFlight, continuationFailureInPane } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

test('a reconnect and a Continue with history of the same chat never run at once', () => {
  assert.equal(recoveryInFlight('s1', {}, {}), false)
  assert.equal(recoveryInFlight('s1', { s1: { phase: 'pending' } }, {}), true)
  assert.equal(recoveryInFlight('s1', {}, { s1: { phase: 'pending' } }), true)
  assert.equal(recoveryInFlight('s1', { s1: { phase: 'failed', reason: 'x' } }, { s1: { phase: 'failed', reason: 'y' } }), false)
  assert.equal(recoveryInFlight('s1', { other: { phase: 'pending' } }, { other: { phase: 'pending' } }), false)
})

test('a failed Continue with history is shown in the pane only where the pane will show it', () => {
  assert.equal(continuationFailureInPane({ structured: true, selected: true, offersReconnect: false }), true)
  assert.equal(continuationFailureInPane({ structured: true, selected: true, offersReconnect: true }), false, 'the pane shows Reconnect instead')
  assert.equal(continuationFailureInPane({ structured: true, selected: false, offersReconnect: false }), false, 'another session is on screen')
  assert.equal(continuationFailureInPane({ structured: false, selected: true, offersReconnect: false }), false, 'a terminal has no chat pane')
})
