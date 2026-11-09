import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./machineItems.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { itemMachineState } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const item = (changes = {}) => ({ id: 'a'.repeat(32), name: 'code-review', itemType: 'skill', status: 'in_use', contentState: 'complete', ...changes })
const skill = (changes = {}) => ({ id: 'local/code-review', itemId: 'a'.repeat(32), name: 'code-review', slug: 'code-review', source: 'private:x:y:z', sourceType: 'private', ...changes })
const target = (changes = {}) => ({ target: { id: 't' }, desired: [skill()], missing: [], extra: [], ...changes })
const job = (changes = {}) => ({ id: 'j', agentId: 'a', targetId: 't', kind: 'apply', status: 'succeeded', revision: 'r', hash: 'h', plan: {}, message: '', createdAt: 1, updatedAt: 2, ...changes })
const state = (input) => itemMachineState({ selected: true, jobs: [], online: true, ...input })

test('an unselected item is not on the machine and says nothing else', () => {
  const s = state({ item: item(), selected: false, target: target() })
  assert.equal(s.key, 'unselected')
})

test('a selected item that is not In use is kept in the library only, whatever the machine reports', () => {
  for (const status of ['to_test', 'on_shelf', 'disabled']) {
    const s = state({ item: item({ status }), target: target() })
    assert.equal(s.key, 'inactive')
    assert.match(s.detail, new RegExp(status.replaceAll('_', ' ')))
    assert.match(s.detail, /removed|not installed/i)
  }
})

test('content that is still being captured is waiting for content, not an error', () => {
  assert.equal(state({ item: item({ contentState: 'pending_capture' }), target: target() }).key, 'waiting_content')
  assert.equal(state({ item: item({ problem: 'capture unavailable' }), target: target() }).key, 'waiting_content')
})

test('a saved selection before the controller has planned it is waiting, and offline says so', () => {
  assert.equal(state({ item: item(), target: undefined, online: true }).key, 'waiting')
  const offline = state({ item: item(), target: undefined, online: false })
  assert.equal(offline.key, 'waiting_offline')
  assert.match(offline.label, /offline/i)
})

test('desired but missing on the machine is waiting, offline or online', () => {
  const t = target({ missing: [skill()] })
  assert.equal(state({ item: item(), target: t, online: true }).key, 'waiting')
  assert.equal(state({ item: item(), target: t, online: false }).key, 'waiting_offline')
})

test('a queued or running job for the machine shows syncing', () => {
  for (const status of ['queued', 'running', 'awaiting-approval']) {
    assert.equal(state({ item: item(), target: target({ missing: [skill()] }), jobs: [job({ status })] }).key, 'syncing')
  }
})

test('a problem from the latest finished plan is shown beside its own item only', () => {
  const failing = job({ status: 'succeeded', message: 'Some selected items are waiting; healthy items continue syncing.', plan: { problems: { 'code-review': 'The local entry is not a managed directory.' } } })
  const mine = state({ item: item(), target: target({ missing: [skill()] }), jobs: [failing] })
  assert.equal(mine.key, 'attention')
  assert.match(mine.detail, /not a managed directory/)
  const other = state({ item: item({ id: 'b'.repeat(32), name: 'other' }), target: target({ desired: [skill({ itemId: 'b'.repeat(32), slug: 'other', name: 'other' })] }), jobs: [failing] })
  assert.equal(other.key, 'verified', 'one item’s problem leaked onto a healthy item')
})

test('only the newest finished job counts: an old problem is history once a later run fixed it', () => {
  const old = job({ id: 'old', updatedAt: 1, plan: { problems: { 'code-review': 'old problem' } } })
  const fixed = job({ id: 'new', updatedAt: 5, plan: {} })
  assert.equal(state({ item: item(), target: target(), jobs: [old, fixed] }).key, 'verified')
})

test('a failed apply that names the item is attention with the controller’s message', () => {
  const failed = job({ status: 'failed', message: 'sync stopped at code-review: installer failed', plan: { actions: [{ add: true, skill: skill() }] } })
  const s = state({ item: item(), target: target({ missing: [skill()] }), jobs: [failed] })
  assert.equal(s.key, 'attention')
  assert.match(s.detail, /installer failed/)
})

test('present in the machine report is installed, and the wording never claims byte verification', () => {
  const s = state({ item: item(), target: target() })
  assert.equal(s.key, 'verified')
  assert.doesNotMatch(`${s.label} ${s.detail ?? ''}`, /up to date|byte|digest/i)
  assert.match(s.detail, /last report/i)
})

test('an item selected but absent from the controller’s desired set is waiting', () => {
  assert.equal(state({ item: item(), target: target({ desired: [] }) }).key, 'waiting')
})
