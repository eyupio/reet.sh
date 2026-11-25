import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const transpile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const terminalApi = `data:text/javascript;base64,${Buffer.from(transpile('../terminal/api.ts')).toString('base64')}`
const source = transpile('./commandBar.ts').replace("'../terminal/api'", `'${terminalApi}'`)
const c = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

const folders = [{ name: 'cmd' }, { name: '.git', hidden: true }, { name: 'my docs' }, { name: 'web' }, { name: 'Worktrees' }]
const from = { folders, parent: true, recent: ['git status', 'go test ./...', 'git status'], saved: ['df -h', 'echo a\necho b'] }
const texts = list => list.map(item => item.text)

test('an empty bar offers the next actions: folders to cd into, then recent and saved commands', () => {
  assert.deepEqual(texts(c.suggest('', from)), ['cd cmd/', "cd 'my docs'/", 'cd web/', 'cd Worktrees/', 'cd ..', 'git status', 'go test ./...', 'df -h'])
  assert.equal(c.suggest('', from)[1].label, 'cd my docs/')
  assert.deepEqual(texts(c.suggest('', { ...from, parent: false, folders: [] })), ['git status', 'go test ./...', 'df -h'])
})

test('a command word completes from the commands run or saved', () => {
  assert.deepEqual(texts(c.suggest('g', from)), ['git status', 'go test ./...'])
  assert.deepEqual(texts(c.suggest('git status', from)), [], 'what is already typed is not suggested')
})

test('the last word completes to a folder, whatever the letter case, and keeps what was typed before it', () => {
  assert.deepEqual(texts(c.suggest('cd w', from)), ['cd web/', 'cd Worktrees/'])
  assert.deepEqual(texts(c.suggest('ls -la src/w', from)), ['ls -la src/web/', 'ls -la src/Worktrees/'])
  assert.deepEqual(texts(c.suggest('cd ', from)), ['cd cmd/', "cd 'my docs'/", 'cd web/', 'cd Worktrees/'])
  assert.deepEqual(texts(c.suggest('cd .', from)), ['cd .git/'], 'a hidden folder only once a dot is typed')
})

test('a command with a hidden character is never suggested', () => {
  assert.ok(!texts(c.suggest('e', from)).some(text => text.includes('\n')))
  assert.deepEqual(texts(c.suggest('', { folders: [{ name: 'a\u202eb' }], recent: [], saved: [] })), [])
})

test('the row is capped', () => {
  const many = Array.from({ length: 40 }, (_, i) => ({ name: `d${i}` }))
  const list = c.suggest('', { ...from, folders: many })
  assert.deepEqual(list.map(item => item.kind), [...Array(7).fill('folder'), 'recent', 'recent', 'saved'], 'six folders and cd ..')
  assert.equal(c.suggest('cd d', { ...from, folders: many }).length, 12)
  assert.ok(texts(list).includes('git status'), 'commands keep a place beside many folders')
})

test('the path being typed', () => {
  assert.equal(c.pathToken('cd'), null)
  assert.deepEqual(c.pathToken('cd '), { head: 'cd ', dir: '', prefix: '' })
  assert.deepEqual(c.pathToken('cd web/sr'), { head: 'cd ', dir: 'web/', prefix: 'sr' })
  assert.equal(c.pathToken("cd 'my d"), null)
  assert.equal(c.pathToken('echo $HO'), null)
})

test('the folder a typed path names', () => {
  assert.equal(c.resolveDir('/home/me/code', '/home/me', ''), '/home/me/code')
  assert.equal(c.resolveDir('/home/me/code', '/home/me', 'web/src/'), '/home/me/code/web/src')
  assert.equal(c.resolveDir('/home/me/code', '/home/me', '../'), '/home/me')
  assert.equal(c.resolveDir('/home/me/code', '/home/me', './a/../b/'), '/home/me/code/b')
  assert.equal(c.resolveDir('/home/me/code', '/home/me', '/etc/'), '/etc')
  assert.equal(c.resolveDir('/home/me/code', '/home/me', '~/x/'), '/home/me/x')
  assert.equal(c.resolveDir('/', '/root', '../../'), '/')
  assert.equal(c.resolveDir('~/code', '', 'web/'), '~/code/web')
})

test('a name as one shell word', () => {
  assert.equal(c.shellWord('web'), 'web')
  assert.equal(c.shellWord('my docs'), "'my docs'")
  assert.equal(c.shellWord("it's"), "'it'\\''s'")
})
