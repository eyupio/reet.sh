import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./pullFacts.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { reviewsLine, threadsFact, conflictFact, placeholderLine } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

const pull = (over = {}) => ({ number: 7, title: 't', url: 'https://github.com/o/r/pull/7', state: 'OPEN', draft: false, review: '', mergeable: 'MERGEABLE', added: 1, removed: 1, checks: [], ...over })

test('the reviews line tallies who said what when the machine read the detail', () => {
  assert.equal(reviewsLine(pull({ detailKnown: true, reviews: { approved: 1, changesRequested: 1, commented: 0, requested: 0 } })), '1 approved · changes requested by 1')
  assert.equal(reviewsLine(pull({ detailKnown: true, reviews: { approved: 0, changesRequested: 0, commented: 2, requested: 1 } })), '2 commented · 1 requested')
})

test('the reviews line falls back to the decision word on a machine that did not read the detail', () => {
  assert.equal(reviewsLine(pull({ review: 'APPROVED', reviews: { approved: 3, changesRequested: 0, commented: 0, requested: 0 } })), 'Approved')
  assert.equal(reviewsLine(pull({ review: 'CHANGES_REQUESTED' })), 'Changes requested')
  assert.equal(reviewsLine(pull({ review: '' })), 'No review decision yet')
})

test('an empty tally keeps the decision the pull request reports', () => {
  const empty = { approved: 0, changesRequested: 0, commented: 0, requested: 0 }
  assert.equal(reviewsLine(pull({ detailKnown: true, reviews: empty, review: 'REVIEW_REQUIRED' })), 'Review required')
  assert.equal(reviewsLine(pull({ detailKnown: true, reviews: empty })), 'No reviews yet')
})

test('threads that were not read are never reported as none', () => {
  assert.equal(threadsFact(pull()).text, 'Review threads not read on this machine')
  assert.equal(threadsFact(pull({ detailKnown: true })).text, 'Review threads not read on this machine')
  assert.equal(threadsFact(pull({ detailKnown: false, threads: { open: 0, resolved: 0 } })).text, 'Review threads not read on this machine')
})

test('threads count open ones, say none only when read, and admit a truncated list', () => {
  assert.deepEqual(threadsFact(pull({ detailKnown: true, threads: { open: 3, resolved: 1 } })), { text: '3 open threads', tone: 'warn' })
  assert.equal(threadsFact(pull({ detailKnown: true, threads: { open: 1, resolved: 0 } })).text, '1 open thread')
  assert.deepEqual(threadsFact(pull({ detailKnown: true, threads: { open: 0, resolved: 4 } })), { text: 'No open threads', tone: 'good' })
  assert.equal(threadsFact(pull({ detailKnown: true, threads: { open: 0, resolved: 0, more: true } })).text, '100+ threads, not all read')
})

test('a pull request that is not open has no threads or conflicts to report', () => {
  for (const state of ['MERGED', 'CLOSED']) {
    assert.equal(threadsFact(pull({ state })).text, '')
    assert.equal(conflictFact(pull({ state, mergeable: 'CONFLICTING' })).text, '')
  }
})

test('conflicts name the base branch and never invent an answer', () => {
  assert.deepEqual(conflictFact(pull({ mergeable: 'CONFLICTING', base: 'main' })), { text: 'Conflicts with main', tone: 'bad' })
  assert.equal(conflictFact(pull({ mergeable: 'CONFLICTING' })).text, 'Conflicts with the base branch')
  assert.equal(conflictFact(pull({ mergeable: 'MERGEABLE' })).text, 'No conflicts reported')
  assert.equal(conflictFact(pull({ mergeable: 'UNKNOWN' })).text, 'Conflicts not reported yet')
})

test('only no_pr says there is no pull request; every other absence says it was not read', () => {
  const read = over => placeholderLine({ supported: true, loading: false, ...over })
  assert.equal(read({ reason: 'no_pr' }), 'No pull request yet.')
  assert.equal(read({ reason: 'github_unavailable' }), 'GitHub status is not available on this machine.')
  assert.equal(read({ reason: 'busy' }), 'GitHub status could not be read.')
  assert.equal(read({}), 'GitHub status could not be read.')
  assert.equal(read({ loading: true }), 'Checking GitHub…')
  assert.equal(placeholderLine({ supported: false, loading: true }), 'Update reet on this machine for GitHub status.')
})
