import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
const transpile = file => ts.transpileModule(readFileSync(new URL(file, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const dataURL = source => `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`
const machinesURL = dataURL(transpile('./machines.ts'))
const overviewURL = dataURL(transpile('./syncOverview.ts'))
const activityURL = dataURL(transpile('./syncActivity.ts').replace("from './machines'", `from '${machinesURL}'`).replace("from './syncOverview'", `from '${overviewURL}'`))
const { syncActivityOverview } = await import(activityURL)

const now = Date.now()
const agent = (id, host) => ({ id, host, version: 'test', lastSeen: now, paused: false, autoUpdate: false })
const target = (id, host, name) => ({ target: { id, host, agent: name, name: host }, sourceId: 'shelf', sourceName: 'Shelf', desired: [], missing: [], extra: [], revision: 'r', updatedAt: now, mode: 'additive' })
const job = changes => ({ id: 'job', agentId: 'a1', targetId: 't1', kind: 'preview', status: 'failed', revision: 'r', hash: 'h', plan: {}, message: 'cannot read target skills directory', createdAt: now - 1000, updatedAt: now - 1000, ...changes })
const view = {
  agents: [agent('a1', 'laptop'), agent('a2', 'build')],
  targets: [target('t1', 'laptop', 'opencode'), target('t2', 'laptop', 'claude'), target('t3', 'build', 'agents')],
  environments: [],
}

test('identical failures on one machine become one entry with every scope', () => {
  const overview = syncActivityOverview([job({ id: 'one' }), job({ id: 'two', targetId: 't2' })], view)
  const laptop = overview.hosts.find(host => host.host === 'laptop')
  assert.equal(overview.total, 2)
  assert.equal(overview.failed, 2)
  assert.equal(laptop.entries.length, 1)
  assert.equal(laptop.entries[0].jobs.length, 2)
  assert.deepEqual(laptop.entries[0].scopes, ['OpenCode', 'Claude Code'])
})

test('a different cause, machine or kind stays a separate entry', () => {
  const overview = syncActivityOverview([
    job({ id: 'one' }),
    job({ id: 'two', message: 'remote apply disabled locally' }),
    job({ id: 'three', agentId: 'a2', targetId: 't3' }),
    job({ id: 'four', kind: 'apply' }),
  ], view)
  assert.equal(overview.total, 4)
  assert.equal(overview.hosts.find(host => host.host === 'laptop').entries.length, 3)
  assert.equal(overview.hosts.find(host => host.host === 'build').entries.length, 1)
})

test('a job id reported twice is counted once', () => {
  const overview = syncActivityOverview([job({ id: 'same' }), job({ id: 'same', updatedAt: now })], view)
  assert.equal(overview.total, 1)
  assert.equal(overview.failed, 1)
})

test('running work comes before failed work, and both come before finished history', () => {
  const overview = syncActivityOverview([
    job({ id: 'done', status: 'succeeded' }),
    job({ id: 'failed', agentId: 'a2', targetId: 't3' }),
    job({ id: 'running', status: 'running', agentId: 'a1', targetId: 't2' }),
  ], view)
  assert.equal(overview.hosts[0].host, 'laptop')
  assert.equal(overview.hosts[0].entries[0].job.id, 'running')
  assert.equal(overview.hosts[1].host, 'build')
})

test('review, done and cancelled work keep honest counts', () => {
  const overview = syncActivityOverview([
    job({ id: 'review', status: 'awaiting-approval' }),
    job({ id: 'done', status: 'succeeded', kind: 'apply', targetId: 't2' }),
    job({ id: 'cancelled', status: 'cancelled', agentId: 'a2', targetId: 't3' }),
  ], view)
  assert.equal(overview.review, 1)
  assert.equal(overview.done, 1)
  assert.equal(overview.cancelled, 1)
  assert.equal(overview.hosts.find(host => host.host === 'build').entries[0].cancelled, true)
})

test('host names that differ only by case remain one machine', () => {
  const caseView = { ...view, agents: [agent('a1', 'Laptop'), agent('a2', 'laptop')] }
  const overview = syncActivityOverview([
    job({ id: 'one', agentId: 'a1', targetId: 't1' }),
    job({ id: 'two', agentId: 'a2', targetId: 't2' }),
  ], caseView)
  assert.equal(overview.hosts.length, 1)
  assert.equal(overview.hosts[0].entries.length, 1)
  assert.equal(overview.failed, 2)
})

test('an update job with no target still belongs to its machine', () => {
  const overview = syncActivityOverview([job({ id: 'update', kind: 'update', targetId: '', status: 'succeeded' })], view)
  assert.equal(overview.hosts[0].host, 'laptop')
  assert.deepEqual(overview.hosts[0].entries[0].scopes, [])
  assert.equal(overview.done, 1)
})

test('a newer retry moves the old failure to history without losing its evidence', () => {
  for (const status of ['queued', 'running', 'awaiting-approval', 'succeeded']) {
    const overview = syncActivityOverview([
      job({ id: 'failed', kind: 'apply', createdAt: now - 2000 }),
      job({ id: 'retry', kind: 'auto', status, createdAt: now, updatedAt: now }),
    ], view)
    assert.equal(overview.failed, 0)
    const old = overview.hosts[0].entries.find(entry => entry.job.id === 'failed')
    assert.equal(old.resolved, true)
    assert.equal(old.superseded, true)
    assert.equal(overview.total, 2)
  }
})

test('a scan or a retry on another scope does not resolve an apply failure', () => {
  const overview = syncActivityOverview([
    job({ id: 'failed', kind: 'apply', createdAt: now - 2000 }),
    job({ id: 'scan', kind: 'scan', status: 'succeeded', createdAt: now }),
    job({ id: 'other', targetId: 't2', status: 'succeeded', createdAt: now }),
  ], view)
  assert.equal(overview.failed, 1)
})
