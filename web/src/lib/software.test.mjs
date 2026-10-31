import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./software.ts', import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
})
const { softwareLine, softwarePlanAction } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const installed = { tool: 'opencode', state: 'installed', version: '1.19.5', manager: 'node_package', package: 'opencode-ai', path: '/usr/local/bin/opencode', platform: 'linux', arch: 'amd64', source: 'package.json', operations: ['plan_update', 'plan_remove'], checkedAt: 1 }
const missing = { tool: 'opencode', state: 'missing', package: 'opencode-ai', source: 'npm', platform: 'linux', arch: 'amd64', reason: 'not_installed', operations: ['plan_install'], checkedAt: 1 }

test('installed line names the manager, source and path', () => {
  assert.equal(softwareLine(installed), 'node_package · package.json · /usr/local/bin/opencode')
})

test('missing line names the reviewed package and source', () => {
  assert.equal(softwareLine(missing), 'Not installed. The reviewed package is opencode-ai from npm.')
})

test('unknown and unsupported installation lines are distinct', () => {
  assert.match(softwareLine({ ...installed, state: 'unknown', reason: 'unreadable' }), /not recognised/)
  assert.match(softwareLine({ ...installed, state: 'unsupported', reason: 'unsupported' }), /no reviewed maintenance plan/)
})

test('the plan action follows the evidence and its operations', () => {
  assert.equal(softwarePlanAction(installed), 'update')
  assert.equal(softwarePlanAction(missing), 'install')
  assert.equal(softwarePlanAction({ ...installed, operations: [] }), '')
  assert.equal(softwarePlanAction({ ...missing, operations: ['plan_update'] }), '')
  assert.equal(softwarePlanAction({ ...installed, state: 'unknown', operations: ['plan_update'] }), '')
})
