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
const colour = asURL(compile('./projectColour.ts'))
const api = asURL(compile('./terminal/api.ts'))
const source = compile('./sessions.ts').replace("from './home'", `from '${home}'`).replace("from './agentLaunch'", `from '${agentLaunch}'`).replace("from './catalogue'", `from '${catalogue}'`).replace("from './projectColour'", `from '${colour}'`).replace("from './terminal/api'", `from '${api}'`)
const { sessionColourOf, searchSessions, filterSessions, filtersOn, agentChoicesOf, sessionGroups, offlineSentence, rowMeta, sortSessions, stateBucket, worktreeTargetOf, continueLaunch, canContinue, continueChatLaunch, canContinueChat, MAX_TABS, openTab, closeTab, startedAgo, headerMeta, decisionsLine, changedList } = await import(asURL(source))

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

test('a closed session continues in the folder it ran in', () => {
  const exited = session('s6', 'm-studio', 'Continue me', { tool: 'opencode', projectId: 'p-reet', dir: '/home/u/reet', state: 'exited', exitCode: 0 })
  const resumeMachine = { capabilities: { resume: true } }
  assert.deepEqual(continueLaunch(exited, projects), { tool: 'opencode', dir: '/home/u/reet', resume: true, projectId: 'p-reet' })
  assert.equal(canContinue(exited, resumeMachine, projects), true)
  assert.deepEqual(continueLaunch({ ...exited, model: 'sonnet', effort: 'high' }, projects),
    { tool: 'opencode', dir: '/home/u/reet', resume: true, projectId: 'p-reet', model: 'sonnet', effort: 'high' })
  // A moved project folder does not move the continuation: the session's own folder is used, and the project is not named.
  assert.deepEqual(continueLaunch({ ...exited, dir: '/home/u/reet-old' }, projects), { tool: 'opencode', dir: '/home/u/reet-old', resume: true })
  // A worktree session continues in its worktree, without claiming the project's folder.
  const worktree = session('s7', 'm-studio', 'Worktree', { tool: 'opencode', projectId: 'p-reet', dir: '/home/u/.reet/worktrees/reet/reet/abc', state: 'exited' })
  assert.deepEqual(continueLaunch(worktree, projects), { tool: 'opencode', dir: '/home/u/.reet/worktrees/reet/reet/abc', resume: true })
  // A session with no folder of its own cannot be continued; a live one is not offered.
  assert.equal(continueLaunch({ ...exited, dir: '' }, projects), null)
  assert.deepEqual(continueLaunch({ ...exited, projectId: null }, projects), { tool: 'opencode', dir: '/home/u/reet', resume: true })
  assert.equal(continueLaunch(sessions[0], projects), null)
  assert.equal(canContinue({ ...exited, tool: 'claude' }, resumeMachine, projects), false)
  assert.equal(canContinue(exited, { capabilities: {} }, projects), false)
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

test('session colours: the panel choice, else the project colour, and a colour filter', () => {
  const colours = { s3: 'red', s4: 'nope' }
  const withColour = [{ ...projects[0], colour: 'blue' }, projects[1]]
  assert.equal(sessionColourOf(sessions[0], colours, withColour), 'blue')
  assert.equal(sessionColourOf(sessions[2], colours, withColour), 'red')
  // An unknown stored value is no colour.
  assert.equal(sessionColourOf(sessions[3], colours, withColour), '')
  const colourOf = s => sessionColourOf(s, colours, withColour)
  assert.deepEqual(ids(filterSessions(sessions, { colour: 'blue' }, new Set(), colourOf)), ['s1', 's2'])
  assert.deepEqual(ids(filterSessions(sessions, { colour: 'red' }, new Set(), colourOf)), ['s3'])
  assert.deepEqual(ids(filterSessions(sessions, { colour: 'none' }, new Set(), colourOf)), ['s4', 's5'])
  assert.deepEqual(ids(filterSessions(sessions, { colour: 'red', state: 'running' }, new Set(), colourOf)), [])
  assert.equal(filtersOn({ colour: 'red' }), 1)
})

test('session tabs: at most eight in the order opened, the ninth drops the oldest, closing picks a neighbour', () => {
  assert.equal(MAX_TABS, 8)
  let tabs = []
  for (const id of ['a', 'b', 'c']) tabs = openTab(tabs, id)
  assert.deepEqual(tabs, ['a', 'b', 'c'])
  // Opening one already open keeps its place.
  assert.deepEqual(openTab(tabs, 'a'), ['a', 'b', 'c'])
  for (const id of ['d', 'e', 'f', 'g', 'h', 'i']) tabs = openTab(tabs, id)
  assert.deepEqual(tabs, ['b', 'c', 'd', 'e', 'f', 'g', 'h', 'i'])
  // Closing a tab that is not shown keeps the current one; closing the current one shows the tab now in its place, else the one before.
  assert.deepEqual(closeTab(['a', 'b', 'c'], 'a', 'b'), { tabs: ['b', 'c'], next: 'b' })
  assert.deepEqual(closeTab(['a', 'b', 'c'], 'b', 'b'), { tabs: ['a', 'c'], next: 'c' })
  assert.deepEqual(closeTab(['a', 'b', 'c'], 'c', 'c'), { tabs: ['a', 'b'], next: 'b' })
  assert.deepEqual(closeTab(['a'], 'a', 'a'), { tabs: [], next: '' })
})

test('session header: agent · model · started, and the decisions line', () => {
  assert.equal(startedAgo(30_000), 'started just now')
  assert.equal(startedAgo(MIN), 'started 1 minute ago')
  assert.equal(startedAgo(41 * MIN), 'started 41 minutes ago')
  assert.equal(startedAgo(2 * 60 * MIN), 'started 2 hours ago')
  assert.equal(startedAgo(60 * MIN), 'started 1 hour ago')
  assert.equal(startedAgo(3 * 24 * 60 * MIN), 'started 3 days ago')
  assert.equal(headerMeta(sessions[0], NOW - 41 * MIN, NOW), 'Claude Code · sonnet · started 41 minutes ago')
  assert.equal(headerMeta(sessions[3], NOW - 2 * MIN, NOW), 'Shell · started 2 minutes ago')
  assert.equal(decisionsLine({ yolo: false }), 'Asked in the terminal.')
  assert.equal(decisionsLine({}), 'Asked in the terminal.')
  assert.equal(decisionsLine({ yolo: true, structured: true }), 'YOLO: reet answers yes, this once, to each request.')
  assert.equal(decisionsLine({ yolo: true, structured: false }), 'YOLO: the agent asked not to be prompted; the launch is audited.')
  assert.equal(decisionsLine({ structured: true }), 'Asked in the chat: Allow once or Deny.')
  assert.deepEqual(continueChatLaunch({ structured: true, acpSessionId: 'ses_1', state: 'exited', dir: '/home/u/site', projectId: 'p1', model: 'anthropic/claude-sonnet-4-5' }), { tool: 'opencode', dir: '/home/u/site', model: 'anthropic/claude-sonnet-4-5', structured: true, trust: true, resumeAcp: 'ses_1', projectId: 'p1' })
  assert.equal(continueChatLaunch({ structured: true, acpSessionId: 'ses_1', state: 'running', dir: '/home/u/site' }), null)
  assert.deepEqual(continueChatLaunch({ structured: true, acpSessionId: 'ses_1', state: 'exited' }), { tool: 'opencode', structured: true, trust: true, resumeAcp: 'ses_1' })
  assert.equal(continueChatLaunch({ structured: true, state: 'exited', dir: '/home/u/site' }), null)
  assert.equal(canContinueChat({ structured: true, acpSessionId: 'ses_1', state: 'exited', dir: '/home/u/site' }, { capabilities: { acp: true, acpHistory: true } }), true)
  assert.equal(canContinueChat({ structured: true, acpSessionId: 'ses_1', state: 'exited', dir: '/home/u/site' }, { capabilities: { acp: true } }), false)
  assert.equal(canContinueChat({ structured: true, acpSessionId: 'ses_1', state: 'exited', dir: '/home/u/site' }, { capabilities: {} }), false)
})

test('changed files: at most twenty paths and the total', () => {
  const paths = Array.from({ length: 25 }, (_, i) => `f${i}`)
  assert.deepEqual(changedList({ changed: 30, paths }), { shown: paths.slice(0, 20), total: 30, more: 10 })
  assert.deepEqual(changedList({ changed: 2, paths: ['a', 'b'] }), { shown: ['a', 'b'], total: 2, more: 0 })
  assert.deepEqual(changedList({ changed: 0, paths: [] }), { shown: [], total: 0, more: 0 })
})
