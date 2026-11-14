import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = readFileSync(new URL('./agentAccounts.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const a = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)

const now = Date.UTC(2026, 10, 14, 9, 0, 0)
const minute = 60_000, hour = 60 * minute, day = 24 * hour
const signedIn = (over = {}) => ({ tool: 'claude', state: 'signed_in', method: 'subscription', account: 'person@example.com', plan: 'max', limitsState: 'not_asked', ...over })
const limits = [{ kind: 'session', usedPercent: 19, resetsAt: now + 2 * hour + 10 * minute }, { kind: 'week', usedPercent: 60, resetsAt: now + 3 * day }, { kind: 'week', label: 'Opus', usedPercent: 92 }]

test('the sign-in line says who is signed in and on which plan', () => {
  assert.equal(a.signInLine(signedIn()), 'Signed in as person@example.com · Max plan')
  assert.equal(a.signInLine(signedIn({ plan: '' })), 'Signed in as person@example.com')
  assert.equal(a.signInLine(signedIn({ account: '' })), 'Signed in · Max plan')
  assert.equal(a.signInLine(signedIn({ account: '', plan: '' })), 'Signed in')
  assert.equal(a.signInLine(signedIn({ method: 'api_key', account: '', plan: '', limitsState: 'unsupported' })), 'Signed in with an API key')
})

test('what was not found, and what could not be read, are said as that and no more', () => {
  assert.equal(a.signInLine({ tool: 'codex', state: 'none', limitsState: 'not_asked' }), 'No sign-in found on this machine')
  assert.equal(a.signInLine({ tool: 'claude', state: 'unreadable', limitsState: 'not_asked' }), "reet can't read this agent's sign-in on this machine")
  // An agent the machine said nothing about gets no line at all.
  assert.equal(a.signInLine(undefined), '')
  assert.equal(a.accountFor({ accounts: [signedIn()] }, 'codex'), undefined)
  assert.equal(a.accountFor({ accounts: [signedIn()] }, 'claude').account, 'person@example.com')
  assert.equal(a.accountFor(null, 'claude'), undefined)
})

test('limits read as what is left, plan-wide windows first, with the reset', () => {
  const lines = a.limitLines(signedIn({ limitsState: 'read', limits, limitsAt: now }), now)
  assert.deepEqual(lines, [
    { key: 'session', name: 'Session', left: 81, low: false, text: 'Session 81% left', resets: 'resets in 2 h 10 min' },
    { key: 'week', name: 'Week', left: 40, low: false, text: 'Week 40% left', resets: 'resets in 3 days' },
    { key: 'week:Opus', name: 'Opus this week', left: 8, low: true, text: 'Opus this week 8% left', resets: '' },
  ])
  assert.deepEqual(a.limitLines(signedIn(), now), [])
  assert.deepEqual(a.limitLines(undefined, now), [])
})

test('a window is low at 15% left and under, and its bounds are held', () => {
  const low = left => a.limitLines(signedIn({ limitsState: 'read', limitsAt: now, limits: [{ kind: 'day', usedPercent: 100 - left }] }), now)[0]
  assert.equal(low(15).low, true)
  assert.equal(low(16).low, false)
  assert.equal(low(0).text, 'Day 0% left')
  const odd = a.limitLines(signedIn({ limitsState: 'read', limitsAt: now, limits: [{ kind: 'month', usedPercent: 140 }, { kind: 'month', usedPercent: -3 }] }), now)
  assert.deepEqual(odd.map(line => line.left), [0, 100])
  assert.equal(odd[0].name, 'Month')
})

test('a reset is worded by how far off it is', () => {
  assert.equal(a.resetsIn(now + 40 * 1000, now), 'resets in under a minute')
  assert.equal(a.resetsIn(now + 5 * minute, now), 'resets in 5 min')
  assert.equal(a.resetsIn(now + hour, now), 'resets in 1 h')
  assert.equal(a.resetsIn(now + 26 * hour, now), 'resets in 1 day 2 h')
  assert.equal(a.resetsIn(now + 2 * day, now), 'resets in 2 days')
  assert.equal(a.resetsIn(now - minute, now), 'reset due')
  assert.equal(a.resetsIn(0, now), '')
  assert.equal(a.resetsIn(undefined, now), '')
})

test('why there are no new figures is said, with the age of the last ones', () => {
  const kept = { limits, limitsAt: now - 12 * minute }
  assert.equal(a.limitsNote(signedIn({ limitsState: 'read', ...kept }), 'Claude Code', now), '')
  assert.equal(a.limitsNote(signedIn(), 'Claude Code', now), '')
  assert.equal(a.limitsNote(signedIn({ limitsState: 'renew' }), 'Claude Code', now), "Claude Code's sign-in needs renewing before its limits can be read. Start Claude Code on this machine once; reet never renews a sign-in itself.")
  assert.equal(a.limitsNote(signedIn({ limitsState: 'renew', ...kept }), 'Claude Code', now), "Claude Code's sign-in needs renewing before its limits can be read. Start Claude Code on this machine once; reet never renews a sign-in itself. Showing the figures read 12 min ago.")
  assert.equal(a.limitsNote(signedIn({ limitsState: 'refused' }), 'Codex', now), "Codex's provider refused the sign-in when asked for its limits.")
  assert.equal(a.limitsNote(signedIn({ limitsState: 'rate_limited', ...kept }), 'Codex', now), "Codex's provider asked reet to slow down. Showing the figures read 12 min ago.")
  assert.equal(a.limitsNote(signedIn({ limitsState: 'failed' }), 'Codex', now), "This machine couldn't read Codex's limits just now.")
  assert.equal(a.limitsNote(signedIn({ method: 'api_key', limitsState: 'unsupported' }), 'Codex', now), 'An API key has no plan limits to read.')
})

test('the start hint speaks only when no sign-in was found', () => {
  assert.equal(a.startHint({ tool: 'codex', state: 'none', limitsState: 'not_asked' }, 'Codex', 'studio'), 'No Codex sign-in was found on studio. Sign in to Codex there first, or its first message may be refused.')
  assert.equal(a.startHint(signedIn(), 'Claude Code', 'studio'), '')
  assert.equal(a.startHint({ tool: 'claude', state: 'unreadable', limitsState: 'not_asked' }, 'Claude Code', 'studio'), '')
  assert.equal(a.startHint(undefined, 'Cline', 'studio'), '')
})

test('the age of a read is short and plain', () => {
  assert.equal(a.readAgo(now - 20 * 1000, now), 'just now')
  assert.equal(a.readAgo(now - 3 * minute, now), '3 min ago')
  assert.equal(a.readAgo(now - 2 * hour, now), '2 h ago')
  assert.equal(a.readAgo(now - 3 * day, now), '3 days ago')
  assert.equal(a.readAgo(null, now), '')
})
