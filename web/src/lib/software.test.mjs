import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./software.ts', import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
})
const { softwareLine, softwarePlanActions } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

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
