import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./skills.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const s = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

const skill = (id, over) => ({ id, name: id, description: '', source: 'machine', agent: 'claude', native: false, onMachine: true, invocation: `/${id}`, ...over })

test('groupSkills orders native, other, reet and omits empty groups', () => {
  const list = [skill('r', { source: 'reet', onMachine: false }), skill('o', { agent: 'codex' }), skill('n', { native: true })]
  const groups = s.groupSkills(list, '', 'claude')
  assert.deepEqual(groups.map(g => g.key), ['native', 'other', 'reet'])
  assert.deepEqual(groups.map(g => g.label), ['On this machine for claude', 'On this machine for other agents', 'reet-hosted'])
  assert.deepEqual(s.groupSkills([skill('n', { native: true })], '', 'claude').map(g => g.key), ['native'])
  assert.deepEqual(s.groupSkills([], '', 'claude'), [])
})

test('groupSkills filters by name or description, case-insensitively', () => {
  const list = [skill('pdf', { native: true }), skill('x', { native: true, description: 'Make a PDF file' }), skill('other', { native: true })]
  const groups = s.groupSkills(list, 'PdF', 'claude')
  assert.equal(groups.length, 1)
  assert.deepEqual(groups[0].skills.map(k => k.id), ['pdf', 'x'])
  assert.deepEqual(s.groupSkills(list, 'zzz', 'claude'), [])
})

test('slashQuery', () => {
  assert.equal(s.slashQuery('/pdf'), 'pdf')
  assert.equal(s.slashQuery('/'), '')
  assert.equal(s.slashQuery('fix /etc/hosts'), null)
  assert.equal(s.slashQuery('/etc/hosts now'), null)
  assert.equal(s.slashQuery('http://x'), null)
  assert.equal(s.slashQuery('/etc/hosts'), 'etc/hosts')
})

test('applyInvocation replaces, appends, and refuses', () => {
  assert.deepEqual(s.applyInvocation('/pd', '/pdf', 2000), { draft: '/pdf' })
  assert.deepEqual(s.applyInvocation('fix the bug', '/pdf', 2000), { draft: 'fix the bug /pdf' })
  assert.deepEqual(s.applyInvocation('', '/pdf', 2000), { draft: '/pdf' })
  assert.deepEqual(s.applyInvocation('fix ', '/pdf', 2000), { draft: 'fix /pdf' })
  assert.deepEqual(s.applyInvocation('fix the bug in the parser', '/pdf', 20), { error: "That would pass the 2,000-character limit. Nothing was inserted." })
  assert.deepEqual(s.applyInvocation('abc', '', 2000), { error: "This skill can't be inserted from here." })
})

test('seenLine', () => {
  const now = 1_000_000_000_000
  assert.equal(s.seenLine(0, 'box', now), 'Skills on box have not been read yet.')
  assert.equal(s.seenLine(now - 3 * 60_000, 'box', now), 'Skills on box were read 3 minutes ago.')
  assert.equal(s.seenLine(now - 5 * 3_600_000, 'box', now), 'Skills on box were read 5 hours ago.')
  assert.equal(s.seenLine(now - 2 * 86_400_000, 'box', now), 'Skills on box were read 2 days ago.')
  assert.equal(s.seenLine(now - 60_000, 'box', now), 'Skills on box were read 1 minute ago.')
})
