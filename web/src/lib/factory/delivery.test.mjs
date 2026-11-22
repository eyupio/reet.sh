import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = readFileSync(new URL('./delivery.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const d = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)

const COMMIT = '9f2c1ab0d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8'
const criterion = (id, status) => ({ id, text: id, status })
const review = (statuses, extra = {}) => ({ title: 'Export API', commit: COMMIT, commitState: 'verified', next: 'ready_to_merge', criteria: statuses.map((s, n) => criterion(`c${n + 1}`, s)), ...extra })

test('the statuses are sent exactly as shown, by criterion', () => {
  assert.deepEqual(d.statusMap(review(['passed', 'needs_work', 'unverified'])), { c1: 'passed', c2: 'needs_work', c3: 'unverified' })
  assert.deepEqual(d.statusCount(review(['passed', 'needs_work', 'unverified', 'unverified'])), { passed: 1, needsWork: 1, unverified: 2, notPassed: 3, total: 4 })
  assert.equal(d.summaryLine(d.statusCount(review(['passed', 'needs_work', 'unverified']))), '1 of 3 criteria passed · 1 needs work · 1 unverified')
  assert.equal(d.summaryLine(d.statusCount(review(['passed']))), '1 of 1 criterion passed')
  assert.equal(d.summaryLine(d.statusCount(review([]))), 'No criteria are agreed for this pipeline.')
})

test('each status has a word and a shape, so none rests on colour', () => {
  assert.deepEqual(['passed', 'needs_work', 'unverified'].map(s => d.STATUS_WORD[s]), ['Passed', 'Needs work', 'Unverified'])
  assert.equal(new Set(Object.values(d.STATUS_MARK)).size, 3)
  assert.deepEqual(['note', 'artefact', 'check'].map(k => d.KIND_WORD[k]), ['Reviewer note', 'Artefact', 'GitHub check'])
})

test('a request ID is 32 lowercase hex characters and differs each time', () => {
  const a = d.newRequestId(), b = d.newRequestId()
  assert.match(a, /^[0-9a-f]{32}$/)
  assert.notEqual(a, b)
})

test('an attempt keeps its request ID for a retry and takes a new one when what is sent changes', () => {
  const ids = d.attemptIds()
  const first = ids.idFor('accept@9f2c')
  assert.equal(ids.idFor('accept@9f2c'), first)
  const other = ids.idFor('accept@aaaa')
  assert.notEqual(other, first)
  ids.clear()
  assert.notEqual(ids.idFor('accept@aaaa'), other)
})

test('only a failure that may have reached the server keeps the request ID', () => {
  assert.equal(d.mayHaveArrived(new TypeError('Failed to fetch')), true)
  assert.equal(d.mayHaveArrived({ status: 502 }), true)
  assert.equal(d.mayHaveArrived({ status: 408 }), true)
  assert.equal(d.mayHaveArrived({ status: 409 }), false)
  assert.equal(d.mayHaveArrived({ status: 400 }), false)
})

test('accepting names how many criteria are not Passed, and says it is not the merge approval', () => {
  const clean = d.acceptConfirm(review(['passed', 'passed']))
  assert.equal(clean.title, 'Accept this delivery?')
  assert.equal(clean.confirmLabel, 'Accept delivery')
  assert.match(clean.body, /commit 9f2c1ab of “Export API”\./)
  assert.doesNotMatch(clean.body, /not Passed/)
  assert.match(clean.body, /Accepting is not approving the merge/)
  const gap = d.acceptConfirm(review(['passed', 'needs_work', 'unverified']))
  assert.equal(gap.title, 'Accept with 2 not Passed?')
  assert.equal(gap.confirmLabel, 'Accept anyway')
  assert.match(gap.body, /2 of 3 criteria are not Passed \(1 needs work, 1 unverified\)\./)
  assert.match(d.acceptConfirm(review(['needs_work'])).body, /1 of 1 criterion is not Passed \(1 needs work\)\./)
  assert.match(d.acceptConfirm(review(['passed'], { commit: '', commitState: 'none' })).body, /which has no Git commit/)
})

test('what accepting leads to is said for each of the three outcomes', () => {
  assert.match(d.nextWords('ready_to_merge'), /waits at Ready to merge\. Accepting is not approving the merge: Approve the merge stays your separate decision\./)
  assert.match(d.nextWords('merge'), /merges by itself once its checks pass/)
  assert.match(d.nextWords('done'), /no merge stage/)
  assert.match(d.nextWords(undefined), /not approving the merge/)
})

test('a send-back needs notes unless a criterion needs work, and a stage', () => {
  assert.match(d.sendBackBlocker(review(['passed', 'unverified']), '   ', 'code'), /Say what is wrong/)
  assert.equal(d.sendBackBlocker(review(['passed', 'needs_work']), '', 'code'), '')
  assert.equal(d.sendBackBlocker(review(['passed']), 'The export leaks rows.', 'code'), '')
  assert.match(d.sendBackBlocker(review(['needs_work']), 'x', ''), /Choose the stage/)
})

test('a decision is refused while the commit cannot be verified, with the reason the machine gave', () => {
  assert.equal(d.commitBlocker({ commitState: 'verified' }), '')
  assert.equal(d.commitBlocker({ commitState: 'none' }), '')
  assert.match(d.commitBlocker({ commitState: 'unverified', commitNote: 'The machine is too old to report it.' }), /could not be verified: The machine is too old to report it\. A decision is not recorded/)
  assert.match(d.commitBlocker({ commitState: 'unreachable' }), /could not be verified\./)
  assert.deepEqual(['verified', 'none', 'unverified', 'unreachable'].map(d.commitStateWord), ['Verified on the machine', 'Not a Git repository', 'Could not be verified', 'Could not be verified'])
})

test('earlier evidence says whether the commit or the criteria moved on', () => {
  const now = { commit: COMMIT }
  assert.equal(d.earlierLabel({ historical: false, commit: COMMIT }, now), '')
  assert.equal(d.earlierLabel({ historical: true, commit: '1111111aaaa' }, now), 'Earlier commit')
  assert.equal(d.earlierLabel({ historical: true, commit: COMMIT }, now), 'Earlier criteria')
  assert.equal(d.earlierLabel({ historical: true, commit: '' }, now), 'Earlier evidence')
  assert.equal(d.earlierLabel({ historical: true, commit: COMMIT }, { commit: '' }), 'Earlier evidence')
})

test('each documented conflict code is said plainly; an unknown one is left to the server’s own sentence', () => {
  assert.equal(d.conflictWords('delivery_head_changed'), 'A new commit arrived since you looked.')
  assert.equal(d.conflictWords('delivery_closed'), 'Someone else already decided this delivery.')
  for (const code of ['delivery_changed', 'delivery_criteria_changed', 'delivery_head_unverified', 'delivery_request_reused']) assert.notEqual(d.conflictWords(code), '', code)
  assert.equal(d.conflictWords('something_else'), '')
  assert.match(d.conflictAdvice, /What you typed is kept/)
})

test('a recorded decision reads as one line', () => {
  assert.equal(d.decisionLine({ decision: 'accept', commit: COMMIT, byName: 'Paul' }), 'Accepted on 9f2c1ab by Paul')
  assert.equal(d.decisionLine({ decision: 'send_back', commit: COMMIT, byName: 'Paul', stage: 'Code' }), 'Sent back to Code on 9f2c1ab by Paul')
  assert.equal(d.decisionLine({ decision: 'accept', commit: '' }), 'Accepted with no Git commit')
  assert.equal(d.shortCommit(COMMIT), '9f2c1ab')
  assert.equal(d.shortCommit(undefined), '')
})
