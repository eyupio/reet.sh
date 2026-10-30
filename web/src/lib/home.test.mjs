import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = readFileSync(new URL('./home.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const h = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
const { fleetCards, sessionOwners, inFlight, rowSubtitle, stateWord, needsYou, attentionCount, announceWaiting, reporting, sessionTabTitle, titleWithAttention } = h

const NOW = 1_800_000_000_000
const MIN = 60_000, HOUR = 60 * MIN

const report = (cpu, over = {}) => ({ cpuPercent: { value: cpu }, memory: { totalBytes: { value: 1 }, availableBytes: { value: 1 } }, disk: { totalBytes: { value: 1 }, availableBytes: { value: 1 } }, hostUptimeSeconds: { value: 1 }, scope: 'host', ...over })
const machine = (agentId, host, { cpu = 12.4, state = 'connected', fresh = true, os = 'linux', connection } = {}) => ({
  agentId, host, state,
  capabilities: { protocolVersion: 1, os, osUser: 'me', terminalEnabled: true, healthEnabled: true, readiness: 'ready' },
  connection: connection === null ? undefined : (connection ?? { connected: state === 'connected', reportFresh: fresh, missedProbes: 0, reconnects: 0, path: 'direct', report: cpu === 'no report' ? undefined : report(cpu) }),
})
const session = (id, agentId, state = 'running', over = {}) => ({ id, agentId, title: `Session ${id}`, state, createdAt: NOW - 2 * HOUR, updatedAt: NOW - 5 * MIN, exitCode: null, ...over })
const emptySync = () => ({ environments: [], targets: [], agents: [], jobs: [] })
const target = (id, host) => ({ target: { id, host, agent: 'claude' }, sourceId: 's', sourceName: 'src', desired: [], missing: [], extra: [], revision: 'r', updatedAt: NOW })
const job = (id, agentId, targetId, kind, status, actions, updatedAt) => ({ id, agentId, targetId, kind, status, revision: 'r', hash: 'h', plan: actions === undefined ? {} : { actions: Array.from({ length: actions }, (_, i) => ({ add: true, skill: { id: `k${i}`, name: `k${i}` } })) }, message: '', createdAt: updatedAt - MIN, updatedAt })
const syncWith = (jobs, targets = [target('t1', 'studio')]) => ({ ...emptySync(), agents: [{ id: 'a1', host: 'studio', version: '1', lastSeen: NOW, paused: false, autoUpdate: false }], targets, jobs })
// A folder project as GET /api/v2/folder-projects lists it.
const project = (id, name, host = 'a1', over = {}) => ({ id, host, machine: 'studio', dir: `/home/me/${name}`, name, isRepo: true, baseBranch: '', worktreeDefault: 'inherit', defaultTool: '', defaultModel: '', hasStarterPrompt: false, note: '', position: 0, createdAt: 1, updatedAt: 1, version: 1, ...over })
const tree = (name) => ({ worktreePath: `/home/me/.reet/worktrees/site/${name}`, worktreeRepo: name, worktreeBranch: `reet/site-${name}` })
const FORBIDDEN = /\b(healthy|verified)\b/i

// The session listing carries worktreePath but no removal record (a removal moves only an old Project run member to
// "removed"), so it cannot say "made by reet and not removed". What it can say is which worktrees live sessions are
// working in now: the card counts those, labelled "in use", and never a finished session's worktree.
test('worktree figure counts only the worktrees live sessions are in', () => {
  const sessions = [
    session('s1', 'a1', 'running', tree('web')), session('s2', 'a1', 'creating', tree('api')), session('s3', 'a1', 'running', tree('web')),
    session('s4', 'a1', 'exited', { exitCode: 0, ...tree('old') }), session('s5', 'a1', 'running'), session('s6', 'a2', 'end_requested', tree('desk')),
  ]
  const cards = fleetCards([machine('a1', 'studio'), machine('a2', 'desk'), machine('a3', 'empty')], sessions, emptySync(), NOW)
  assert.deepEqual(cards.map(c => c.worktrees), [2, 1, 0])
  assert.deepEqual([0, 1, 2].map(h.worktreesInUse), ['no worktrees in use', '1 worktree in use', '2 worktrees in use'])
})

test('the Sessions tab names the session and its project, or its machine', () => {
  assert.equal(sessionTabTitle(undefined), 'Sessions · reet.sh')
  assert.equal(sessionTabTitle({ title: 'Build', projectId: 'p1' }, 'reet'), 'Build · reet · reet.sh')
  assert.equal(sessionTabTitle({ title: 'Build', projectId: null }, '', 'studio'), 'Build · studio · reet.sh')
})

test('titleWithAttention prefixes positive attention counts to the document title', () => {
  assert.equal(titleWithAttention('Home · reet.sh', 0), 'Home · reet.sh')
  assert.equal(titleWithAttention('Home · reet.sh', 3), '(3) Home · reet.sh')
  assert.equal(titleWithAttention(sessionTabTitle({ title: 'Build', projectId: 'p1' }, 'reet'), 1), '(1) Build · reet · reet.sh')
  assert.equal(titleWithAttention(sessionTabTitle(undefined), 2), '(2) Sessions · reet.sh')
})

test('cpu null shows No reading, labelled CPU', () => {
  const cards = fleetCards([
    machine('a1', 'studio', { cpu: 37.6 }),
    machine('a2', 'laptop', { cpu: null }),
    machine('a3', 'pi', { cpu: 'no report' }),
    machine('a4', 'old', { connection: null, state: 'never_connected' }),
    machine('a5', 'stale', { cpu: 80, fresh: false }),
  ], [], emptySync(), NOW)
  assert.deepEqual(cards[0].cpu, { label: 'CPU', percent: 37.6, text: '38%' })
  for (const card of cards.slice(1)) assert.deepEqual(card.cpu, { label: 'CPU', percent: null, text: 'No reading' }, card.name)
})

test('memory and disk come from the same fresh report as the CPU', () => {
  const live = machine('a1', 'studio', { connection: { connected: true, reportFresh: true, missedProbes: 0, reconnects: 0, path: 'direct', report: report(10, {
    memory: { totalBytes: { value: 8_000_000_000 }, availableBytes: { value: 2_000_000_000 } },
    disk: { totalBytes: { value: 100_000_000_000 }, availableBytes: { value: 25_000_000_000 } },
  }) } })
  const [card] = fleetCards([live], [], emptySync(), NOW)
  assert.deepEqual(card.memory, { label: 'Memory', percent: 75, text: '75%' })
  assert.deepEqual(card.disk, { label: 'Disk', percent: 75, availableBytes: 25_000_000_000 })
  const [stale] = fleetCards([machine('a2', 'old', { fresh: false })], [], emptySync(), NOW)
  assert.deepEqual(stale.memory, { label: 'Memory', percent: null, text: 'No reading' })
  assert.deepEqual(stale.disk, { label: 'Disk', percent: null, availableBytes: null })
})

test('cards keep the machine order, name, OS and connection word', () => {
  const cards = fleetCards([machine('a2', 'laptop', { os: 'darwin' }), machine('a1', 'studio'), machine('a3', 'pi', { state: 'unknown', connection: { connected: false, reportFresh: false, missedProbes: 2, reconnects: 1, path: 'direct' } }), machine('a4', 'gone', { state: 'unreachable' }), machine('a5', 'new', { state: 'never_connected', connection: null })], [], emptySync(), NOW)
  assert.deepEqual(cards.map(c => [c.agentId, c.name, c.os, c.status]), [['a2', 'laptop', 'darwin', 'online'], ['a1', 'studio', 'linux', 'online'], ['a3', 'pi', 'linux', 'reconnecting'], ['a4', 'gone', 'linux', 'offline'], ['a5', 'new', 'linux', 'offline']])
})

// Control skips a paused agent with the reason "paused" (fleet.ts); Home says the same word instead of "online".
test('a paused agent that still reports reads paused, and one that stopped reporting reads offline', () => {
  const sync = { ...emptySync(), agents: [{ id: 'a1', host: 'studio', version: '1', lastSeen: NOW, paused: true, autoUpdate: false }, { id: 'a2', host: 'desk', version: '1', lastSeen: NOW - 2 * HOUR, paused: true, autoUpdate: false }, { id: 'a3', host: 'pi', version: '1', lastSeen: NOW, paused: false, autoUpdate: false }] }
  const cards = fleetCards([machine('a1', 'studio'), machine('a2', 'desk', { state: 'never_connected', connection: null }), machine('a3', 'pi')], [session('s1', 'a1')], sync, NOW)
  assert.deepEqual(cards.map(c => c.status), ['paused', 'offline', 'online'])
  assert.deepEqual(cards.map(c => reporting(c)), [true, false, true], 'a paused machine still reporting keeps its count current')
  assert.equal(cards[0].sessionsRunning, 1)
})

test('sessions running counts running and starting sessions; ending, finished and exited are not counted', () => {
  const sessions = [
    session('live', 'a1', 'running'),
    session('starting', 'a1', 'creating'),
    session('ending', 'a1', 'end_requested'),
    session('shell', 'a1', 'running'),
    session('exited', 'a1', 'exited', { exitCode: 0 }),
    session('failed', 'a1', 'exited', { exitCode: 2 }),
    session('ended', 'a1', 'ended'),
    session('lost', 'a1', 'lost'),
    session('other', 'a2', 'running'),
    session('going', 'a3', 'end_requested'),
    session('going too', 'a3', 'end_requested'),
  ]
  const [studio, laptop, idle] = fleetCards([machine('a1', 'studio'), machine('a2', 'laptop'), machine('a3', 'idle')], sessions, emptySync(), NOW)
  assert.equal(studio.sessionsRunning, 3)
  assert.equal(studio.sessionsEnding, 1)
  assert.equal(laptop.sessionsRunning, 1)
  assert.equal(laptop.sessionsEnding, 0)
  // A machine whose only sessions are ending runs none.
  assert.equal(idle.sessionsRunning, 0)
  assert.equal(idle.sessionsEnding, 2)
  // The listing cannot say which sessions are agents, so the card never claims agents.
  for (const card of [studio, laptop, idle]) assert.equal('agentsRunning' in card, false)
  assert.equal(h.agentSessionIds, undefined)
})

test('installed pills report agents, skills and MCP from that machine only', () => {
  const sync = {
    ...emptySync(),
    machines: [{ host: 'studio', groups: [], skillCount: 3 }],
    environments: [
      { id: 'e1', name: 'Claude', kind: 'machine', agent: 'claude', host: 'studio', skills: [{ id: 'k1', name: 'One' }, { id: 'k2', name: 'Two' }, { id: 'k3', name: 'Three' }], scannedAt: NOW, complete: true, os: 'linux', arch: 'x64', version: '1' },
      { id: 'e2', name: 'Codex', kind: 'machine', agent: 'agents', host: 'studio', skills: [{ id: 'k1', name: 'One' }], scannedAt: NOW, complete: true, os: 'linux', arch: 'x64', version: '1' },
      { id: 'e3', name: 'Claude', kind: 'machine', agent: 'claude', host: 'desk', skills: [], scannedAt: NOW, complete: true, os: 'darwin', arch: 'arm64', version: '1' },
    ],
  }
  const terminals = [{ agentId: 'a1', host: 'studio', capabilities: { tools: ['claude', 'codex'], protocolVersion: 1, os: 'linux', osUser: 'me', terminalEnabled: true, healthEnabled: true, readiness: 'ready' } }]
  const [studio, desk, unknown] = fleetCards([machine('a1', 'studio'), machine('a2', 'desk'), machine('a3', 'pi')], [], sync, NOW, terminals)
  assert.deepEqual(studio.installed, { agents: ['claude', 'codex'], skills: 3, mcp: null })
  assert.deepEqual(desk.installed, { agents: ['claude'], skills: 0, mcp: null })
  assert.deepEqual(unknown.installed, { agents: null, skills: null, mcp: null })
})

test('skills line never says in sync without a check', () => {
  const studio = [machine('a1', 'studio')]
  const line = sync => fleetCards(studio, [], sync, NOW)[0].skills
  const notChecked = { text: 'Not checked', tone: 'muted' }
  assert.deepEqual(line(emptySync()), notChecked, 'no sync state at all')
  assert.deepEqual(line(syncWith([])), notChecked, 'a target but no job')
  assert.deepEqual(line(syncWith([], [])), notChecked, 'no target to compare')
  assert.deepEqual(line(syncWith([job('j', 'a1', 't1', 'scan', 'succeeded', 0, NOW - MIN)])), notChecked, 'a scan is not a check')
  assert.deepEqual(line(syncWith([job('j', 'a1', 't1', 'preview', 'queued', undefined, NOW - MIN)])), notChecked, 'a queued check')
  assert.deepEqual(line(syncWith([job('j', 'a1', 't1', 'preview', 'running', undefined, NOW - MIN)])), notChecked, 'a running check')
  assert.deepEqual(line(syncWith([job('j', 'a1', 't1', 'preview', 'failed', 0, NOW - MIN)])), notChecked, 'a failed check')
  assert.deepEqual(line(syncWith([job('j', 'a1', 't1', 'apply', 'succeeded', 2, NOW - MIN)])), notChecked, 'an apply that changed files is not a check that found none')
  assert.deepEqual(line(syncWith([job('j', 'a2', 't1', 'preview', 'awaiting-approval', 0, NOW - MIN)])), notChecked, "another machine's check")
  // One target checked, the other never: the machine as a whole is not known to be in sync.
  assert.deepEqual(line(syncWith([job('j', 'a1', 't1', 'preview', 'awaiting-approval', 0, NOW - MIN)], [target('t1', 'studio'), target('t2', 'Studio')])), notChecked, 'a target without a check')

  assert.deepEqual(line(syncWith([job('j', 'a1', 't1', 'preview', 'awaiting-approval', 0, NOW - 3 * HOUR)])), { text: 'skills in sync · checked 3 h ago', tone: 'good' })
  assert.deepEqual(line(syncWith([job('j', 'a1', 't1', 'apply', 'succeeded', 0, NOW - 20 * MIN)])), { text: 'skills in sync · checked 20 min ago', tone: 'good' }, 'an automatic check that found nothing to do')
  assert.deepEqual(line(syncWith([job('j', 'a1', 't1', 'preview', 'awaiting-approval', 1, NOW - MIN)])), { text: '1 skill out of date', tone: 'warn' })
  assert.deepEqual(line(syncWith([job('j', 'a1', 't1', 'auto', 'awaiting-approval', 3, NOW - MIN)])), { text: '3 skills out of date', tone: 'warn' })
  // The newest finished check wins; an older clean one does not hide a newer difference, nor the reverse.
  assert.deepEqual(line(syncWith([job('old', 'a1', 't1', 'preview', 'awaiting-approval', 0, NOW - 2 * HOUR), job('new', 'a1', 't1', 'preview', 'awaiting-approval', 1, NOW - HOUR)])), { text: '1 skill out of date', tone: 'warn' })
  assert.deepEqual(line(syncWith([job('old', 'a1', 't1', 'preview', 'awaiting-approval', 1, NOW - 2 * HOUR), job('new', 'a1', 't1', 'preview', 'awaiting-approval', 0, NOW - HOUR)])), { text: 'skills in sync · checked 1 h ago', tone: 'good' })
  // Two targets both clean: the age is the older check's.
  assert.deepEqual(line(syncWith([job('a', 'a1', 't1', 'preview', 'awaiting-approval', 0, NOW - 2 * 86_400_000), job('b', 'a1', 't2', 'preview', 'awaiting-approval', 0, NOW - MIN)], [target('t1', 'studio'), target('t2', 'studio')])), { text: 'skills in sync · checked 2 d ago', tone: 'good' })
  // A difference on one target is reported even while another is unchecked.
  assert.deepEqual(line(syncWith([job('a', 'a1', 't1', 'preview', 'awaiting-approval', 1, NOW - MIN)], [target('t1', 'studio'), target('t2', 'studio')])), { text: '1 skill out of date', tone: 'warn' })
  assert.deepEqual(line(syncWith([job('j', 'a1', 't1', 'preview', 'awaiting-approval', 0, NOW - 10_000)])), { text: 'skills in sync · checked just now', tone: 'good' })
})

test('group by folder project from projectId', () => {
  const projects = [project('p1', 'site'), project('p2', 'docs')]
  const sessions = [session('s1', 'a1', 'running', { projectId: 'p1' }), session('s2', 'a2', 'running', { projectId: 'p1' }), session('s3', 'a1', 'running', { projectId: 'p2' }),
    session('s4', 'a1', 'running', { projectId: 'gone' }), session('s5', 'a1', 'running', { projectId: null }), session('s6', 'a1')]
  const owners = sessionOwners(sessions, projects)
  assert.deepEqual(owners.get('s1'), { id: 'p1', name: 'site' })
  assert.deepEqual(owners.get('s3'), { id: 'p2', name: 'docs' })
  assert.equal(owners.has('s4'), false, 'a project no longer on the account owns nothing')
  assert.equal(owners.has('s5'), false)
  assert.equal(owners.has('s6'), false)
  const machines = [machine('a1', 'studio'), machine('a2', 'laptop')]
  const groups = inFlight(sessions, owners, machines, 'project', NOW)
  assert.deepEqual(groups.map(g => [g.label, g.sessions.map(s => s.id).sort()]), [['docs', ['s3']], ['site', ['s1', 's2']], ['No project · studio', ['s4', 's5', 's6']]])
  assert.equal(groups[1].count, 2)
  // A rename shows at once: the name comes from the project, not from the session.
  assert.equal(sessionOwners(sessions, [project('p1', 'website')]).get('s1').name, 'website')
})

test('session outside every project groups under its machine', () => {
  const machines = [machine('a2', 'laptop'), machine('a1', 'studio')]
  const sessions = [session('s1', 'a1'), session('s2', 'a2'), session('s3', 'a9')]
  const groups = inFlight(sessions, new Map(), machines, 'project', NOW)
  assert.deepEqual(groups.map(g => [g.label, g.sessions.map(s => s.id)]), [['No project · laptop', ['s2']], ['No project · studio', ['s1']], ['No project · a9', ['s3']]])
  const byMachine = inFlight(sessions, new Map([['s1', { id: 'p1', name: 'Site' }]]), machines, 'machine', NOW)
  assert.deepEqual(byMachine.map(g => [g.label, g.sessions.map(s => s.id)]), [['laptop', ['s2']], ['studio', ['s1']], ['a9', ['s3']]])
  const byState = inFlight([session('s1', 'a1', 'running'), session('s2', 'a1', 'exited', { exitCode: 2 }), session('s3', 'a1', 'running', { updatedAt: NOW - MIN })], new Map(), machines, 'state', NOW)
  assert.deepEqual(byState.map(g => [g.label, g.sessions.map(s => s.id)]), [['Running', ['s3', 's1']], ['Exited', ['s2']]])
})

test('finished leaves In flight after 24 h', () => {
  const sessions = [
    session('live-old', 'a1', 'running', { updatedAt: NOW - 30 * HOUR }),
    session('exited-recent', 'a1', 'exited', { exitCode: 0, updatedAt: NOW - 23 * HOUR }),
    session('exited-old', 'a1', 'exited', { exitCode: 0, updatedAt: NOW - 24 * HOUR - 1 }),
    session('ended-old', 'a1', 'ended', { updatedAt: NOW - 25 * HOUR }),
    session('lost-old', 'a1', 'lost', { updatedAt: NOW - 48 * HOUR }),
    session('lost-recent', 'a1', 'lost', { updatedAt: NOW - HOUR }),
  ]
  const ids = inFlight(sessions, new Map(), [machine('a1', 'studio')], 'machine', NOW).flatMap(g => g.sessions.map(s => s.id))
  assert.deepEqual(ids.sort(), ['exited-recent', 'live-old', 'lost-recent'])
})

test('branch in the subtitle from worktreeBranch', () => {
  const studio = machine('a1', 'studio')
  const owners = sessionOwners([session('s1', 'a1', 'running', { projectId: 'p1' })], [project('p1', 'site')])
  assert.equal(rowSubtitle(session('s1', 'a1', 'running', { projectId: 'p1', worktreeBranch: 'reet/site-abc' }), owners.get('s1'), studio), 'site · studio · reet/site-abc')
  assert.equal(rowSubtitle(session('s1', 'a1', 'running', { projectId: 'p1' }), owners.get('s1'), studio), 'site · studio')
  assert.equal(rowSubtitle(session('s1', 'a1'), undefined, studio), 'studio')
  assert.equal(rowSubtitle(session('s1', 'a1', 'running', { worktreeBranch: 'reet/x-1' }), undefined, studio), 'studio · reet/x-1')
  assert.equal(rowSubtitle(session('s1', 'a9'), undefined, undefined), 'a9')
})

test('state word names its source', () => {
  const said = s => { const w = stateWord(s); return `${w.word} · ${w.source}` }
  assert.equal(said(session('s', 'a', 'running')), "Running · from the session's state")
  assert.equal(said(session('s', 'a', 'creating')), "Starting · from the session's state")
  assert.equal(said(session('s', 'a', 'end_requested')), "Ending · from the session's state")
  assert.equal(said(session('s', 'a', 'ended')), "Ended · from the session's state")
  assert.equal(said(session('s', 'a', 'exited', { exitCode: 2 })), 'Exited 2 · from the exit code')
  assert.equal(said(session('s', 'a', 'exited', { exitCode: 0 })), 'Exited 0 · from the exit code')
  assert.equal(said(session('s', 'a', 'exited')), "Exited · from the session's state")
  assert.equal(stateWord(session('s', 'a', 'lost')).word, 'Connection lost')
  assert.equal(stateWord(session('s', 'a', 'exited', { exitCode: 2 })).tone, 'bad')
  assert.equal(stateWord(session('s', 'a', 'exited', { exitCode: 0 })).tone, 'muted')
  assert.equal(stateWord(session('s', 'a', 'running')).tone, 'good')
  assert.equal(stateWord(session('s', 'a', 'lost')).tone, 'warn')
  for (const s of ['creating', 'running', 'end_requested', 'ended', 'exited', 'lost']) assert.ok(stateWord(session('s', 'a', s)).source.startsWith('from the '), s)
})

test('no step text for terminal sessions', () => {
  const machines = [machine('a1', 'studio')]
  const sessions = ['creating', 'running', 'end_requested', 'ended', 'exited', 'lost'].map((s, i) => session(`s${i}`, 'a1', s, { exitCode: s === 'exited' ? 1 : null }))
  for (const groupBy of ['project', 'machine', 'state']) {
    for (const group of inFlight(sessions, new Map(), machines, groupBy, NOW)) {
      for (const row of group.sessions) {
        const text = JSON.stringify(row)
        assert.doesNotMatch(text, /\bstep\b|\bsteps\b|\d+ of \d+|progress|percent/i, text)
        assert.equal('progress' in row, false)
        assert.equal('step' in row, false)
      }
    }
  }
  assert.doesNotMatch(source, /Step \d|of \$\{|\bprogress\s*:/, 'home.ts never builds step text')
})

test('needs you is empty and the count is 0', () => {
  const sessions = [session('s1', 'a1', 'exited', { exitCode: 2 }), session('s2', 'a1', 'lost'), session('s3', 'a1', 'running'), session('s4', 'a1', 'exited', { exitCode: 0 })]
  const cards = needsYou(sessions, NOW)
  assert.deepEqual(cards, [])
  assert.equal(attentionCount(cards), 0)
  assert.equal(attentionCount([{ kind: 'x' }, { kind: 'y' }]), 2)
})

test('a waiting chat card carries its earliest approval, and a terminal card carries none', () => {
  const pending = { permissionId: 'p1', title: 'go test', command: 'go test ./...', path: '', truncated: false, queued: 2 }
  const cards = needsYou([
    session('c1', 'a1', 'running', { attention: 'waiting', attentionSource: 'chat', bellAt: NOW - MIN, changedSinceSeen: true, pendingPermission: { permissionId: 'p1', title: 'go test', command: 'go test ./...', queued: 2 } }),
    session('t1', 'a1', 'running', { attention: 'waiting', attentionSource: 'terminal', bellAt: NOW - 2 * MIN, changedSinceSeen: true }),
    session('c2', 'a1', 'running', { attention: 'waiting', attentionSource: 'chat', bellAt: NOW - 3 * MIN, changedSinceSeen: true, pendingPermission: { permissionId: 'p9', truncated: true } }),
  ], NOW)
  assert.deepEqual(cards.find(c => c.sessionId === 'c1').approval, pending)
  assert.equal(cards.find(c => c.sessionId === 't1').approval, undefined)
  assert.deepEqual(cards.find(c => c.sessionId === 'c2').approval, { permissionId: 'p9', title: 'A tool call', command: '', path: '', truncated: true, queued: 0 })
})

test('bell count equals queue length', () => {
  const owners = new Map([['s1', { id: 'p1', name: 'site' }]])
  const machines = [machine('a1', 'studio')]
  const sessions = [
    session('s1', 'a1', 'running', { attention: 'waiting', attentionSource: 'terminal', bellAt: NOW - 2 * MIN, changedSinceSeen: true }),
    session('s2', 'a1', 'exited', { exitCode: 2, attention: 'failed', attentionSource: 'terminal', updatedAt: NOW - 5 * MIN, changedSinceSeen: true }),
    session('s3', 'a1', 'exited', { exitCode: 0, attention: 'finished', attentionSource: 'terminal', updatedAt: NOW - 10 * MIN, changedSinceSeen: true, ...tree('web') }),
    session('s4', 'a1', 'exited', { exitCode: 0, attention: 'finished', attentionSource: 'terminal', updatedAt: NOW - 15 * MIN, changedSinceSeen: false }),
    session('s4b', 'a1', 'running', { attention: 'waiting', attentionSource: 'chat', bellAt: NOW - 3 * MIN }),
    session('s5', 'a1', 'running', { attention: 'working', attentionSource: 'terminal', changedSinceSeen: true }),
  ]
  const cards = needsYou(sessions, NOW, owners, machines)
  assert.deepEqual(cards.map(c => [c.sessionId, c.kind, c.label, c.where, c.age, c.markSeen]), [
    ['s1', 'waiting', 'Waiting for you', 'site · studio', '2m', false],
    ['s2', 'failed', 'Exited with an error', 'studio', '5m', true],
    ['s3', 'review', 'Ready to review', 'studio', '10m', true],
  ])
  assert.equal(attentionCount(cards), 3)
  assert.deepEqual(stateWord(sessions[0]), { word: 'Waiting for you', source: 'from terminal activity', tone: 'warn' })
  assert.deepEqual(stateWord(sessions[5]), { word: 'Working', source: 'from terminal activity', tone: 'good' })
  assert.deepEqual(stateWord(session('s6', 'a1', 'running', { attention: 'quiet', attentionSource: 'terminal' })), { word: 'Quiet', source: 'from terminal activity', tone: 'muted' })
  assert.deepEqual(stateWord(session('s7', 'a1', 'running', { attention: 'waiting', attentionSource: 'chat' })), { word: 'Waiting for you', source: 'from the chat', tone: 'warn' })
})

test('announce at most once a minute', () => {
  const last = new Map()
  const card = { kind: 'waiting', sessionId: 's1', title: 'Add browse-v1', where: 'site · studio' }
  assert.equal(announceWaiting([card], new Set(), last, NOW), 'Add browse-v1 is waiting for you (site · studio).')
  assert.equal(announceWaiting([card], new Set(['s1']), last, NOW + 5_000), '')
  assert.equal(announceWaiting([card], new Set(), last, NOW + 30_000), '')
  assert.equal(announceWaiting([card], new Set(), last, NOW + MIN), 'Add browse-v1 is waiting for you (site · studio).')
})

test('no record word claims health or verification', () => {
  assert.doesNotMatch(source, FORBIDDEN)
  const machines = [machine('a1', 'studio'), machine('a2', 'off', { state: 'unreachable', cpu: null })]
  const sessions = ['creating', 'running', 'end_requested', 'ended', 'exited', 'lost'].map((s, i) => session(`s${i}`, i % 2 ? 'a1' : 'a2', s))
  const out = JSON.stringify([fleetCards(machines, sessions, syncWith([job('j', 'a1', 't1', 'preview', 'awaiting-approval', 0, NOW - MIN)]), NOW), ...['project', 'machine', 'state'].map(g => inFlight(sessions, new Map(), machines, g, NOW)), sessions.map(s => stateWord(s))])
  assert.doesNotMatch(out, FORBIDDEN)
})

test('empty lists give no cards and no groups', () => {
  assert.deepEqual(fleetCards([], [], emptySync(), NOW), [])
  for (const groupBy of ['project', 'machine', 'state']) assert.deepEqual(inFlight([], new Map(), [], groupBy, NOW), [])
  assert.deepEqual(inFlight([], new Map(), [machine('a1', 'studio')], 'machine', NOW), [], 'a machine with no sessions makes no group')
})

test('an offline machine keeps its live sessions in In flight and on its card', () => {
  const machines = [machine('a1', 'studio'), machine('a2', 'desk', { state: 'unreachable', fresh: false })]
  const sessions = [session('s1', 'a2', 'running'), session('s2', 'a2', 'creating'), session('s3', 'a1', 'running')]
  const cards = fleetCards(machines, sessions, emptySync(), NOW)
  assert.equal(cards[1].status, 'offline')
  assert.equal(cards[1].sessionsRunning, 2, 'the last known count is kept, not zeroed')
  for (const groupBy of ['project', 'machine']) {
    const desk = inFlight(sessions, new Map(), machines, groupBy, NOW).find(g => g.key === 'machine:a2')
    assert.deepEqual(desk.sessions.map(s => s.id).sort(), ['s1', 's2'], groupBy)
  }
})

test('a check made for an older desired revision is not counted', () => {
  const t = { ...target('t1', 'studio'), revision: 'r2' }
  // The only finished check was made before the desired skills changed: nothing is known about the current set.
  assert.deepEqual(fleetCards([machine('a1', 'studio')], [], syncWith([job('j1', 'a1', 't1', 'preview', 'awaiting-approval', 0, NOW - HOUR)], [t]), NOW)[0].skills, { text: 'Not checked', tone: 'muted' })
  // A newer check for the current revision is what the line reports, even when an older one found differences.
  const current = { ...job('j2', 'a1', 't1', 'preview', 'awaiting-approval', 0, NOW - 2 * HOUR), revision: 'r2' }
  const stale = job('j3', 'a1', 't1', 'preview', 'awaiting-approval', 3, NOW - MIN)
  assert.deepEqual(fleetCards([machine('a1', 'studio')], [], syncWith([current, stale], [t]), NOW)[0].skills, { text: 'skills in sync · checked 2 h ago', tone: 'good' })
})

test('a project counts only its own live sessions as running, and its worktrees in use', () => {
  const sessions = [session('s1', 'a1', 'running', { projectId: 'p1', ...tree('a') }), session('s2', 'a1', 'exited', { exitCode: 0, projectId: 'p1', ...tree('b') }),
    session('s3', 'a1', 'creating', { projectId: 'p1' }), session('s4', 'a1', 'running'), session('s5', 'a1', 'end_requested', { projectId: 'p2' })]
  assert.deepEqual([...h.projectRunning(sessions)], [['p1', 2]])
  assert.deepEqual([...h.projectWorktrees(sessions)], [['p1', 1]])
})

test('rail state in words: running, machine offline or idle', () => {
  assert.deepEqual(h.railState(2, true), { text: '2 running', tone: 'good' })
  assert.deepEqual(h.railState(1, false), { text: '1 running', tone: 'good' }, 'a last known count is still the count')
  assert.deepEqual(h.railState(0, false), { text: 'machine offline', tone: 'warn' })
  assert.deepEqual(h.railState(0, true), { text: 'idle', tone: 'muted' })
  assert.deepEqual(h.railState(0, undefined), { text: 'idle', tone: 'muted' }, 'an unknown machine is not called offline')
})

test('rail state says nothing about running or idle when the sessions were not read', () => {
  assert.equal(h.railState(null, true), null, 'no false "idle" without a sessions read')
  assert.equal(h.railState(null, undefined), null)
  assert.deepEqual(h.railState(null, false), { text: 'machine offline', tone: 'warn' }, 'the machine word comes from health, not sessions')
})

test('short age', () => {
  assert.deepEqual([0, 59_000, 12 * MIN, 3 * HOUR, 49 * HOUR].map(h.shortAge), ['now', 'now', '12m', '3h', '2d'])
})


 test('fleet OS version comes only from machine capabilities', () => {
  const m=machine('a1','studio')
  m.capabilities.osVersion='6.1'
  assert.equal(fleetCards([m], [], emptySync(), NOW)[0].osVersion,'6.1')
  delete m.capabilities.osVersion
  assert.equal(fleetCards([m], [], emptySync(), NOW)[0].osVersion,'')
 })

// The projects the Sessions screen's rail hands to Home's launch bar in the address (`?projects=<id>,<id>`): only ids that
// name a project in the list, each once, in rail order, at most 12 (a start's limit).
test('handed projects: known ids only, once each, in rail order, at most 12', () => {
  const list = Array.from({ length: 15 }, (_, i) => ({ id: `p${i}` }))
  assert.deepEqual(h.handedProjects('p3,p1', list), ['p1', 'p3'])
  assert.deepEqual(h.handedProjects('p2', list), ['p2'])
  assert.deepEqual(h.handedProjects('nope,p4,,gone', list), ['p4'])
  assert.deepEqual(h.handedProjects('p5,p5,p0,p5', list), ['p0', 'p5'])
  assert.deepEqual(h.handedProjects('x1,x2,x3,x4,x5,x6,x7,x8,x9,x10,x11,x12,p14,p13', list), ['p13', 'p14'])
  assert.deepEqual(h.handedProjects(list.map(p => p.id).reverse().join(','), list), list.slice(0, 12).map(p => p.id))
  assert.deepEqual(h.handedProjects(null, list), [])
  assert.deepEqual(h.handedProjects('', list), [])
})
