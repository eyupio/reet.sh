import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = readFileSync(new URL('./gafferOutcome.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const o = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
const bytes = text => new TextEncoder().encode(text).length
const row = (text, state = 'active', attention = false) => ({ goal: { text, state }, attention })
const plan = (...titles) => titles.map((title, n) => ({ key: `k${n}`, title, prompt: `Do ${title}. Do not merge.` }))

test('a session goal names its outcome, fits the goal limit and keeps merging with the person', () => {
  const goal = o.outcomeGoal('Ship CSV export\n  for billing', 'Export API')
  assert.equal(goal.split('\n')[0], 'Outcome: Ship CSV export for billing')
  assert.match(goal, /This session’s part: Export API\n/)
  assert.match(goal, /Do not merge\.$/)
  const huge = o.outcomeGoal('é'.repeat(4000), 'T'.repeat(500))
  assert.ok(bytes(huge) <= 2000, `goal is ${bytes(huge)} bytes`)
  assert.match(huge, /Do not merge\.$/)
  assert.equal(o.outcomeOf(huge), o.outcomeLine('é'.repeat(4000)))
})

test('only a goal that starts with the outcome line belongs to an outcome', () => {
  assert.equal(o.outcomeOf('Outcome: Ship it\nThis session’s part: A'), 'Ship it')
  assert.equal(o.outcomeOf('Raise the PR. Outcome: no'), '')
  assert.equal(o.outcomeOf('  Outcome: indented'), '')
  assert.equal(o.outcomeOf(''), '')
  assert.equal(o.outcomePart(o.outcomeGoal('Ship it', 'Export API')), 'Export API')
  assert.equal(o.outcomePart('Outcome: Ship it'), '')
  assert.equal(o.outcomePart('Ordinary goal\nThis session’s part: no'), '')
})

test('followed sessions group by outcome with honest counts', () => {
  const a = o.outcomeGoal('Ship export', 'API'), b = o.outcomeGoal('Ship export', 'UI'), c = o.outcomeGoal('Fix login', 'Tokens')
  const groups = o.outcomeGroups([row(a), row('Ordinary goal. Do not merge.'), row(b, 'needs_user', true), row(c, 'completed'), row(o.outcomeGoal('Ship export', 'Docs'), 'paused')])
  assert.deepEqual(groups.map(g => [g.objective, g.rows.length, g.needsReview, g.following, g.finished]), [['Ship export', 3, 1, 1, 0], ['Fix login', 1, 0, 0, 1]])
  assert.equal(o.outcomeProgress(groups[0]), '3 sessions · 1 needs review · 1 following · 1 paused')
  assert.equal(o.outcomeProgress(groups[1]), '1 session · 1 finished')
})

test('the objective says what is missing before a plan is asked for', () => {
  const ready = { objective: 'Ship it', project: true, machineReady: true, agent: true, room: 4 }
  assert.equal(o.objectiveBlocker(ready), '')
  assert.match(o.objectiveBlocker({ ...ready, objective: ' ' }), /Write the outcome/)
  assert.match(o.objectiveBlocker({ ...ready, objective: 'é'.repeat(2001) }), /within 4,000 bytes/)
  assert.match(o.objectiveBlocker({ ...ready, project: false }), /Choose the project/)
  assert.match(o.objectiveBlocker({ ...ready, machineReady: false }), /not connected/)
  assert.match(o.objectiveBlocker({ ...ready, agent: false }), /no agent/)
  assert.match(o.objectiveBlocker({ ...ready, room: 0 }), /already following ten/)
})

test('a plan is approved only when every session can start and be followed', () => {
  assert.equal(o.planBlocker(plan('A', 'B'), 5), '')
  assert.match(o.planBlocker([], 5), /at least one session/)
  assert.match(o.planBlocker(plan('A', 'B', 'C', 'D', 'E', 'F'), 9), /at most 5 sessions/)
  assert.match(o.planBlocker(plan('A', 'B', 'C'), 2), /room to follow 2 more sessions\. Remove 1\./)
  assert.match(o.planBlocker(plan('A'), 0), /already following ten/)
  assert.match(o.planBlocker(plan('A', ' '), 5), /Give session 2 a title/)
  assert.match(o.planBlocker(plan('x'.repeat(81)), 5), /Shorten the title of session 1/)
  assert.match(o.planBlocker(plan('A', ' A '), 5), /both called “A”/)
  assert.match(o.planBlocker([{ key: 'k', title: 'A', prompt: '  ' }], 5), /Write the prompt for “A”/)
  assert.match(o.planBlocker([{ key: 'k', title: 'A', prompt: 'é'.repeat(4001) }], 5), /over 8,000 bytes/)
})

test('the result never claims a session that was not started or not followed', () => {
  const r = (state) => ({ key: state, title: state, state, note: '' })
  assert.equal(o.outcomeSummary([r('following'), r('following')]), 'Reet. Gaffer owns this outcome: 2 started and followed.')
  assert.equal(o.outcomeSummary([r('following'), r('unfollowed'), r('failed'), r('cancelled')]), '1 started and followed, 1 started but not followed yet, 2 not started.')
  assert.equal(o.outcomeSummary([r('failed')]), '1 not started.')
  assert.equal(o.outcomeSummary([]), 'Nothing was started.')
  assert.equal(o.startingProgress([r('following'), r('starting'), r('waiting')]), 'Starting session 2 of 3. The controller carries on if you close this.')
  assert.equal(o.startingProgress([r('following'), r('failed')]), 'Starting session 2 of 2. The controller carries on if you close this.')
})

test('a prompt the launch would refuse is caught before anything starts', () => {
  assert.equal(o.cleanPrompt('  a\r\nb\tc\rd  '), 'a\nb    c\nd')
  assert.equal(o.planBlocker([{ key: 'k', title: 'A', prompt: 'Line one\r\n\tindented' }], 5), '')
  assert.match(o.planBlocker([{ key: 'k', title: 'A', prompt: '- Add the endpoint' }], 5), /with a word, not a dash/)
  assert.match(o.planBlocker([{ key: 'k', title: 'A', prompt: 'Do it\u0000now' }], 5), /cannot be sent/)
  assert.match(o.planBlocker([{ key: 'k', title: 'A\u200Db', prompt: 'Do it' }], 5), /title of session 1/)
})

test('a limit must be a whole number in range', () => {
  assert.equal(o.wholeBetween(12, 1, 48), true)
  for (const bad of [1.5, 0, 49, NaN, '12', null, undefined]) assert.equal(o.wholeBetween(bad, 1, 48), false, String(bad))
})

test('the floor groups by the controller record first, then by goals that only name an outcome', () => {
  const g = (sessionId, text, state = 'active', attention = false) => ({ goal: { sessionId, text, state }, attention })
  const record = { id: 'o1', objective: 'Ship export', state: 'starting', sessions: [
    { key: 'a', title: 'API', state: 'following', note: '', sessionId: 's1' },
    { key: 'b', title: 'UI', state: 'unfollowed', note: '', sessionId: 's2' },
    { key: 'c', title: 'Docs', state: 'waiting', note: '' },
  ] }
  const rows = [g('s1', o.outcomeGoal('Ship export', 'API')), g('s9', o.outcomeGoal('Old outcome', 'Part'), 'needs_user', true), g('s8', 'Ordinary goal')]
  const groups = o.floorOutcomes([record], rows)
  assert.deepEqual(groups.map(x => [x.id, x.objective, x.rows.length]), [['o1', 'Ship export', 1], ['text:Old outcome', 'Old outcome', 1]])
  assert.equal(o.floorOutcomeLine(groups[0]), '3 sessions planned · starting · 1 following · 1 not followed')
  assert.equal(o.floorOutcomeLine(groups[1]), '1 session · 1 needs review')
  const settled = o.floorOutcomes([{ ...record, state: 'stopped', sessions: [record.sessions[0], { ...record.sessions[2], state: 'cancelled' }] }], rows)
  assert.equal(o.floorOutcomeLine(settled[0]), '2 sessions planned · 1 following · 1 not started')
})

test('stages keep pipeline order and take the agent and model of their crew role', () => {
  const crew = { think: { tool: 'claude', model: ' opus ' }, build: { tool: 'codex', model: '' }, ship: { tool: 'claude', model: 'haiku' } }
  assert.deepEqual(o.orderStages(['merge', 'code', 'bogus', 'plan']), ['plan', 'code', 'merge'])
  assert.deepEqual(o.stagePlan(['review', 'code', 'pr'], crew), [
    { kind: 'code', tool: 'codex', model: '' }, { kind: 'review', tool: 'claude', model: 'opus' }, { kind: 'pr', tool: 'claude', model: 'haiku' },
  ])
  assert.equal(o.sessionCount(o.stagePlan(o.DEFAULT_STAGES, crew)), 4)
  const one = { tool: 'claude', model: '' }
  assert.equal(o.sessionCount(o.stagePlan(o.DEFAULT_STAGES, { think: one, build: one, ship: one })), 1)
  assert.equal(o.sessionCount([]), 0)
  assert.equal(o.stageLabel('babysit'), 'Babysit')
})

test('a pipeline track shows what is done, in hand, waiting on the person or failed', () => {
  const stages = [{ kind: 'code', tool: 'codex', state: 'done', summary: 'Committed.' }, { kind: 'review', tool: 'claude', model: 'opus', state: 'running' }, { kind: 'merge', tool: 'claude', state: 'pending' }]
  assert.deepEqual(o.track({ state: 'following', stage: 1, stages }).map(n => n.state), ['done', 'running', 'pending'])
  assert.deepEqual(o.track({ state: 'ready_to_merge', stage: 2, stages: [stages[0], { ...stages[1], state: 'done' }, stages[2]] }).map(n => n.state), ['done', 'done', 'waiting'])
  assert.deepEqual(o.track({ state: 'unfollowed', stage: 1, stages }).map(n => n.state), ['done', 'failed', 'pending'])
  assert.deepEqual(o.track({ state: 'done', stage: 2, stages: stages.map(s => ({ ...s, state: 'done' })) }).map(n => n.state), ['done', 'done', 'done'])
  assert.equal(o.track({ state: 'following', stage: 1, stages })[1].model, 'opus')
  assert.deepEqual(o.track({ state: 'following' }), [])
})

test('a pipeline badge and the page tally say who has to act', () => {
  assert.deepEqual(o.pipelineStatus({ state: 'ready_to_merge' }, false), { label: 'Ready to merge', tone: 'you' })
  assert.deepEqual(o.pipelineStatus({ state: 'following' }, true), { label: 'Needs you', tone: 'you' })
  assert.deepEqual(o.pipelineStatus({ state: 'following' }, false), { label: 'Running', tone: 'busy' })
  assert.deepEqual(o.pipelineStatus({ state: 'done' }, true), { label: 'Done', tone: 'good' })
  assert.equal(o.pipelineStatus({ state: 'failed' }, false).tone, 'bad')
  const records = [{ sessions: [{ state: 'following', conversationId: 'a' }, { state: 'following', conversationId: 'b' }, { state: 'ready_to_merge' }, { state: 'done' }, { state: 'failed' }, { state: 'waiting' }] }]
  assert.deepEqual(o.factoryTally(records, new Set(['b'])), { running: 2, needYou: 1, ready: 1, done: 1 })
})

test('usage counts what Gaffer used, stage by stage, and says when a count is missing', () => {
  const done = (reviews, i, o, reported = true) => ({ state: 'done', reviews, inputTokens: i, outputTokens: o, usageReported: reported })
  const goal = { usedReviews: 2, inputTokens: 500, outputTokens: 100, usageReported: true }
  assert.deepEqual(o.pipelineUsage({ stages: [done(3, 1000, 200), { state: 'running' }] }, goal), { tokens: 1800, reviews: 5, reported: true })
  assert.deepEqual(o.pipelineUsage({ stages: [done(1, 0, 0, false)] }, goal), { tokens: 600, reviews: 3, reported: false })
  assert.deepEqual(o.pipelineUsage({ stages: [] }), { tokens: 0, reviews: 0, reported: false })
  assert.deepEqual(o.pipelineUsage({}, { ...goal, usedReviews: 0 }), { tokens: 0, reviews: 0, reported: false })
  const a = o.pipelineUsage({ stages: [done(3, 1000, 200)] }), b = o.pipelineUsage({})
  assert.deepEqual(o.outcomeUsage([a, b]), { tokens: 1200, reviews: 3, reported: true })
  assert.deepEqual(o.outcomeUsage([a], { inputTokens: 300, outputTokens: 80, usageReported: true }), { tokens: 1580, reviews: 3, reported: true })
  assert.deepEqual(o.outcomeUsage([b]), { tokens: 0, reviews: 0, reported: false })
  assert.equal(o.tokensText(a), '1,200')
  assert.equal(o.tokensText(b), 'None yet')
  assert.equal(o.tokensText({ tokens: 600, reviews: 3, reported: false }), '600+ (not all reported)')
  assert.equal(o.tokensText({ tokens: 0, reviews: 2, reported: false }), 'Not reported')
})

test('time left reads in words', () => {
  const now = 1_000_000_000
  assert.equal(o.timeLeft(now - 1, now), 'Ended')
  assert.equal(o.timeLeft(now + 45 * 60000, now), '45 min')
  assert.equal(o.timeLeft(now + (5 * 60 + 12) * 60000, now), '5 h 12 min')
  assert.equal(o.timeLeft(now + 72 * 3600000, now), '3 days')
})

test('sessions of a pipeline are marked with the stage they are doing', () => {
  const records = [{ id: 'o1', objective: 'Ship export', sessions: [
    { title: 'API', sessionId: 's2', stage: 1, stages: [{ kind: 'plan', sessionId: 's1' }, { kind: 'code', sessionId: 's2' }, { kind: 'review' }] },
    { title: 'Docs', sessionId: 's9' },
    { title: 'One agent', sessionId: 's5', stage: 1, stages: [{ kind: 'code', sessionId: 's5' }, { kind: 'test', sessionId: 's5' }] },
  ] }]
  const members = o.factoryMembers(records)
  assert.equal(o.factoryBadge(members.get('s1')), 'Factory · Plan 1 of 3')
  assert.equal(members.get('s1').current, false)
  assert.equal(o.factoryBadge(members.get('s2')), 'Factory · Code 2 of 3')
  assert.equal(members.get('s2').current, true)
  assert.equal(o.factoryBadge(members.get('s9')), 'Factory')
  assert.equal(o.factoryBadge(members.get('s5')), 'Factory · Test 2 of 2')
  assert.equal(members.get('unknown'), undefined)
  assert.equal(members.get('s2').pipeline, 'API')
})

test('a crew role\'s effort is sent with its stages and starts a new session when it changes', () => {
  const one = { tool: 'claude', model: 'opus' }
  const crew = { think: { ...one, effort: 'high' }, build: { ...one, effort: '' }, ship: { ...one, effort: 'low' } }
  assert.deepEqual(o.stagePlan(['design', 'code', 'merge'], crew), [
    { kind: 'design', tool: 'claude', model: 'opus', effort: 'high' },
    { kind: 'code', tool: 'claude', model: 'opus' },
    { kind: 'merge', tool: 'claude', model: 'opus', effort: 'low' },
  ])
  assert.equal(o.sessionCount(o.stagePlan(['design', 'code', 'merge'], crew)), 3)
  assert.equal(o.sessionCount(o.stagePlan(['plan', 'design', 'review'], crew)), 1)
})
