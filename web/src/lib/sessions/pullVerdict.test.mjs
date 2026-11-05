import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

// pullVerdict.ts needs checkTone at run time, so both files are transpiled and the import specifier points at a data URL.
const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const source = compile('./pullVerdict.ts').replace("from '../sessionInsights'", `from '${asURL(compile('../sessionInsights.ts'))}'`)
const { pullVerdict, splitChecks } = await import(asURL(source))

const pull = (over = {}) => ({ number: 7, title: 't', url: 'https://github.com/o/r/pull/7', state: 'OPEN', draft: false, review: '', mergeable: 'MERGEABLE', added: 1, removed: 1, checks: [], ...over })
const ok = name => ({ name, state: 'SUCCESS' })
const bad = name => ({ name, state: 'FAILURE' })
const run = name => ({ name, state: 'IN_PROGRESS' })
const v = over => pullVerdict(pull(over))

test('checks split into failed, running and passed by their tone', () => {
  const s = splitChecks([ok('a'), bad('b'), run('c'), { name: 'd', state: 'SKIPPED' }, { name: 'e', state: 'TIMED_OUT' }])
  assert.deepEqual(s.failed.map(c => c.name), ['b', 'e'])
  assert.deepEqual(s.running.map(c => c.name), ['c'])
  assert.deepEqual(s.passed.map(c => c.name), ['a', 'd'])
})

test('a merged pull request is good whatever its checks say', () => {
  assert.deepEqual(v({ state: 'MERGED', checks: [bad('x')] }), { tone: 'good', icon: 'check', title: 'Pull request merged', detail: 'This branch’s pull request is merged.' })
})

test('a closed pull request is muted', () => {
  assert.deepEqual(v({ state: 'CLOSED' }), { tone: 'muted', icon: 'github', title: 'Pull request closed', detail: 'This pull request was closed without merging.' })
})

test('conflicts alone ask for them to be resolved', () => {
  assert.deepEqual(v({ mergeable: 'CONFLICTING' }), { tone: 'bad', icon: 'conflict', title: 'Resolve merge conflicts', detail: 'GitHub reports conflicts with the base branch.' })
})

test('conflicts name one failing check in the singular', () => {
  const r = v({ mergeable: 'CONFLICTING', checks: [bad('a')] })
  assert.equal(r.title, 'Resolve merge conflicts')
  assert.equal(r.detail, 'GitHub reports conflicts with the base branch. 1 check is also failing.')
})

test('conflicts count several failing checks in the plural', () => {
  assert.equal(v({ mergeable: 'CONFLICTING', checks: [bad('a'), bad('b')] }).detail, 'GitHub reports conflicts with the base branch. 2 checks are also failing.')
})

test('conflicts mention requested changes when no check fails', () => {
  assert.equal(v({ mergeable: 'CONFLICTING', review: 'CHANGES_REQUESTED' }).detail, 'GitHub reports conflicts with the base branch. A reviewer also requested changes.')
})

test('failing checks come before a review decision', () => {
  assert.deepEqual(v({ checks: [bad('lint')] }), { tone: 'bad', icon: 'conflict', title: '1 check failed', detail: 'Failing: lint.' })
})

test('failing checks list two names and count the rest', () => {
  const r = v({ checks: [bad('a'), bad('b'), bad('c'), bad('d')] })
  assert.equal(r.title, '4 checks failed')
  // "mores" is what the pane has always said (count() pluralises the word); this refactor keeps the text exactly.
  assert.equal(r.detail, 'Failing: a and b, plus 2 mores.')
})

test('failing checks also report requested changes', () => {
  assert.equal(v({ checks: [bad('a'), bad('b')], review: 'CHANGES_REQUESTED' }).detail, 'Failing: a and b. A reviewer also requested changes.')
})

test('requested changes alone are bad', () => {
  assert.deepEqual(v({ review: 'CHANGES_REQUESTED', checks: [ok('a')] }), { tone: 'bad', icon: 'conflict', title: 'Changes requested', detail: 'A reviewer asked for changes before this can merge.' })
})

test('running checks are a warning and name what is awaited', () => {
  assert.deepEqual(v({ checks: [run('build'), ok('a')] }), { tone: 'warn', icon: 'clock', title: '1 check running', detail: 'Waiting on build.' })
})

test('a draft is muted once nothing is failing or running', () => {
  assert.deepEqual(v({ draft: true, checks: [ok('a')] }), { tone: 'muted', icon: 'github', title: 'Draft pull request', detail: 'Mark it ready for review when the work is complete.' })
})

test('a required review is a warning', () => {
  assert.deepEqual(v({ review: 'REVIEW_REQUIRED', checks: [ok('a')] }), { tone: 'warn', icon: 'clock', title: 'Review required', detail: 'A review is required before this can merge.' })
})

test('no checks reported is muted and does not claim CI passed', () => {
  assert.deepEqual(v({}), { tone: 'muted', icon: 'github', title: 'No checks reported', detail: 'This does not confirm that CI passed.' })
})

test('an approved pull request with passing checks says so', () => {
  assert.deepEqual(v({ review: 'APPROVED', checks: [ok('a'), ok('b')] }), { tone: 'good', icon: 'check', title: 'Checks passed', detail: 'Review approved. 2 checks passed.' })
})

test('passing checks without a decision say no review decision yet', () => {
  assert.deepEqual(v({ checks: [ok('a')] }), { tone: 'good', icon: 'check', title: 'Checks passed', detail: '1 check passed. No review decision yet.' })
})
