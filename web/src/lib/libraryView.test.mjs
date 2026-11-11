import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./libraryView.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { STATUSES, statusLabel, filterLibrary, statusCounts, machinesByItem, itemNotes, receiptLine, differenceFileLine, differenceSummary } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const item = (name, changes = {}) => ({ id: name.padEnd(32, '0'), name, itemType: 'skill', status: 'in_use', contentState: 'complete', version: 1, ...changes })
const library = [item('alpha'), item('beta', { status: 'to_test' }), item('gamma', { status: 'disabled' }), item('delta', { status: 'on_shelf' }), item('Alpine', { status: 'in_use' })]

test('the four statuses keep the product’s plain words', () => {
  assert.deepEqual(STATUSES.map(s => s.label), ['In use', 'To test', 'Parked', 'Disabled'])
  assert.equal(statusLabel('on_shelf'), 'Parked')
  assert.equal(statusLabel('something_new'), 'something new', 'an unknown status is shown readably, never dropped')
})

test('search is case-insensitive over the name and combines with the status filter', () => {
  assert.deepEqual(filterLibrary(library, 'AL', 'all').map(i => i.name), ['alpha', 'Alpine'])
  assert.deepEqual(filterLibrary(library, 'al', 'in_use').map(i => i.name), ['alpha', 'Alpine'])
  assert.deepEqual(filterLibrary(library, 'al', 'disabled').map(i => i.name), [])
  assert.deepEqual(filterLibrary(library, '  ', 'to_test').map(i => i.name), ['beta'])
})

test('inactive items stay findable: every status can be filtered to and nothing is hidden by All', () => {
  assert.equal(filterLibrary(library, '', 'all').length, library.length)
  for (const status of ['in_use', 'to_test', 'on_shelf', 'disabled']) assert.ok(filterLibrary(library, '', status).length > 0, status)
})

test('counts per status always add up to the whole library, search ignored', () => {
  const counts = statusCounts(library)
  assert.deepEqual(counts, { all: 5, in_use: 2, to_test: 1, on_shelf: 1, disabled: 1 })
})

test('machines per item come from saved selections, one entry per machine, sorted', () => {
  const map = machinesByItem([
    { host: 'laptop', itemIds: [library[0].id, library[1].id] },
    { host: 'desktop', itemIds: [library[0].id] },
    { host: 'laptop', itemIds: [library[0].id] }, // a second folder on the same machine is still one machine
  ])
  assert.deepEqual(map.get(library[0].id), ['desktop', 'laptop'])
  assert.deepEqual(map.get(library[1].id), ['laptop'])
  assert.equal(map.get(library[2].id), undefined)
})

test('notes come from the shelf metadata that migration preserves and say nothing when there is none', () => {
  assert.deepEqual(itemNotes(item('plain')), [])
  const rich = item('rich', { metadata: { liked: true, lists: [{ id: 1, name: 'Daily' }, { id: 2, name: 'Work' }], tags: [{ id: 1, name: 'review' }] }, aliases: [{ kind: 'hosted', reference: 'rich' }], published: false })
  assert.deepEqual(itemNotes(rich), ['Written here · private', 'Liked', 'On list Daily, Work', 'Tagged review'])
  assert.deepEqual(itemNotes(item('public', { aliases: [{ kind: 'hosted', reference: 'public' }], published: true })), ['Written here · public'])
})

test('a history line says when it was saved and whether it became the current version', () => {
  const when = Date.UTC(2026, 10, 9, 10, 12)
  assert.match(receiptLine({ eventId: 'a', revisionId: 'r', won: true, base: 'b', savedAt: when, orderAt: when, uncertainty: 0, clockFallback: false }, 'UTC'), /9 Nov 2026.*10:12.*current version/)
  const other = receiptLine({ eventId: 'a', revisionId: 'r', won: false, base: 'b', savedAt: when, orderAt: when, uncertainty: 0, clockFallback: false }, 'UTC')
  assert.match(other, /kept as another version/)
  assert.match(receiptLine({ eventId: 'a', revisionId: 'r', won: true, base: 'b', savedAt: when, orderAt: when, uncertainty: 0, clockFallback: true }, 'UTC'), /time estimated/)
})

test('a differing file says where it exists and how big each copy is, never what changed inside', () => {
  assert.equal(differenceFileLine({ path: 'SKILL.md', change: 'changed', reetBytes: 1100, machineBytes: 1300 }, 'laptop'), 'Changed (1.1 KB in reet, 1.3 KB on laptop)')
  assert.equal(differenceFileLine({ path: 'a', change: 'changed', reetBytes: 12, machineBytes: 12 }, 'laptop'), 'Changed (same size, 12 B)')
  assert.equal(differenceFileLine({ path: 'run.sh', change: 'machine', machineBytes: 40 }, 'laptop'), 'Only on laptop (40 B)')
  assert.equal(differenceFileLine({ path: 'old.md', change: 'reet', reetBytes: 2 * 1024 * 1024 }, 'laptop'), 'Only in reet’s copy (2.0 MB)')
  assert.equal(differenceSummary([{ change: 'changed' }, { change: 'machine' }, { change: 'machine' }], 'laptop'), '3 files differ: 1 changed, 2 only on laptop.')
  assert.equal(differenceSummary([{ change: 'reet' }], 'laptop'), '1 file differs: 1 only in reet’s copy.')
  assert.match(differenceSummary([], 'laptop'), /now match reet’s copy/)
})
