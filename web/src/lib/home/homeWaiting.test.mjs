import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = path => ts.transpileModule(readFileSync(new URL(path, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const dataUrl = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
// The list model imports the outcome model, and the merged list imports the list model: both are pointed at compiled copies.
const outcomeUrl = dataUrl(compile('../gafferOutcome.ts'))
const listUrl = dataUrl(compile('../factory/factoryList.ts').replace(/(['"])\.\.\/gafferOutcome\1/, JSON.stringify(outcomeUrl)))
const w = await import(dataUrl(compile('./homeWaiting.ts').replace(/(['"])\.\.\/factory\/factoryList\1/, JSON.stringify(listUrl))))

const HOUR = 3_600_000
const item = (id, kind, section, since, extra = {}) => ({ id, sessionId: '', outcomeId: 'o', key: id, title: `T ${id}`, objective: '', projectId: 'p', index: 1, of: 1, section, kind, chip: kind, why: '', since, nodes: [], at: -1, archived: false, ...extra })
const card = (sessionId, kind, at, extra = {}) => ({ kind, reasons: [], label: '', tone: 'warn', sessionId, agentId: 'a', title: sessionId, where: '', href: '', at, age: '', markSeen: false, worktreeTarget: null, ...extra })

test('a finished session is not waiting, a failed one and one to review are', () => {
  const out = w.homeWaiting([], [card('a', 'finished', 1), card('b', 'failed', 2), card('c', 'review', 3)])
  assert.deepEqual(out.map(e => e.id), ['session:b', 'session:c'])
  assert.equal(out[0].source, 'session')
  assert.equal(out[0].since, 2)
})

test('a pipeline’s own session appears once, as the Factory item', () => {
  const items = [item('o.a', 'decision', 'waiting', 100, { sessionId: 's1' }), item('o.b', 'running', 'working', 50, { sessionId: 's2' }), item('o.c', 'done', 'finished', 50, { sessionId: 's3' })]
  const out = w.homeWaiting(items, [card('s1', 'waiting', 90), card('s2', 'failed', 90), card('s3', 'review', 90), card('s4', 'review', 90)])
  assert.deepEqual(out.map(e => e.id), ['session:s4', 'o.a'])
})

test('entries come oldest first across both sources', () => {
  const out = w.homeWaiting([item('o.a', 'decision', 'waiting', 100)], [card('x', 'failed', 300), card('y', 'failed', 200)])
  assert.deepEqual(out.map(e => e.since), [100, 200, 300])
  assert.deepEqual(out.map(e => e.source), ['factory', 'session', 'session'])
  const tied = w.homeWaiting([], [card('b', 'failed', 5), card('a', 'failed', 5)])
  assert.deepEqual(tied.map(e => e.id), ['session:a', 'session:b'])
})

test('an archived pipeline is not waiting', () => {
  assert.deepEqual(w.homeWaiting([item('o.a', 'decision', 'waiting', 1, { archived: true })], []), [])
})

test('the summary counts each kind and names the oldest', () => {
  const items = [
    item('o.a', 'decision', 'waiting', 0), item('o.b', 'approval', 'waiting', 10), item('o.c', 'merge', 'waiting', 20),
  ]
  const cards = [
    card('s1', 'waiting', 30, { reasons: ['approval'], approval: { permissionId: 'p', title: 't', command: '', path: '', truncated: false, queued: 0 } }),
    card('s2', 'waiting', 40, { reasons: ['question'], question: { questionId: 'q', message: 'm', truncated: false, queued: 0 } }),
    card('s3', 'failed', 50), card('s4', 'review', 60),
  ]
  // The second approval is a Factory approval, so two in all.
  const now = HOUR
  assert.equal(w.waitingSummary(w.homeWaiting(items, cards), now), '1 decision, 2 approvals, 1 merge, 1 question, 1 failed, 1 to review · oldest 1 h')
  assert.equal(w.waitingSummary([], now), '')
  const stopped = w.homeWaiting([item('o.s', 'stopped', 'waiting', 0), item('o.f', 'failed', 'waiting', 0)], [])
  assert.equal(w.waitingSummary(stopped, 5 * 60000), '1 stopped, 1 failed · oldest 5 min')
})

test('the working summary names the pipelines', () => {
  const items = ['a', 'b', 'c', 'd', 'e'].map(k => item(`o.${k}`, 'running', 'working', 1))
  assert.equal(w.workingSummary(items), 'T o.a, T o.b, T o.c and 2 more')
  assert.equal(w.workingSummary([item('o.z', 'decision', 'waiting', 1)]), '')
})
