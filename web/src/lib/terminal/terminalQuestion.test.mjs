import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const asURL = source => `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`
const q = await import(asURL(compile('./terminalQuestion.ts')))
const api = await import(asURL(compile('./api.ts').replace("from '../projects'", `from '${asURL('export {}')}'`)))

test('a label that grants standing permission is spotted, whatever its case', () => {
  for (const label of ["Yes, and don't ask again for npm run", 'Yes, don’t ask again', 'ALWAYS allow', 'Allow for this session', 'Yes, allow all edits during this session'])
    assert.equal(q.standingPermission(label), true, label)
  for (const label of ['Yes', 'No', 'Yes, proceed', 'No, and tell Claude what to do differently', 'Approve once'])
    assert.equal(q.standingPermission(label), false, label)
})

test('an option reads as its number and its words', () => {
  assert.equal(q.optionText({ index: 2, label: 'No' }), '2. No')
})

test('each result maps to what the card does', () => {
  assert.deepEqual(q.outcomeOf({ kind: 'answered' }, 'Yes'), { say: 'Answered: Yes', done: true, locked: true, reread: false })
  assert.deepEqual(q.outcomeOf({ kind: 'alreadyAnswered' }, 'Yes'), q.outcomeOf({ kind: 'answered' }, 'Yes'))
  assert.deepEqual(q.outcomeOf({ kind: 'changed', sentence: 'It changed.' }, 'Yes'), { say: 'It changed.', done: false, locked: false, reread: true })
  assert.deepEqual(q.outcomeOf({ kind: 'updateRequired', sentence: 'Update reet.' }, 'Yes'), { say: 'Update reet.', done: false, locked: true, reread: false })
  assert.equal(q.outcomeOf({ kind: 'readOnly', sentence: 'x' }, 'Yes').locked, true)
  for (const kind of ['busy', 'offline', 'failed']) {
    const out = q.outcomeOf({ kind, sentence: 'Said so.' }, 'Yes')
    assert.deepEqual(out, { say: 'Said so.', done: false, locked: false, reread: false }, kind)
    assert.ok(q.outcomeOf({ kind }, 'Yes').say.length > 0, `${kind} has a fallback sentence`)
  }
})

function reply(status, body) {
  const calls = []
  globalThis.fetch = async (url, init) => { calls.push({ url, init }); return new Response(body === undefined ? null : JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }) }
  return calls
}

test('answerTerminalQuestion posts the fingerprint and option, and maps every reply', async () => {
  let calls = reply(200, { answered: true })
  assert.deepEqual(await api.answerTerminalQuestion('s 1', 'fp1', 2), { kind: 'answered' })
  assert.equal(calls[0].url, '/api/v2/terminal/sessions/s%201/answer')
  assert.equal(calls[0].init.method, 'POST')
  assert.deepEqual(JSON.parse(calls[0].init.body), { fingerprint: 'fp1', option: 2 })
  reply(200, { answered: true, alreadyAnswered: true })
  assert.deepEqual(await api.answerTerminalQuestion('s1', 'f', 1), { kind: 'alreadyAnswered' })
  const cases = [[409, 'question_changed', 'changed'], [409, 'update_required', 'updateRequired'], [409, 'answer_busy', 'busy'], [409, 'other', 'failed'], [403, undefined, 'readOnly'], [503, undefined, 'offline'], [404, undefined, 'failed'], [400, undefined, 'failed']]
  for (const [status, code, kind] of cases) {
    reply(status, { error: 'Server words.', ...(code ? { code } : {}) })
    assert.deepEqual(await api.answerTerminalQuestion('s1', 'f', 1), { kind, sentence: 'Server words.' }, `${status} ${code}`)
  }
  globalThis.fetch = async () => { throw new TypeError('offline') }
  assert.equal((await api.answerTerminalQuestion('s1', 'f', 1)).kind, 'failed')
})
