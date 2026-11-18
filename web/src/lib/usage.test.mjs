import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

// usage.ts takes its token formatter from gafferOutcome.ts and its plan wording from agentAccounts.ts; both are
// self-contained, so each is compiled on its own and handed to usage.ts in place of its relative import.
const compile = file => ts.transpileModule(readFileSync(new URL(file, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const url = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const source = compile('./usage.ts').replace(`'./gafferOutcome'`, `'${url(compile('./gafferOutcome.ts'))}'`).replace(`'./agentAccounts'`, `'${url(compile('./agentAccounts.ts'))}'`)
const u = await import(url(source))

const minute = 60_000, hour = 60 * minute, day = 24 * hour
const now = Date.UTC(2026, 10, 18, 15, 30)

test('the three periods start at local midnight in the named zone and end at the next one', () => {
  // 18 November 2026, 15:30 UTC is 10:30 in New York (UTC-5) and 02:30 on the 19th in Tokyo (UTC+9).
  assert.deepEqual(u.periodRange('today', 'America/New_York', now), { from: Date.UTC(2026, 10, 18, 5), to: Date.UTC(2026, 10, 19, 5) })
  assert.deepEqual(u.periodRange('today', 'Asia/Tokyo', now), { from: Date.UTC(2026, 10, 18, 15), to: Date.UTC(2026, 10, 19, 15) })
  assert.deepEqual(u.periodRange('today', 'UTC', now), { from: Date.UTC(2026, 10, 18), to: Date.UTC(2026, 10, 19) })
  // 7 days is today and the six before it; 30 days likewise.
  assert.deepEqual(u.periodRange('week', 'UTC', now), { from: Date.UTC(2026, 10, 12), to: Date.UTC(2026, 10, 19) })
  assert.deepEqual(u.periodRange('month', 'UTC', now), { from: Date.UTC(2026, 9, 20), to: Date.UTC(2026, 10, 19) })
  // An unknown period is today; an unknown zone falls back to the browser's own and still gives a day-long range.
  assert.deepEqual(u.periodRange('nonsense', 'UTC', now), u.periodRange('today', 'UTC', now))
  const loose = u.periodRange('today', 'Not/AZone', now)
  assert.ok(loose.from <= now && loose.to > now && loose.to - loose.from >= 23 * hour && loose.to - loose.from <= 25 * hour)
})

test('a range across a change of clocks counts local days, not 24-hour blocks', () => {
  // London's clocks went back at 02:00 BST on 25 October 2026. On 26 October at noon, seven days back starts at local
  // midnight on 20 October, which is still summer time (23:00 UTC the day before).
  const at = Date.UTC(2026, 9, 26, 12)
  const week = u.periodRange('week', 'Europe/London', at)
  assert.equal(week.from, Date.UTC(2026, 9, 19, 23))
  assert.equal(week.to, Date.UTC(2026, 9, 27, 0))
  // That week holds a 25-hour day, so it is longer than seven times 24 hours.
  assert.equal(week.to - week.from, 7 * day + hour)
  // The day of the change itself is 25 hours long.
  const sunday = u.periodRange('today', 'Europe/London', Date.UTC(2026, 9, 25, 12))
  assert.equal(sunday.from, Date.UTC(2026, 9, 24, 23))
  assert.equal(sunday.to - sunday.from, 25 * hour)
  // New York springs forward on 8 March 2026: that day is 23 hours long.
  const spring = u.periodRange('today', 'America/New_York', Date.UTC(2026, 2, 8, 18))
  assert.equal(spring.from, Date.UTC(2026, 2, 8, 5))
  assert.equal(spring.to - spring.from, 23 * hour)
  // Every period stays inside what the controller accepts.
  for (const period of ['today', 'week', 'month']) {
    const r = u.periodRange(period, 'Pacific/Chatham', at)
    assert.ok(r.to > r.from && r.to - r.from <= 92 * day, period)
  }
})

test('totals are words a person reads, with the counted-since note only when the range starts before it', () => {
  const answer = (over = {}) => ({ from: Date.UTC(2026, 10, 12), to: Date.UTC(2026, 10, 19), since: Date.UTC(2026, 10, 14, 9), agents: [
    { tool: 'claude', name: 'Claude Code', reports: 'chat', input: 1_200_000, output: 100_000, cached: 0, sessions: 14, factoryStages: 3 },
    { tool: 'opencode', name: 'OpenCode', reports: 'chat', input: 250_000, output: 50_000, cached: 0, sessions: 7, factoryStages: 0 },
    { tool: 'copilot', name: 'GitHub Copilot', reports: 'terminal_only', input: 0, output: 0, cached: 0, sessions: 0, factoryStages: 0 },
  ], gaffer: [], sessions: [], more: false, ...over })
  assert.equal(u.totalsLine(answer(), 'UTC', 'dmy'), '1.6M tokens across 21 sessions · counted since 14 Nov')
  // A range that starts after the first count does not say since when.
  assert.equal(u.totalsLine(answer({ since: Date.UTC(2026, 9, 1) }), 'UTC', 'dmy'), '1.6M tokens across 21 sessions')
  assert.equal(u.totalsLine(answer({ agents: [{ tool: 'claude', name: 'Claude Code', reports: 'chat', input: 900, output: 99, cached: 0, sessions: 1, factoryStages: 0 }] }), 'UTC', 'dmy'), '999 tokens across 1 session · counted since 14 Nov')
  // Counted before, but nothing in this range.
  assert.equal(u.totalsLine(answer({ agents: [] }), 'UTC', 'dmy'), 'No tokens reported in this period · counted since 14 Nov')
  // Nothing counted at all, ever.
  assert.equal(u.totalsLine(answer({ since: 0, agents: [] }), 'UTC', 'dmy'), 'Nothing has been counted yet. reet counts what your agents report from now on.')
  assert.equal(u.tokens(1_234_567), '1.2M')
  assert.equal(u.tokens(12_345), '12k')
  assert.equal(u.tokens(1_234), '1,234')
})

test('each agent says what it used, or why there is nothing', () => {
  const agent = (over = {}) => ({ tool: 'claude', name: 'Claude Code', reports: 'chat', input: 1_100_000, output: 100_000, cached: 900_000, sessions: 14, factoryStages: 3, ...over })
  assert.deepEqual(u.agentTokens(agent()), { line: '1.2M tokens · 14 sessions · 3 Factory stages', detail: 'Input 1.1M · Output 100k · Cached 900k' })
  assert.deepEqual(u.agentTokens(agent({ input: 10, output: 5, cached: 0, sessions: 1, factoryStages: 1 })), { line: '15 tokens · 1 session · 1 Factory stage', detail: 'Input 10 · Output 5' })
  assert.deepEqual(u.agentTokens(agent({ input: 0, output: 0, cached: 0, sessions: 0, factoryStages: 0 })), { line: 'No tokens reported in this period.', detail: '' })
  assert.deepEqual(u.agentTokens(agent({ tool: 'copilot', name: 'GitHub Copilot', reports: 'terminal_only', input: 0, output: 0, cached: 0, sessions: 0, factoryStages: 0 })), { line: 'GitHub Copilot runs only in a terminal here, which reports no token counts.', detail: '' })
})

test('an agent without plan figures gets one sentence chosen from what is known', () => {
  const machine = (host, over = {}) => ({ host, reads: true, terminalOff: false, answer: { state: 'read', sentence: '', planLimits: true, checkedAt: now, accounts: [] }, ...over })
  const withAccount = (host, account) => machine(host, { answer: { state: 'read', sentence: '', planLimits: true, checkedAt: now, accounts: [account] } })
  const claude = over => ({ tool: 'claude', state: 'signed_in', method: 'subscription', account: 'sam@example.test', plan: 'max', limitsState: 'read', ...over })

  assert.equal(u.planSentence('opencode', 'OpenCode', [machine('studio')], now), "reet can't read this agent's plan. Only Claude Code's and Codex's are read today.")
  assert.equal(u.planSentence('claude', 'Claude Code', [], now), 'No machine is connected that could read Claude Code’s sign-in.')
  assert.equal(u.planSentence('claude', 'Claude Code', [machine('old-box', { reads: false, answer: null })], now), 'Update reet on old-box to see which agents are signed in.')
  assert.equal(u.planSentence('claude', 'Claude Code', [machine('studio', { reads: false, terminalOff: true, answer: null })], now), "Terminal access is off on studio, so reet doesn't read its agents' sign-ins.")
  assert.equal(u.planSentence('claude', 'Claude Code', [withAccount('studio', { tool: 'claude', state: 'none', limitsState: 'not_asked' })], now), 'No sign-in found on studio.')
  assert.equal(u.planSentence('claude', 'Claude Code', [machine('studio'), machine('laptop')], now), 'No sign-in found on studio or laptop.')
  assert.equal(u.planSentence('codex', 'Codex', [withAccount('studio', { tool: 'codex', state: 'signed_in', method: 'api_key', limitsState: 'unsupported' })], now), 'Signed in with an API key on studio: there is no plan allowance to read.')
  assert.equal(u.planSentence('claude', 'Claude Code', [withAccount('studio', claude({ limitsState: 'failed' }))], now), "The latest read on studio failed: this machine couldn't read Claude Code's limits just now.")
  assert.equal(u.planSentence('claude', 'Claude Code', [withAccount('studio', claude({ limitsState: 'renew' }))], now), "The latest read on studio failed: Claude Code's sign-in needs renewing before its limits can be read. Start Claude Code on this machine once; reet never renews a sign-in itself.")
  assert.equal(u.planSentence('claude', 'Claude Code', [withAccount('studio', { tool: 'claude', state: 'unreadable', limitsState: 'not_asked' })], now), "reet can't read Claude Code's sign-in on studio.")
  assert.equal(u.planSentence('claude', 'Claude Code', [withAccount('studio', claude({ limitsState: 'not_asked' }))], now), 'studio has not read the plan yet. It reads it the next time it is asked.')
  // The machine's own answer says why it has nothing newer.
  assert.equal(u.planSentence('claude', 'Claude Code', [machine('studio', { answer: { state: 'offline', sentence: 'That machine is not connected.', planLimits: true, checkedAt: null, accounts: [] } })], now), 'studio: That machine is not connected.')
  assert.equal(u.planSentence('claude', 'Claude Code', [machine('studio', { answer: null, error: true })], now), "reet couldn't ask studio for its sign-ins just now.")
  // A failed read is said before a machine that simply has no sign-in.
  assert.equal(u.planSentence('claude', 'Claude Code', [machine('laptop'), withAccount('studio', claude({ limitsState: 'refused' }))], now), "The latest read on studio failed: Claude Code's provider refused the sign-in when asked for its limits.")
})

test('the plans to draw for an agent come from every machine, one line per account', () => {
  const read = (host, limitsAt, usedPercent) => ({ host, reads: true, terminalOff: false, answer: { state: 'read', sentence: '', planLimits: true, checkedAt: now, accounts: [{ tool: 'claude', state: 'signed_in', method: 'subscription', account: 'sam@example.test', plan: 'max', limitsState: 'read', limitsAt, limits: [{ kind: 'session', usedPercent, resetsAt: now + 2 * hour + 10 * minute }] }] } })
  const plans = u.agentPlans('claude', [read('laptop', now - hour, 50), read('studio', now - minute, 19)], now)
  assert.equal(plans.length, 1)
  assert.equal(plans[0].host, 'studio')
  assert.deepEqual(u.planLines(plans[0], now).map(line => line.words), ['Session 81% left · resets in 2 h 10 min'])
  assert.deepEqual(u.agentPlans('codex', [read('studio', now, 19)], now), [])
})

test('a session row says how much context is left, and where a session cannot be opened', () => {
  assert.equal(u.contextLeftText({ contextUsed: 90_000, contextSize: 200_000 }), '55% left')
  assert.equal(u.contextLeftText({ contextUsed: 0, contextSize: 0 }), '')
  assert.equal(u.sessionHref({ id: 'abc', archived: false }), '/sessions?session=abc')
  assert.equal(u.sessionHref({ id: 'abc', archived: true }), '')
  assert.equal(u.sessionTitle({ title: '', tool: 'codex' }, 'Codex'), 'Codex session')
  assert.equal(u.sessionTitle({ title: 'Fix the login', tool: 'codex' }, 'Codex'), 'Fix the login')
})

test('the period is remembered per account and read back safely', () => {
  assert.equal(u.periodKey(7), 'reet.usage.period.7')
  assert.equal(u.readPeriod({ getItem: () => 'week' }, 7), 'week')
  assert.equal(u.readPeriod({ getItem: () => 'year' }, 7), 'today')
  assert.equal(u.readPeriod({ getItem: () => { throw new Error('blocked') } }, 7), 'today')
  assert.equal(u.readPeriod(undefined, 7), 'today')
  let kept = ''
  u.keepPeriod({ setItem: (key, value) => { kept = `${key}=${value}` } }, 7, 'month')
  assert.equal(kept, 'reet.usage.period.7=month')
  assert.doesNotThrow(() => u.keepPeriod({ setItem: () => { throw new Error('full') } }, 7, 'month'))
})
