import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./machineLock.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { runningOnMachine, runningLine, lockedStatus, SESSION_LOCKED } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

test('only running and creating sessions on that machine are counted', () => {
  const sessions = [
    { agentId: 'a', state: 'running' }, { agentId: 'a', state: 'creating' }, { agentId: 'a', state: 'ended' },
    { agentId: 'a', state: 'end_requested' }, { agentId: 'b', state: 'running' }, { agentId: 'a', state: 'lost' },
  ]
  assert.equal(runningOnMachine(sessions, 'a'), 2)
  assert.equal(runningOnMachine(sessions, 'c'), 0)
  assert.equal(runningOnMachine(undefined, 'a'), 0)
})

test('the running-sessions line is singular for one, empty for none', () => {
  assert.equal(runningLine(0), '')
  assert.equal(runningLine(1), '1 session is still running on this machine and can run commands there, including `reet agent unlock`. End it from reet, or with `reet agent lock --end-sessions` on the machine.')
  assert.equal(runningLine(3), '3 sessions are still running on this machine and can run commands there, including `reet agent unlock`. End them from reet, or with `reet agent lock --end-sessions` on the machine.')
})

test('the card status says since when, only when the machine said so', () => {
  const ago = at => `at ${at}`
  assert.equal(lockedStatus(undefined, ago), 'Locked')
  assert.equal(lockedStatus(0, ago), 'Locked')
  assert.equal(lockedStatus(1000, ago), 'Locked since at 1000000')
})

test('an open session is told what still works', () => {
  assert.equal(SESSION_LOCKED, 'Locked on the machine. The agent keeps running; from here you can only stop its current turn or end the session.')
})
