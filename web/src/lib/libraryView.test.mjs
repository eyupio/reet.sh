import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./libraryView.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { STATUSES, statusLabel, filterLibrary, statusCounts, itemNotes } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const item = (name, changes = {}) => ({ id: name.padEnd(32, '0'), name, itemType: 'skill', status: 'active', contentState: 'complete', version: 1, ...changes })
const library = [item('alpha'), item('beta', { status: 'archived' }), item('gamma', { status: 'archived' }), item('delta', { status: 'archived' }), item('Alpine', { status: 'active' })]

test('the two statuses keep the product’s plain words', () => {
  assert.deepEqual(STATUSES.map(s => s.label), ['Active', 'Archived'])
  assert.equal(statusLabel('archived'), 'Archived')
  assert.equal(statusLabel('something_new'), 'something new', 'an unknown status is shown readably, never dropped')
})

test('search is case-insensitive over the name and combines with the status filter', () => {
  assert.deepEqual(filterLibrary(library, 'AL', 'all').map(i => i.name), ['alpha', 'Alpine'])
  assert.deepEqual(filterLibrary(library, 'al', 'active').map(i => i.name), ['alpha', 'Alpine'])
  assert.deepEqual(filterLibrary(library, 'al', 'archived').map(i => i.name), [])
  assert.deepEqual(filterLibrary(library, '  ', 'archived').map(i => i.name), ['beta', 'gamma', 'delta'])
})

test('archived items stay findable: every status can be filtered to and nothing is hidden by All', () => {
  assert.equal(filterLibrary(library, '', 'all').length, library.length)
  for (const status of ['active', 'archived']) assert.ok(filterLibrary(library, '', status).length > 0, status)
})

test('counts per status always add up to the whole library, search ignored', () => {
  const counts = statusCounts(library)
  assert.deepEqual(counts, { all: 5, active: 2, archived: 3 })
})

test('notes come from the shelf metadata that migration preserves and say nothing when there is none', () => {
  assert.deepEqual(itemNotes(item('plain')), [])
  const rich = item('rich', { metadata: { liked: true, lists: [{ id: 1, name: 'Daily' }, { id: 2, name: 'Work' }], tags: [{ id: 1, name: 'review' }] }, aliases: [{ kind: 'hosted', reference: 'rich' }], published: false })
  assert.deepEqual(itemNotes(rich), ['Written here · private', 'Liked', 'On list Daily, Work', 'Tagged review'])
  assert.deepEqual(itemNotes(item('public', { aliases: [{ kind: 'hosted', reference: 'public' }], published: true })), ['Written here · public'])
})
