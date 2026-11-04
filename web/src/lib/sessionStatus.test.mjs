import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { sessionStatus, stateStatus, statusLabel, needStatus } = await import(`data:text/javascript;base64,${Buffer.from(compile('./sessionStatus.ts')).toString('base64')}`)

const session = (over = {}) => ({ state: 'running', exitCode: null, attention: undefined, attentionSource: undefined, ...over })

test('attention outranks the lifecycle state', () => {
  assert.equal(sessionStatus(session({ attention: 'waiting' })), 'waiting')
  assert.equal(sessionStatus(session({ attention: 'working' })), 'working')
  assert.equal(sessionStatus(session({ attention: 'starting' })), 'starting')
  assert.equal(sessionStatus(session({ attention: 'finished' })), 'finished')
  assert.equal(sessionStatus(session({ attention: 'failed' })), 'failed')
  assert.equal(sessionStatus(session({ attention: 'quiet', attentionSource: 'chat' })), 'finished')
  assert.equal(sessionStatus(session({ attention: 'quiet', attentionSource: 'terminal' })), 'quiet')
})

test('the lifecycle state fills in when the agent has no attention signal', () => {
  assert.equal(sessionStatus(session({ state: 'creating' })), 'starting')
  assert.equal(sessionStatus(session({ state: 'running' })), 'working')
  assert.equal(sessionStatus(session({ state: 'end_requested' })), 'ending')
  assert.equal(sessionStatus(session({ state: 'ended' })), 'ended')
  assert.equal(sessionStatus(session({ state: 'exited', exitCode: 0 })), 'finished')
  assert.equal(sessionStatus(session({ state: 'exited', exitCode: 2 })), 'failed')
  assert.equal(sessionStatus(session({ state: 'exited', exitCode: null })), 'ended')
  assert.equal(sessionStatus(session({ state: 'lost' })), 'failed')
})

test('every status has one colour class in the stylesheet', () => {
  const css = readFileSync(new URL('../app.css', import.meta.url), 'utf8')
  for (const status of ['working', 'waiting', 'starting', 'ending', 'finished', 'failed', 'ended', 'quiet'])
    assert.match(css, new RegExp(`data-status='${status}'`), status)
})

test('compact labels and need-card colours stay in step with the families', () => {
  assert.deepEqual(['working', 'waiting', 'starting', 'ending', 'finished', 'failed', 'ended', 'quiet'].map(statusLabel), ['Working', 'Waiting', 'Starting', 'Ending', 'Finished', 'Failed', 'Ended', 'Quiet'])
  assert.equal(needStatus('waiting'), 'waiting')
  assert.equal(needStatus('failed'), 'failed')
  assert.equal(needStatus('review'), 'finished')
  assert.equal(needStatus('finished'), 'finished')
})

test('stateStatus is the lifecycle-only form', () => {
  assert.equal(stateStatus('creating'), 'starting')
  assert.equal(stateStatus('running'), 'working')
  assert.equal(stateStatus('lost'), 'failed')
})
