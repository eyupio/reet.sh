import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

// The Skills page's client functions: what each sends, and what a refusal carries back.
const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const dataUrl = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const stub = dataUrl('export const inflight = { begin() { return () => {} } }')
const api = await import(dataUrl(compile('./api.ts').replace(/(['"])\.\/inflight\1/, JSON.stringify(stub))))
globalThis.localStorage = { getItem: () => null, setItem() {}, removeItem() {} }

function replying(status, body) {
  const calls = []
  globalThis.fetch = async (url, init) => {
    calls.push({ url: String(url), init, body: init?.body ? JSON.parse(init.body) : undefined })
    return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
  }
  return calls
}

test('deleteSkills sends both confirmations', async () => {
  let calls = replying(200, { deleted: 1 })
  await api.deleteSkills(['a'])
  assert.deepEqual(calls[0].body, { ids: ['a'], noCopyConfirmed: false, writtenConfirmed: false })
  calls = replying(200, { deleted: 1 })
  await api.deleteSkills(['a'], { noCopyConfirmed: true, writtenConfirmed: true })
  assert.equal(calls[0].url, '/api/v2/skills/delete')
  assert.deepEqual(calls[0].body, { ids: ['a'], noCopyConfirmed: true, writtenConfirmed: true })
})

test('a refused delete keeps the code and the names it was refused for', async () => {
  replying(409, { code: 'written', names: ['reet-babysit'], error: 'You wrote reet-babysit.' })
  await assert.rejects(api.deleteSkills(['a']), e => e.status === 409 && e.code === 'written' && e.message === 'You wrote reet-babysit.' && JSON.stringify(e.names) === '["reet-babysit"]')
  replying(409, { code: 'no-copy', names: ['cq', 'seo'], error: 'reet has no copy of 2 of these skills.' })
  await assert.rejects(api.deleteSkills(['a', 'b']), e => e.code === 'no-copy' && JSON.stringify(e.names) === '["cq","seo"]')
})

test('retrySkill sends the items body', async () => {
  const calls = replying(200, { retried: 1 })
  assert.equal(await api.retrySkill('x'), 1)
  assert.equal(calls[0].url, '/api/v2/skills/retry')
  assert.deepEqual(calls[0].body, { items: ['x'] })
})

// The delete dialog and the flags it sends are tested where they act: skills/skillsPage.test.mjs words deleteConfirm,
// and tests/skills-prune.e2e.mjs drives Delete through the page and checks what it posts.

test('setSkillStatus names the tab the person is on, when given', async () => {
  let calls = replying(200, { skills: [], counts: { active: 0, archived: 0 }, newOnMachines: [], machines: 0, notice: null })
  await api.setSkillStatus(['a'], 'archived', 'active')
  assert.equal(calls[0].url, '/api/v2/skills/status')
  assert.deepEqual(calls[0].body, { ids: ['a'], status: 'archived', tab: 'active' })
  calls = replying(200, { skills: [], counts: { active: 0, archived: 0 }, newOnMachines: [], machines: 0, notice: null })
  await api.setSkillStatus(['a'], 'active')
  assert.deepEqual(calls[0].body, { ids: ['a'], status: 'active' })
})

// replies stubs fetch with one answer per request, in order.
function replies(...answers) {
  const calls = []
  globalThis.fetch = async (url, init) => {
    calls.push({ url: String(url), init, body: init?.body ? JSON.parse(init.body) : undefined })
    const [status, body] = answers[Math.min(calls.length - 1, answers.length - 1)]
    return new Response(status === 204 ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
  }
  return calls
}
const noCopy = [409, { code: 'no-copy', names: ['cq'], error: 'reet has no copy of cq, so it was not removed.' }]

test('the old Remove can confirm a removal reet has no copy of', async () => {
  let calls = replies([204])
  await api.removeSkill('local/cq', true, true)
  assert.equal(calls[0].url, '/api/skill?id=local%2Fcq&ignore=1&noCopyConfirmed=1')
  calls = replies([204])
  await api.removeSkill('local/cq')
  assert.equal(calls[0].url, '/api/skill?id=local%2Fcq')
  calls = replies([200, { removed: 2 }])
  await api.removeSkills(['a', 'b'], false, true)
  assert.deepEqual(calls[0].body, { ids: ['a', 'b'], ignore: false, noCopyConfirmed: true })
})

test('a no-copy refusal asks once and, confirmed, removes it confirmed', async () => {
  let asked = []
  let calls = replies(noCopy, [204])
  const removed = await api.removeConfirmingNoCopy(c => api.removeSkill('local/cq', false, c), async e => { asked.push(e.names); return true })
  assert.equal(removed, true)
  assert.deepEqual(asked, [['cq']])
  assert.deepEqual(calls.map(c => c.url), ['/api/skill?id=local%2Fcq', '/api/skill?id=local%2Fcq&noCopyConfirmed=1'])

  asked = []
  calls = replies(noCopy, [204])
  assert.equal(await api.removeConfirmingNoCopy(c => api.removeSkill('local/cq', false, c), async e => { asked.push(e.names); return false }), false)
  assert.equal(calls.length, 1, 'declined: nothing more is sent')

  calls = replies([409, { code: 'written', names: ['mine'], error: 'You wrote mine. Delete it from Write a skill.' }])
  await assert.rejects(api.removeConfirmingNoCopy(c => api.removeSkill('hosted/x/mine', false, c), async () => true),
    e => e.code === 'written' && e.message === 'You wrote mine. Delete it from Write a skill.')
  assert.equal(calls.length, 1, 'written is shown as it is, never retried')
})

// The page asking again after a refused Delete, single and bulk, once a flag, is driven through the page in
// tests/skills-prune.e2e.mjs (its delete sections check each post's flags).
