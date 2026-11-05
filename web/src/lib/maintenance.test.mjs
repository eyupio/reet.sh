import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = readFileSync(new URL('./maintenance.ts', import.meta.url), 'utf8').replace(/^import type .*$/m, '')
const m = await import(`data:text/javascript;base64,${Buffer.from(ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText).toString('base64')}`)
const item = (over = {}) => ({ agentId: 'a1', run: 'r1', status: 'safe', changed: 0, unpushed: 0, ...over })

test('only worktrees with nothing unsaved are cleaned in bulk', () => {
  const all = [item(), item({ run: 'r2', status: 'review', changed: 2 }), item({ run: 'r3', status: 'blocked', reason: 'nested_repository' }), item({ run: 'r4', status: 'unreachable', reason: 'offline' }), item({ run: 'r5', removed: true })]
  assert.deepEqual(m.cleanable(all).map(i => i.run), ['r1'])
  assert.deepEqual(m.needsReview(all).map(i => i.run), ['r2', 'r3'])
  assert.equal(m.itemKey(item()), 'a1/r1')
})

test('each outcome has a plain sentence', () => {
  assert.equal(m.sweepReason(item({ status: 'review', changed: 1 })), '1 file has uncommitted changes.')
  assert.equal(m.sweepReason(item({ status: 'review', changed: 3 })), '3 files have uncommitted changes.')
  assert.equal(m.sweepReason(item({ status: 'blocked', reason: 'unreferenced_commits' })), 'Some commits are on no branch.')
  assert.equal(m.sweepReason(item({ status: 'unreachable', reason: 'offline' })), "That machine isn't connected.")
})
