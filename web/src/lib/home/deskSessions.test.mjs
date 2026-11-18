import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = path => ts.transpileModule(readFileSync(new URL(path, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const dataUrl = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const { deskHref, deskOf, offDesks, deskPaneReader } = await import(dataUrl(compile('./deskSessions.ts')))

const desk = (id, sessions, over = {}) => ({ id, name: `Desk ${id}`, template: false, version: 1, sessions, ...over })
const doc = (id, panes, version = 1) => ({ id, version, doc: { tabs: [{ panes: Object.fromEntries(Object.entries(panes).map(([pane, session]) => [pane, { id: pane, session }])) }] } })

test('a desk link names the pane when it is known, else only the desk', () => {
  assert.equal(deskHref('d 1', 'p/1'), '/desks/d%201#p%2F1')
  assert.equal(deskHref('d1'), '/desks/d1')
})

test('a session on a desk is marked with the desk and a link to it; a template desk holds nothing', () => {
  const desks = [desk('t', ['s1'], { template: true }), desk('d1', ['s1', 's2'])]
  assert.deepEqual(deskOf(desks, 's1'), { id: 'd1', name: 'Desk d1', href: '/desks/d1' })
  assert.deepEqual(deskOf(desks, 's2', new Map([['s2', 'pane-2']])), { id: 'd1', name: 'Desk d1', href: '/desks/d1#pane-2' })
  assert.equal(deskOf(desks, 's3'), undefined)
  assert.equal(deskOf([desk('t', ['s9'], { template: true })], 's9'), undefined)
})

test('desk sessions are split out, keeping a pipeline’s session and every session when no desks were read', () => {
  const sessions = [{ id: 's1' }, { id: 's2' }, { id: 's3' }, { id: 'p1' }]
  const desks = [desk('d1', ['s1', 'p1']), desk('t', ['s2'], { template: true })]
  assert.deepEqual(offDesks(sessions, desks, new Set(['p1'])).map(s => s.id), ['s2', 's3', 'p1'])
  assert.deepEqual(offDesks(sessions, undefined).map(s => s.id), ['s1', 's2', 's3', 'p1'])
  assert.deepEqual(offDesks(sessions, []).map(s => s.id), ['s1', 's2', 's3', 'p1'])
})

test('the pane reader reads only the desks that hold a wanted session, once per version, and keeps going when one fails', async () => {
  const reads = []
  let fail = new Set(['d2'])
  const docs = { d1: doc('d1', { a: 's1', b: 's2' }), d2: doc('d2', { c: 's3' }), d3: doc('d3', { e: 's4' }) }
  const reader = deskPaneReader(async id => { reads.push(id); if (fail.has(id)) throw new Error('down'); return docs[id] })
  const desks = [desk('d1', ['s1', 's2']), desk('d2', ['s3']), desk('d3', ['s4']), desk('t', ['s1'], { template: true })]
  let panes = await reader(desks, new Set(['s1', 's3']))
  assert.deepEqual([...panes], [['s1', 'a']])
  assert.deepEqual(reads, ['d1', 'd2'])
  fail = new Set()
  panes = await reader(desks, new Set(['s1', 's3']))
  assert.deepEqual([...panes].sort(), [['s1', 'a'], ['s3', 'c']])
  assert.deepEqual(reads, ['d1', 'd2', 'd2'])
  // A newer version is read again.
  docs.d1 = doc('d1', { z: 's1' }, 2)
  panes = await reader([desk('d1', ['s1'], { version: 2 })], new Set(['s1']))
  assert.deepEqual([...panes], [['s1', 'z']])
  assert.deepEqual(reads, ['d1', 'd2', 'd2', 'd1'])
  // Nothing wanted reads nothing.
  await reader(desks, new Set())
  assert.equal(reads.length, 4)
})

test('a pane read is bounded: the reader hands its signal to each read, and a read that is cut off links nothing', async () => {
  const seen = []
  const reader = deskPaneReader((id, signal) => new Promise((_, reject) => { seen.push(signal); signal.addEventListener('abort', () => reject(new Error('cut off'))) }))
  const cut = new AbortController()
  const pending = reader([desk('d1', ['s1'])], new Set(['s1']), cut.signal)
  cut.abort()
  assert.deepEqual([...await pending], [])
  assert.equal(seen[0], cut.signal)
})
