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
