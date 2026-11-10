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
  const r = (step) => ({ key: step, title: step, step, note: '' })
  assert.equal(o.outcomeSummary([r('following'), r('following')]), 'Reet. Gaffer owns this outcome: 2 started and followed.')
  assert.equal(o.outcomeSummary([r('following'), r('unfollowed'), r('failed')]), '1 started and followed, 1 started but not followed yet, 1 not started.')
  assert.equal(o.outcomeSummary([r('failed')]), '1 not started.')
  assert.equal(o.outcomeSummary([]), 'Nothing was started.')
})
