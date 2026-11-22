import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = path => ts.transpileModule(readFileSync(new URL(path, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const url = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const catalogue = url(compile('../catalogue.ts'))
const models = url(compile('../models.ts').replace("'./catalogue'", JSON.stringify(catalogue)))
const accounts = url(compile('../agentAccounts.ts'))
const api = url(compile('../terminal/api.ts'))
const r = await import(url(compile('./launchReadiness.ts')
  .replace("'../agentAccounts'", JSON.stringify(accounts)).replace("'../catalogue'", JSON.stringify(catalogue))
  .replace("'../models'", JSON.stringify(models)).replace("'../terminal/api'", JSON.stringify(api))))

const now = Date.UTC(2026, 10, 22, 12, 0, 0), HOUR = 3_600_000
const machine = (over = {}) => ({ agentId: 'a1', host: 'workshop', ready: true, serverEnabled: true, capabilities: { terminalControl: true, terminalEnabled: true, readiness: 'ready', acpStartOptions: true }, ...over })
const listed = (ids, over = {}) => ({ tool: 'claude', state: 'listed', models: ids.map(id => ({ id, provider: '' })), more: false, source: 'cli', version: '1', fetchedAt: now - 5 * 60_000, checkedAt: now, reason: '', sentence: '', ...over })
const signed = (tool, over = {}) => ({ tool, state: 'signed_in', method: 'subscription', account: 'me@example.test', plan: 'max', limitsState: 'read', limitsAt: now - 10 * 60_000, limits: [{ kind: 'week', usedPercent: 30 }], ...over })
const read = (...list) => ({ state: 'read', sentence: '', planLimits: true, checkedAt: now, accounts: list })
const input = (over = {}) => ({
  machine: machine(), tools: ['claude', 'codex', 'opencode'], efforts: tool => tool === 'claude' ? ['', 'low', 'high'] : [''],
  models: tool => listed([`${tool}-big`]), accounts: read(signed('claude'), signed('codex')), now,
  member: { tool: 'claude', model: 'claude-big', effort: 'high' }, spares: [], saved: { tool: 'claude', model: 'claude-big', effort: 'high' }, edited: true, ...over,
})

test('a plan the machine reports on is ready', () => {
  const s = r.launchReadiness(input())
  assert.equal(s.level, 'ready'); assert.equal(s.issue, ''); assert.equal(s.machine, 'workshop')
  assert.deepEqual([s.agent.state, s.model.state, s.effort.state, s.signIn.state], ['ready', 'ready', 'ready', 'ready'])
  assert.equal(s.checkedAtStart, false)
})

test('leaving model and effort to the agent is a choice, not something missing', () => {
  const s = r.launchReadiness(input({ member: { tool: 'claude', model: '', effort: '' }, saved: { tool: 'claude' } }))
  assert.equal(s.model.state, 'default'); assert.equal(s.model.label, 'Agent’s default model')
  assert.equal(s.effort.state, 'default'); assert.equal(s.effort.label, 'Agent’s default effort')
  assert.equal(s.level, 'ready')
})

test('missing, stale, unavailable and default stay distinct for the model', () => {
  const missing = r.launchReadiness(input({ member: { tool: 'claude', model: 'nope' } }))
  assert.equal(missing.model.state, 'missing'); assert.match(missing.model.text, /workshop’s list, read 5 min ago, has no nope/); assert.equal(missing.level, 'look'); assert.equal(missing.checkedAtStart, true)
  const stale = r.launchReadiness(input({ models: () => listed(['claude-big'], { fetchedAt: now - 30 * HOUR }) }))
  assert.equal(stale.model.state, 'stale'); assert.match(stale.model.text, /read 1 d ago/); assert.equal(stale.level, 'look')
  for (const state of ['offline', 'update', 'off', 'failed', 'unsupported']) {
    const s = r.launchReadiness(input({ models: () => listed([], { state }) }))
    assert.equal(s.model.state, 'unavailable', state); assert.equal(s.model.later, true); assert.equal(s.checkedAtStart, true); assert.equal(s.level, 'look')
  }
  assert.equal(r.launchReadiness(input({ models: () => null })).model.state, 'unavailable')
  assert.equal(r.launchReadiness(input({ models: () => listed([], { more: true }), member: { tool: 'claude', model: 'x' } })).model.state, 'unavailable')
})

test('an effort the agent does not offer starts on its default, and is unavailable, not missing', () => {
  const s = r.launchReadiness(input({ member: { tool: 'claude', model: 'claude-big', effort: 'max' } }))
  assert.equal(s.effort.state, 'unavailable'); assert.match(s.effort.text, /Max effort is not offered by Claude Code here, so the stage starts on the agent’s default effort/)
  const unknown = r.launchReadiness(input({ machine: machine({ capabilities: { terminalControl: true, terminalEnabled: true, readiness: 'ready' } }) }))
  assert.equal(unknown.effort.state, 'unavailable'); assert.equal(unknown.checkedAtStart, true)
})

test('no sign-in found is something missing that still lets the stage start', () => {
  const s = r.launchReadiness(input({ accounts: read({ tool: 'claude', state: 'none', limitsState: 'not_asked' }) }))
  assert.equal(s.signIn.state, 'missing'); assert.equal(s.signIn.text, 'No Claude Code sign-in was found on workshop. Sign in to Claude Code there first, or its first message may be refused.')
  assert.equal(s.level, 'look'); assert.match(s.issue, /No Claude Code sign-in/)
})

test('a machine that cannot read accounts is unavailable for sign-in and the stage may still start', () => {
  for (const state of ['offline', 'update', 'off', 'busy', 'failed']) {
    const s = r.launchReadiness(input({ accounts: { state, sentence: `sentence ${state}`, planLimits: true, checkedAt: null, accounts: [] } }))
    assert.equal(s.signIn.state, 'unavailable'); assert.equal(s.signIn.text, `sentence ${state}`); assert.equal(s.level, 'look')
  }
  const failed = r.launchReadiness(input({ accounts: null }))
  assert.equal(failed.signIn.state, 'unavailable'); assert.equal(failed.level, 'look')
  const reading = r.launchReadiness(input({ accounts: undefined }))
  assert.equal(reading.signIn.pending, true); assert.equal(reading.level, 'ready')
  assert.equal(r.launchReadiness(input({ accounts: read() })).signIn.state, 'unavailable')
  assert.equal(r.launchReadiness(input({ accounts: read({ tool: 'claude', state: 'unreadable', limitsState: 'not_asked' }) })).signIn.state, 'unavailable')
})

test('an agent not on the machine, or a machine not connected, blocks', () => {
  const gone = r.launchReadiness(input({ member: { tool: 'cline', model: '' }, saved: null }))
  assert.equal(gone.level, 'blocked'); assert.equal(gone.agent.state, 'unavailable'); assert.equal(gone.agent.text, 'Cline is not on workshop now. Choose another agent for the stage.')
  const off = r.launchReadiness(input({ machine: machine({ ready: false }) }))
  assert.equal(off.level, 'blocked'); assert.equal(off.connected.state, 'unavailable'); assert.equal(off.issue, 'The machine is reconnecting. Existing shells will be kept.')
  assert.equal(r.launchReadiness(input({ machine: null })).level, 'blocked')
  assert.equal(r.launchReadiness(input({ member: { tool: '', model: '' } })).agent.state, 'missing')
})

test('each source of the agent', () => {
  const saved = { tool: 'claude', model: 'claude-big', effort: 'high' }
  const s = over => r.launchReadiness(input(over))
  assert.equal(s({}).source, 'saved'); assert.match(s({}).sourceFact.text, /saved crew/)
  assert.equal(s({ member: { tool: 'claude', model: 'other', effort: 'high' } }).source, 'chosen')
  assert.equal(s({ edited: false, saved: null }).source, 'outcome'); assert.equal(s({ edited: false, saved: null }).sourceFact.state, 'missing')
  assert.match(s({ edited: false, saved: null }).sourceFact.text, /No crew is saved for this tier, so it uses the outcome’s agent, Claude Code\./)
  assert.equal(s({ edited: false, saved }).source, 'outcome'); assert.equal(s({ edited: false, saved }).sourceFact.state, 'ready')
  const fallback = s({ edited: false, saved: { tool: 'cline' } })
  assert.equal(fallback.source, 'fallback'); assert.equal(fallback.sourceFact.state, 'unavailable')
  assert.equal(fallback.sourceFact.text, 'Cline from your saved crew is not on workshop, so this tier uses the outcome’s agent, Claude Code.')
  assert.equal(fallback.level, 'look')
  assert.equal(s({ edited: false, saved: undefined }).sourceFact.state, 'ready')
})

test('every backup is listed in order with its own eligibility', () => {
  const s = r.launchReadiness(input({
    accounts: read(signed('claude'), signed('codex', { limitsAt: now - 8 * HOUR }), { tool: 'opencode', state: 'none', limitsState: 'not_asked' }),
    spares: [{ tool: 'codex', model: 'gpt', effort: '' }, { tool: 'opencode', model: '' }, { tool: 'cline', model: '' }, { tool: 'claude', model: 'claude-small', effort: 'low' }],
  }))
  assert.deepEqual(s.backups.map(b => [b.agent, b.eligible, b.balances, b.stale]), [['Codex', true, false, true], ['OpenCode', false, false, false], ['Cline', false, false, false], ['Claude Code', true, true, false]])
  assert.match(s.backups[0].reason, /Codex’s plan figures were read 8 h ago; figures over 6 hours old are not used to balance/)
  assert.match(s.backups[1].reason, /No sign-in found on this machine, so it cannot take over/)
  assert.match(s.backups[2].reason, /Cline is not on workshop, so it cannot take over/)
  assert.equal(s.backups[3].reason, ''); assert.equal(s.backups[3].model, 'claude-small')
  assert.equal(s.level, 'look')
})

test('stale or absent figures on the first choice mean no balancing, and absent figures are only a note', () => {
  const stale = r.launchReadiness(input({ accounts: read(signed('claude', { limitsAt: now - 7 * HOUR }), signed('codex')), spares: [{ tool: 'codex', model: '' }] }))
  assert.equal(stale.backups[0].balances, false); assert.match(stale.backups[0].reason, /Claude Code’s plan figures were read 7 h ago/); assert.equal(stale.level, 'look')
  const none = r.launchReadiness(input({ accounts: read(signed('claude', { limits: [], limitsState: 'unsupported' }), signed('codex')), spares: [{ tool: 'codex', model: '' }] }))
  assert.equal(none.backups[0].balances, false); assert.match(none.backups[0].reason, /Claude Code has no plan figures/); assert.equal(none.level, 'ready')
  const blind = r.launchReadiness(input({ accounts: null, spares: [{ tool: 'codex', model: '' }] }))
  assert.equal(blind.backups[0].eligible, true); assert.equal(blind.backups[0].balances, false)
})

test('the summary counts stages and names the first thing to fix', () => {
  const ok = r.launchReadiness(input()), look = r.launchReadiness(input({ member: { tool: 'claude', model: 'nope' } })), block = r.launchReadiness(input({ member: { tool: 'cline', model: '' } }))
  const row = (label, readiness) => ({ label, where: 'pipeline 1', readiness })
  assert.equal(r.launchSummary([row('Code', ok), row('Review', ok)]).text, '2 of 2 stages ready.')
  const mixed = r.launchSummary([row('Code', ok), row('Review', look)])
  assert.equal(mixed.ready, 1); assert.equal(mixed.blocker, ''); assert.match(mixed.text, /^1 of 2 stages ready, 1 to look at\. First: Review in pipeline 1: workshop’s list/)
  const stop = r.launchSummary([row('Code', look), row('Review', block)])
  assert.match(stop.text, /1 cannot start/); assert.equal(stop.blocker, 'Review in pipeline 1: Cline is not on workshop now. Choose another agent for the stage.')
})
