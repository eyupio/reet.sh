import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./panelState.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const p = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

const waiting = extra => ({ state: 'running', attention: 'waiting', changedSinceSeen: true, ...extra })

test('a waiting, unseen session needs the person; seen or working ones do not', () => {
  assert.equal(p.needsInput(waiting({ pendingPermission: { permissionId: 'a' } })), 'approval')
  assert.equal(p.needsInput(waiting({ pendingQuestion: { questionId: 'q' } })), 'question')
  assert.equal(p.needsInput(waiting({})), 'approval', 'a terminal bell has no typed card')
  assert.equal(p.needsInput(waiting({ changedSinceSeen: false })), null)
  assert.equal(p.needsInput({ state: 'running', attention: 'working', changedSinceSeen: true }), null)
  assert.equal(p.needsInput({ state: 'running' }), null)
})

test('an approval is named before a question when both wait, as the bell does', () => {
  assert.equal(p.needsInput(waiting({ pendingPermission: { permissionId: 'a' }, pendingQuestion: { questionId: 'q' } })), 'approval')
})

test('the status line says what is awaited, else the state', () => {
  assert.equal(p.statusLine(waiting({ pendingQuestion: { questionId: 'q' } })), 'Needs you · question')
  assert.equal(p.statusLine({ state: 'end_requested' }), 'end requested')
  assert.equal(p.statusLine({ state: 'running' }), 'running')
})

test('needCount counts only sessions that need the person', () => {
  assert.equal(p.needCount([waiting({}), { state: 'running' }, waiting({ changedSinceSeen: false }), waiting({ pendingQuestion: { questionId: 'q' } })]), 2)
  assert.equal(p.needCount([]), 0)
})

test('saved collapse state keeps only usable strings', () => {
  assert.deepEqual(p.sanitiseCollapsed(['a', 'a', 3, '', null, 'b']), ['a', 'b'])
  assert.deepEqual(p.sanitiseCollapsed('nope'), [])
  assert.deepEqual(p.sanitiseCollapsed(undefined), [])
  assert.equal(p.sanitiseCollapsed(Array.from({ length: 900 }, (_, i) => `k${i}`)).length, 500)
})

test('toggleCollapsed folds and unfolds without mutating', () => {
  const start = ['a']
  const next = p.toggleCollapsed(start, 'b')
  assert.deepEqual(next, ['a', 'b'])
  assert.deepEqual(start, ['a'])
  assert.deepEqual(p.toggleCollapsed(next, 'a'), ['b'])
})

const projects = Array.from({ length: 8 }, (_, i) => ({ id: `p${i}` }))

test('few projects are all shown', () => {
  assert.deepEqual(p.visibleProjects(projects.slice(0, 5), new Map(), new Set(), '', false), { shown: projects.slice(0, 5), hidden: 0 })
})

test('live, waiting and current projects always show, the rest fill the cap', () => {
  const r = p.visibleProjects(projects, new Map([['p6', 2]]), new Set(['p7']), 'p5', false)
  assert.deepEqual(r.shown.map(x => x.id), ['p0', 'p1', 'p5', 'p6', 'p7'])
  assert.equal(r.hidden, 3)
})

test('more live projects than the cap still all show', () => {
  const running = new Map(projects.map(x => [x.id, 1]))
  const r = p.visibleProjects(projects, running, new Set(), '', false)
  assert.equal(r.shown.length, 8)
  assert.equal(r.hidden, 0)
})

test('expanded lists every project', () => {
  assert.deepEqual(p.visibleProjects(projects, new Map(), new Set(), '', true), { shown: projects, hidden: 0 })
})
