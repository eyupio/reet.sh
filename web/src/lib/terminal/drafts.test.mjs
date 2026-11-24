import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { DRAFTS_KEY, MAX_DRAFTS, MAX_DRAFT_BYTES, DRAFT_DAYS, loadKept, saveKept, clearKept, keepDraft, restoreDraft, setDraftOwner } = await import(`data:text/javascript;base64,${Buffer.from(compile('./drafts.ts')).toString('base64')}`)

const memory = () => { const data = new Map(); return { getItem: k => data.get(k) ?? null, setItem: (k, v) => void data.set(k, String(v)), removeItem: k => void data.delete(k), data } }
const broken = { getItem() { throw new Error('off') }, setItem() { throw new Error('off') }, removeItem() { throw new Error('off') } }
const T = 1_700_000_000_000

test('a draft comes back for the same account and session, and for nobody else', () => {
  const s = memory()
  saveKept(s, 7, 'a', { chat: 'fix the build' }, T)
  assert.deepEqual(loadKept(s, 7, 'a', T + 1), { chat: 'fix the build' })
  assert.equal(loadKept(s, 8, 'a', T + 1), null)
  assert.equal(loadKept(s, 7, 'b', T + 1), null)
})

test('fields join in one entry per session; emptying the last one removes the entry', () => {
  const s = memory()
  saveKept(s, 1, 'a', { chat: 'one' }, T)
  saveKept(s, 1, 'a', { compose: 'two', queue: ['x', 'y'] }, T)
  assert.deepEqual(loadKept(s, 1, 'a', T), { chat: 'one', compose: 'two', queue: ['x', 'y'] })
  assert.equal(JSON.parse(s.getItem(DRAFTS_KEY)).length, 1)
  saveKept(s, 1, 'a', { chat: '', queue: [] }, T)
  assert.deepEqual(loadKept(s, 1, 'a', T), { compose: 'two' })
  saveKept(s, 1, 'a', { compose: '' }, T)
  assert.equal(s.getItem(DRAFTS_KEY), null)
})

test('a draft older than fourteen days is gone', () => {
  const s = memory()
  saveKept(s, 1, 'a', { chat: 'old' }, T)
  assert.deepEqual(loadKept(s, 1, 'a', T + (DRAFT_DAYS * 864e5) - 1), { chat: 'old' })
  assert.equal(loadKept(s, 1, 'a', T + DRAFT_DAYS * 864e5), null)
  saveKept(s, 1, 'b', { chat: 'new' }, T + DRAFT_DAYS * 864e5)
  assert.equal(JSON.parse(s.getItem(DRAFTS_KEY)).length, 1)
})

test('at most twenty drafts are kept, the oldest dropped first', () => {
  const s = memory()
  for (let i = 0; i < MAX_DRAFTS + 3; i++) saveKept(s, 1, `s${i}`, { chat: `d${i}` }, T + i)
  assert.equal(JSON.parse(s.getItem(DRAFTS_KEY)).length, MAX_DRAFTS)
  assert.equal(loadKept(s, 1, 's0', T + 100), null)
  assert.equal(loadKept(s, 1, 's2', T + 100), null)
  assert.deepEqual(loadKept(s, 1, 's3', T + 100), { chat: 'd3' })
  assert.deepEqual(loadKept(s, 1, `s${MAX_DRAFTS + 2}`, T + 100), { chat: `d${MAX_DRAFTS + 2}` })
})

test('a draft over 32 KiB is not kept, and does not leave an older copy behind', () => {
  const s = memory()
  saveKept(s, 1, 'a', { compose: 'small' }, T)
  saveKept(s, 1, 'a', { compose: 'x'.repeat(MAX_DRAFT_BYTES) }, T + 1)
  assert.equal(loadKept(s, 1, 'a', T + 2), null)
  saveKept(s, 1, 'a', { compose: 'é'.repeat(MAX_DRAFT_BYTES / 2 - 100) }, T + 3)
  assert.equal(loadKept(s, 1, 'a', T + 4)?.compose.length, MAX_DRAFT_BYTES / 2 - 100)
})

test('clearing removes every draft of every account', () => {
  const s = memory()
  saveKept(s, 1, 'a', { chat: 'x' }, T); saveKept(s, 2, 'b', { chat: 'y' }, T)
  clearKept(s)
  assert.equal(loadKept(s, 1, 'a', T), null); assert.equal(loadKept(s, 2, 'b', T), null)
})

test('storage that is off, full or holds rubbish never throws', () => {
  assert.equal(loadKept(broken, 1, 'a', T), null)
  assert.doesNotThrow(() => { saveKept(broken, 1, 'a', { chat: 'x' }, T); clearKept(broken) })
  for (const rubbish of ['{', 'null', '{"a":1}', '[1,"x",{"u":"1","s":"a","at":1}]', '[{"u":1,"s":"a","at":' + T + ',"chat":5,"queue":[1]}]']) {
    const s = memory(); s.setItem(DRAFTS_KEY, rubbish)
    assert.equal(loadKept(s, 1, 'a', T), null)
    assert.doesNotThrow(() => saveKept(s, 1, 'a', { chat: 'ok' }, T))
    assert.deepEqual(loadKept(s, 1, 'a', T), { chat: 'ok' })
  }
})

test('without a signed-in account nothing is kept or restored', () => {
  globalThis.window = { localStorage: memory(), addEventListener() {} }
  setDraftOwner(null)
  keepDraft('a', { chat: '' })
  assert.equal(restoreDraft('a'), null)
  assert.equal(window.localStorage.getItem(DRAFTS_KEY), null)
  setDraftOwner(3)
  keepDraft('a', { chat: 'hi' })
  keepDraft('a', { chat: '' }) // emptied: written at once, nothing waits for the timer
  assert.equal(restoreDraft('a'), null)
  delete globalThis.window
})
