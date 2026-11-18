import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = path => ts.transpileModule(readFileSync(new URL(path, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const dataUrl = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const point = (code, from, url) => code.replace(new RegExp(`(['"])${from.replace(/[./]/g, c => `\\${c}`)}\\1`), JSON.stringify(url))
// The one rule imports the session model, the list model and the merged list: each is pointed at a compiled copy.
const attentionUrl = dataUrl(compile('../attention.ts'))
const homeUrl = dataUrl(point(compile('../home.ts'), './attention', attentionUrl))
const outcomeUrl = dataUrl(compile('../gafferOutcome.ts'))
const listUrl = dataUrl(point(compile('../factory/factoryList.ts'), '../gafferOutcome', outcomeUrl))
const waitingUrl = dataUrl(point(compile('./homeWaiting.ts'), '../factory/factoryList', listUrl))
let source = compile('./whatWaits.ts')
source = point(source, '../home', homeUrl)
source = point(source, '../factory/factoryList', listUrl)
source = point(source, './homeWaiting', waitingUrl)
source = point(source, './deskSessions', dataUrl(compile('./deskSessions.ts')))
const { whatWaits } = await import(dataUrl(source))

const NOW = 1_800_000_000_000
const MIN = 60_000
const session = (id, over = {}) => ({ id, agentId: 'a1', title: `Session ${id}`, state: 'running', createdAt: NOW - 60 * MIN, updatedAt: NOW - 5 * MIN, exitCode: null, ...over })
const waitingOn = (id, over = {}) => session(id, { attention: 'waiting', changedSinceSeen: true, ...over })
const goalRow = (conversationId, over = {}, goal = {}) => ({ goal: { conversationId, sessionId: '', state: 'active', note: '', updatedAt: NOW - 10 * MIN, ...goal }, available: true, attention: true, reason: 'Look at this.', ...over })
const outcome = (id, sessions, over = {}) => ({ id, revision: 1, objective: `Objective ${id}`, state: 'settled', note: '', agentId: 'a1', projectId: 'p', projectName: 'p', dir: '/d', tool: 'claude', providerId: '', maxReviews: 3, hours: 1, sessions, createdAt: 1, updatedAt: 1, ...over })
const piece = (key, over = {}) => ({ key, title: `Pipe ${key}`, prompt: '', state: 'following', note: '', updatedAt: NOW - 20 * MIN, stages: [{ kind: 'build', tool: 'claude', state: 'running' }], stage: 0, ...over })
const run = (sessions, outcomes, goals) => whatWaits(sessions, NOW, new Map(), [{ agentId: 'a1', host: 'studio' }], outcomes, goals)

test('a goal that names its session only in the goal is that session’s card’s second reason', () => {
  const { entries } = run([waitingOn('s1')], [], [goalRow('c1', {}, { sessionId: 's1' })])
  assert.deepEqual(entries.map(e => e.id), ['session:s1'])
  assert.ok(entries[0].card.reasons.includes('gaffer'))
})

test('a goal of an archived outcome’s pipeline is never a stray Gaffer entry', () => {
  const archived = outcome('o1', [piece('k', { conversationId: 'c1', sessionId: 's9' })], { archivedAt: NOW - MIN })
  const { entries } = run([], [archived], [goalRow('c1', { session: { id: 's9', agentId: 'a1', title: 'Old', state: 'running' } })])
  assert.deepEqual(entries, [])
})

test('the approval a pipeline’s session holds rides on its approval item', () => {
  const pendingPermission = { permissionId: 'perm1', title: 'Run a command', command: 'ls', path: '', truncated: false, queued: 0 }
  const record = outcome('o1', [piece('k', { conversationId: 'c1', sessionId: 's1' })])
  const { entries } = run([waitingOn('s1', { pendingPermission })], [record], [goalRow('c1', { session: { id: 's1', agentId: 'a1', title: 'Pipe', state: 'running' } }, { sessionId: 's1', waitingOnWorker: true })])
  assert.deepEqual(entries.map(e => e.id), ['o1.k'])
  assert.equal(entries[0].item.kind, 'approval')
  assert.equal(entries[0].approval?.permissionId, 'perm1')
})

const desk = (id, sessions, over = {}) => ({ id, name: `Front ${id}`, template: false, version: 1, sessions, ...over })
const runOn = (sessions, desks, panes, outcomes = [], goals = []) => whatWaits(sessions, NOW, new Map(), [{ agentId: 'a1', host: 'studio' }], outcomes, goals, { desks, panes })

test('a waiting session on a desk is marked with its desk and opens on it, at its pane when known', () => {
  const pendingPermission = { permissionId: 'perm1', title: 'Run a command', command: 'ls', path: '', truncated: false, queued: 0 }
  const sessions = [waitingOn('s1', { pendingPermission }), waitingOn('s2'), session('s3', { attention: 'finished', changedSinceSeen: true }), waitingOn('s4')]
  const { entries } = runOn(sessions, [desk('d1', ['s1', 's2', 's3'])], new Map([['s1', 'pane-1']]))
  assert.deepEqual(entries.map(e => e.id).sort(), ['session:s1', 'session:s2', 'session:s4'])
  const byId = new Map(entries.map(e => [e.id, e]))
  assert.deepEqual(byId.get('session:s1').desk, { id: 'd1', name: 'Front d1', href: '/desks/d1#pane-1' })
  assert.equal(byId.get('session:s1').card.href, '/desks/d1#pane-1')
  assert.equal(byId.get('session:s2').card.href, '/desks/d1')
  assert.equal(byId.get('session:s4').desk, undefined)
  assert.match(byId.get('session:s4').card.href, /^\/sessions\?/)
})

test('a Gaffer goal on a desk session is marked and opens on the desk', () => {
  const { entries } = runOn([session('s1')], [desk('d1', ['s1'])], undefined, [], [goalRow('c1', { session: { id: 's1', agentId: 'a1', title: 'Desk work', state: 'running' } }, { sessionId: 's1' })])
  assert.equal(entries.length, 1)
  assert.equal(entries[0].source, 'gaffer')
  assert.equal(entries[0].desk?.name, 'Front d1')
  assert.equal(entries[0].goal.href, '/desks/d1')
})

test('a template desk marks nothing, and the count is the same with desks as without', () => {
  const sessions = [waitingOn('s1'), waitingOn('s2')]
  const plain = run(sessions, [], undefined).entries
  const marked = runOn(sessions, [desk('t', ['s1'], { template: true }), desk('d1', ['s2'])]).entries
  assert.equal(marked.length, plain.length)
  assert.equal(marked.find(e => e.id === 'session:s1').desk, undefined)
  assert.equal(marked.find(e => e.id === 'session:s2').desk?.id, 'd1')
})

test('a Factory pipeline’s session on a desk stays its Factory item, unmarked', () => {
  const record = outcome('o1', [piece('k', { conversationId: 'c1', sessionId: 's1', state: 'ready_to_merge' })])
  const { entries } = runOn([waitingOn('s1')], [desk('d1', ['s1'])], undefined, [record], [])
  assert.ok(entries.every(e => !e.desk))
})
