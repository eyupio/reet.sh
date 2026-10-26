import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = readFileSync(new URL('./home.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const h = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
const { fleetCards, sessionProjects, inFlight, rowSubtitle, stateWord, needsYou, attentionCount } = h

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
const project = (id, name) => ({ id, name, defaultTool: 'claude', defaultModel: '', useWorktrees: true, hasSharedPrompt: false, members: [], createdAt: 1, updatedAt: 1, version: 1 })
const run = (id, projectId, sessionIds) => ({ id, projectId, projectName: 'old name', state: 'running', startedAt: NOW - HOUR, useWorktrees: false, worktreeProject: '', members: sessionIds.map((sessionId, position) => ({ position, host: 'a1', dir: '~/x', tool: 'claude', model: '', state: 'started', code: '', sentence: '', sessionId, worktreePath: '', worktreeRepo: '', worktreeBranch: '', worktreeRun: '', worktreeProject: '', sharesCheckout: true, attempt: 1 })) })
const FORBIDDEN = /\b(healthy|verified)\b/i

// Step 1 finding: the session listing carries worktreePath but no removal record (a removal moves only the Project
// run member to "removed"), and it drops archived sessions and stops at 100, so it cannot say "created by reet on that
// machine and not removed". The card omits the figure rather than show a guess or a zero.
test('worktree figure omitted without a source', () => {
  const sessions = [session('s1', 'a1', 'running', { worktreePath: '/home/me/.reet/worktrees/site/web/abc', worktreeRepo: 'web', worktreeBranch: 'reet/site-abc' }), session('s2', 'a1', 'exited', { exitCode: 0, worktreePath: '/home/me/.reet/worktrees/site/api/def' })]
  for (const card of [...fleetCards([machine('a1', 'studio')], sessions, emptySync(), NOW), ...fleetCards([machine('a2', 'empty')], [], emptySync(), NOW)]) {
    assert.equal('worktrees' in card, false, 'no worktree key without a record of removal')
    assert.equal(card.worktrees, undefined)
  }
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

test('cards keep the machine order, name, OS and connection word', () => {
  const cards = fleetCards([machine('a2', 'laptop', { os: 'darwin' }), machine('a1', 'studio'), machine('a3', 'pi', { state: 'unknown', connection: { connected: false, reportFresh: false, missedProbes: 2, reconnects: 1, path: 'direct' } }), machine('a4', 'gone', { state: 'unreachable' }), machine('a5', 'new', { state: 'never_connected', connection: null })], [], emptySync(), NOW)
  assert.deepEqual(cards.map(c => [c.agentId, c.name, c.os, c.status]), [['a2', 'laptop', 'darwin', 'online'], ['a1', 'studio', 'linux', 'online'], ['a3', 'pi', 'linux', 'reconnecting'], ['a4', 'gone', 'linux', 'offline'], ['a5', 'new', 'linux', 'offline']])
})

test('sessions running counts live sessions of any kind; finished and exited are not counted', () => {
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
  ]
  const [studio, laptop, idle] = fleetCards([machine('a1', 'studio'), machine('a2', 'laptop'), machine('a3', 'idle')], sessions, emptySync(), NOW)
  assert.equal(studio.sessionsRunning, 4)
  assert.equal(laptop.sessionsRunning, 1)
  assert.equal(idle.sessionsRunning, 0)
  // The listing cannot say which sessions are agents, so the card never claims agents.
  for (const card of [studio, laptop, idle]) assert.equal('agentsRunning' in card, false)
  assert.equal(h.agentSessionIds, undefined)
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

test('session from a Project\'s run groups under that Project', () => {
  const owners = sessionProjects([project('p1', 'Site'), project('p2', 'Docs')], [run('r1', 'p1', ['s1', 's2']), run('r2', 'p2', ['s3']), run('r3', 'gone', ['s4'])])
  assert.deepEqual(owners.get('s1'), { id: 'p1', name: 'Site' })
  assert.deepEqual(owners.get('s3'), { id: 'p2', name: 'Docs' })
  assert.equal(owners.has('s4'), false, 'a run of a Project not in the list owns nothing')
  assert.equal(owners.has(''), false)
  const machines = [machine('a1', 'studio'), machine('a2', 'laptop')]
  const sessions = [session('s1', 'a1'), session('s2', 'a2'), session('s3', 'a1'), session('s5', 'a1')]
  const groups = inFlight(sessions, owners, machines, 'project', NOW)
  assert.deepEqual(groups.map(g => [g.label, g.sessions.map(s => s.id)]), [['Docs', ['s3']], ['Site', ['s1', 's2']], ['No project · studio', ['s5']]])
  assert.equal(groups[1].count, 2)
})

test('session outside every Project groups under its machine', () => {
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

test('row subtitle names project and machine, and the branch only when the listing carries it', () => {
  const studio = machine('a1', 'studio')
  assert.equal(rowSubtitle(session('s1', 'a1'), { id: 'p1', name: 'Site' }, studio), 'Site · studio')
  assert.equal(rowSubtitle(session('s1', 'a1'), undefined, studio), 'studio')
  assert.equal(rowSubtitle(session('s1', 'a1', 'running', { worktreeBranch: 'reet/site-abc' }), { id: 'p1', name: 'Site' }, studio), 'Site · studio · reet/site-abc')
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

test('no record word claims health or verification', () => {
  assert.doesNotMatch(source, FORBIDDEN)
  const machines = [machine('a1', 'studio'), machine('a2', 'off', { state: 'unreachable', cpu: null })]
  const sessions = ['creating', 'running', 'end_requested', 'ended', 'exited', 'lost'].map((s, i) => session(`s${i}`, i % 2 ? 'a1' : 'a2', s))
  const out = JSON.stringify([fleetCards(machines, sessions, syncWith([job('j', 'a1', 't1', 'preview', 'awaiting-approval', 0, NOW - MIN)]), NOW), ...['project', 'machine', 'state'].map(g => inFlight(sessions, new Map(), machines, g, NOW)), sessions.map(s => stateWord(s))])
  assert.doesNotMatch(out, FORBIDDEN)
})
