import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./skills.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const s = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)
const asURL = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const api = await import(asURL(ts.transpileModule(readFileSync(new URL('./api.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText.replace("from '../projects'", `from '${asURL('export {}')}'`)))

const skill = (id, over) => ({ id, name: id, description: '', source: 'machine', agent: 'claude', native: false, onMachine: true, invocation: `/${id}`, ...over })

test('groupSkills orders native, other and new, and omits empty groups', () => {
  const list = [skill('n2', { source: 'new' }), skill('o', { agent: 'codex' }), skill('n', { native: true })]
  const groups = s.groupSkills(list, '', 'claude')
  assert.deepEqual(groups.map(g => g.key), ['native', 'other', 'new'])
  assert.deepEqual(groups.map(g => g.label), ['On this machine for claude', 'On this machine for other agents', 'New on this machine'])
  assert.deepEqual(s.groupSkills([skill('n', { native: true })], '', 'claude').map(g => g.key), ['native'])
  assert.deepEqual(s.groupSkills([], '', 'claude'), [])
})

test('the Your skills group is gone; machine, project and unknown skills are still listed', () => {
  const list = [
    skill('pdf', { native: true }),
    skill('in-project', { native: true, id: 'a/b/in-project' }),
    skill('codex-only', { agent: 'codex' }),
    skill('cq', { source: 'new', native: true }),
    skill('written', { source: 'reet', onMachine: false }),
  ]
  const groups = s.groupSkills(list, '', 'claude')
  assert.ok(groups.every(g => g.label !== 'Your skills'))
  assert.deepEqual(groups.flatMap(g => g.skills.map(k => k.name)), ['pdf', 'in-project', 'codex-only', 'cq'])
  assert.equal(s.keepable(skill('cq', { source: 'new' })), true)
  assert.equal(s.keepable(skill('pdf')), false)
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
  assert.deepEqual(s.applyInvocation('fix the bug in the parser', '/pdf', 20), { error: "That would make the message longer than 2,000 bytes. Nothing was inserted." })
  assert.deepEqual(s.applyInvocation('abc', '', 2000), { error: "This skill can't be inserted from here." })
})

test('applyInvocation counts UTF-16 units and UTF-8 bytes, like the composer', () => {
  const emoji = '😀'.repeat(6) // 6 code points, 12 UTF-16 units
  assert.deepEqual(s.applyInvocation(emoji, '/a', 10), { error: 'That would make the message longer than 2,000 bytes. Nothing was inserted.' })
  const cjk = '中'.repeat(5) // 5 UTF-16 units, 15 UTF-8 bytes
  assert.deepEqual(s.applyInvocation(cjk, '/a', 10), { error: 'That would make the message longer than 2,000 bytes. Nothing was inserted.' })
  assert.deepEqual(s.applyInvocation('a'.repeat(16), '/pd', 20), { draft: 'a'.repeat(16) + ' /pd' }, 'exactly at the limit is accepted')
  assert.ok('error' in s.applyInvocation('a'.repeat(17), '/pd', 20))
})

test('seenLine', () => {
  const now = 1_000_000_000_000
  assert.equal(s.seenLine(0, 'box', now), 'Skills on box have not been read yet.')
  assert.equal(s.seenLine(now - 3 * 60_000, 'box', now), 'Skills on box were read 3 minutes ago.')
  assert.equal(s.seenLine(now - 5 * 3_600_000, 'box', now), 'Skills on box were read 5 hours ago.')
  assert.equal(s.seenLine(now - 2 * 86_400_000, 'box', now), 'Skills on box were read 2 days ago.')
  assert.equal(s.seenLine(now - 60_000, 'box', now), 'Skills on box were read 1 minute ago.')
})

function fetchReplying(status, body) {
  const calls = []
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init, headers: new Headers(init.headers) })
    return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
  }
  return calls
}

test('addSessionSkill', async () => {
  const added = { name: 'slides', status: 'active', added: true }
  const calls = fetchReplying(200, added)
  assert.deepEqual(await api.addSessionSkill('s/1', 'reet:pub/slides'), added)
  assert.equal(calls[0].url, '/api/v2/terminal/sessions/s%2F1/skills/install')
  assert.equal(calls[0].init.method, 'POST')
  assert.deepEqual(JSON.parse(calls[0].init.body), { skill: 'reet:pub/slides' })
  assert.equal(calls[0].init.credentials, 'same-origin')

  const text = 'This skill is no longer available. Refresh the list.'
  fetchReplying(422, { error: text, code: 'missing' })
  const err = await api.addSessionSkill('s1', 'reet:gone').then(() => assert.fail('resolved'), e => e)
  assert.equal(err.code, 'missing')
  assert.equal(err.message, text)
  assert.equal(err.status, 422)
  assert.equal(api.cancelSessionSkillInstall, undefined)
})
