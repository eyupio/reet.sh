import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

// The browser's reading of a GitHub repository name must agree with the controller's (terminalproto.ParseGitHubRepo):
// every case of the Go test is here, with the same answer, plus the whitespace both sides trim. The watcher of an
// outstanding clone is checked with a stand-in for the status read and its own short interval.
const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const { parseGitHubRepo, cloneFolder, lostCloneLine, startUnheardLine, timedOut, clonedProject, watchClone, outstandingClone, rememberClone } = await import(asURL(compile('./githubRepo.ts')))

const shown = r => r ? `${r.owner}/${r.repo}` : null

test('accepts what the controller accepts, as the controller reads it', () => {
  for (const [input, want] of [
    ['eyupio/reet.sh', 'eyupio/reet.sh'],
    ['https://github.com/eyupio/reet.sh', 'eyupio/reet.sh'],
    ['https://github.com/eyupio/reet.sh.git', 'eyupio/reet.sh'],
    ['https://github.com/eyupio/reet.sh/', 'eyupio/reet.sh'],
    ['https://github.com/eyupio/reet.sh.git/', 'eyupio/reet.sh'],
    ['eyupio/reet.sh.git', 'eyupio/reet.sh'],
    ['  eyupio/reet.sh \n', 'eyupio/reet.sh'],
    ['https://GitHub.com/eyupio/reet.sh', 'eyupio/reet.sh'],
    ['HTTPS://GITHUB.COM/eyupio/reet.sh', 'eyupio/reet.sh'],
    ['a/.github', 'a/.github'],
    ['a/-x', 'a/-x'],
    ['a/b', 'a/b'],
    ['a/_', 'a/_'],
    ['Owner-1/Repo_2.x', 'Owner-1/Repo_2.x'],
    [`${'a'.repeat(39)}/r`, `${'a'.repeat(39)}/r`],
    [`a/${'b'.repeat(100)}`, `a/${'b'.repeat(100)}`],
    // Go's TrimSpace: Unicode spaces, the next-line control and the no-break space.
    [' a/b　', 'a/b'],
    ['\u0085a/b\u0085', 'a/b'],
    [' a/b ', 'a/b'],
  ]) assert.equal(shown(parseGitHubRepo(input)), want, JSON.stringify(input))
})

test('refuses what the controller refuses', () => {
  for (const input of [
    '', ' ', 'a', 'a/b/c', 'a/..', 'a/.', '-a/b', 'a-/b', 'a/b c', 'a/.git', '/b', 'a/',
    'http://github.com/a/b',
    'https://gitlab.com/a/b',
    'https://github.example.com/a/b',
    'https://github.com.evil.example/a/b',
    'ssh://git@github.com/a/b',
    'git@github.com:a/b.git',
    'github.com/a/b',
    'https://user@github.com/a/b',
    'https://user:pw@github.com/a/b',
    'https://github.com:443/a/b',
    'https://github.com/a/b?x=1',
    'https://github.com/a/b#frag',
    'https://github.com/a/b//',
    'https://github.com/a/b/extra',
    'https://github.com/a',
    'https://github.com/',
    'a/b?x=1',
    'a/b#x',
    'a/b/',
    'a/b//',
    `${'a'.repeat(40)}/b`,
    `a/${'b'.repeat(101)}`,
    'a/...', 'a/....git', 'a/b.', 'a/b..git', 'https://github.com/a/b.',
    'a/x.git.git',
    'a_b/c', 'a.b/c',
    'a/b\x00',
    'a\x00/b',
    'a/b\tc',
    'a/b\nc',
    'a\x7f/b',
    'a/b\u0085c',
    // Not trimmed by Go (a byte-order mark is not a space there), so it stays part of the owner and is refused.
    '﻿a/b',
    'a/b﻿',
    // Non-ASCII letters that fold to ASCII ones are not ASCII to GitHub.
    'httpſ://github.com/a/b',
    'https://github.com/Ka/b',
    'é/b', 'a/é',
  ]) assert.equal(parseGitHubRepo(input), null, JSON.stringify(input))
})

test('the folder the machine clones into', () => {
  assert.equal(cloneFolder({ owner: 'eyupio', repo: 'reet.sh' }), '~/reet/repos/eyupio/reet.sh')
})

const tick = ms => new Promise(r => setTimeout(r, ms))
const clone = (over = {}) => ({ id: 'c1', host: 'a1', machine: 'studio', repo: 'eyupio/reet.sh', ...over })

test('the outstanding clone lives in memory only and is forgotten when it ends', async () => {
  rememberClone(null)
  assert.equal(outstandingClone(), null)
  rememberClone(clone())
  assert.deepEqual(outstandingClone(), clone())
  const answers = [{ state: 'cloning' }, { state: 'done', project: { id: 'fp9' } }]
  const asked = []
  const ended = await new Promise(resolve => watchClone(clone(), {
    interval: 5,
    status: async (id) => { asked.push(id); return answers.shift() },
    onend: resolve,
  }))
  assert.deepEqual(ended, { state: 'done', project: { id: 'fp9' } })
  assert.deepEqual(asked, ['c1', 'c1'])
  assert.equal(outstandingClone(), null)
})

test('a failed clone ends with the controller’s sentence', async () => {
  rememberClone(clone())
  const ended = await new Promise(resolve => watchClone(clone(), {
    interval: 5,
    status: async () => ({ state: 'failed', code: 'no_access', sentence: "GitHub has no eyupio/reet.sh that studio's sign-in can read." }),
    onend: resolve,
  }))
  assert.deepEqual(ended, { state: 'failed', code: 'no_access', sentence: "GitHub has no eyupio/reet.sh that studio's sign-in can read." })
  assert.equal(outstandingClone(), null)
})

test('an unknown or expired clone ends quietly; a signed-out answer keeps it outstanding', async () => {
  rememberClone(clone())
  const gone = await new Promise(resolve => watchClone(clone(), {
    interval: 5,
    status: async () => { throw Object.assign(new Error("That clone isn't on this account."), { status: 404, code: 'clone_not_found' }) },
    onend: resolve,
  }))
  assert.deepEqual(gone, { state: 'gone' })
  assert.equal(outstandingClone(), null)

  rememberClone(clone())
  const out = await new Promise(resolve => watchClone(clone(), {
    interval: 5,
    status: async () => { throw Object.assign(new Error('signed out'), { status: 401 }) },
    onend: resolve,
  }))
  assert.equal(out.state, 'signedout')
  assert.deepEqual(outstandingClone(), clone())
  rememberClone(null)
})

test('a read that does not arrive is tried again at the same pace', async () => {
  rememberClone(clone())
  let reads = 0
  const ended = await new Promise(resolve => watchClone(clone(), {
    interval: 5,
    status: async () => { reads++; if (reads < 3) throw new TypeError('Failed to fetch'); return { state: 'done', project: { id: 'fp9' } } },
    onend: resolve,
  }))
  assert.equal(reads, 3)
  assert.equal(ended.state, 'done')
})

test('stopping the watch stops the reads and leaves the clone outstanding', async () => {
  rememberClone(clone())
  let reads = 0, ends = 0
  const stop = new AbortController()
  watchClone(clone(), { interval: 5, signal: stop.signal, status: async () => { reads++; return { state: 'cloning' } }, onend: () => { ends++ } })
  await tick(30)
  assert.ok(reads >= 2, `read ${reads} times`)
  stop.abort()
  const after = reads
  await tick(30)
  assert.equal(reads, after)
  assert.equal(ends, 0)
  assert.deepEqual(outstandingClone(), clone())
  rememberClone(null)
})

test('the first read waits one interval', async () => {
  let reads = 0
  const stop = new AbortController()
  watchClone(clone(), { interval: 40, signal: stop.signal, status: async () => { reads++; return { state: 'cloning' } }, onend: () => {} })
  await tick(10)
  assert.equal(reads, 0)
  await tick(50)
  assert.equal(reads, 1)
  stop.abort()
})

test('a clone the controller lost is said with its repository', () => {
  assert.equal(lostCloneLine('eyupio/reet.sh'), 'reet lost track of cloning eyupio/reet.sh. Add it again; if the clone finished, reet picks it up.')
})

test('a start that got no answer in time is said as that, never as a request that did not arrive', () => {
  assert.equal(startUnheardLine, "reet didn't hear back about starting the clone. Check the project list in a minute.")
  assert.equal(timedOut(new DOMException('The operation timed out.', 'TimeoutError')), true)
  assert.equal(timedOut(new DOMException('Aborted', 'AbortError')), false)
  assert.equal(timedOut(new TypeError('Failed to fetch')), false)
  assert.equal(timedOut(undefined), false)
})

test('a lost clone is found in the list by its machine and repository', () => {
  const lost = { id: 'c1', host: 'a1', machine: 'studio', repo: 'Eyupio/Reet.sh' }
  const p = (over) => ({ id: 'fp9', host: 'a1', origin: 'eyupio/reet.sh', ...over })
  assert.equal(clonedProject([p({ id: 'x', origin: undefined }), p({})], lost)?.id, 'fp9')
  assert.equal(clonedProject([p({ host: 'a2' })], lost), undefined)
  assert.equal(clonedProject([p({ origin: 'eyupio/other' })], lost), undefined)
  assert.equal(clonedProject([p({ origin: undefined })], lost), undefined)
  assert.equal(clonedProject([], lost), undefined)
})
