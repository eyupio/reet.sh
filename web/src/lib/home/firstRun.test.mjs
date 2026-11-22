import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const code = ts.transpileModule(readFileSync(new URL('./firstRun.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { guideShown, replyOf, FIRST_PROMPT } = await import(`data:text/javascript;base64,${Buffer.from(code).toString('base64')}`)

test('the guide shows for a connected machine with no sessions, and not before sessions are read', () => {
  assert.equal(guideShown('', true, [], []), true)
  assert.equal(guideShown('', true, undefined, []), false)
  assert.equal(guideShown('', false, [], []), false)
})

test('someone who already has sessions never sees it; the guide’s own session keeps it until the reply', () => {
  assert.equal(guideShown('', true, [{ id: 'a' }], []), false)
  assert.equal(guideShown('', true, [{ id: 'a' }], ['a']), true)
  assert.equal(guideShown('', true, [{ id: 'a' }, { id: 'b' }], ['a', 'b']), true)
  assert.equal(guideShown('', true, [{ id: 'a' }, { id: 'b' }], ['a']), false)
})

test('a dismissal or a finished guide sticks', () => {
  assert.equal(guideShown('dismissed', true, [], []), false)
  assert.equal(guideShown('done', true, [], []), false)
})

test('a reply is a finished turn or a wait with nothing to approve; an ended session has not replied', () => {
  assert.equal(replyOf(undefined), 'none')
  assert.equal(replyOf({ state: 'running', attention: 'working' }), 'asking')
  assert.equal(replyOf({ state: 'running', attention: 'waiting' }), 'replied')
  assert.equal(replyOf({ state: 'running', attention: 'finished' }), 'replied')
  assert.equal(replyOf({ state: 'running', attention: 'waiting', pendingPermission: { id: 'p' } }), 'asking')
  assert.equal(replyOf({ state: 'exited', attention: 'working' }), 'ended')
  assert.equal(replyOf({ state: 'running', attention: 'failed' }), 'ended')
})

test('the suggested prompt asks for a look and says nothing may change', () => {
  assert.match(FIRST_PROMPT, /Do not change, create or delete anything/)
  assert.ok(!FIRST_PROMPT.startsWith('-') && /[ \n]/.test(FIRST_PROMPT))
})
