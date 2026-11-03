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

test('an archived chat is a final refusal: no Try again, no Continue with history', () => {
  assert.equal(reattachRetryable('chat_reattach_archived'), false)
  assert.equal(offersContinueAfterReattach({ phase: 'failed', code: 'chat_reattach_archived', reason: 'x' }), false)
  assert.equal(quietReattachRefusal('chat_reattach_archived', true), null)
})

const { offersTrustedReattach, reattachTrust } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

test('only a folder-trust refusal offers Trust this folder and reconnect', () => {
  assert.equal(offersTrustedReattach({ phase: 'failed', code: 'chat_reattach_trust', reason: 'x', detail: 'plugins' }), true)
  assert.equal(offersTrustedReattach({ phase: 'pending', code: 'chat_reattach_trust' }), false)
  for (const code of [undefined, 'chat_reattach_failed', 'chat_reattach_ended', 'chat_reattach_archived', 'chat_reattach_starting', 'chat_reattach_conflict', 'chat_reattach_yolo_refused']) {
    assert.equal(offersTrustedReattach({ phase: 'failed', code, reason: 'x' }), false, String(code))
  }
  assert.equal(offersTrustedReattach(undefined), false)
  // The trust refusal still offers no Try again, and keeps Continue with history.
  assert.equal(reattachRetryable('chat_reattach_trust'), false)
  assert.equal(offersContinueAfterReattach({ phase: 'failed', code: 'chat_reattach_trust', reason: 'x' }), true)
})

test('trust is sent only for an explicit trusted reconnect, never by an automatic attempt', () => {
  assert.equal(reattachTrust({ auto: false, trust: true }), true)
  assert.equal(reattachTrust({ trust: true }), true)
  assert.equal(reattachTrust({ auto: true, trust: true }), false, 'an automatic attempt never trusts')
  assert.equal(reattachTrust({ auto: true }), false)
  assert.equal(reattachTrust({ auto: false }), false)
  assert.equal(reattachTrust({}), false)
  assert.equal(reattachTrust({ trust: 'true' }), false, 'only a real true trusts')
  assert.equal(reattachTrust({ trust: 1 }), false)
})

// The API helper: the exact earlier request without trust, the trust body only when asked.
const compileTS = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const api = await import(asURL(compileTS('./api.ts').replace("from '../projects'", `from '${asURL('export {}')}'`)))

function fetchRecording(status = 204, body) {
  const calls = []
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init, headers: new Headers(init.headers) })
    return new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
  }
  return calls
}

test('reattachChat sends the trust body only for trust, and the earlier request otherwise', async () => {
  const signal = new AbortController().signal
  for (const [opts, body] of [[undefined, '{}'], [{}, '{}'], [{ trust: false }, '{}'], [{ trust: true }, '{"trust":true}']]) {
    const calls = fetchRecording()
    await api.reattachChat('s 1', opts, signal)
    assert.equal(calls.length, 1)
    assert.equal(calls[0].url, '/api/v2/terminal/sessions/s%201/reattach')
    assert.equal(calls[0].init.method, 'POST')
    assert.equal(calls[0].init.body, body, JSON.stringify(opts))
    assert.equal(calls[0].init.signal, signal)
    assert.equal(calls[0].headers.get('Content-Type'), 'application/json')
  }
  // Anything but a real true sends no trust.
  for (const trust of ['true', 1, null]) {
    const calls = fetchRecording()
    await api.reattachChat('s1', { trust })
    assert.equal(calls[0].init.body, '{}', String(trust))
  }
})

test('a refused trusted reconnect surfaces the server code and detail', async () => {
  fetchRecording(409, { error: 'This chat was archived, so its history is gone.', code: 'chat_reattach_archived' })
  await assert.rejects(api.reattachChat('s1', { trust: true }), err => err.code === 'chat_reattach_archived' && err.status === 409)
})

const { reattachDetail } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

test('a folder-trust refusal shows the plugin files, not the machine\'s new-chat instruction', () => {
  const detail = 'This folder has agent extensions: .opencode/plugins/x.js, .opencode/package.json. Start a chat only in a folder you trust.'
  assert.equal(reattachDetail({ phase: 'failed', code: 'chat_reattach_trust', detail }), 'This folder has agent extensions: .opencode/plugins/x.js, .opencode/package.json.')
  // Any other detail, or a trust detail in another shape, is shown as it is.
  assert.equal(reattachDetail({ phase: 'failed', code: 'chat_reattach_trust', detail: '.opencode/plugins/x.js' }), '.opencode/plugins/x.js')
  assert.equal(reattachDetail({ phase: 'failed', code: 'chat_reattach_failed', detail }), detail)
  assert.equal(reattachDetail({ phase: 'failed', code: 'chat_reattach_trust' }), undefined)
  assert.equal(reattachDetail(undefined), undefined)
})
