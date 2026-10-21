import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
// Compile the pure comparison helper without a browser or an extra test dependency.
const { outputText } = ts.transpileModule(readFileSync(new URL('./machines.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { inventoryMachines, compareInventories, inventory, agentGroups, installationReports, installationStatus } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
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
