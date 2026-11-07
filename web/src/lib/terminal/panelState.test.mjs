import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const compileTs = text => ts.transpileModule(text, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const attention = `data:text/javascript;base64,${Buffer.from(compileTs(readFileSync(new URL('../attention.ts', import.meta.url), 'utf8'))).toString('base64')}`
const source = compileTs(readFileSync(new URL('./panelState.ts', import.meta.url), 'utf8')).replace("from '../attention'", `from '${attention}'`)
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

test('a seen chat still holding an approval or question needs the person; a seen bell does not', () => {
  const seen = waiting({ changedSinceSeen: false })
  assert.equal(p.needsInput({ ...seen, pendingPermission: { permissionId: 'a' } }), 'approval')
  assert.equal(p.needsInput({ ...seen, pendingQuestion: { questionId: 'q' } }), 'question')
  assert.equal(p.needsInput(seen), null)
  assert.equal(p.statusLine({ ...seen, pendingPermission: { permissionId: 'a' } }), 'Needs you · approval')
})

test('the status line says what is awaited, else the state', () => {
  assert.equal(p.statusLine(waiting({ pendingQuestion: { questionId: 'q' } })), 'Needs you · question')
  assert.equal(p.statusLine({ state: 'end_requested' }), 'end requested')
  assert.equal(p.statusLine({ state: 'running' }), 'running')
})

test('a finished or failed turn is named over the still-running state', () => {
  assert.equal(p.statusLine({ state: 'running', attention: 'finished' }), 'Finished')
  assert.equal(p.statusLine({ state: 'running', attention: 'failed' }), 'Failed')
  assert.equal(p.statusLine({ state: 'lost', attention: 'failed' }), 'Connection lost')
  assert.equal(p.statusLine({ state: 'running', attention: 'working' }), 'running')
})

test('a quiet turn is not still running, in the chat or at an idle terminal', () => {
  assert.equal(p.statusLine({ state: 'running', attention: 'quiet', attentionSource: 'chat' }), 'Quiet')
  assert.equal(p.statusLine({ state: 'running', attention: 'quiet', attentionSource: 'terminal' }), 'Quiet')
})

test('the row mark follows attention before the lifecycle state', () => {
  assert.equal(p.sessionMark({ state: 'running', attention: 'finished' }), 'finished')
  assert.equal(p.sessionMark({ state: 'running', attention: 'failed' }), 'failed')
  assert.equal(p.sessionMark({ state: 'lost', attention: 'failed' }), 'lost')
  assert.equal(p.sessionMark({ state: 'running', attention: 'quiet', attentionSource: 'chat' }), 'finished')
  assert.equal(p.sessionMark({ state: 'running', attention: 'quiet', attentionSource: 'terminal' }), 'running')
  assert.equal(p.sessionMark({ state: 'running' }), 'running')
  assert.equal(p.sessionMark({ state: 'creating' }), 'starting')
  assert.equal(p.sessionMark({ state: 'end_requested' }), 'ending')
  assert.equal(p.sessionMark({ state: 'ended' }), 'finished')
})

test('needCount counts only sessions that need the person', () => {
  assert.equal(p.needCount([waiting({}), { state: 'running' }, waiting({ changedSinceSeen: false }), waiting({ pendingQuestion: { questionId: 'q' } })]), 2)
  assert.equal(p.needCount([]), 0)
})

test('needCount follows the shared attention rule, including unseen failed and finished within a day', () => {
  const now = 2 * 24 * 60 * 60_000
  const HOUR = 60 * 60_000
  const blockedSeen = waiting({ changedSinceSeen: false, pendingPermission: { permissionId: 'a' }, updatedAt: now - 30 * HOUR })
  const failedRecent = { state: 'exited', attention: 'failed', changedSinceSeen: true, updatedAt: now - HOUR }
  const finishedOld = { state: 'running', attention: 'finished', changedSinceSeen: true, updatedAt: now - 25 * HOUR }
  const failedSeen = { state: 'exited', attention: 'failed', changedSinceSeen: false, updatedAt: now - HOUR }
  const working = { state: 'running', attention: 'working', changedSinceSeen: true, updatedAt: now }
  assert.equal(p.needCount([blockedSeen, failedRecent, finishedOld, failedSeen, working], now), 2)
})

test('needLabel words the badge count for one or many', () => {
  assert.equal(p.needLabel(1), '1 needs you')
  assert.equal(p.needLabel(2), '2 need you')
  assert.equal(p.needLabel(0), '0 need you')
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

test('project groups: sessions group by project, with "No project" last', async () => {
  const { NO_PROJECT, projectGroupKey, orderProjectGroups } = p
  assert.equal(projectGroupKey({ projectId: 'p1' }), 'p1')
  assert.equal(projectGroupKey({}), NO_PROJECT)
  assert.equal(projectGroupKey({ projectId: '' }), NO_PROJECT)
  const out = orderProjectGroups([{ key: NO_PROJECT }, { key: 'gone' }, { key: 'b' }, { key: 'a' }], ['a', 'b'])
  assert.deepEqual(out.map(g => g.key), ['a', 'b', 'gone', NO_PROJECT])
})
