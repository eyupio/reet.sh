import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { compileModule } from 'svelte/compiler'

// The Skills page's reads: the poll never asks twice for a tab that has not answered, and an older answer never replaces a
// newer one. The state is compiled as the app builds it (a .svelte.ts module); its imports are stood in for.
const dataUrl = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const api = dataUrl(`export class UnauthorizedError extends Error {}\nexport const loadSkillsPage = status => globalThis.__skillsRead(status)`)
const skillsPage = dataUrl('export const healthLine = () => ({ tone: "ok" }); export const matchesSearch = () => true')
const source = ts.transpileModule(readFileSync(new URL('./skillsState.svelte.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const compiled = compileModule(source, { generate: 'client', filename: 'skillsState.svelte.js' }).js.code
  .replace("'svelte/internal/client'", JSON.stringify(import.meta.resolve('svelte/internal/client')))
  .replace("'../api'", JSON.stringify(api))
  .replace("'./skillsPage'", JSON.stringify(skillsPage))
const { SkillsPageState } = await import(dataUrl(compiled))

// Every call to the server waits until the test answers it.
function server() {
  const calls = []
  globalThis.__skillsRead = status => new Promise((resolve, reject) => calls.push({ status, resolve, reject }))
  return calls
}
const page = (...names) => ({ skills: names.map(name => ({ id: name, name, tags: [], lists: [] })), counts: { active: names.length, archived: 0 }, newOnMachines: [], machines: 1, notice: null, work: { failed: 0, installing: 0, removing: 0, updating: 0 } })
const names = state => state.held.active.map(r => r.name)

test('an older answer for a tab never replaces a newer one', async () => {
  const calls = server()
  const state = new SkillsPageState()
  const first = state.read('active', true)
  const second = state.read('active')
  calls[1].resolve(page('new'))
  assert.equal(await second, true)
  calls[0].resolve(page('old'))
  assert.equal(await first, false)
  assert.deepEqual(names(state), ['new'])
})

test('answers in order are each shown', async () => {
  const calls = server()
  const state = new SkillsPageState()
  const first = state.read('active', true)
  const second = state.read('active')
  calls[0].resolve(page('old'))
  assert.equal(await first, true)
  assert.deepEqual(names(state), ['old'])
  calls[1].resolve(page('new'))
  assert.equal(await second, true)
  assert.deepEqual(names(state), ['new'])
})

test('the other tab keeps its own order', async () => {
  const calls = server()
  const state = new SkillsPageState()
  const active = state.read('active', true)
  const archived = state.read('archived', true)
  calls[1].resolve(page('kept'))
  calls[0].resolve(page('shown'))
  assert.deepEqual([await active, await archived], [true, true])
  assert.deepEqual(names(state), ['shown'])
  assert.deepEqual(state.held.archived.map(r => r.name), ['kept'])
})

test('a tick reads both tabs, and none again while they have not answered', async () => {
  const calls = server()
  const state = new SkillsPageState()
  state.poll()
  assert.deepEqual(calls.map(c => c.status), ['active', 'archived'])
  state.poll()
  state.poll()
  assert.equal(calls.length, 2, 'a slow controller is asked once per tab, not once per tick')
  calls[0].resolve(page('a'))
  calls[1].resolve(page('b'))
  await new Promise(r => setTimeout(r))
  state.poll()
  assert.equal(calls.length, 4, 'the next tick reads again once both answered')
})

test('a tick skips only the tab that is still being read', async () => {
  const calls = server()
  const state = new SkillsPageState()
  const archived = state.read('archived', true)
  state.poll()
  assert.deepEqual(calls.map(c => c.status), ['archived', 'active'])
  calls[0].resolve(page('b'))
  await archived
})

test('a failed read frees its tab for the next tick', async () => {
  const calls = server()
  const state = new SkillsPageState()
  state.poll()
  calls[0].reject(new Error('down'))
  calls[1].reject(new Error('down'))
  await new Promise(r => setTimeout(r))
  assert.equal(state.error, '', 'a quiet read says nothing')
  state.poll()
  assert.equal(calls.length, 4)
})

test('a read the person waits for runs while a tick has one on its way', async () => {
  const calls = server()
  const state = new SkillsPageState()
  state.poll()
  const mine = state.read('active')
  assert.deepEqual(calls.map(c => c.status), ['active', 'archived', 'active'])
  calls[2].resolve(page('mine'))
  assert.equal(await mine, true)
  calls[0].resolve(page('stale'))
  await new Promise(r => setTimeout(r))
  assert.deepEqual(names(state), ['mine'])
})
