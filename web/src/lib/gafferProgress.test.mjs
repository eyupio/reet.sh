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
const summarySource = compile('./gafferSummary.ts').replace("from './chat'", `from '${asURL(compile('./chat.ts'))}'`)
const { gafferSummary } = await import(asURL(summarySource))
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
  assert.equal(gafferProgress(state(prompt, working, tool('one', 'pending')), '', true, false, 'OpenCode').label, 'OpenCode preparing a tool')
  assert.equal(gafferProgress(state(prompt, working, tool('one', 'pending'))).label, 'Agent preparing a tool')
  assert.equal(gafferProgress(state(prompt, working, tool('thought', 'in_progress', { toolKind: 'think' }))).phase, 'planning')
  assert.equal(gafferProgress(state(prompt, working, { kind: 'agent', messageId: 'm', text: 'Here is what changed' })).phase, 'responding')
  assert.equal(gafferProgress(state(prompt, { kind: 'status', state: 'waiting' })).active, false)
  assert.equal(gafferProgress(state(prompt, { kind: 'status', state: 'cancelled' })).phase, 'cancelled')
})

test('quotes are contextual, name the agent, do not immediately repeat, across every phase', () => {
  for (const phase of ['starting', 'planning', 'reading', 'searching', 'editing', 'running', 'fetching', 'working', 'responding', 'approval', 'question', 'waiting', 'connecting', 'reconnecting', 'error', 'cancelled', 'ended', 'idle']) {
    const first = gafferQuote(phase, 'Claude Code', '', () => 0)
    const next = gafferQuote(phase, 'Claude Code', first, () => 0)
    assert.equal(typeof first, 'string')
    assert.notEqual(first, next)
    assert.match(first, /Claude Code/)
    assert.doesNotMatch(first, /worker/i)
  }
  const ended = gafferQuote('ended')
  assert.notEqual(gafferQuote('ended', 'Agent', ended), ended)
  assert.equal(observedTime(0), '0s')
  assert.equal(observedTime(83), '1m 23s')
  assert.equal(observedTime(3601), '60m 01s')
})

test('progress and quotes fall back to Agent when the name is empty', () => {
  const chat = state(prompt, working, tool('one', 'pending'))
  assert.equal(gafferProgress(chat, '', true, false, '').label, 'Agent preparing a tool')
  assert.match(gafferQuote('planning', '  ', '', () => 0), /Agent/)
})

test('executive summaries wait for a settled stopped turn and never summarize live or reconnecting work', () => {
  const chat = state(prompt, working, tool('one'))
  const idle = state(prompt, { kind: 'agent', messageId: 'done', text: 'Here is the answer.' }, { kind: 'status', state: 'idle' })
  assert.equal(gafferSummary(initialChat()), null)
  assert.equal(gafferSummary(chat), null)
  assert.equal(gafferSummary(idle, '', false), null)
  assert.equal(gafferSummary({ ...idle, connection: 'reconnecting' }), null)
  assert.equal(gafferSummary(state(prompt, { kind: 'status', state: 'waiting' })), null)
  assert.equal(gafferSummary(state(prompt, { kind: 'permission', permissionId: 'p', title: 'Allow?' })), null)
  assert.equal(gafferSummary(idle).handover, 'Here is the answer.')
  assert.equal(gafferSummary(chat, '', false, true), null)
})

test('a closed session summarizes all retained turns, counts tool updates once and lists only completed file work', () => {
  const chat = state(prompt, tool('old', 'completed', { toolKind: 'edit', path: '/project/src/old.ts' }),
    { kind: 'agent', messageId: 'old', text: 'Previous handover.' }, { ...prompt, text: 'Now polish the page' }, working,
    tool('thought', 'in_progress', { toolKind: 'think' }),
    tool('edit', 'in_progress', { toolKind: 'edit' }), tool('edit', 'completed', { toolKind: 'edit' }),
    tool('repeat', 'completed', { toolKind: 'edit', locations: ['/project/src/app.ts', '/project/src/theme.ts'] }),
    tool('bad-edit', 'failed', { toolKind: 'edit', path: '/project/src/failed.ts' }),
    tool('check', 'completed', { toolKind: 'execute', command: 'npm test' }),
    tool('bad-check', 'failed', { toolKind: 'execute', command: 'npm run lint' }),
    { kind: 'agent', messageId: 'done', text: 'Updated the page. Lint still needs attention.' }, { kind: 'status', state: 'idle' })
  const turn = gafferSummary(chat, '/project')
  assert.equal(turn.scope, 'turn')
  assert.equal(turn.completed, 3)
  assert.equal(turn.failed, 2)
  assert.equal(turn.stopped, 0)
  assert.deepEqual(turn.files, ['src/app.ts', 'src/theme.ts'])
  assert.deepEqual(turn.commands.map(c => [c.target, c.status]), [['npm test', 'completed'], ['npm run lint', 'failed']])
  assert.match(turn.attention.join(' '), /2 tool calls reported failure/)
  const session = gafferSummary(chat, '/project', true, true)
  assert.equal(session.scope, 'session')
  assert.equal(session.completed, 4)
  assert.deepEqual(session.files, ['src/old.ts', 'src/app.ts', 'src/theme.ts'])
  assert.equal(session.handover, 'Updated the page. Lint still needs attention.')
  assert.match(session.next, /new session/)
  assert.equal(gafferSummary(state(prompt, { kind: 'status', state: 'ended' })).scope, 'session')
})

test('errors and interruptions retain snags and never mistake an opening plan or previous reply for a handover', () => {
  const chat = state(prompt, { kind: 'agent', messageId: 'old', text: 'All done in the previous turn.' },
    { ...prompt, text: 'Start a new job' }, working, { kind: 'agent', messageId: 'plan', text: 'I will check the page.' },
    tool('unfinished'), { kind: 'status', state: 'error', detail: 'Provider quota exhausted' })
  const summary = gafferSummary(chat)
  assert.equal(summary.handover, '')
  assert.equal(summary.completed, 0)
  assert.equal(summary.stopped, 1)
  assert.match(summary.attention.join(' '), /Provider quota exhausted/)
  assert.match(summary.next, /partial work/)
  const cancelled = gafferSummary(state(prompt, working, tool('one'), { kind: 'status', state: 'cancelled' }))
  assert.equal(cancelled.stopped, 1)
  assert.match(cancelled.attention.join(' '), /stopped before/)
  const ended = gafferSummary(state(prompt, working, tool('one')), '', true, true)
  assert.equal(ended.stopped, 1)
  assert.equal(ended.completed, 0)
  const unfinishedCommand = gafferSummary(state(prompt, working, tool('command', 'in_progress', { toolKind: 'execute', command: 'npm test' })), '', true, true)
  assert.equal(unfinishedCommand.commands[0].status, 'cancelled')
})

test('summaries disclose truncated history, bound the handover, omit code blocks and preserve filename underscores', () => {
  const chat = state(prompt, { kind: 'agent', messageId: 'done', text: '# Handover\n- Updated `my_file.ts`.\n```sh\nrm -rf example\n```\n' + 'More detail. '.repeat(100) }, { kind: 'status', state: 'idle' })
  const summary = gafferSummary({ ...chat, truncated: true, items: chat.items.slice(1) })
  assert.equal(summary.partial, true)
  assert.ok(summary.handover.length <= 540)
  assert.match(summary.handover, /Updated my_file.ts/)
  assert.doesNotMatch(summary.handover, /rm -rf|```|# Handover/)
  assert.match(summary.handover, /…$/)
  assert.equal(gafferProgress({ ...chat, truncated: true, items: chat.items.slice(1) }).visible, true)
  assert.equal(gafferProgress({ ...chat, items: chat.items.slice(1) }, '', true, true).visible, true)
})

const { gafferPose, gafferGesture, gafferAside } = await import(asURL(compile('./gafferPersonality.ts')))

test('returning to work means tea, returning to a stopped session means sleep, and problems or decisions always win', () => {
  for (const phase of ['planning', 'reading', 'searching', 'editing', 'running', 'responding']) assert.equal(gafferPose(phase, true), 'tea')
  for (const phase of ['idle', 'ended']) assert.equal(gafferPose(phase, true), 'asleep')
  for (const phase of ['error', 'cancelled', 'connecting', 'reconnecting']) assert.equal(gafferPose(phase, true), 'concerned')
  for (const phase of ['approval', 'question']) assert.equal(gafferPose(phase, true), 'attention')
  assert.equal(gafferPose('editing'), 'duty')
  assert.equal(gafferGesture('error'), 'shake')
  assert.notEqual(gafferGesture('reading'), gafferGesture('editing'))
  assert.notEqual(gafferGesture('planning'), gafferGesture('running'))
})

test('a failed closed session keeps its error view instead of sleeping or implying success', () => {
  const chat = state(prompt, working, tool('one'), { kind: 'status', state: 'error', detail: 'Provider failed' })
  assert.equal(gafferProgress(chat, '', true, true).phase, 'error')
  assert.equal(gafferProgress(state(prompt, { kind: 'status', state: 'ended' }), '', true, true, 'OpenCode', true).phase, 'error')
  assert.equal(gafferSummary(chat, '', true, true).scope, 'session')
})

test('click and return banter stays contextual, names the agent, and avoids immediate repeats', () => {
  for (const kind of ['click', 'return']) for (const pose of ['duty', 'tea', 'asleep', 'attention', 'concerned']) {
    const first = gafferAside(kind, pose, 'Claude Code', '', () => 0)
    assert.match(first, /Claude Code/)
    if (pose !== 'duty' || kind !== 'return') assert.notEqual(gafferAside(kind, pose, 'Claude Code', first, () => 0), first)
  }
  assert.match(gafferAside('return', 'concerned', 'Agent'), /snag|needs a look/)
})
