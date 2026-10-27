import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
// Compile the pure comparison helper without a browser or an extra test dependency.
const { outputText } = ts.transpileModule(readFileSync(new URL('./machines.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { inventoryMachines, compareInventories, inventory, agentGroups, installationReports, installationStatus, agentBuild, agentUpdateTarget, latestRelease, updateAvailable, versionLess, updateJobFor, updateProgress, agentUpdateState, updateStateLabel, updateStepsShown, updateAction, channelBuild } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
const now = Date.now()
const skill = (id, source = 'org/repo') => ({ id, name: id, source, sourceType: source ? 'github' : 'local' })
const group = (id, skills, changes = {}) => ({ id, agent: 'claude', skills, complete: true, scannedAt: now, ...changes })
test('a machine inventory deduplicates skills across its agents', () => {
  assert.equal(inventory([group('claude', [skill('a')]), group('agents', [skill('a')])]).length, 1)
  assert.equal(agentGroups({ groups: [group('a', []), group('server', [], { agent: 'reet-server' })] }).length, 1)
})
test('comparison preserves identity/source differences and direction', () => {
  const diff = compareInventories([group('left', [skill('a'), skill('b')])], [group('right', [skill('a'), skill('c')])], now)
  assert.deepEqual(diff.common.map(s => s.id), ['a'])
  assert.deepEqual(diff.leftOnly.map(s => s.id), ['b'])
  assert.deepEqual(diff.rightOnly.map(s => s.id), ['c'])
  assert.equal(diff.confirmed, true)
})
test('stale, incomplete or missing agent scans never confirm sync', () => {
  for (const left of [[], [group('a', [], { complete: false })], [group('a', [], { scannedAt: now - 15 * 86400000 })]]) {
    assert.equal(compareInventories(left, [group('b', [])], now).confirmed, false)
  }
})
test('unknown sources require review even if names/ids match', () => {
  assert.equal(compareInventories([group('a', [skill('local', '')])], [group('b', [skill('local', '')])], now).unknownSources, true)
})
test('installation badges distinguish a fresh positive report from a confirmed absence', () => {
  const [report] = installationReports([{ host: 'laptop', groups: [group('claude', [skill('a')]), group('agents', [skill('a')])] }], now)
  assert.equal(report.installed.size, 1)
  assert.equal(installationStatus(report, 'a'), 'installed')
  assert.equal(installationStatus(report, 'b'), 'missing')
  const [empty] = installationReports([{ host: 'empty', groups: [group('agents', [])] }], now)
  assert.equal(installationStatus(empty, 'a'), 'missing')
})
test('stale, incomplete and heartbeat-only machines never claim a skill is missing', () => {
  for (const groups of [[], [group('old', [skill('a')], { scannedAt: now - 15 * 86400000 })], [group('partial', [skill('a')], { complete: false })], [group('server', [], { agent: 'reet-server' })]]) {
    const [report] = installationReports([{ host: 'unconfirmed', groups }], now)
    assert.equal(installationStatus(report, 'a'), 'unknown')
    assert.equal(installationStatus(report, 'b'), 'unknown')
  }
})
test('one fresh installation confirms presence but mixed freshness cannot confirm absence', () => {
  const [report] = installationReports([{ host: 'mixed', groups: [group('fresh', [skill('a')]), group('old', [skill('b')], { scannedAt: now - 15 * 86400000 })] }], now)
  assert.equal(installationStatus(report, 'a'), 'installed')
  assert.equal(installationStatus(report, 'b'), 'unknown')
  assert.equal(installationStatus(report, 'c'), 'unknown')
})

test('shelf environments exclude server-only heartbeats but retain empty and stale agent inventories', () => {
  const machines = [
    { host: 'container', groups: [group('server', [], { agent: 'reet-server' })] },
    { host: 'heartbeat', groups: [] },
    { host: 'empty-laptop', groups: [group('claude', [])] },
    { host: 'old-laptop', groups: [group('claude', [], { scannedAt: now - 15 * 86400000 })] },
    { host: 'server-with-skills', groups: [group('server', [], { agent: 'reet-server' }), group('claude', [skill('a')])] },
  ]
  assert.deepEqual(inventoryMachines(machines).map(machine => machine.host), ['empty-laptop', 'old-laptop', 'server-with-skills'])
  assert.equal(machines.length, 5)
})


test('agent builds show a short commit or say it is unknown', () => {
  assert.equal(agentBuild({ version: 'dev', commit: '0123456789abcdef0123456789abcdef01234567' }), 'dev · 0123456789ab')
  assert.equal(agentBuild({ version: 'dev', commit: 'unknown' }), 'dev · commit unknown')
  assert.equal(agentBuild({ version: 'dev' }), 'dev · commit unknown')
  assert.equal(agentBuild({ version: 'v1.4.2', commit: '0123456789abcdef0123456789abcdef01234567' }), 'v1.4.2 · 0123456789ab')
  assert.equal(agentBuild({ version: 'v1.4.2' }), 'v1.4.2')
})

test('dev machines are current only when their commit matches the published pointer', () => {
  const releases = {
    dev: { version: 'dev', commit: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' },
    latest: { version: 'v1.4.2' },
  }
  assert.equal(updateAvailable({ version: 'dev', commit: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' }, releases), false)
  assert.equal(updateAvailable({ version: 'dev', commit: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa' }, releases), true)
  assert.equal(updateAvailable({ version: 'dev', commit: 'unknown' }, releases), true)
  assert.equal(updateAvailable({ version: 'dev' }, releases), true)
  assert.equal(latestRelease({ version: 'dev' }, releases).label, 'dev · bbbbbbbbbbbb')
})

test('release machines compare semantic versions and never downgrade newer builds', () => {
  const releases = { dev: { commit: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' }, latest: { version: 'v1.4.2' } }
  assert.equal(updateAvailable({ version: 'v1.2.9' }, releases), true)
  assert.equal(updateAvailable({ version: 'v1.4.2' }, releases), false)
  assert.equal(updateAvailable({ version: 'v1.5.0' }, releases), false)
  assert.equal(updateAvailable({ version: 'v1.2.3', commit: 'whatever' }, releases), true)
  assert.equal(agentUpdateTarget({ version: 'dev' }), 'dev')
  assert.equal(agentUpdateTarget({ version: 'v1.4.2' }), 'latest')
})

test('unknown or failed channel checks do not claim an update is waiting', () => {
  assert.equal(updateAvailable({ version: 'dev', commit: 'abc' }, undefined), false)
  assert.equal(updateAvailable({ version: 'dev', commit: 'abc' }, { dev: { error: 'offline' } }), false)
  assert.equal(updateAvailable({ version: 'v1.0.0' }, { latest: { error: 'offline' } }), false)
  assert.equal(latestRelease({ version: 'dev' }, { dev: { error: 'offline' } }).error, 'offline')
})

test('semantic version comparison handles prereleases and unparseable builds', () => {
  assert.equal(versionLess('v1.2.3', 'v1.2.4'), true)
  assert.equal(versionLess('v1.2.10', 'v1.2.9'), false)
  assert.equal(versionLess('v2.0.0-rc.1', 'v2.0.0'), true)
  assert.equal(versionLess('not-a-version', 'v1.0.0'), false)
})

test('one machine update is chosen by kind and recency', () => {
  const jobs = [
    { id: 'old', agentId: 'a', kind: 'update', createdAt: 1 },
    { id: 'scan', agentId: 'a', kind: 'scan', createdAt: 5 },
    { id: 'new', agentId: 'a', kind: 'update', createdAt: 9 },
    { id: 'other', agentId: 'b', kind: 'update', createdAt: 99 },
  ]
  assert.equal(updateJobFor(jobs, 'a').id, 'new')
  assert.equal(updateJobFor(undefined, 'a'), undefined)
  assert.equal(updateJobFor(jobs, 'c'), undefined)
})

test('update progress counts only completed stages and never goes backwards on failure', () => {
  const job = (status, stages) => ({ id: 'j', agentId: 'a', kind: 'update', status, stages })
  const running = job('running', [{ key: 'a', status: 'done' }, { key: 'b', status: 'running' }, { key: 'c', status: 'pending' }])
  const progress = updateProgress(running)
  assert.equal(progress.done, 1)
  assert.equal(progress.total, 3)
  assert.ok(progress.value > 0.3 && progress.value < 0.4)
  const failed = updateProgress(job('failed', [{ key: 'a', status: 'done' }, { key: 'b', status: 'failed' }]))
  assert.equal(failed.done, 1)
  assert.equal(failed.value, 0.5)
  assert.deepEqual(updateProgress(undefined), { done: 0, total: 0, value: 0 })
})

test('the machine update state follows the last job, then the published channel', () => {
  const releases = { dev: { version: 'dev', commit: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' }, latest: { version: 'v1.4.2' } }
  const agent = { version: 'v1.2.0' }
  const job = (status) => ({ id: 'j', agentId: 'a', kind: 'update', status, updatedAt: Date.now() })
  assert.equal(agentUpdateState(agent, releases), 'available')
  assert.equal(agentUpdateState({ version: 'v1.4.2' }, releases), 'current')
  assert.equal(agentUpdateState(agent, releases, job('queued')), 'queued')
  assert.equal(agentUpdateState(agent, releases, job('running')), 'running')
  assert.equal(agentUpdateState(agent, releases, job('failed')), 'failed')
  assert.equal(agentUpdateState(agent, releases, job('rolled_back')), 'rolled_back')
  assert.equal(agentUpdateState(agent, releases, job('cancelled')), 'cancelled')
  assert.equal(agentUpdateState(agent, releases, job('succeeded')), 'available')
  assert.equal(agentUpdateState({ version: 'v1.4.2' }, releases, job('succeeded')), 'current')
  assert.equal(agentUpdateState(agent, { latest: { error: 'offline' } }), 'unknown')
  assert.equal(agentUpdateState(agent, undefined), 'unknown')
  assert.equal(updateStateLabel('rolled_back'), 'Rolled back')
})

test('the steps leave the card when an update ends well and fold away when it does not', () => {
  const job = (status) => ({ id: 'j', agentId: 'a', kind: 'update', status, stages: [{ key: 'a', status: 'done' }] })
  // While the agent works, the steps are the only way to see where it is.
  assert.equal(updateStepsShown(job('queued')), 'live')
  assert.equal(updateStepsShown(job('running')), 'live')
  // A finished update is one line of history; the steps would only say what the pill already does.
  assert.equal(updateStepsShown(job('succeeded')), 'hidden')
  assert.equal(updateStepsShown(job('cancelled')), 'hidden')
  // A failure is diagnosed by the step it stopped at, so those stay reachable without staying in the way.
  assert.equal(updateStepsShown(job('failed')), 'folded')
  assert.equal(updateStepsShown(job('rolled_back')), 'folded')
  assert.equal(updateStepsShown(undefined), 'hidden')
  assert.equal(updateStepsShown({ ...job('running'), stages: [] }), 'hidden')
})

test('the card offers a button only when a person has something to do', () => {
  const action = (phase, autoUpdate, updateWaiting = phase === 'available') => updateAction(phase, { autoUpdate, updateWaiting })
  assert.equal(action('current', true), undefined)
  assert.equal(action('current', false), undefined)
  assert.equal(action('running', true), undefined)
  assert.deepEqual(action('queued', true), { intent: 'cancel', emphasis: 'quiet', label: 'Cancel update' })
  // With automatic updates on the controller will install it anyway, so the offer is quiet.
  assert.deepEqual(action('available', true), { intent: 'review', emphasis: 'quiet', label: 'Update now…' })
  assert.deepEqual(action('available', false), { intent: 'review', emphasis: 'primary', label: 'Review update…' })
  assert.deepEqual(action('failed', true), { intent: 'review', emphasis: 'primary', label: 'Try again…' })
  assert.deepEqual(action('rolled_back', false), { intent: 'review', emphasis: 'primary', label: 'Try again…' })
  // A cancelled attempt is history: what matters is whether an update still waits.
  assert.deepEqual(action('cancelled', false, true), { intent: 'review', emphasis: 'primary', label: 'Review update…' })
  assert.equal(action('cancelled', false, false), undefined)
  // The controller could not read the channel, but the agent resolves the release itself, so a deliberate update is still possible.
  assert.deepEqual(action('unknown', true), { intent: 'review', emphasis: 'quiet', label: 'Update reet…' })
})

test('a channel is named by the build it points at, and says nothing it has not checked', () => {
  const releases = { dev: { version: 'dev', commit: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb' }, latest: { version: 'v1.4.2' } }
  assert.equal(channelBuild(releases, 'latest'), 'v1.4.2')
  assert.equal(channelBuild(releases, 'dev'), 'dev · bbbbbbbbbbbb')
  // An old controller publishes the dev pointer without a commit; the channel still has a name.
  assert.equal(channelBuild({ dev: { version: 'dev' } }, 'dev'), 'dev')
  assert.equal(channelBuild({ latest: { version: 'v1.4.2' } }, 'dev'), '')
  assert.equal(channelBuild(undefined, 'latest'), '')
})

