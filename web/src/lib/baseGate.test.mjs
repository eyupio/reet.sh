import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

// The one check-then-ask step both start paths use: it reads the base check and answers with the sentence to confirm, or
// nothing at all. Every refusal and every silence must come back as nothing, because a start is never held up for it.
const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const launchURL = asURL(compile('./agentLaunch.ts')
  .replace("from './catalogue'", `from '${asURL(compile('./catalogue.ts'))}'`)
  .replace("from './clineSource'", `from '${asURL(compile('./clineSource.ts'))}'`))
const noticeURL = asURL(compile('./baseNotice.ts'))
const apiURL = asURL(compile('./terminal/api.ts'))
const { BASE_CHECK_TIMEOUT_MS } = await import(apiURL)
const { baseKey, baseQuestion, repoProjectAt } = await import(asURL(compile('./baseGate.ts')
  .replace("from './agentLaunch'", `from '${launchURL}'`)
  .replace("from './baseNotice'", `from '${noticeURL}'`)
  .replace("from './terminal/api'", `from '${apiURL}'`)))

const START = 'abc1234def5678901234567890abcdef12345678'
const repo = { isRepo: true }
// Each call is recorded; `answer` decides what the route says.
let asked = [], answer = () => ({ ok: true, json: async () => ({ base: 'main', relation: 'ahead', local: START, start: START }) })
globalThis.fetch = async (url, init) => { asked.push({ url: String(url), init }); return answer(url, init) }
const fresh = () => { asked = [] }

test('an isolated start with a base behind GitHub comes back as the sentence to confirm', async () => {
  fresh()
  assert.equal(await baseQuestion('a1', 'studio', { tool: 'claude', dir: '/home/dev/store', base: 'main' }, repo),
    "main on studio has commits GitHub doesn't have. This session starts from the local main (abc1234).")
  assert.equal(asked.length, 1)
  assert.equal(asked[0].url, '/api/v2/machines/a1/base-check?dir=%2Fhome%2Fdev%2Fstore&base=main')
})

test('a base with nothing to say comes back as nothing', async () => {
  fresh()
  answer = () => ({ ok: true, json: async () => ({ base: 'main', relation: 'same', local: START, remote: START, start: START }) })
  assert.equal(await baseQuestion('a1', 'studio', { tool: 'claude', dir: '/home/dev/store' }, repo), '')
  assert.equal(asked.length, 1)
  // A base check that names no base asks about the project's own.
  assert.equal(asked[0].url, '/api/v2/machines/a1/base-check?dir=%2Fhome%2Fdev%2Fstore')
})

test('a launch that will not be isolated is never even asked about', async () => {
  for (const [name, launch, project] of [
    ['a plain shell', { dir: '/home/dev/store' }, repo],
    ['a folder that is no repository', { tool: 'claude', dir: '/home/dev/notes' }, { isRepo: false }],
    ['the home folder', { tool: 'claude' }, undefined],
    ['a continuation', { tool: 'claude', dir: '/home/dev/store', resume: true }, repo],
  ]) {
    fresh()
    assert.equal(await baseQuestion('a1', 'studio', launch, project), '', name)
    assert.deepEqual(asked, [], name)
  }
})

test('a repository reet cloned itself is not asked about', async () => {
  fresh()
  answer = () => ({ ok: true, json: async () => ({ base: 'main', relation: 'ahead', local: START, start: START }) })
  assert.equal(await baseQuestion('a1', 'studio', { tool: 'claude', dir: '/home/dev/store' }, { isRepo: true, cloned: true }), '')
})

test('every way the check can fail comes back as nothing, never as a throw', async () => {
  const fails = {
    'an older machine': () => ({ ok: false, status: 409, json: async () => ({ error: 'Update reet on that machine.', code: 'base_check_unsupported' }) }),
    'a busy machine': () => ({ ok: false, status: 409, json: async () => ({ error: 'That machine is busy.', code: 'branches_busy' }) }),
    'a missing branch': () => ({ ok: false, status: 422, json: async () => ({ error: 'No such branch.', code: 'base_missing' }) }),
    'too many reads': () => ({ ok: false, status: 429, json: async () => ({ error: 'Slow down.' }) }),
    'a viewer': () => ({ ok: false, status: 403, json: async () => ({ error: 'Read-only.' }) }),
    'nothing at all': () => { throw new TypeError('Failed to fetch') },
    'a body that is not the answer': () => ({ ok: true, json: async () => { throw new SyntaxError('Unexpected token') } }),
  }
  for (const [name, fail] of Object.entries(fails)) {
    fresh()
    answer = fail
    assert.equal(await baseQuestion('a1', 'studio', { tool: 'claude', dir: '/home/dev/store', base: 'main' }, repo), '', name)
    assert.equal(asked.length, 1, name)
  }
})

test('a reply that never comes is given up on, and the deadline is ten seconds', async () => {
  fresh()
  // As fetch itself behaves: the request only ends when its signal aborts.
  answer = (url, init) => new Promise((_, reject) => {
    init.signal.addEventListener('abort', () => reject(new DOMException('The operation was aborted.', 'AbortError')))
  })
  const began = Date.now()
  // Node leaves AbortSignal.timeout's own timer unreferenced, so without something holding the loop open the process
  // would end before the deadline fires. A browser has no such rule.
  const awake = setInterval(() => {}, 5)
  // The deadline is shortened here so the suite does not wait ten seconds; the default is checked below.
  const say = await baseQuestion('a1', 'studio', { tool: 'claude', dir: '/home/dev/store', base: 'main' }, repo, undefined, 40)
  clearInterval(awake)
  assert.equal(say, '')
  const waited = Date.now() - began
  assert.ok(waited >= 30 && waited < 4000, `gave up after ${waited}ms`)
  assert.equal(asked.length, 1)
  assert.equal(BASE_CHECK_TIMEOUT_MS, 10000)
})

test('a caller that gives up takes the question with it', async () => {
  fresh()
  const life = new AbortController()
  const timer = setTimeout(() => life.abort(), 20)
  assert.equal(await baseQuestion('a1', 'studio', { tool: 'claude', dir: '/home/dev/store' }, repo, life.signal, 5000), '')
  clearTimeout(timer)
  assert.equal(asked.length, 1)
})

test('the key a yes is tied to changes with the machine, the folder, the base and the agent', () => {
  const launch = { tool: 'claude', dir: '/home/dev/store', base: 'main' }
  const key = baseKey('a1', launch)
  assert.equal(baseKey('a1', { ...launch }), key)
  for (const [name, other] of [
    ['another machine', () => baseKey('a2', launch)],
    ['another folder', () => baseKey('a1', { ...launch, dir: '/home/dev/site' })],
    ['another base', () => baseKey('a1', { ...launch, base: 'develop' })],
    ['no base at all', () => baseKey('a1', { ...launch, base: '' })],
    ['another agent', () => baseKey('a1', { ...launch, tool: 'codex' })],
  ]) assert.notEqual(other(), key, name)
  // The run token is new on every press, so it must not be part of what was agreed to.
  assert.equal(baseKey('a1', { ...launch, worktree: 'a'.repeat(32) }), key)
  // One folder cannot be read as another: the parts are kept apart.
  assert.notEqual(baseKey('a1', { tool: '', dir: '/home/dev/store', base: 'main' }), baseKey('a1', { tool: 'main', dir: '/home/dev/store', base: '' }))
})

test('the saved repository a launch would be isolated into is the one with exactly that folder', () => {
  const projects = [
    { id: 'fp1', host: 'a1', dir: '/home/dev/store', isRepo: true },
    { id: 'fp2', host: 'a1', dir: '/home/dev/notes', isRepo: false },
    { id: 'fp3', host: 'a2', dir: '/home/dev/store', isRepo: true },
  ]
  assert.equal(repoProjectAt(projects, 'a1', '/home/dev/store')?.id, 'fp1')
  // Another machine's project, a folder that is not a repository, a folder nobody saved, and no list at all.
  assert.equal(repoProjectAt(projects, 'a3', '/home/dev/store'), null)
  assert.equal(repoProjectAt(projects, 'a1', '/home/dev/notes'), null)
  assert.equal(repoProjectAt(projects, 'a1', '/home/dev/other'), null)
  assert.equal(repoProjectAt(undefined, 'a1', '/home/dev/store'), null)
  assert.equal(repoProjectAt(projects, 'a1', ''), null)
})
