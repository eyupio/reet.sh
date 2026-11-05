import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

// overviewStages.ts needs checkTone/safeInsightURL and splitChecks at run time, and pullVerdict.ts needs checkTone, so the
// files are transpiled innermost first and each import specifier is pointed at a data URL.
const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const insights = asURL(compile('../sessionInsights.ts'))
const verdict = asURL(compile('./pullVerdict.ts').replace("from '../sessionInsights'", `from '${insights}'`))
const source = compile('./overviewStages.ts').replace("from '../sessionInsights'", `from '${insights}'`).replace("from './pullVerdict'", `from '${verdict}'`)
const { deriveStages } = await import(asURL(source))

const pull = (over = {}) => ({ number: 42, title: 't', url: 'https://github.com/o/r/pull/42', state: 'OPEN', draft: false, review: 'APPROVED', mergeable: 'MERGEABLE', added: 1, removed: 1,
  checks: [{ name: 'build', state: 'SUCCESS' }], base: 'main', detailKnown: true, threads: { open: 0, resolved: 2 }, ...over })
const NOTHING = 'Nothing needs you right now.'
const input = (over = {}) => ({ live: true, planning: false, needsInput: '', active: true, pull: pull(), ...over })
const states = r => r.steps.map(s => s.state)
const bad = (name, url) => ({ name, state: 'FAILURE', url })
const run = name => ({ name, state: 'IN_PROGRESS' })
const link = (label, href) => ({ kind: 'link', label, href })

test('the five steps are fixed, in order, with a non-empty detail each', () => {
  const r = deriveStages(input())
  assert.deepEqual(r.steps.map(s => s.key), ['code', 'pr', 'checks', 'review', 'merge'])
  assert.deepEqual(r.steps.map(s => s.label), ['Code', 'PR', 'Checks', 'Review', 'Merge'])
  for (const s of r.steps) assert.ok(s.detail.length > 0, s.key)
})

// One case per cell of the spec's stage table.
const cells = [
  ['Code done: a PR exists', input(), 0, 'done'],
  ['Code current: tool activity and the session live', input({ pull: undefined, pullReason: 'no_pr' }), 0, 'current'],
  ['Code attention: finished with changes and no PR', input({ pull: undefined, pullReason: 'no_pr', live: false }), 0, 'attention'],
  ['Code upcoming: nothing yet', input({ pull: undefined, pullReason: 'no_pr', active: false }), 0, 'upcoming'],
  ['PR done: open', input(), 1, 'done'],
  ['PR current: draft', input({ pull: pull({ draft: true }) }), 1, 'current'],
  ['PR upcoming: no_pr', input({ pull: undefined, pullReason: 'no_pr' }), 1, 'upcoming'],
  ['PR unknown: github_unavailable', input({ pull: undefined, pullReason: 'github_unavailable' }), 1, 'unknown'],
  ['Checks done: all passed or skipped', input({ pull: pull({ checks: [{ name: 'a', state: 'SUCCESS' }, { name: 'b', state: 'SKIPPED' }] }) }), 2, 'done'],
  ['Checks current: any running', input({ pull: pull({ checks: [run('a')] }) }), 2, 'current'],
  ['Checks attention: any failing', input({ pull: pull({ checks: [bad('a'), run('b')] }) }), 2, 'attention'],
  ['Checks upcoming: no PR', input({ pull: undefined, pullReason: 'no_pr' }), 2, 'upcoming'],
  ['Checks unknown: none reported', input({ pull: pull({ checks: [] }) }), 2, 'unknown'],
  ['Review done: approved and no open threads', input(), 3, 'done'],
  ['Review current: review required', input({ pull: pull({ review: 'REVIEW_REQUIRED' }) }), 3, 'current'],
  ['Review attention: changes requested', input({ pull: pull({ review: 'CHANGES_REQUESTED' }) }), 3, 'attention'],
  ['Review attention: an open thread', input({ pull: pull({ threads: { open: 1, resolved: 0 } }) }), 3, 'attention'],
  ['Review upcoming: draft', input({ pull: pull({ draft: true, review: '' }) }), 3, 'upcoming'],
  ['Review upcoming: no PR', input({ pull: undefined, pullReason: 'no_pr' }), 3, 'upcoming'],
  ['Review unknown: threads not read', input({ pull: pull({ review: '', threads: undefined }) }), 3, 'unknown'],
  ['Merge done: merged', input({ pull: pull({ state: 'MERGED' }) }), 4, 'done'],
  ['Merge current: auto-merge on', input({ pull: pull({ autoMerge: true, review: 'REVIEW_REQUIRED' }) }), 4, 'current'],
  ['Merge current: ready', input(), 4, 'current'],
  ['Merge attention: conflicts', input({ pull: pull({ mergeable: 'CONFLICTING' }) }), 4, 'attention'],
  ['Merge upcoming: not yet', input({ pull: pull({ review: 'REVIEW_REQUIRED' }) }), 4, 'upcoming'],
  ['Merge unknown: closed without merge', input({ pull: pull({ state: 'CLOSED' }) }), 4, 'unknown'],
]
for (const [name, inp, i, want] of cells) test(`stage table: ${name}`, () => assert.equal(deriveStages(inp).steps[i].state, want))

// One case per Next rule, in priority order.
test('next 1: input needed wins over everything and shows the text verbatim', () => {
  const r = deriveStages(input({ needsInput: 'Allow rm -rf?', pull: pull({ mergeable: 'CONFLICTING', checks: [bad('a')] }) }))
  assert.equal(r.next.text, 'Allow rm -rf?')
  assert.equal(r.next.tone, 'warn')
  assert.deepEqual(r.next.primary, { kind: 'session', label: 'Review' })
  assert.equal(r.next.fix, undefined)
})
test('next 1b: input needed also wins over a merged pull request', () => {
  assert.equal(deriveStages(input({ needsInput: 'Pick one', pull: pull({ state: 'MERGED' }) })).next.text, 'Pick one')
})
test('next 2: conflicts name the base branch and offer the conflicts fix', () => {
  const r = deriveStages(input({ pull: pull({ mergeable: 'CONFLICTING', checks: [bad('a')], review: 'CHANGES_REQUESTED' }) }))
  assert.equal(r.next.text, 'Conflicts with main.')
  assert.equal(r.next.tone, 'bad')
  assert.deepEqual(r.next.primary, link('Open pull request', 'https://github.com/o/r/pull/42'))
  assert.equal(r.next.fix, 'conflicts')
})
test('next 2b: conflicts without a known base say the base branch', () => {
  assert.equal(deriveStages(input({ pull: pull({ mergeable: 'CONFLICTING', base: undefined }) })).next.text, 'Conflicts with the base branch.')
})
test('next 3: one failing check, in the singular, links to that check', () => {
  const r = deriveStages(input({ pull: pull({ checks: [bad('lint', 'https://ci/x')], review: 'CHANGES_REQUESTED' }) }))
  assert.equal(r.next.text, '1 check failing.')
  assert.equal(r.next.tone, 'bad')
  assert.deepEqual(r.next.primary, link('Open failing check', 'https://ci/x'))
  assert.equal(r.next.fix, 'checks')
})
test('next 3b: several failing checks are counted and omit the link when none is safe', () => {
  const r = deriveStages(input({ pull: pull({ checks: [bad('a', 'http://insecure'), bad('b')] }) }))
  assert.equal(r.next.text, '2 checks failing.')
  assert.equal(r.next.primary, undefined)
  assert.equal(r.next.fix, 'checks')
})
test('next 4: changes requested offers the threads fix only with open threads', () => {
  const without = deriveStages(input({ pull: pull({ review: 'CHANGES_REQUESTED' }) }))
  assert.equal(without.next.text, 'A reviewer asked for changes.')
  assert.equal(without.next.tone, 'bad')
  assert.deepEqual(without.next.primary, link('Open review', 'https://github.com/o/r/pull/42'))
  assert.equal(without.next.fix, undefined)
  const withThreads = deriveStages(input({ pull: pull({ review: 'CHANGES_REQUESTED', threads: { open: 2, resolved: 0 } }) }))
  assert.equal(withThreads.next.fix, 'threads')
})
test('next 5: open threads are counted and offer the threads fix', () => {
  const one = deriveStages(input({ pull: pull({ threads: { open: 1, resolved: 0 } }) }))
  assert.equal(one.next.text, '1 unresolved review thread.')
  assert.equal(one.next.tone, 'warn')
  assert.deepEqual(one.next.primary, link('Open review', 'https://github.com/o/r/pull/42'))
  assert.equal(one.next.fix, 'threads')
  assert.equal(deriveStages(input({ pull: pull({ threads: { open: 3, resolved: 0 } }) })).next.text, '3 unresolved review threads.')
})
test('next 6: running checks are named, with a plus count past two', () => {
  const t = checks => deriveStages(input({ pull: pull({ checks }) })).next
  assert.deepEqual(t([run('a')]), { text: 'Waiting on a.', tone: 'warn' })
  assert.equal(t([run('a'), run('b')]).text, 'Waiting on a and b.')
  assert.equal(t([run('a'), run('b'), run('c'), run('d')]).text, 'Waiting on a and b, plus 2 more.')
})
test('next 7: a draft asks for it to be marked ready', () => {
  const r = deriveStages(input({ pull: pull({ draft: true }) }))
  assert.deepEqual(r.next, { text: 'Draft pull request. Mark it ready when the work is complete.', tone: 'muted' })
})
test('next 8: review required says so', () => {
  const r = deriveStages(input({ pull: pull({ review: 'REVIEW_REQUIRED' }) }))
  assert.deepEqual(r.next, { text: 'A review is required before this can merge.', tone: 'warn' })
})
test('next 9: failing checks come before a draft, and conflicts before failing checks', () => {
  assert.equal(deriveStages(input({ pull: pull({ draft: true, checks: [bad('a')] }) })).next.fix, 'checks')
  assert.equal(deriveStages(input({ pull: pull({ mergeable: 'CONFLICTING', checks: [bad('a')] }) })).next.fix, 'conflicts')
})
test('next 10: closed without merging is muted and ignores conflicts', () => {
  const r = deriveStages(input({ pull: pull({ state: 'CLOSED', mergeable: 'CONFLICTING', checks: [bad('a')] }) }))
  assert.deepEqual(r.next, { text: 'Closed without merging.', tone: 'muted' })
})
test('next 11: ready names GitHub as the decider and links the pull request', () => {
  const r = deriveStages(input())
  assert.equal(r.next.text, 'Ready. GitHub decides whether it can merge.')
  assert.equal(r.next.tone, 'good')
  assert.deepEqual(r.next.primary, link('Open pull request', 'https://github.com/o/r/pull/42'))
  assert.equal(r.next.fix, undefined)
})
test('next 12: no pull request yet depends on whether the session is live', () => {
  const base = { pull: undefined, pullReason: 'no_pr' }
  assert.deepEqual(deriveStages(input({ ...base, live: false })).next, { text: 'No pull request yet.', tone: 'muted' })
  assert.deepEqual(deriveStages(input({ ...base, live: true })).next, { text: 'Nothing needs you right now.', tone: 'muted' })
  assert.deepEqual(deriveStages(input({ ...base, live: false, active: false })).next, { text: 'Nothing needs you right now.', tone: 'muted' })
})
test('next 13: an open pull request with a review requested and nothing else wrong leaves nothing to do', () => {
  const r = deriveStages(input({ pull: pull({ review: '', reviews: { approved: 0, changesRequested: 0, commented: 0, requested: 1 } }) }))
  assert.deepEqual(r.next, { text: NOTHING, tone: 'muted' })
  assert.equal(r.steps[3].state, 'current')
  assert.equal(r.steps[3].detail, 'Review requested')
  assert.notEqual(r.steps[4].state, 'current')
})
test('a repository that requires no review reaches Ready once reviews were read and none are requested', () => {
  const r = deriveStages(input({ pull: pull({ review: '', reviews: { approved: 0, changesRequested: 0, commented: 0, requested: 0 } }) }))
  assert.equal(r.steps[3].state, 'done')
  assert.equal(r.steps[3].detail, 'No review required, no open threads')
  assert.equal(r.next.text, 'Ready. GitHub decides whether it can merge.')
})
test('with reviews unread and no decision, Review stays current and Ready does not fire', () => {
  const r = deriveStages(input({ pull: pull({ review: '', reviews: undefined }) }))
  assert.equal(r.steps[3].state, 'current')
  assert.equal(r.next.text, 'Nothing needs you right now.')
})
test('zero open threads in a truncated list do not make Review done', () => {
  for (const review of ['APPROVED', '']) {
    const r = deriveStages(input({ pull: pull({ review, reviews: { approved: 0, changesRequested: 0, commented: 0, requested: 0 }, threads: { open: 0, resolved: 100, more: true } }) }))
    assert.equal(r.steps[3].state, 'current', review)
    assert.equal(r.steps[3].detail, '100+ threads, not all read')
    assert.notEqual(r.next.text, 'Ready. GitHub decides whether it can merge.')
    assert.notEqual(r.steps[4].state, 'current')
  }
})
test('a missing or unexpected pullReason is unread, never a claim that there is no pull request', () => {
  for (const pullReason of [undefined, 'busy', 'denied', 'timed_out']) {
    const r = deriveStages(input({ pull: undefined, pullReason, live: false }))
    assert.deepEqual(states(r).slice(1), ['unknown', 'unknown', 'unknown', 'unknown'], String(pullReason))
    assert.notEqual(r.steps[0].state, 'attention')
    assert.doesNotMatch(r.steps[0].detail, /no pull request/i)
    assert.deepEqual(r.next, { text: 'GitHub status could not be read.', tone: 'muted' })
  }
})

test('a pull request with no reported checks never marks Checks done or Merge ready', () => {
  for (const checks of [[], undefined]) {
    const r = deriveStages(input({ pull: pull({ checks }) }))
    assert.equal(r.steps[2].state, 'unknown')
    assert.equal(r.steps[2].detail, 'None reported')
    assert.notEqual(r.steps[4].state, 'current')
    assert.notEqual(r.next.text, 'Ready. GitHub decides whether it can merge.')
    assert.equal(r.next.tone, 'muted')
  }
})

test('threads not read on an older machine are unknown, never zero', () => {
  const r = deriveStages(input({ pull: pull({ review: '', detailKnown: undefined, threads: undefined, reviews: undefined }) }))
  assert.equal(r.steps[3].state, 'unknown')
  assert.equal(r.steps[3].detail, 'Threads not read on this machine')
})

test('an approval with threads unread is current, never done', () => {
  const r = deriveStages(input({ pull: pull({ threads: undefined }) }))
  assert.equal(r.steps[3].state, 'current')
  assert.equal(r.steps[3].detail, 'Approved. Threads not read on this machine')
  assert.notEqual(r.steps[4].state, 'current')
  assert.notEqual(r.next.text, 'Ready. GitHub decides whether it can merge.')
})

test('threads sent by a machine that has not read detail are not trusted', () => {
  const r = deriveStages(input({ pull: pull({ detailKnown: false, threads: { open: 0, resolved: 0 } }) }))
  assert.equal(r.steps[3].state, 'current')
})

test('a merged pull request collapses to a merged row', () => {
  const r = deriveStages(input({ pull: pull({ state: 'MERGED', mergedAt: 1700000000000, mergeable: 'CONFLICTING', checks: [bad('a')] }) }))
  assert.equal(r.next.text, 'Merged.')
  assert.equal(r.next.tone, 'good')
  assert.deepEqual(r.next.primary, link('Open pull request', 'https://github.com/o/r/pull/42'))
  assert.equal(r.next.fix, undefined)
  assert.deepEqual(r.merged, { at: 1700000000000, url: 'https://github.com/o/r/pull/42' })
  assert.equal(r.steps[4].state, 'done')
  assert.equal(deriveStages(input()).merged, undefined)
})

test('planning relabels the Code step and marks it current', () => {
  const r = deriveStages(input({ planning: true, pull: undefined, pullReason: 'no_pr', active: false }))
  assert.equal(r.steps[0].label, 'Planning')
  assert.equal(r.steps[0].state, 'current')
  assert.equal(deriveStages(input()).steps[0].label, 'Code')
})

test('github unavailable leaves PR, Checks, Review and Merge unknown with the reason', () => {
  const r = deriveStages(input({ pull: undefined, pullReason: 'github_unavailable' }))
  assert.deepEqual(states(r).slice(1), ['unknown', 'unknown', 'unknown', 'unknown'])
  assert.equal(r.steps[1].detail, 'Not read: GitHub is not available on this machine')
  assert.deepEqual(r.next, { text: 'GitHub status is not available on this machine.', tone: 'muted' })
})

test('merge is never claimed from green checks alone', () => {
  const r = deriveStages(input())
  assert.deepEqual(states(r), ['done', 'done', 'done', 'done', 'current'])
  assert.equal(r.next.text, 'Ready. GitHub decides whether it can merge.')
  assert.doesNotMatch(r.steps[4].detail, /^Merged/)
})
