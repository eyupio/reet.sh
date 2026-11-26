import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compiled = ts.transpileModule(readFileSync(new URL('./finished.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const f = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)

const memory = (seed = {}) => {
  const data = new Map(Object.entries(seed))
  return { getItem: key => data.get(key) ?? null, setItem: (key, value) => data.set(key, value), data }
}
const outcome = (id, pieces, extra = {}) => ({
  id, objective: `Objective ${id}`, ...extra,
  sessions: pieces.map(([key, state, more = {}]) => ({ key, title: `Title ${key}`, state, sessionId: `s-${key}`, updatedAt: 100, ...more })),
})
const done = (...keys) => outcome('o1', keys.map(key => [key, 'done']))

test('the first read observes without toasting and seeds a new key', () => {
  const storage = memory()
  const watch = f.createFinishedWatch(storage, 'k')
  const first = watch.observe([outcome('o1', [['a', 'following']])])
  assert.deepEqual(first.finished, [])
  assert.equal(first.unread, 0)
  const nowDone = watch.observe([done('a')])
  assert.deepEqual(nowDone.finished.map(item => item.id), ['o1.a'])
  assert.equal(nowDone.unread, 1)
  assert.equal(storage.data.get('k'), '[]')
})

test('a completion fires once, then the same snapshot is idempotent', () => {
  const watch = f.createFinishedWatch(memory(), 'k')
  watch.observe([outcome('o1', [['a', 'following']])])
  assert.equal(watch.observe([done('a')]).finished.length, 1)
  assert.equal(watch.observe([done('a')]).finished.length, 0)
  assert.equal(watch.observe([done('a')]).finished.length, 0)
})

test('acknowledging clears the dot, and a reload does not toast again', () => {
  const storage = memory()
  const watch = f.createFinishedWatch(storage, 'k')
  watch.observe([outcome('o1', [['a', 'following']])])
  assert.equal(watch.observe([done('a')]).unread, 1)
  watch.acknowledgeAll([done('a')])
  assert.equal(watch.observe([done('a')]).unread, 0)

  const reloaded = f.createFinishedWatch(storage, 'k')
  const after = reloaded.observe([done('a')])
  assert.deepEqual(after.finished, [])
  assert.equal(after.unread, 0)
})

test('a finish left unacknowledged stays unread across a reload but does not toast', () => {
  const storage = memory()
  const watch = f.createFinishedWatch(storage, 'k')
  watch.observe([outcome('o1', [['a', 'following']])])
  watch.observe([done('a')])
  const reloaded = f.createFinishedWatch(storage, 'k')
  const after = reloaded.observe([done('a')])
  assert.deepEqual(after.finished, [])
  assert.equal(after.unread, 1)
})

test('an item that leaves done clears its acknowledgement and notifies on a later finish', () => {
  const storage = memory()
  const watch = f.createFinishedWatch(storage, 'k')
  watch.observe([outcome('o1', [['a', 'following']])])
  watch.observe([done('a')])
  watch.acknowledgeAll([done('a')])
  const back = watch.observe([outcome('o1', [['a', 'following']])])
  assert.equal(back.unread, 0)
  assert.deepEqual(JSON.parse(storage.data.get('k')), [])
  const again = watch.observe([done('a')])
  assert.deepEqual(again.finished.map(item => item.id), ['o1.a'])
  assert.equal(again.unread, 1)
})

test('failed, cancelled, unfollowed, ready-to-merge and archived pipelines are not finished', () => {
  const record = outcome('o1', [['a', 'failed'], ['b', 'cancelled'], ['c', 'unfollowed'], ['d', 'ready_to_merge'], ['e', 'done']])
  assert.deepEqual(f.finishedItems([record]).map(item => item.id), ['o1.e'])
  const archived = outcome('o2', [['f', 'done']], { archivedAt: 1 })
  assert.deepEqual(f.finishedItems([archived]), [])
})

test('the toast names one pipeline and opens it, or counts several and opens the Factory', () => {
  const one = f.finishedToast([{ id: 'o1.a/b c', title: 'Ship it' }])
  assert.equal(one.message, '“Ship it” finished.')
  assert.equal(one.href, '/factory?pipeline=o1.a%2Fb%20c')
  const several = f.finishedToast([{ id: 'o1.a', title: 'A' }, { id: 'o2.b', title: 'B' }])
  assert.equal(several.message, '2 pipelines finished.')
  assert.equal(several.href, '/factory')
})

test('corrupt storage is ignored and reseeded', () => {
  const storage = memory({ k: '{not json' })
  const watch = f.createFinishedWatch(storage, 'k')
  const first = watch.observe([done('a')])
  assert.deepEqual(first.finished, [])
  assert.equal(first.unread, 0)
  assert.deepEqual(JSON.parse(storage.data.get('k')), ['o1.a'])
})

test('a storage that refuses falls back to this visit only', () => {
  const storage = { getItem() { throw new Error('no') }, setItem() { throw new Error('no') } }
  const watch = f.createFinishedWatch(storage, 'k')
  watch.observe([outcome('o1', [['a', 'following']])])
  assert.equal(watch.observe([done('a')]).finished.length, 1)
})
