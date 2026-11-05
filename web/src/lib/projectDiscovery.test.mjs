import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = readFileSync(new URL('./projectDiscovery.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const d = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
const { joinFolder, reposFrom, nextFolders, scanOrder, mergeRepos, repoKey, savedKeys, skipFolder } = d

const entry = (name, over = {}) => ({ name, dir: true, repoRoot: false, hidden: false, ...over })
const page = (path, entries) => ({ path, home: '/home/me', entries })

test('a folder is joined without doubling its slash', () => {
  assert.equal(joinFolder('/home/me', 'code'), '/home/me/code')
  assert.equal(joinFolder('/home/me/', 'code'), '/home/me/code')
  assert.equal(joinFolder('', 'code'), '/code')
})

test('only repository tops are found, named and placed', () => {
  const found = reposFrom(page('/home/me/code', [
    entry('site', { repoRoot: true }),
    entry('plain'),
    entry('notes.txt', { dir: false }),
    entry('nested', { hidden: true, repoRoot: true }),
  ]), 'agent-1')
  assert.deepEqual(found, [
    { host: 'agent-1', dir: '/home/me/code/site', name: 'site' },
    { host: 'agent-1', dir: '/home/me/code/nested', name: 'nested' },
  ])
})

test('the scan opens ordinary folders but never a repository or a cache', () => {
  const next = nextFolders(page('/home/me', [
    entry('code'), entry('site', { repoRoot: true }), entry('node_modules'), entry('.config'),
    entry('readme', { dir: false }),
  ]), 0, 3, new Set())
  assert.deepEqual(next, [{ path: '/home/me/code', depth: 1 }])
  assert.deepEqual(nextFolders(page('/home/me', [entry('code')]), 3, 3, new Set()), [])
  assert.deepEqual(nextFolders(page('/home/me', [entry('code')]), 0, 3, new Set(['/home/me/code'])), [])
})

test('folder names that scans skip are named once', () => {
  for (const name of ['node_modules', '.git', '.cache', 'dist', 'target', '']) assert.equal(skipFolder(name), true, name)
  for (const name of ['code', 'src', 'reet', 'Projects']) assert.equal(skipFolder(name), false, name)
})

test('scan order prefers shallow folders and common repository parents', () => {
  const order = [
    { path: '/home/me/a/deep', depth: 2 },
    { path: '/home/me/projects', depth: 1 },
    { path: '/home/me/code', depth: 1 },
    { path: '/home/me/zeta', depth: 1 },
  ].sort(scanOrder)
  assert.deepEqual(order.map(f => f.path), ['/home/me/code', '/home/me/projects', '/home/me/zeta', '/home/me/a/deep'])
})

test('repositories merge once by machine and folder', () => {
  const a = { host: 'a', dir: '/home/me/site', name: 'site' }
  const b = { host: 'a', dir: '/home/me/lib', name: 'lib' }
  const c = { host: 'b', dir: '/home/me/site', name: 'site' }
  assert.deepEqual(mergeRepos([b, a], [a, c]), [b, a, c])
  assert.equal(repoKey(a), 'a\u0000/home/me/site')
  assert.deepEqual([...savedKeys([a, c])], ['a\u0000/home/me/site', 'b\u0000/home/me/site'])
})
