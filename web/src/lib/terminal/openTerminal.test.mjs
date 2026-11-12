import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./openTerminal.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { OPEN_TERMINAL_PARAM, openTerminalHref, wantsOpenTerminal, reusableSessionFor, openTerminalRequestId, clearOpenTerminalRequestId } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

const session = (id, agentId, state) => ({ id, agentId, title: id, state, createdAt: 1, updatedAt: 1, exitCode: null })

test('Open terminal is a Sessions deep link with the one-action intent', () => {
  assert.equal(OPEN_TERMINAL_PARAM, 'open')
  assert.equal(openTerminalHref('agent a/b'), '/sessions?agent=agent%20a%2Fb&open=1')
  assert.equal(wantsOpenTerminal('?agent=a&open=1'), true)
  assert.equal(wantsOpenTerminal('?agent=a'), false)
  assert.equal(wantsOpenTerminal('?agent=a&open=0'), false)
  assert.equal(wantsOpenTerminal(''), false)
})

test('a running session is reused before a creation still in flight', () => {
  const sessions = [session('ended', 'a', 'ended'), session('creating', 'a', 'creating'), session('running', 'a', 'running')]
  assert.equal(reusableSessionFor(sessions, 'a').id, 'running')
})

test('a creating session is reused when no session is running yet', () => {
  const sessions = [session('ended', 'a', 'ended'), session('creating', 'a', 'creating'), session('other', 'b', 'running')]
  assert.equal(reusableSessionFor(sessions, 'a').id, 'creating')
})

test('ended and exited sessions are never reused', () => {
  const sessions = ['ended', 'exited', 'end_requested'].map(state => session(state, 'a', state))
  assert.equal(reusableSessionFor(sessions, 'a'), undefined)
  assert.equal(reusableSessionFor([], 'a'), undefined)
})

test('one Open terminal request id survives repeats and is cleared once it is settled', () => {
  const values = new Map()
  const store = { getItem: key => values.get(key) ?? null, setItem: (key, value) => values.set(key, value), removeItem: key => values.delete(key) }
  let made = 0
  const make = () => `request-${++made}`
  assert.equal(openTerminalRequestId('a', store, make), 'request-1')
  assert.equal(openTerminalRequestId('a', store, make), 'request-1')
  assert.equal(openTerminalRequestId('b', store, make), 'request-2')
  clearOpenTerminalRequestId('a', store)
  assert.equal(openTerminalRequestId('a', store, make), 'request-3')
})

test('without usable storage each attempt still gets a request id', () => {
  const make = () => 'request-only'
  assert.equal(openTerminalRequestId('a', null, make), 'request-only')
  assert.doesNotThrow(() => clearOpenTerminalRequestId('a', null))
})
