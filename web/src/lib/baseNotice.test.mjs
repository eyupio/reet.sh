import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

// What the browser says about where an isolated session will start, and when it asks first. The sentences are the only
// place the relations are put into words, so each one is checked exactly as it is shown.
const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const { baseSentence, needsConfirm } = await import(asURL(compile('./baseNotice.ts')))
const { isolatedLaunch } = await import(asURL(compile('./agentLaunch.ts')
  .replace("from './catalogue'", `from '${asURL(compile('./catalogue.ts'))}'`)
  .replace("from './clineSource'", `from '${asURL(compile('./clineSource.ts'))}'`)))

const START = 'abc1234def5678901234567890abcdef12345678'
const state = (relation, over = {}) => ({ base: 'main', relation, local: START, remote: START, start: START, ...over })

test('a base that agrees with GitHub, or has no branch there, says nothing', () => {
  for (const relation of ['same', 'no_remote']) assert.equal(baseSentence(state(relation), 'studio'), null)
  // An unknown relation is not put into words: a newer machine never makes the browser invent a sentence.
  assert.equal(baseSentence(state('elsewhere'), 'studio'), null)
  assert.equal(baseSentence({}, 'studio'), null)
})

test('a base behind GitHub, or missing locally, says the session starts from GitHub', () => {
  const sentence = "main on studio is behind GitHub. This session starts from GitHub's main (abc1234)."
  assert.equal(baseSentence(state('behind'), 'studio'), sentence)
  assert.equal(baseSentence(state('no_local', { local: '' }), 'studio'), sentence)
})

test('a base with commits GitHub lacks says the session starts from the local branch', () => {
  const sentence = "main on studio has commits GitHub doesn't have. This session starts from the local main (abc1234)."
  assert.equal(baseSentence(state('ahead'), 'studio'), sentence)
  assert.equal(baseSentence(state('diverged'), 'studio'), sentence)
})

test('a fetch that never reached GitHub says the base may be out of date', () => {
  assert.equal(baseSentence(state('fetch_failed', { remote: '' }), 'studio'),
    "Couldn't reach GitHub from studio, so main may be out of date. This session starts from the last known main (abc1234).")
})

test('the sentences name the branch and the machine they were given', () => {
  assert.equal(baseSentence(state('behind', { base: 'release/1.2' }), 'desk'),
    "release/1.2 on desk is behind GitHub. This session starts from GitHub's release/1.2 (abc1234).")
  assert.equal(baseSentence(state('ahead', { base: 'develop' }), 'desk'),
    "develop on desk has commits GitHub doesn't have. This session starts from the local develop (abc1234).")
})

test('with no branch name the sentences say the base branch instead', () => {
  assert.equal(baseSentence(state('behind', { base: '' }), 'studio'),
    "The base branch on studio is behind GitHub. This session starts from GitHub's base branch (abc1234).")
  assert.equal(baseSentence(state('ahead', { base: '' }), 'studio'),
    "The base branch on studio has commits GitHub doesn't have. This session starts from the local base branch (abc1234).")
  assert.equal(baseSentence(state('fetch_failed', { base: '' }), 'studio'),
    "Couldn't reach GitHub from studio, so the base branch may be out of date. This session starts from the last known base branch (abc1234).")
})

test('the short commit is the first seven characters, and no commit leaves the sentence whole', () => {
  assert.match(baseSentence(state('behind', { start: 'fedcba9876543210fedcba9876543210fedcba98' }), 'studio'), /\(fedcba9\)\.$/)
  assert.equal(baseSentence(state('behind', { start: '' }), 'studio'),
    "main on studio is behind GitHub. This session starts from GitHub's main.")
})

test('a machine with no name is still said plainly', () => {
  assert.equal(baseSentence(state('behind'), ''), "main on that machine is behind GitHub. This session starts from GitHub's main (abc1234).")
})

test('a start asks first whenever there is something to say', () => {
  for (const relation of ['behind', 'no_local', 'ahead', 'diverged', 'fetch_failed']) {
    assert.equal(needsConfirm(state(relation), {}), true, relation)
  }
  for (const relation of ['same', 'no_remote', 'elsewhere']) {
    assert.equal(needsConfirm(state(relation), {}), false, relation)
  }
  assert.equal(needsConfirm({}, {}), false)
})

test('a project reet cloned itself is never asked about', () => {
  assert.equal(needsConfirm(state('ahead'), { cloned: true }), false)
  assert.equal(needsConfirm(state('ahead'), { cloned: false }), true)
})

test('only a launch that will be isolated into a worktree is checked', () => {
  const repo = { isRepo: true }, folder = { isRepo: false }
  assert.equal(isolatedLaunch({ tool: 'claude', dir: '/home/dev/site' }, repo), true)
  // The controller isolates a chat in a saved repository too, so it is checked the same way.
  assert.equal(isolatedLaunch({ tool: 'opencode', dir: '/home/dev/site', structured: true }, repo), true)
  // A folder that is not a repository, a plain shell, a continuation and a reopened chat are never isolated.
  assert.equal(isolatedLaunch({ tool: 'claude', dir: '/home/dev/notes' }, folder), false)
  assert.equal(isolatedLaunch({ dir: '/home/dev/site' }, repo), false)
  assert.equal(isolatedLaunch({ tool: 'claude', dir: '/home/dev/site', resume: true }, repo), false)
  assert.equal(isolatedLaunch({ tool: 'claude', dir: '/home/dev/site', resumeAcp: 'abc' }, repo), false)
  // The home folder names no folder at all.
  assert.equal(isolatedLaunch({ tool: 'claude' }, undefined), false)
  // A launch that names its own worktree is isolated whatever the page knows about the folder.
  assert.equal(isolatedLaunch({ tool: 'claude', dir: '/home/dev/site', worktree: 'a'.repeat(32) }, undefined), true)
  // Already inside a worktree reet made: the controller leaves it alone, so nothing is checked.
  assert.equal(isolatedLaunch({ tool: 'claude', dir: `/home/dev/.reet/worktrees/site/site/${'a'.repeat(32)}` }, repo), false)
})
