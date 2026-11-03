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

const { reattachRetryable, offersContinueAfterReattach, quietReattachRefusal } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

test('only a refusal a retry can fix offers Try again', () => {
  for (const code of [undefined, 'chat_reattach_failed', 'chat_reattach_starting', 'chat_reattach_conflict']) assert.equal(reattachRetryable(code), true, String(code))
  for (const code of ['chat_reattach_yolo_not_allowed', 'chat_reattach_yolo_refused', 'chat_reattach_capacity', 'chat_reattach_trust', 'chat_reattach_ended', 'chat_reattach_unsupported', 'chat_reattach_no_conversation']) assert.equal(reattachRetryable(code), false, code)
})

test('a folder with plugins offers Continue with history again, nothing else does', () => {
  assert.equal(offersContinueAfterReattach({ phase: 'failed', code: 'chat_reattach_trust', reason: 'x' }), true)
  assert.equal(offersContinueAfterReattach({ phase: 'pending', code: 'chat_reattach_trust' }), false)
  assert.equal(offersContinueAfterReattach({ phase: 'failed', code: 'chat_reattach_failed' }), false)
  assert.equal(offersContinueAfterReattach(undefined), false)
  assert.equal(offersContinueAfterReattach({ phase: 'failed', code: 'chat_reattach_ended', reason: 'x' }), true)
  assert.equal(offersContinueAfterReattach({ phase: 'pending', code: 'chat_reattach_ended' }), false)
  assert.equal(reattachRetryable('chat_reattach_ended'), false)
  assert.equal(quietReattachRefusal('chat_reattach_trust', false), null)
  assert.equal(quietReattachRefusal('chat_reattach_trust', true), null)
})

test('a chat the person ended falls back quietly for an automatic attempt only, a manual one shows the refusal; a still-running chat does so only for an automatic attempt', () => {
  assert.equal(quietReattachRefusal('chat_reattach_ended', true), 'ended')
  assert.equal(quietReattachRefusal('chat_reattach_ended', false), null)
  assert.equal(quietReattachRefusal('chat_reattach_not_closed', true), 'drop')
  assert.equal(quietReattachRefusal('chat_reattach_not_closed', false), null)
  assert.equal(quietReattachRefusal('chat_reattach_failed', true), null)
  assert.equal(quietReattachRefusal(undefined, true), null)
})
