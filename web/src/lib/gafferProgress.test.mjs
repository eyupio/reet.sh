import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
import { applyChatEvent, initialChat } from './chat.ts'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const source = compile('./gafferProgress.ts').replace("from './chat'", `from '${asURL(compile('./chat.ts'))}'`)
const { gafferProgress, gafferQuote, observedTime } = await import(asURL(source))
const state = (...events) => events.reduce((chat, event, i) => applyChatEvent(chat, { seq: i + 1, ...event }), { ...initialChat(), connection: 'open' })
const prompt = { kind: 'user', text: 'Make the page accessible' }
const working = { kind: 'status', state: 'working' }
const tool = (id, status = 'in_progress', extra = {}) => ({ kind: 'tool', toolCallId: id, toolKind: 'read', status, path: '/project/src/app.ts', ...extra })

test('new chat and unsettled replay never claim work or a completed turn', () => {
  assert.equal(gafferProgress(initialChat()).visible, false)
  const chat = state(prompt, working, tool('one'))
  assert.equal(gafferProgress(chat, '', false).phase, 'connecting')
  assert.equal(gafferProgress(chat, '', false).visible, false)
})

test('tool updates count once, parallel calls are preserved, and the latest active target is relative', () => {
  const chat = state(prompt, working, tool('one'), tool('one', 'completed'), tool('two'), tool('three', 'in_progress', { toolKind: 'edit', path: '/project/src/theme.ts' }))
  const progress = gafferProgress(chat, '/project')
  assert.equal(progress.completed, 1)
  assert.equal(progress.running, 2)
  assert.equal(progress.phase, 'editing')
  assert.equal(progress.target, 'src/theme.ts')
  assert.equal(progress.recent.length, 3)
})

test('an approval or question takes precedence over a running tool', () => {
  let chat = state(prompt, working, tool('one'), { kind: 'permission', permissionId: 'p', title: 'Run checks', command: 'npm test' })
  assert.equal(gafferProgress(chat).phase, 'approval')
  assert.equal(gafferProgress(chat).active, false)
  assert.equal(gafferProgress(chat).target, 'npm test')
  chat = state(prompt, working, tool('one'), { kind: 'question', questionId: 'q', question: { message: 'Which theme?', fields: [{ key: 'theme', kind: 'text' }] } })
  assert.equal(gafferProgress(chat).phase, 'question')
})

test('reconnect, error and session end override stale work and approvals', () => {
  const chat = state(prompt, working, tool('one'), { kind: 'permission', permissionId: 'p', title: 'Allow?' })
  assert.equal(gafferProgress({ ...chat, connection: 'reconnecting' }).phase, 'reconnecting')
  assert.equal(gafferProgress({ ...chat, status: { state: 'error', detail: 'No credits' } }).phase, 'error')
  assert.equal(gafferProgress(chat, '', true, true).phase, 'ended')
})

test('current turn excludes earlier tools and reasoning, and never treats failed tools as success', () => {
  const chat = state(prompt, tool('old', 'completed'), { ...prompt, text: 'Next turn' }, working,
    tool('thought', 'in_progress', { toolKind: 'think' }), tool('bad', 'failed'), { kind: 'status', state: 'idle' })
  const progress = gafferProgress(chat)
  assert.equal(progress.completed, 0)
  assert.equal(progress.failed, 1)
  assert.equal(progress.running, 0)
  assert.equal(progress.recent.length, 1)
  assert.equal(progress.label, 'Turn finished')
  assert.equal(gafferProgress({ ...chat, truncated: true }).partial, true)
})

test('pending, thinking, response and generic waiting do not invent a running command', () => {
  assert.equal(gafferProgress(state(prompt, working, tool('one', 'pending'))).label, 'Worker preparing a tool')
  assert.equal(gafferProgress(state(prompt, working, tool('thought', 'in_progress', { toolKind: 'think' }))).phase, 'planning')
  assert.equal(gafferProgress(state(prompt, working, { kind: 'agent', messageId: 'm', text: 'Here is what changed' })).phase, 'responding')
  assert.equal(gafferProgress(state(prompt, { kind: 'status', state: 'waiting' })).active, false)
  assert.equal(gafferProgress(state(prompt, { kind: 'status', state: 'cancelled' })).phase, 'cancelled')
})

test('quotes are contextual, do not immediately repeat, and single-choice states remain valid', () => {
  for (const phase of ['planning', 'reading', 'editing', 'running', 'approval', 'question', 'error', 'reconnecting', 'idle']) {
    const first = gafferQuote(phase, '', () => 0)
    const next = gafferQuote(phase, first, () => 0)
    assert.equal(typeof first, 'string')
    assert.notEqual(first, next)
    assert.match(first, /worker/)
  }
  const ended = gafferQuote('ended')
  assert.equal(gafferQuote('ended', ended), ended)
  assert.equal(observedTime(0), '0s')
  assert.equal(observedTime(83), '1m 23s')
  assert.equal(observedTime(3601), '60m 01s')
})
