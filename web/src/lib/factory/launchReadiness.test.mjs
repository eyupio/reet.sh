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
    assert.equal(s.model.state, 'unavailable', state); assert.equal(s.model.later, r.MODEL_LATER); assert.equal(s.checkedAtStart, true); assert.equal(s.level, 'look')
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
  // A read that does not cover the agent (Cline has no reader) is a quiet note: never a thing to look at.
  const quiet = r.launchReadiness(input({ accounts: read() }))
  assert.equal(quiet.signIn.quiet, true); assert.equal(quiet.level, 'ready')
  assert.equal(r.launchReadiness(input({ accounts: read({ tool: 'claude', state: 'unreadable', limitsState: 'not_asked' }) })).signIn.state, 'unavailable')
})

test('an agent not on the machine, or a machine not connected, blocks', () => {
  const gone = r.launchReadiness(input({ member: { tool: 'cline', model: '' }, saved: null }))
  assert.equal(gone.level, 'blocked'); assert.equal(gone.agent.state, 'unavailable'); assert.equal(gone.agent.text, 'Cline is not on workshop now.')
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
  assert.deepEqual(s.backups.map(b => [b.agent, b.eligible, b.balances]), [['Codex', true, true], ['OpenCode', false, false], ['Cline', false, false], ['Claude Code', true, false]])
  assert.match(s.backups[0].reason, /Codex’s plan figures were read 8 h ago, so it takes over without a comparison when Claude Code’s plan runs low\./)
  assert.match(s.backups[1].reason, /No sign-in found on this machine\. It is still tried after a usage limit, and is likely to be refused there\./)
  assert.match(s.backups[2].reason, /Cline is not on workshop\. It is still tried after a usage limit/)
  assert.equal(s.backups[3].model, 'claude-small'); assert.equal(s.backups[3].verdict, 'takes over only after a usage limit'); assert.match(s.backups[3].reason, /shares Claude Code’s plan with the first choice/)
  assert.equal(s.backups[0].verdict, 'can take over'); assert.equal(s.backups[1].verdict, 'balancing will not use it')
  assert.equal(s.level, 'look')
})

test('stale or absent figures on the first choice mean no balancing, and absent figures are only a note', () => {
  // A backup with no figures of its own still takes over once the first choice runs low: it is not compared.
  const bare = r.launchReadiness(input({ accounts: read(signed('claude'), signed('codex', { limits: [], limitsState: 'unsupported' })), spares: [{ tool: 'codex', model: '' }] }))
  assert.deepEqual([bare.backups[0].eligible, bare.backups[0].balances], [true, true]); assert.equal(bare.level, 'ready')
  const stale = r.launchReadiness(input({ accounts: read(signed('claude', { limitsAt: now - 7 * HOUR }), signed('codex')), spares: [{ tool: 'codex', model: '' }] }))
  assert.equal(stale.backups[0].balances, false); assert.match(stale.backups[0].reason, /Claude Code’s plan figures were read 7 h ago, so stages stay on the first choice/)
  // Old figures on a backup that can still take over are quiet.
  assert.equal(stale.backups[0].eligible, true); assert.equal(stale.level, 'ready')
  const none = r.launchReadiness(input({ accounts: read(signed('claude', { limits: [], limitsState: 'unsupported' }), signed('codex')), spares: [{ tool: 'codex', model: '' }] }))
  assert.equal(none.backups[0].balances, false); assert.match(none.backups[0].reason, /Claude Code has no plan figures, so stages stay on the first choice/); assert.equal(none.level, 'ready')
  const blind = r.launchReadiness(input({ accounts: null, spares: [{ tool: 'codex', model: '' }] }))
  assert.equal(blind.backups[0].eligible, true); assert.equal(blind.backups[0].balances, false)
})

test('the summary counts stages and names the cause that blocks', () => {
  const ok = r.launchReadiness(input()), look = r.launchReadiness(input({ member: { tool: 'claude', model: 'nope' } })), block = r.launchReadiness(input({ member: { tool: 'cline', model: '' } }))
  const row = (stage, crew, readiness) => ({ stage, crew, readiness })
  const all = r.launchSummary([row('Code', 'Standard', ok), row('Review', 'Heavy', ok)])
  assert.equal(all.text, '2 of 2 stages ready.'); assert.deepEqual(all.causes, []); assert.equal(all.blocker, '')
  const mixed = r.launchSummary([row('Code', 'Standard', ok), row('Review', 'Heavy', look)])
  assert.equal(mixed.ready, 1); assert.equal(mixed.blocker, ''); assert.equal(mixed.text, '1 of 2 stages ready, 1 to look at.')
  assert.deepEqual(mixed.causes.map(c => [c.level, c.crews, c.stages]), [['look', ['Heavy'], ['Review']], ['note', ['Heavy'], ['Review']]])
  assert.match(mixed.causes[0].text, /workshop’s list/); assert.equal(mixed.causes[1].text, r.MODEL_LATER)
  const stop = r.launchSummary([row('Code', 'Standard', look), row('Review', 'Heavy', block)])
  assert.match(stop.text, /1 cannot start/); assert.equal(stop.blocker, 'Cline is not on workshop now. Change the agent of the Heavy crew under “Choose who does what”.')
  assert.equal(stop.causes[0].level, 'blocked')
})

test('a cause is said once, with the tiers and stages it affects', () => {
  const gone = r.launchReadiness(input({ member: { tool: 'cline', model: 'x', effort: 'high' }, spares: [{ tool: 'codex', model: '' }] }))
  const row = (stage, crew, readiness) => ({ stage, crew, readiness })
  const s = r.launchSummary([row('Code', 'Standard', gone), row('Plan', 'Light', r.launchReadiness(input())), row('Review', 'Heavy', gone), row('Test', 'Standard', gone), row('Ship', 'Light', gone)])
  // The model, effort and backups of a missing agent are not mentioned.
  assert.deepEqual(s.causes, [{ level: 'blocked', text: 'Cline is not on workshop now.', machine: false, crews: ['Heavy', 'Standard', 'Light'], stages: ['Code', 'Review', 'Test', 'Ship'] }])
  assert.equal(r.inWords(['Heavy', 'Standard']), 'Heavy and Standard'); assert.equal(r.inWords(['Code', 'Review', 'Test']), 'Code, Review and Test')
  assert.equal(s.blocker, 'Cline is not on workshop now. Change the agent of the Heavy, Standard and Light crews under “Choose who does what”.')
  const two = r.launchSummary([row('Code', 'Standard', gone), row('Review', 'Heavy', gone)])
  assert.equal(two.blocker, 'Cline is not on workshop now. Change the agent of the Heavy and Standard crews under “Choose who does what”.')
})

test('a machine that is not connected is one cause for every tier, and has no crew to change', () => {
  const off = r.launchReadiness(input({ machine: machine({ ready: false }) }))
  const s = r.launchSummary([{ stage: 'Code', crew: 'Standard', readiness: off }, { stage: 'Review', crew: 'Heavy', readiness: off }])
  assert.equal(s.causes.length, 1); assert.equal(s.causes[0].machine, true); assert.equal(s.blocker, 'The machine is reconnecting. Existing shells will be kept.')
})

test('causes are sorted blocked, look, note and a backup that is not eligible is something to look at', () => {
  const one = r.launchReadiness(input({ member: { tool: 'claude', model: 'nope', effort: 'high' }, spares: [{ tool: 'cline', model: '' }] }))
  assert.deepEqual(r.readinessItems(one).map(item => item.level), ['look', 'note', 'look'])
  assert.equal(r.readinessItems(one)[2].text, 'Backup 1, Cline: Cline is not on workshop. It is still tried after a usage limit, and is likely to be refused there.')
  assert.deepEqual(r.launchSummary([{ stage: 'Code', crew: 'Standard', readiness: one }]).causes.map(c => c.level), ['look', 'look', 'note'])
})

test('backups that fail for one reason are one cause, and the first choice’s own missing sign-in is not said again for them', () => {
  const two = r.launchReadiness(input({ spares: [{ tool: 'cline', model: '' }, { tool: 'codex', model: '' }, { tool: 'cline', model: 'other' }] }))
  assert.deepEqual(r.readinessItems(two).map(item => item.text), ['Backups 1 and 3, Cline: Cline is not on workshop. It is still tried after a usage limit, and is likely to be refused there.'])
  const none = r.launchReadiness(input({ accounts: read({ tool: 'claude', state: 'none', limitsState: 'not_asked' }), spares: [{ tool: 'claude', model: 'claude-big' }, { tool: 'claude', model: '' }] }))
  assert.equal(none.backups.filter(item => !item.eligible).length, 2)
  assert.deepEqual(r.readinessItems(none).map(item => item.text), [none.signIn.text])
})

test('a backup is compared with the first choice when both have fresh figures', () => {
  const more = r.launchReadiness(input({ accounts: read(signed('claude', { limits: [{ kind: 'week', usedPercent: 90 }] }), signed('codex', { limits: [{ kind: 'week', usedPercent: 20 }] })), spares: [{ tool: 'codex', model: '' }] }))
  assert.deepEqual([more.backups[0].balances, more.backups[0].eligible], [true, true]); assert.match(more.backups[0].reason, /Codex has 80% of its plan left against Claude Code’s 10%, so balancing can move a stage onto it/)
  const less = r.launchReadiness(input({ accounts: read(signed('claude', { limits: [{ kind: 'week', usedPercent: 20 }] }), signed('codex', { limits: [{ kind: 'week', usedPercent: 60 }] })), spares: [{ tool: 'codex', model: '' }] }))
  assert.equal(less.backups[0].balances, false); assert.equal(less.backups[0].eligible, true); assert.match(less.backups[0].reason, /Codex has 40% of its plan left, no more than Claude Code’s 80%, so balancing will not move a stage onto it; it takes over only after a usage limit/)
})

test('an effort the machine does not offer, asked by the saved crew, is not the person’s choice and is not hidden', () => {
  // The dialog clears the effort it cannot send and passes the one that was asked.
  const s = r.launchReadiness(input({ member: { tool: 'claude', model: 'claude-big', effort: 'xhigh' }, saved: { tool: 'claude', model: 'claude-big', effort: 'xhigh' } }))
  assert.equal(s.source, 'saved'); assert.equal(s.effort.state, 'unavailable'); assert.match(s.effort.text, /Xhigh effort is not offered by Claude Code here, so the stage starts on the agent’s default effort\./)
  assert.equal(s.level, 'look'); assert.match(s.issue, /Xhigh effort is not offered/)
  // An agent with no effort setting has no choice to repeat back.
  const none = r.launchReadiness(input({ member: { tool: 'opencode', model: '', effort: '' }, saved: null, edited: false }))
  assert.equal(none.effort.state, 'default'); assert.equal(none.effort.quiet, true)
})

test('what is being read is neither ready nor a problem', () => {
  const reading = r.launchReadiness(input({ models: () => 'reading', accounts: undefined, spares: [{ tool: 'codex', model: '' }] }))
  assert.equal(reading.model.pending, true); assert.equal(reading.signIn.pending, true); assert.equal(reading.backups[0].pending, true)
  assert.equal(reading.level, 'ready'); assert.equal(reading.pending, true)
  const row = { stage: 'Code', crew: 'Standard', readiness: reading }
  assert.equal(r.launchSummary([row]).text, '0 of 1 stage ready, 1 still being read.')
  assert.equal(r.launchReadiness(input({ models: () => null })).model.state, 'unavailable')
})

test('an answer that is not a fresh read still carries the accounts the controller kept', () => {
  const busy = { state: 'busy', sentence: 'That machine is already reading its agents’ sign-ins. Try again in a moment.', planLimits: true, checkedAt: now - 20 * 60_000, accounts: [signed('claude'), signed('codex', { limits: [{ kind: 'week', usedPercent: 5 }] })] }
  const s = r.launchReadiness(input({ accounts: busy, spares: [{ tool: 'codex', model: '' }] }))
  assert.equal(s.signIn.state, 'stale'); assert.match(s.signIn.text, /Signed in as me@example\.test.*Last read 20 min ago\. That machine is already reading/)
  assert.equal(s.backups[0].balances, true); assert.equal(s.backups[0].eligible, true)
  // Nothing kept: the read itself is the problem.
  assert.equal(r.launchReadiness(input({ accounts: { ...busy, accounts: [] } })).signIn.state, 'unavailable')
})

test('an agent the machine never reports is a quiet note, not the first thing to look at', () => {
  const s = r.launchReadiness(input({ tools: ['claude', 'cline'], member: { tool: 'cline', model: '' }, saved: { tool: 'cline' }, edited: false, spares: [{ tool: 'cline', model: 'x' }] }))
  assert.equal(s.signIn.quiet, true); assert.equal(s.level, 'ready'); assert.equal(s.backups[0].eligible, true)
  assert.equal(r.launchSummary([{ stage: 'Code', crew: 'Light', readiness: s }]).text, '1 of 1 stage ready.')
})

test('the stage line says what it runs on once, with defaults as a choice and nothing for an agent with no effort setting', () => {
  const line = over => r.launchReadiness(input(over)).line
  assert.equal(line({}), 'Claude Code · claude-big · high effort')
  assert.equal(line({ member: { tool: 'claude', model: '', effort: '' } }), 'Claude Code · Agent’s default model and effort')
  assert.equal(line({ member: { tool: 'claude', model: '', effort: 'low' } }), 'Claude Code · Agent’s default model · low effort')
  assert.equal(line({ member: { tool: 'claude', model: 'claude-big', effort: '' } }), 'Claude Code · claude-big · Agent’s default effort')
  assert.equal(line({ member: { tool: 'opencode', model: '', effort: '' } }), 'OpenCode · Agent’s default model')
  // An effort the machine does not offer is not part of what it runs on; its own line says so.
  assert.equal(line({ member: { tool: 'claude', model: 'claude-big', effort: 'xhigh' } }), 'Claude Code · claude-big')
})
