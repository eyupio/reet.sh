import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
const { outputText } = ts.transpileModule(readFileSync(new URL('./syncOverview.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const {
  machineSyncOverview, syncJobStages, jobPlanCounts, savedInstallTargets, fallbackInstallTarget, pickSyncCandidate, agoLabel,
} = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const now = Date.now()
const skill = (id, source = 'org/repo') => ({ id, name: id, slug: id, source, sourceType: source ? 'github' : 'local' })
const group = (id, skills = [], changes = {}) => ({ id, name: id, agent: 'claude', host: 'laptop', lastSeen: now, scannedAt: now, complete: true, skills, ...changes })
const machine = (groups, host = 'laptop') => ({ host, groups, skillCount: 1 })
const target = (id, missing = [], extra = [], changes = {}) => ({
  target: { id, host: 'laptop', agent: 'claude' },
  sourceId: 'shelf', sourceName: 'Shelf', desired: [], revision: 'r1', appliedRevision: 'r1', updatedAt: now,
  mode: 'additive', missing, extra, ...changes,
})
const state = (changes = {}) => ({ agents: [{ id: 'agent-1', host: 'laptop', lastSeen: now, paused: false }], jobs: [], runs: [], targets: [], environments: [], machines: [], ...changes })

test('machine band names the next useful action instead of a pile of controls', () => {
  const ready = machineSyncOverview(machine([group('a', [skill('one')])]), state({ targets: [target('a', [skill('two')])], environments: [group('a')] }), now)
  assert.equal(ready.tone, 'action')
  assert.equal(ready.label, '1 skill to install')
  assert.equal(ready.canSync, true)
})

test('a synced machine has no filled primary action', () => {
  const synced = machineSyncOverview(machine([group('a', [skill('one')])]), state({ targets: [target('a')], environments: [group('a')] }), now)
  assert.equal(synced.tone, 'synced')
  assert.equal(synced.label, 'Everything in sync')
  assert.equal(synced.canSync, false)
})

test('an active run wins over counts and exposes live progress', () => {
  const run = { id: 'run-1', agentId: 'agent-1', targets: ['a', 'b'], cursor: 1, status: 'running' }
  const overview = machineSyncOverview(machine([group('a')]), state({ runs: [run], targets: [target('a', [skill('two')])], environments: [group('a')] }), now)
  assert.equal(overview.tone, 'working')
  assert.deepEqual(overview.progress, { done: 1, total: 2 })
  assert.equal(overview.canSync, false)
})

test('review, stale and unconnected machines each say what is actually wrong', () => {
  const review = machineSyncOverview(machine([group('a')]), state({ jobs: [{ id: 'j', agentId: 'agent-1', kind: 'auto', status: 'awaiting-approval', updatedAt: now }] }), now)
  assert.equal(review.tone, 'review')
  const stale = machineSyncOverview(machine([group('a', [], { scannedAt: now - 15 * 86400000 })]), state({ targets: [target('a')], environments: [group('a', [], { scannedAt: now - 15 * 86400000 })] }), now)
  assert.equal(stale.tone, 'attention')
  assert.equal(stale.label, 'Rescan needed')
  const unconnected = machineSyncOverview(machine([group('a')]), state({ agents: [] }), now)
  assert.equal(unconnected.tone, 'setup')
  assert.match(unconnected.href, /connect-machine/)
})

test('add destinations reuse fresh saved scopes and never reuse stale ones', () => {
  const fresh = savedInstallTargets(machine([group('a')]), state({ targets: [target('a')], environments: [group('a')] }), now)
  assert.equal(fresh.length, 1)
  assert.equal(fresh[0].targetId, 'a')
  const stale = savedInstallTargets(machine([group('a')]), state({ targets: [target('a')], environments: [group('a', [], { scannedAt: now - 15 * 86400000 })] }), now)
  assert.equal(stale.length, 0)
})

test('a list-backed sync set keeps its list for candidates and saves as a shelf selection', () => {
  const [destination] = savedInstallTargets(machine([group('a')]), state({ targets: [target('a', [], [], { sourceId: 'list:7' })], environments: [group('a')] }), now)
  assert.equal(destination.sourceId, 'shelf')
  assert.equal(destination.candidateKey, 'list:7')
  assert.equal(destination.listId, 7)
})

test('a first install prefers the shared agents scope and stays additive', () => {
  const fallback = fallbackInstallTarget({ host: 'laptop', groups: [group('claude-scope'), group('agents-scope', [], { agent: 'agents' })] }, state(), now)
  assert.equal(fallback.targetId, 'agents-scope')
  assert.equal(fallback.mode, 'additive')
  assert.equal(fallback.sourceId, 'shelf')
})

test('only complete linked candidates can be installed', () => {
  const linked = skill('one')
  const local = skill('one', '')
  assert.equal(pickSyncCandidate([local, linked], linked)?.id, 'one')
  assert.equal(pickSyncCandidate([local], local), null)
})

test('job stages explain queued, planning, applying and failure states', () => {
  const base = { id: 'j', agentId: 'a', targetId: 't', kind: 'apply', status: 'running', revision: 'r', hash: '', plan: {}, message: '', createdAt: now, updatedAt: now }
  assert.deepEqual(syncJobStages({ ...base, status: 'queued', kind: 'auto' }).map(stage => stage.state), ['pending', 'pending', 'pending'])
  assert.deepEqual(syncJobStages({ ...base, status: 'running', kind: 'auto' }).map(stage => stage.state), ['done', 'active', 'pending'])
  assert.deepEqual(syncJobStages({ ...base, status: 'running', kind: 'apply' }).map(stage => stage.state), ['done', 'done', 'active'])
  assert.deepEqual(syncJobStages({ ...base, status: 'awaiting-approval', kind: 'auto' }).map(stage => stage.state), ['done', 'done', 'review'])
  assert.deepEqual(syncJobStages({ ...base, status: 'failed', kind: 'apply' }).map(stage => stage.state), ['done', 'done', 'failed'])
})

test('plan counts and relative time labels are stable', () => {
  const counts = jobPlanCounts({ plan: { actions: [{ add: true, skill: skill('a') }, { add: false, skill: skill('b') }] } })
  assert.deepEqual(counts, { installs: 1, removals: 1, total: 2 })
  assert.equal(agoLabel(now - 5 * 60_000, now), '5 min ago')
  assert.equal(agoLabel(0, now), 'never')
})


test('disconnected machines wait without claiming to install or being ready to sync', () => {
  for (const jobs of [[], [{ id: 'j', agentId: 'agent-1', kind: 'apply', status: 'queued', updatedAt: now }]]) {
    const overview = machineSyncOverview(machine([group('a')]), state({
      agents: [{ id: 'agent-1', host: 'laptop', lastSeen: now - 90_000 }],
      jobs, targets: [target('a', [skill('two')])], environments: [group('a')],
    }), now)
    assert.equal(overview.canSync, false)
    assert.equal(overview.label, jobs.length ? 'Waiting for machine' : 'Machine offline')
    assert.match(overview.detail, /Reconnect laptop/)
  }
})

test('a legacy review run with no pending plan does not keep a synced machine busy forever', () => {
  const overview = machineSyncOverview(machine([group('a')]), state({
    runs: [{ id: 'old', agentId: 'agent-1', targets: ['a'], cursor: 1, status: 'review' }],
    targets: [target('a')], environments: [group('a')], jobs: [],
  }), now)
  assert.equal(overview.tone, 'synced')
})
