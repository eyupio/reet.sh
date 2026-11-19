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
  await api.retrySkill('x')
  assert.equal(calls[0].url, '/api/v2/skills/retry')
  assert.deepEqual(calls[0].body, { items: ['x'] })
})

test('the old Remove dialogs say what Remove now does to machines', () => {
  const app = readFileSync(new URL('../App.svelte', import.meta.url), 'utf8')
  assert.ok(!app.includes('Installed files and sync plans stay as they are'), 'Remove no longer leaves installed files')
  assert.ok(app.includes('Removing deletes it from your library and takes it off every machine. Each machine keeps a backup for 30 days.'))
  assert.ok(app.includes('They will be deleted from your library and taken off every machine. Each machine keeps a backup for 30 days.'))
})
