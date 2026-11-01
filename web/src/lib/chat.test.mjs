import { test } from 'node:test'
import assert from 'node:assert/strict'
import { applyChatEvent, endedReason, enqueuePrompt, initialChat, isWorking, pendingPermissions, resetChat } from './chat.ts'

const event = (seq, fields) => ({ seq, ...fields })

test('agent chunks append and tool updates merge by call', () => {
  let state = initialChat()
  state = applyChatEvent(state, event(1, { kind: 'agent', messageId: 'm1', text: 'hel' }))
  state = applyChatEvent(state, event(2, { kind: 'agent', messageId: 'm1', text: 'lo' }))
  state = applyChatEvent(state, event(3, { kind: 'tool', toolCallId: 'c1', title: 'write', toolKind: 'edit', status: 'pending' }))
  state = applyChatEvent(state, event(4, { kind: 'tool', toolCallId: 'c1', status: 'completed', text: 'done' }))
  assert.equal(state.items.length, 2)
  assert.equal(state.items[0].text, 'hello')
  assert.equal(state.items[1].status, 'completed')
  assert.equal(state.items[1].text, 'done')
})

test('permission cards resolve, cancel, and are pending until then', () => {
  let state = initialChat()
  state = applyChatEvent(state, event(1, { kind: 'permission', permissionId: 'p1', title: 'write', command: 'echo ok' }))
  assert.equal(pendingPermissions(state).length, 1)
  state = applyChatEvent(state, event(2, { kind: 'permission_resolved', permissionId: 'p1', allow: false }))
  assert.equal(pendingPermissions(state).length, 0)
  assert.equal(state.items[0].decision, 'denied')
  state = applyChatEvent(state, event(3, { kind: 'permission', permissionId: 'p2', title: 'bash' }))
  state = applyChatEvent(state, event(4, { kind: 'status', state: 'cancelled' }))
  assert.equal(pendingPermissions(state).length, 0)
  assert.equal(state.items[1].decision, 'cancelled')
})

test('dedupe, reset and working state', () => {
  let state = initialChat()
  state = applyChatEvent(state, event(1, { kind: 'status', state: 'working' }))
  assert.equal(isWorking(state), true)
  const same = applyChatEvent(state, event(1, { kind: 'user', text: 'again' }))
  assert.equal(same, state)
  state = applyChatEvent(state, event(2, { kind: 'user', text: 'hi' }))
  assert.equal(isWorking(state), true)
  state = applyChatEvent(state, event(3, { kind: 'status', state: 'idle' }))
  assert.equal(isWorking(state), false)
  state = Object.assign(resetChat(true), { connection: 'open' })
  assert.equal(state.items.length, 0)
  assert.equal(state.lastSeq, 0)
  assert.equal(state.truncated, true)
})

test('the log is bounded and says so', async () => {
  const { MAX_ITEMS, MAX_MESSAGE_TEXT, trimChat } = await import('./chat.ts')
  let state = initialChat()
  for (let i = 1; i <= MAX_ITEMS + 100; i++) {
    state = applyChatEvent(state, event(i, { kind: 'tool', toolCallId: `c${i}`, title: 'Run', toolKind: 'execute', status: 'completed', text: 'out' }))
  }
  assert.equal(state.items.length, MAX_ITEMS)
  assert.equal(state.truncated, true)
  assert.equal(state.items.at(-1).key, `tc${MAX_ITEMS + 100}`)
  assert.equal(state.items[0].key, `tc101`)

  let long = initialChat()
  long = applyChatEvent(long, event(1, { kind: 'agent', messageId: 'm1', text: 'x'.repeat(MAX_MESSAGE_TEXT) }))
  long = applyChatEvent(long, event(2, { kind: 'agent', messageId: 'm1', text: 'more' }))
  assert.equal(long.items[0].text.length, MAX_MESSAGE_TEXT)
  assert.equal(long.truncated, true)
})

test('a pending approval is not trimmed away', () => {
  let state = initialChat()
  state = applyChatEvent(state, event(1, { kind: 'permission', permissionId: 'p1', title: 'write', command: 'echo ok' }))
  for (let i = 2; i <= 600; i++) state = applyChatEvent(state, event(i, { kind: 'user', text: `message ${i}` }))
  assert.equal(pendingPermissions(state).length, 1)
  assert.equal(state.items[0].permissionId, 'p1')
  state = applyChatEvent(state, event(601, { kind: 'permission_resolved', permissionId: 'p1', allow: true }))
  state = applyChatEvent(state, event(602, { kind: 'user', text: 'after the answer' }))
  assert.equal(pendingPermissions(state).length, 0)
  assert.equal(state.items.some(item => item.kind === 'permission' && item.permissionId === 'p1'), false)
  assert.equal(state.items.at(-1).text, 'after the answer')
})

test('tool details merge and a thought streams in deltas', async () => {
  const { diffCounts, displayPath, formatDuration, textSegments, thoughtPreview, toolLabel, toolState, toolTarget } = await import('./chat.ts')
  let state = initialChat()
  state = applyChatEvent(state, event(1, { kind: 'tool', toolCallId: 'c1', title: 'notes.txt', toolKind: 'edit', status: 'in_progress', path: '/work/notes.txt' }))
  state = applyChatEvent(state, event(2, { kind: 'tool', toolCallId: 'c1', title: 'notes.txt', toolKind: 'edit', status: 'completed', path: '/work/notes.txt', diff: '@@ -1 +1 @@\n-alpha\n+ALPHA\n', additions: 1, deletions: 1, durationMs: 179 }))
  const edit = state.items[0]
  assert.equal(edit.kind, 'tool')
  assert.equal(edit.path, '/work/notes.txt')
  assert.equal(edit.additions, 1)
  assert.equal(edit.deletions, 1)
  assert.equal(edit.durationMs, 179)
  assert.equal(toolTarget(edit, '/work'), 'notes.txt')
  assert.equal(toolLabel(edit.toolKind), 'Edit')
  assert.deepEqual(toolState(edit.status), { word: 'Done', tone: 'good' })

  let thought = initialChat()
  thought = applyChatEvent(thought, event(1, { kind: 'tool', toolCallId: 'thought-m1', title: 'Thinking', toolKind: 'think', status: 'in_progress', text: 'hello ' }))
  thought = applyChatEvent(thought, event(2, { kind: 'tool', toolCallId: 'thought-m1', title: 'Thinking', toolKind: 'think', status: 'in_progress', text: 'there' }))
  assert.equal(thought.items[0].text, 'hello there')
  assert.equal(thoughtPreview(thought.items[0].text), 'hello there')
  thought = applyChatEvent(thought, event(3, { kind: 'status', state: 'idle' }))
  assert.equal(thought.items[0].status, 'completed')

  assert.equal(displayPath('/home/u/site/internal/x.go', '/home/u/site'), 'internal/x.go')
  assert.equal(displayPath('/other/x.go', '/home/u/site'), '/other/x.go')
  assert.equal(formatDuration(179), '179 ms')
  assert.equal(formatDuration(1234), '1.2s')
  assert.deepEqual(diffCounts('+++ b/x\n--- a/x\n@@ -1 +1 @@\n-a\n+b\n'), { additions: 1, deletions: 1 })
  assert.deepEqual(textSegments('before\n```go\nfmt.Println()\n```\nafter').map(s => s.kind), ['text', 'code', 'text'])
})

test('prose markdown is split into safe blocks and inline spans', async () => {
  const { inlineSegments, proseBlocks } = await import('./chat.ts')
  assert.deepEqual(proseBlocks('## Fix\n\n- one\n- two\n\n1. first\n2. second\n\n> note\n\nplain *text*'), [
    { kind: 'h', level: 2, text: 'Fix' },
    { kind: 'ul', items: ['one', 'two'] },
    { kind: 'ol', items: ['first', 'second'] },
    { kind: 'quote', text: 'note' },
    { kind: 'p', text: 'plain *text*' },
  ])
  assert.deepEqual(inlineSegments('run `go test` and **keep** it'), [
    { kind: 'text', text: 'run ' },
    { kind: 'code', text: 'go test' },
    { kind: 'text', text: ' and ' },
    { kind: 'strong', text: 'keep' },
    { kind: 'text', text: ' it' },
  ])
})

// --- typed agent questions -------------------------------------------------------------------------------------------
import { checkQuestionDraft, emptyDraft, pendingQuestions, readQuestion, summarizeAnswers, trimChat, MAX_ITEMS } from './chat.ts'

const sampleQuestion = () => ({
  message: 'Which option?',
  fields: [
    { key: 'question_0', title: 'Probe', kind: 'single', required: true, options: [{ value: 'a', label: 'Option A', description: 'First' }, { value: 'b', label: 'Option B' }] },
    { key: 'tags', kind: 'multi', minItems: 1, maxItems: 2, options: [{ value: 'x', label: 'X' }, { value: 'y', label: 'Y' }, { value: 'z', label: 'Z' }] },
    { key: 'note', title: 'Other', kind: 'text', maxLength: 10 },
  ],
})

test('a question card is pending until it is answered, declined or cancelled', () => {
  let state = initialChat()
  state = applyChatEvent(state, event(1, { kind: 'question', questionId: 'q1', toolCallId: 't1', question: sampleQuestion() }))
  assert.equal(pendingQuestions(state).length, 1)
  assert.equal(state.items[0].toolCallId, 't1')
  // A replay of the same card keeps one item and its decision.
  state = applyChatEvent(state, event(2, { kind: 'question', questionId: 'q1', question: sampleQuestion() }))
  assert.equal(state.items.length, 1)
  state = applyChatEvent(state, event(3, { kind: 'question_resolved', questionId: 'q1', reason: 'accept' }))
  assert.equal(state.items[0].decision, 'answered')
  assert.equal(pendingQuestions(state).length, 0)
  state = applyChatEvent(state, event(4, { kind: 'question', questionId: 'q2', question: sampleQuestion() }))
  state = applyChatEvent(state, event(5, { kind: 'question_resolved', questionId: 'q2', reason: 'decline' }))
  assert.equal(state.items[1].decision, 'declined')
  state = applyChatEvent(state, event(6, { kind: 'question', questionId: 'q3', question: sampleQuestion() }))
  state = applyChatEvent(state, event(7, { kind: 'question_cancelled', questionId: 'q3', reason: 'timeout' }))
  assert.deepEqual([state.items[2].decision, state.items[2].reason], ['cancelled', 'timeout'])
})

test('a stopped turn closes waiting questions as it closes waiting approvals', () => {
  let state = initialChat()
  state = applyChatEvent(state, event(1, { kind: 'question', questionId: 'q1', question: sampleQuestion() }))
  state = applyChatEvent(state, event(2, { kind: 'permission', permissionId: 'p1', title: 'write' }))
  state = applyChatEvent(state, event(3, { kind: 'status', state: 'cancelled' }))
  assert.equal(pendingQuestions(state).length, 0)
  assert.equal(pendingPermissions(state).length, 0)
  assert.equal(state.items[0].decision, 'cancelled')
})

test('a malformed question event is ignored rather than breaking the chat', () => {
  let state = initialChat()
  const bad = [
    undefined, null, 'x', {}, { message: '' , fields: [] }, { message: 'm', fields: [] },
    { message: 'm', fields: [{ key: 'k', kind: 'number' }] },
    { message: 'm', fields: [{ key: 'k', kind: 'single', options: [] }] },
    { message: 'm', fields: [{ key: 'k', kind: 'single', options: [{ value: 'a', label: 'A' }, { value: 'a', label: 'B' }] }] },
    { message: 'm', fields: [{ key: 'k', kind: 'text' }, { key: 'k', kind: 'text' }] },
    { message: 'm', fields: Array.from({ length: 9 }, (_, i) => ({ key: `f${i}`, kind: 'text' })) },
    { message: 'm', fields: [{ key: 'k', kind: 'single', options: Array.from({ length: 13 }, (_, i) => ({ value: `v${i}`, label: 'l' })) }] },
  ]
  bad.forEach((question, i) => {
    const next = applyChatEvent(state, event(i + 1, { kind: 'question', questionId: 'q1', question }))
    assert.equal(next.items.length, 0, `accepted ${JSON.stringify(question)}`)
    state = next
  })
  assert.equal(applyChatEvent(state, event(99, { kind: 'question', question: sampleQuestion() })).items.length, 0, 'a card without an id')
  assert.equal(readQuestion(sampleQuestion()).fields[2].maxLength, 10)
})

test('a waiting question is never trimmed away', () => {
  let state = initialChat()
  state = applyChatEvent(state, event(1, { kind: 'question', questionId: 'q1', question: sampleQuestion() }))
  for (let i = 0; i < MAX_ITEMS + 20; i++) state = applyChatEvent(state, event(i + 2, { kind: 'notice', text: `n${i}` }))
  // Like a waiting approval, the card holds the trim until it is decided, so it can always be answered.
  assert.equal(pendingQuestions(state).length, 1)
  assert.equal(state.items[0].kind, 'question')
  state = applyChatEvent(state, event(9999, { kind: 'question_resolved', questionId: 'q1', reason: 'accept' }))
  state = applyChatEvent(state, event(10000, { kind: 'notice', text: 'after' }))
  assert.equal(state.items.length <= MAX_ITEMS, true, 'the log trims again once the question is decided')
  assert.equal(state.truncated, true)
  const decided = trimChat([{ kind: 'question', key: 'q', questionId: 'q', toolCallId: '', question: sampleQuestion(), decision: 'answered' }, ...Array.from({ length: MAX_ITEMS }, (_, i) => ({ kind: 'notice', key: `n${i}`, text: 'x' }))], false)
  assert.equal(decided.items.some(item => item.kind === 'question'), false, 'a decided question can be trimmed')
})

test('the draft check gives feedback and builds the answers to send', () => {
  const q = readQuestion(sampleQuestion())
  const empty = checkQuestionDraft(q, emptyDraft(q))
  assert.equal(empty.ok, false)
  assert.deepEqual(Object.keys(empty.errors), ['question_0'])
  assert.deepEqual({ ...empty.answers }, {})

  const ok = checkQuestionDraft(q, { question_0: 'a', tags: ['x', 'z'], note: '  hi  ' })
  assert.equal(ok.ok, true)
  assert.deepEqual({ ...ok.answers }, { question_0: 'a', tags: ['x', 'z'], note: '  hi  ' })

  // Optional fields left empty are not sent.
  const minimal = checkQuestionDraft(q, { question_0: 'b', tags: [], note: '   ' })
  assert.equal(minimal.ok, true)
  assert.deepEqual({ ...minimal.answers }, { question_0: 'b' })

  for (const [name, draft, field] of [
    ['unoffered choice', { question_0: 'zzz' }, 'question_0'],
    ['too many items', { question_0: 'a', tags: ['x', 'y', 'z'] }, 'tags'],
    ['repeated item', { question_0: 'a', tags: ['x', 'x'] }, 'tags'],
    ['unoffered item', { question_0: 'a', tags: ['q'] }, 'tags'],
    ['text too long', { question_0: 'a', note: '12345678901' }, 'note'],
    ['multibyte over the byte limit', { question_0: 'a', note: '日本語日' }, 'note'],
  ]) {
    const result = checkQuestionDraft(q, draft)
    assert.equal(result.ok, false, name)
    assert.ok(result.errors[field], `${name}: ${JSON.stringify(result.errors)}`)
  }
})

test('a required multi select needs at least one choice, an optional empty one is skipped', () => {
  const q = readQuestion({ message: 'm', fields: [{ key: 'k', kind: 'multi', required: true, options: [{ value: 'a', label: 'A' }, { value: 'b', label: 'B' }] }] })
  assert.match(checkQuestionDraft(q, { k: [] }).errors.k, /at least 1/)
  assert.equal(checkQuestionDraft(q, { k: ['a'] }).ok, true)
  const optional = readQuestion({ message: 'm', fields: [{ key: 'k', kind: 'multi', minItems: 2, options: [{ value: 'a', label: 'A' }, { value: 'b', label: 'B' }] }] })
  assert.equal(checkQuestionDraft(optional, { k: [] }).ok, true)
  assert.match(checkQuestionDraft(optional, { k: ['a'] }).errors.k, /at least 2/)
})

test('answers are summarized by label for the transcript', () => {
  const q = readQuestion(sampleQuestion())
  assert.deepEqual(summarizeAnswers(q, { question_0: 'a', tags: ['x', 'z'], note: 'hi' }), [
    { key: 'question_0', label: 'Probe', text: 'Option A' }, { key: 'tags', label: 'tags', text: 'X, Z' }, { key: 'note', label: 'Other', text: 'hi' },
  ])
  assert.deepEqual(summarizeAnswers(q, {}), [])
})

test('field keys that collide with object internals are ordinary names', () => {
  for (const key of ['__proto__', 'constructor', 'toString', 'hasOwnProperty']) {
    const q = readQuestion({ message: 'm', fields: [
      { key, kind: 'text', required: true, maxLength: 50 },
      { key: `${key}_pick`, kind: 'single', required: true, options: [{ value: 'a', label: 'A' }] },
    ] })
    const empty = checkQuestionDraft(q, emptyDraft(q))
    assert.equal(empty.ok, false, key)
    assert.ok(Object.hasOwn(empty.errors, key), `${key}: a missing required answer is reported`)
    const draft = { ...emptyDraft(q), [key]: 'hello', [`${key}_pick`]: 'a' }
    const filled = checkQuestionDraft(q, draft)
    assert.equal(filled.ok, true, `${key}: ${JSON.stringify(filled.errors)}`)
    assert.deepEqual(Object.keys(filled.answers).sort(), [key, `${key}_pick`].sort(), key)
    assert.equal(Object.getOwnPropertyDescriptor(filled.answers, key)?.value, 'hello', key)
    // It survives being sent as JSON, the way the request body is built.
    assert.equal(JSON.parse(JSON.stringify(filled.answers))[key], 'hello', key)
    assert.deepEqual(summarizeAnswers(q, filled.answers).map(line => line.text), ['hello', 'A'], key)
  }
})

test('fields that share a title stay distinct in the summary', () => {
  const q = readQuestion({ message: 'm', fields: [{ key: 'a', title: 'Choice', kind: 'text' }, { key: 'b', title: 'Choice', kind: 'text' }] })
  const lines = summarizeAnswers(q, { a: 'x', b: 'y' })
  assert.deepEqual(lines.map(line => line.key), ['a', 'b'])
  assert.equal(new Set(lines.map(line => line.key)).size, 2)
  assert.deepEqual(lines.map(line => line.text), ['x', 'y'])
})

test('a closed chat says why it closed and what the person can do', () => {
  const lost = endedReason('lost', 'Claude Code')
  assert.match(lost, /lost contact with this Claude Code chat/)
  assert.match(lost, /service there restarted/)
  assert.match(lost, /start a new chat/)
  assert.match(endedReason('ended', 'OpenCode'), /^You ended this OpenCode chat/)
  // An exited chat may have stopped on its own or been ended by the person; the wording must not blame either.
  assert.match(endedReason('exited', 'OpenCode'), /^This OpenCode chat has ended/)
  assert.doesNotMatch(endedReason('exited', 'OpenCode'), /You ended|OpenCode ended/)
  // Every reason says the history is gone; none claims it can be continued.
  for (const state of ['lost', 'ended', 'exited']) assert.doesNotMatch(endedReason(state, 'X'), /continue|resume|reopen/i)
  assert.notEqual(endedReason('lost', 'X'), endedReason('exited', 'X'))
})

test('queued prompts keep order, are capped, and can be edited, promoted and dropped', async () => {
  const { dropQueuedPrompt, editQueuedPrompt, enqueuePrompt, promoteQueuedPrompt, MAX_QUEUED_PROMPTS } = await import('./chat.ts')
  let queue = []
  for (let i = 0; i < MAX_QUEUED_PROMPTS; i++) {
    const answer = enqueuePrompt(queue, { id: `q${i}`, text: `message ${i}`, payload: null })
    assert.equal(answer.full, false)
    queue = answer.queue
  }
  const full = enqueuePrompt(queue, { id: 'overflow', text: 'no room', payload: null })
  assert.equal(full.full, true)
  assert.deepEqual(full.queue.map(item => item.id), queue.map(item => item.id))
  queue = editQueuedPrompt(queue, 'q1', 'steer left')
  assert.equal(queue[1].text, 'steer left')
  assert.equal(editQueuedPrompt(editQueuedPrompt(queue, 'q1', '  '), 'q1', '')[1].text, 'steer left')
  queue = promoteQueuedPrompt(queue, 'q3')
  assert.deepEqual(queue.slice(0, 2).map(item => item.id), ['q3', 'q0'])
  queue = dropQueuedPrompt(queue, 'q3')
  assert.equal(queue[0].id, 'q0')
  assert.equal(queue.length, MAX_QUEUED_PROMPTS - 1)
})

test('a tool still running when the turn stops is shown as stopped, and a later update still wins', () => {
  for (const stop of ['cancelled', 'idle', 'error', 'ended']) {
    let state = initialChat()
    state = applyChatEvent(state, event(1, { kind: 'status', state: 'working' }))
    state = applyChatEvent(state, event(2, { kind: 'tool', toolCallId: 'run', title: 'go test', toolKind: 'execute', status: 'in_progress' }))
    state = applyChatEvent(state, event(3, { kind: 'tool', toolCallId: 'wait', title: 'sleep', toolKind: 'execute', status: 'pending' }))
    state = applyChatEvent(state, event(4, { kind: 'tool', toolCallId: 'done', title: 'ls', toolKind: 'execute', status: 'completed' }))
    state = applyChatEvent(state, event(5, { kind: 'status', state: stop }))
    const byId = id => state.items.find(i => i.kind === 'tool' && i.toolCallId === id)
    assert.equal(byId('run').status, 'cancelled', `${stop}: a running tool was left running`)
    assert.equal(byId('wait').status, 'cancelled', `${stop}: a pending tool was left pending`)
    assert.equal(byId('done').status, 'completed', `${stop}: a finished tool changed`)
    // The agent's own late report of how that tool ended is still believed.
    state = applyChatEvent(state, event(6, { kind: 'tool', toolCallId: 'run', status: 'completed', text: 'ok' }))
    assert.equal(byId('run').status, 'completed', `${stop}: a later update was ignored`)
  }
})
