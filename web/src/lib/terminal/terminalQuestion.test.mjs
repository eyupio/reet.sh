import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const asURL = source => `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`
const q = await import(asURL(compile('./terminalQuestion.ts')))
const api = await import(asURL(compile('./api.ts').replace("from '../projects'", `from '${asURL('export {}')}'`)))

test('standing wordings from Claude Code and Codex dialogs are spotted, whatever their case', () => {
  for (const label of ["Yes, and don't ask again for go test commands", 'Yes, and don’t ask again for npm run commands in /work/site', 'Yes, and do not ask again', 'ALWAYS allow',
    'Yes, allow for this session', 'Yes, allow all edits during this session', 'Yes, auto-accept edits', 'Yes, AUTO ACCEPT edits', 'Yes, and apply it without asking', 'Yes, every time', 'Yes, to every later command', 'Yes, from now on', 'Yes, and remember this choice', 'Yes, I trust this folder',
    "Yes, and don't ask again for commands that start with `git status` (p)"])
    assert.equal(q.standingPermission(label), true, label)
  for (const label of ['Yes', 'No', 'Proceed', 'Cancel', 'Yes, proceed (y)', 'Yes, manually approve edits', 'No, keep planning', 'No, and tell Claude what to do differently (esc)', 'No, and tell Codex what to do differently (esc)', 'Approve once'])
    assert.equal(q.standingPermission(label), false, label)
})

test('an option needs a second tap when it grants standing permission or its label was cut', () => {
  assert.equal(q.needsConfirm({ index: 1, label: 'Yes' }), false)
  assert.equal(q.needsConfirm({ index: 2, label: "Yes, and don't ask again" }), true)
  assert.equal(q.needsConfirm({ index: 1, label: 'Yes', truncated: true }), true)
})

test('the second tap state rule: confirm on the same option sends, anything else starts or cancels', () => {
  assert.deepEqual(q.nextTap(null, 1, false), { pending: null, send: true })
  assert.deepEqual(q.nextTap(2, 1, false), { pending: null, send: true })
  assert.deepEqual(q.nextTap(null, 2, true), { pending: 2, send: false })
  assert.deepEqual(q.nextTap(2, 2, true), { pending: null, send: true })
  assert.deepEqual(q.nextTap(2, 3, true), { pending: 3, send: false }, 'another standing option moves the confirm')
})

test('an option that asks for typed words is recognised, a plain yes never is', () => {
  for (const label of ['No, and tell Claude what to do differently (esc)', 'No, and tell Codex what to do differently (esc)', 'Type here to tell Claude what to change', 'Explain what you want instead', 'Provide feedback', 'No, give feedback'])
    assert.equal(q.asksForWords(label), true, label)
  for (const label of ['Yes', 'No', 'Yes, proceed', 'Yes, and tell me when it is done', 'Cancel', 'Yes, type it'])
    assert.equal(q.asksForWords(label), false, label)
})

test('Home shows the full card only for a small, whole question', () => {
  const o = n => Array.from({ length: n }, (_, i) => ({ index: i + 1, label: `Choice ${i + 1}` }))
  assert.equal(q.fitsHome({ question: 'Run it?', options: o(3), fingerprint: 'f' }), true)
  assert.equal(q.fitsHome({ question: 'Run it?', options: o(4), fingerprint: 'f' }), false)
  assert.equal(q.fitsHome({ question: 'Run it?', options: o(2), truncated: true, fingerprint: 'f' }), false)
  assert.equal(q.fitsHome({ question: 'Run it?', options: [{ index: 1, label: 'Yes', truncated: true }], fingerprint: 'f' }), false)
  assert.equal(q.questionSentence({ question: 'Bash command\ngo test\nDo you want to proceed?', options: [], fingerprint: 'f' }), 'Do you want to proceed?')
})

test('an option reads as its number and its words', () => {
  assert.equal(q.optionText({ index: 2, label: 'No' }), '2. No')
})

test('each result maps to what the card does, in the card’s own words', () => {
  assert.equal(q.outcomeOf({ kind: 'updateRequired', sentence: 'Update reet on this machine to answer from here.' }, 'Yes', { machine: 'studio' }).say, 'Update reet on studio to answer from here.')
  assert.deepEqual(q.outcomeOf({ kind: 'answered' }, 'Yes'), { say: 'Answered: Yes', tone: 'done', waitingForWords: false, done: true, locked: true, reread: false })
  assert.deepEqual(q.outcomeOf({ kind: 'alreadyAnswered' }, 'Yes'), q.outcomeOf({ kind: 'answered' }, 'Yes'))
  assert.deepEqual(q.outcomeOf({ kind: 'changed', sentence: 'It changed.' }, 'Yes'), { say: 'It changed.', tone: 'note', waitingForWords: false, done: false, locked: false, reread: true })
  assert.deepEqual(q.outcomeOf({ kind: 'updateRequired', sentence: 'Update reet.' }, 'Yes'), { say: 'Update reet on this machine to answer from here.', tone: 'note', waitingForWords: false, done: false, locked: true, reread: false })
  assert.deepEqual(q.outcomeOf({ kind: 'readOnly', sentence: 'terminal access denied' }, 'Yes'), { say: 'Your account can look but not answer.', tone: 'note', waitingForWords: false, done: false, locked: true, reread: false })
  assert.deepEqual(q.outcomeOf({ kind: 'busy', sentence: 'x' }, 'Yes'), { say: 'Another answer is on its way. Wait a moment.', tone: 'note', waitingForWords: false, done: false, locked: false, reread: false })
  assert.deepEqual(q.outcomeOf({ kind: 'offline', sentence: 'terminal machine is reconnecting; retry after it reconnects' }, 'Yes'), { say: 'The machine is not connected. Try again when it is back.', tone: 'error', waitingForWords: false, done: false, locked: false, reread: false })
  assert.deepEqual(q.outcomeOf({ kind: 'noReply' }, 'Yes'), { say: 'No reply yet. Check the terminal before answering again.', tone: 'error', waitingForWords: false, done: false, locked: false, reread: false })
  // Fallbacks when the server said nothing.
  assert.match(q.outcomeOf({ kind: 'changed' }, 'Yes').say, /changed/)
  assert.match(q.outcomeOf({ kind: 'updateRequired' }, 'Yes').say, /Update reet/)
})

test('an answer that leaves the agent waiting for words says so', () => {
  const out = q.outcomeOf({ kind: 'answered' }, 'No, and tell Claude what to do differently (esc)', { agent: 'Claude Code' })
  assert.equal(out.say, 'Answered: No, and tell Claude what to do differently (esc). Claude Code is waiting for your words in the terminal.')
  assert.equal(out.waitingForWords, true)
  assert.equal(q.outcomeOf({ kind: 'answered' }, 'Yes', { agent: 'Claude Code' }).waitingForWords, false)
})

test('a failure adds the server’s sentence only when it is a full sentence', () => {
  assert.equal(q.outcomeOf({ kind: 'failed', sentence: 'that reet session is not available' }, 'Yes').say, 'The answer could not be sent. Try again, or answer in the terminal.')
  assert.equal(q.outcomeOf({ kind: 'failed' }, 'Yes').say, 'The answer could not be sent. Try again, or answer in the terminal.')
  assert.equal(q.outcomeOf({ kind: 'failed', sentence: 'The session is gone.' }, 'Yes').say, 'The answer could not be sent. The session is gone. Try again, or answer in the terminal.')
  assert.equal(q.fullSentence('terminal machine is reconnecting; retry after it reconnects'), false)
  assert.equal(q.fullSentence('Look again.'), true)
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
  const cases = [[409, 'question_changed', 'changed'], [409, 'update_required', 'updateRequired'], [409, 'answer_busy', 'busy'], [409, 'other', 'failed'], [409, 'machine_locked', 'failed'], [403, undefined, 'readOnly'], [503, undefined, 'offline'], [404, undefined, 'failed'], [400, undefined, 'failed']]
  for (const [status, code, kind] of cases) {
    reply(status, { error: 'Server words.', ...(code ? { code } : {}) })
    assert.deepEqual(await api.answerTerminalQuestion('s1', 'f', 1), { kind, sentence: 'Server words.' }, `${status} ${code}`)
  }
  globalThis.fetch = async () => { throw new TypeError('offline') }
  assert.equal((await api.answerTerminalQuestion('s1', 'f', 1)).kind, 'failed')
  globalThis.fetch = async () => { throw new DOMException('timed out', 'TimeoutError') }
  assert.equal((await api.answerTerminalQuestion('s1', 'f', 1)).kind, 'noReply')
})
