import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./skills.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const s = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)
const asURL = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const api = await import(asURL(ts.transpileModule(readFileSync(new URL('./api.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText.replace("from '../projects'", `from '${asURL('export {}')}'`)))

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

test('installStep maps kind and status', () => {
  const step = (kind, status) => s.installStep({ kind, status })
  assert.equal(step('preview', 'queued'), 'waiting')
  assert.equal(step('preview', 'running'), 'waiting')
  assert.equal(step('preview', 'awaiting-approval'), 'review')
  assert.equal(step('apply', 'queued'), 'installing')
  assert.equal(step('apply', 'running'), 'installing')
  assert.equal(step('apply', 'succeeded'), 'done')
  assert.equal(step('preview', 'failed'), 'failed')
  assert.equal(step('apply', 'failed'), 'failed')
  assert.equal(step('preview', 'cancelled'), 'cancelled')
  assert.equal(step('apply', 'cancelled'), 'cancelled')
  assert.equal(s.installStep(undefined), 'waiting')
})

test('planVerdict approves only plain adds of the slug', () => {
  const act = (add, slug, replace) => ({ add, ...(replace ? { replace } : {}), skill: { slug } })
  assert.deepEqual(s.planVerdict({ actions: [act(true, 'pdf')] }, 'pdf'), { ok: true, lines: ['Add pdf'] })
  const other = 'This install would change other skills. Review it on the Sync page.'
  assert.deepEqual(s.planVerdict({ actions: [act(true, 'xlsx')] }, 'pdf'), { ok: false, reason: other, lines: ['Add xlsx'] })
  assert.deepEqual(s.planVerdict({ actions: [act(true, 'pdf', true)] }, 'pdf'), { ok: false, reason: other, lines: ['Replace pdf'] })
  assert.deepEqual(s.planVerdict({ actions: [act(false, 'pdf')] }, 'pdf'), { ok: false, reason: other, lines: ['Remove pdf'] })
  assert.deepEqual(s.planVerdict({ actions: [act(true, 'pdf'), act(false, 'docx')] }, 'pdf'), { ok: false, reason: other, lines: ['Add pdf', 'Remove docx'] })
  const none = 'The machine has not sent its plan yet.'
  assert.deepEqual(s.planVerdict({ actions: [] }, 'pdf'), { ok: false, reason: none, lines: [] })
  assert.deepEqual(s.planVerdict({}, 'pdf'), { ok: false, reason: none, lines: [] })
  assert.deepEqual(s.planVerdict(undefined, 'pdf'), { ok: false, reason: none, lines: [] })
})

test('planVerdict never approves or throws on an action without a skill', () => {
  const other = 'This install would change other skills. Review it on the Sync page.'
  for (const action of [{ add: true }, { add: true, skill: null }, { add: true, skill: {} }]) {
    assert.deepEqual(s.planVerdict({ actions: [action] }, 'pdf'), { ok: false, reason: other, lines: ['Add'] })
  }
  assert.equal(s.planVerdict({ actions: [{ add: true, skill: { slug: 'pdf' } }, { add: true }] }, 'pdf').ok, false)
})

test('installStatusLine', () => {
  assert.equal(s.installStatusLine('waiting', 'box', 'pdf', 0), 'Waiting for box…')
  assert.equal(s.installStatusLine('waiting', 'box', 'pdf', 59), 'Waiting for box…')
  assert.equal(s.installStatusLine('waiting', 'box', 'pdf', 60), 'Waiting for box… Is reet running on box?')
  assert.equal(s.installStatusLine('review', 'box', 'pdf', 90), 'Review what will be added.')
  assert.equal(s.installStatusLine('installing', 'box', 'pdf', 90), 'Installing pdf on box…')
  assert.equal(s.installStatusLine('done', 'box', 'pdf', 0), 'pdf is installed on box.')
  assert.equal(s.installStatusLine('failed', 'box', 'pdf', 0), 'The install did not finish.')
  assert.equal(s.installStatusLine('cancelled', 'box', 'pdf', 0), 'Install cancelled.')
})

function fetchReplying(status, body) {
  const calls = []
  globalThis.fetch = async (url, init) => {
    calls.push({ url, init, headers: new Headers(init.headers) })
    return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
  }
  return calls
}

test('installSessionSkill and cancelSessionSkillInstall', async () => {
  const started = { job: 'j1', reused: false, directory: '/home/u/.claude/skills', host: 'box' }
  let calls = fetchReplying(200, started)
  assert.deepEqual(await api.installSessionSkill('s/1', 'sk 9'), started)
  assert.equal(calls[0].url, '/api/v2/terminal/sessions/s%2F1/skills/install')
  assert.equal(calls[0].init.method, 'POST')
  assert.deepEqual(JSON.parse(calls[0].init.body), { skill: 'sk 9' })
  assert.equal(calls[0].init.credentials, 'same-origin')

  const text = 'Another install is already running for this session.'
  fetchReplying(409, { error: text, code: 'install-in-progress' })
  const err = await api.installSessionSkill('s1', 'k').then(() => assert.fail('resolved'), e => e)
  assert.equal(err.code, 'install-in-progress')
  assert.equal(err.message, text)
  assert.equal(err.status, 409)

  calls = fetchReplying(200, { reverted: true })
  assert.deepEqual(await api.cancelSessionSkillInstall('s/1', 'j1'), { reverted: true })
  assert.equal(calls[0].url, '/api/v2/terminal/sessions/s%2F1/skills/install/cancel')
  assert.equal(calls[0].init.method, 'POST')
  assert.deepEqual(JSON.parse(calls[0].init.body), { job: 'j1' })
})
