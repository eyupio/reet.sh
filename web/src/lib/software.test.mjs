import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./software.ts', import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
})
const { fleetSoftwareCanApply, fleetSoftwareLabel, fleetSoftwareWarning, softwareLine, softwarePlanActions } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const installed = { tool: 'opencode', state: 'installed', version: '1.19.5', manager: 'reet', package: 'opencode-ai', path: '/home/me/.reet/software/opencode/versions/1.19.5/opencode', platform: 'linux', arch: 'amd64', source: 'verified_tarball', managed: true, operations: ['plan_update', 'plan_remove'], checkedAt: 1 }
const missing = { tool: 'opencode', state: 'missing', package: 'opencode-ai', source: 'npm', platform: 'linux', arch: 'amd64', reason: 'not_installed', operations: ['plan_install'], checkedAt: 1 }

test('installed line names the manager, source and path', () => {
  assert.equal(softwareLine(installed), 'Managed by reet · verified 1.19.5 · /home/me/.reet/software/opencode/versions/1.19.5/opencode')
})

test('missing line names the reviewed package and source', () => {
  assert.equal(softwareLine(missing), 'Not installed. The reviewed package is opencode-ai from npm.')
})

test('unknown and unsupported installation lines are distinct', () => {
  assert.match(softwareLine({ ...installed, state: 'unknown', reason: 'unreadable' }), /not recognised/)
  assert.match(softwareLine({ ...installed, state: 'unsupported', reason: 'unsupported' }), /no reviewed maintenance plan/)
})

test('the plan actions follow the evidence and its operations', () => {
  assert.deepEqual(softwarePlanActions(installed), ['update', 'remove'])
  assert.deepEqual(softwarePlanActions({ ...installed, previous: '1.18.0' }), ['update', 'rollback', 'remove'])
  assert.deepEqual(softwarePlanActions(missing), ['install'])
  assert.deepEqual(softwarePlanActions({ ...installed, managed: false, state: 'installed', operations: ['plan_install'] }), ['install'])
  assert.deepEqual(softwarePlanActions({ ...installed, operations: [] }), [])
  assert.deepEqual(softwarePlanActions({ ...missing, operations: ['plan_update'] }), [])
  assert.deepEqual(softwarePlanActions({ ...installed, state: 'unknown', operations: ['plan_update'] }), [])
})

test('fleet review labels and warnings say what would happen', () => {
  const update = { state: 'update', version: '1.18.0', target: '1.19.5', sessionsRunning: 0, canApply: true }
  assert.equal(fleetSoftwareLabel(update), 'Update to 1.19.5')
  assert.equal(fleetSoftwareLabel({ state: 'install', version: '', target: '1.19.5' }), 'Install 1.19.5')
  assert.equal(fleetSoftwareLabel({ state: 'install', version: '1.18.0', target: '1.19.5' }), 'Install 1.19.5 alongside')
  assert.equal(fleetSoftwareLabel({ state: 'none', version: '1.19.5' }), 'Up to date')
  assert.equal(fleetSoftwareLabel({ state: 'unknown' }), 'Needs attention')
  assert.equal(fleetSoftwareWarning(update), '')
  assert.match(fleetSoftwareWarning({ ...update, sessionsRunning: 2 }), /2 OpenCode sessions are running/)
  assert.match(fleetSoftwareWarning({ ...update, sessionsRunning: 1 }), /1 OpenCode session is running/)
  assert.match(fleetSoftwareWarning({ ...update, canApply: false }), /Update reet/)
  assert.equal(fleetSoftwareWarning({ ...update, state: 'none', sessionsRunning: 2 }), '')
})

test('the fleet review only applies a ready single machine', () => {
  const update = { state: 'update', sessionsRunning: 0, canApply: true }
  assert.equal(fleetSoftwareCanApply(update), true)
  assert.equal(fleetSoftwareCanApply({ ...update, state: 'install' }), true)
  assert.equal(fleetSoftwareCanApply({ ...update, sessionsRunning: 2 }), false)
  assert.equal(fleetSoftwareCanApply({ ...update, canApply: false }), false)
  assert.equal(fleetSoftwareCanApply({ ...update, state: 'none' }), false)
  assert.equal(fleetSoftwareCanApply({ ...update, state: 'unknown' }), false)
})
