import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const catalogue = asURL(compile('./catalogue.ts'))
const agentLaunch = asURL(compile('./agentLaunch.ts').replace("from './catalogue'", `from '${catalogue}'`))
const home = asURL(compile('./home.ts'))
const source = compile('./sessions.ts').replace("from './home'", `from '${home}'`).replace("from './agentLaunch'", `from '${agentLaunch}'`)
const { searchSessions, filterSessions, filtersOn, agentChoicesOf, sessionGroups, offlineSentence, rowMeta, sortSessions, stateBucket, worktreeTargetOf } = await import(asURL(source))

const NOW = 1_800_000_000_000
const MIN = 60_000
const machines = [{ agentId: 'm-studio', host: 'studio' }, { agentId: 'm-server', host: 'server' }, { agentId: 'm-desk', host: 'desk' }]
const project = (id, name, host, machine) => ({ id, name, host, machine, dir: `/home/u/${name}`, isRepo: true, baseBranch: '', worktreeDefault: 'inherit', colour: '', defaultTool: '', defaultModel: '',
  hasStarterPrompt: false, note: '', position: 0, createdAt: 1, updatedAt: 1, version: 1 })
const projects = [project('p-reet', 'reet', 'm-studio', 'studio'), project('p-site', 'site', 'm-server', 'server')]
const session = (id, agentId, title, over = {}) => ({ id, agentId, title, state: 'running', createdAt: NOW - 60 * MIN, updatedAt: NOW - 5 * MIN, exitCode: null, ...over })
const sessions = [
  session('s1', 'm-studio', 'Refactor the store layer', { tool: 'claude', model: 'sonnet', projectId: 'p-reet', worktreePath: '/home/u/.reet/worktrees/reet/reet/a1b2c3d4e5f60718', worktreeRepo: 'reet', worktreeBranch: 'reet/reet-a1b2c3d4e5f60718' }),
  session('s2', 'm-studio', 'Add browse-v1 protocol', { tool: 'codex', projectId: 'p-reet', yolo: true }),
  session('s3', 'm-server', 'Review code', { tool: 'claude', projectId: 'p-site', state: 'exited', exitCode: 0 }),
  session('s4', 'm-desk', 'Shell', { state: 'ended' }),
  session('s5', 'm-desk', 'Docs pass', { tool: 'claude', state: 'lost' }),
]
const ids = list => list.map(s => s.id)

test('search matches branch and machine', () => {
  assert.deepEqual(ids(searchSessions(sessions, projects, machines, 'reet/reet-a1b2')), ['s1'])
  assert.deepEqual(ids(searchSessions(sessions, projects, machines, 'SERVER')), ['s3'])
  // The agent by its name, the model, the project and the title; every word must match somewhere.
  assert.deepEqual(ids(searchSessions(sessions, projects, machines, 'codex')), ['s2'])
  assert.deepEqual(ids(searchSessions(sessions, projects, machines, 'Claude Code')), ['s1', 's3', 's5'])
  assert.deepEqual(ids(searchSessions(sessions, projects, machines, 'sonnet')), ['s1'])
  assert.deepEqual(ids(searchSessions(sessions, projects, machines, 'site')), ['s3'])
  assert.deepEqual(ids(searchSessions(sessions, projects, machines, 'studio browse')), ['s2'])
  assert.deepEqual(ids(searchSessions(sessions, projects, machines, '  ')), ids(sessions))
  assert.deepEqual(ids(searchSessions(sessions, projects, machines, 'nowhere')), [])
})

test('filters combine', () => {
  assert.deepEqual(ids(filterSessions(sessions, { state: 'running' })), ['s1', 's2'])
  assert.deepEqual(ids(filterSessions(sessions, { state: 'finished' })), ['s3', 's5'])
  assert.deepEqual(ids(filterSessions(sessions, { state: 'ended' })), ['s4'])
  assert.deepEqual(ids(filterSessions(sessions, { agent: 'claude' })), ['s1', 's3', 's5'])
  assert.deepEqual(ids(filterSessions(sessions, { agent: 'shell' })), ['s4'])
  assert.deepEqual(ids(filterSessions(sessions, { machine: 'm-studio' })), ['s1', 's2'])
  assert.deepEqual(ids(filterSessions(sessions, { yoloOnly: true })), ['s2'])
  assert.deepEqual(ids(filterSessions(sessions, { worktreeOnly: true })), ['s1'])
  // Together: every filter that is set must hold.
  assert.deepEqual(ids(filterSessions(sessions, { state: 'running', agent: 'claude', machine: 'm-studio' })), ['s1'])
  assert.deepEqual(ids(filterSessions(sessions, { state: 'running', agent: 'claude', yoloOnly: true })), [])
  assert.deepEqual(ids(filterSessions(sessions, { machine: 'm-desk', state: 'finished' })), ['s5'])
  // Needs you matches only what the attention signal names; nothing does today.
  assert.deepEqual(ids(filterSessions(sessions, { state: 'needs' })), [])
  assert.deepEqual(ids(filterSessions(sessions, { state: 'needs' }, new Set(['s2']))), ['s2'])
  assert.equal(filtersOn({ state: 'running', agent: '', yoloOnly: true }), 2)
  assert.equal(stateBucket({ state: 'end_requested' }), 'running')
})

test('agent choices name each agent once, shells last', () => {
  assert.deepEqual(agentChoicesOf(sessions), [{ value: 'claude', label: 'Claude Code' }, { value: 'codex', label: 'Codex' }, { value: 'shell', label: 'Shell' }])
})

test('groups by project put the rest under Other on their machine, with the project machine', () => {
  const groups = sessionGroups(sessions, projects, machines, 'project', new Set())
  assert.deepEqual(groups.map(g => [g.label, g.machine ?? null, ids(g.sessions)]), [
    ['reet', 'studio', ['s1', 's2']], ['site', 'server', ['s3']], ['Other on desk', null, ['s4', 's5']],
  ])
  assert.deepEqual(sessionGroups(sessions, projects, machines, 'machine', new Set()).map(g => g.label), ['studio', 'server', 'desk'])
  assert.deepEqual(sessionGroups(sessions, projects, machines, 'state', new Set()).map(g => g.label), ['Running', 'Connection lost', 'Exited', 'Ended'])
})

test('offline machine group sentence', () => {
  const offline = new Set(['m-desk'])
  const byMachine = sessionGroups(sessions, projects, machines, 'machine', offline)
  assert.deepEqual(byMachine.map(g => g.offline), [[], [], ['desk']])
  assert.equal(offlineSentence('desk'), 'desk is offline')
  // Grouped by state, a group names each offline machine among its sessions once.
  const byState = sessionGroups(sessions, projects, machines, 'state', new Set(['m-desk', 'm-server']))
  assert.deepEqual(byState.map(g => [g.label, g.offline]), [['Running', []], ['Connection lost', ['desk']], ['Exited', ['server']], ['Ended', ['desk']]])
})

test('sorting within a group', () => {
  const list = [session('a', 'm', 'Beta', { updatedAt: 1 }), session('b', 'm', 'Alpha', { updatedAt: 3 }), session('c', 'm', 'Gamma', { updatedAt: 2 })]
  assert.deepEqual(ids(sortSessions(list, 'recent')), ['b', 'c', 'a'])
  assert.deepEqual(ids(sortSessions(list, 'name')), ['b', 'a', 'c'])
  assert.deepEqual(ids(sortSessions(list, 'custom', ['c', 'a'])), ['c', 'a', 'b'])
})

test('row meta says agent, model and branch', () => {
  assert.equal(rowMeta(sessions[0]), 'Claude Code · sonnet · reet/reet-a1b2c3d4e5f60718')
  assert.equal(rowMeta(sessions[3]), 'Shell')
})

test('a session worktree is named only when its path and branch agree', () => {
  assert.deepEqual(worktreeTargetOf(sessions[0]), { agentId: 'm-studio', project: 'reet', repo: 'reet', run: 'a1b2c3d4e5f60718' })
  assert.equal(worktreeTargetOf(sessions[1]), null)
  assert.equal(worktreeTargetOf({ ...sessions[0], worktreeRepo: 'other' }), null)
  assert.equal(worktreeTargetOf({ ...sessions[0], worktreeBranch: 'main' }), null)
  assert.equal(worktreeTargetOf({ ...sessions[0], worktreePath: '/home/u/reet' }), null)
  assert.equal(worktreeTargetOf({ ...sessions[0], worktreePath: '/home/u/elsewhere/reet/reet/a1b2c3d4e5f60718' }), null)
})
