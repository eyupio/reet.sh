import { test } from 'node:test'
import assert from 'node:assert/strict'
import { applyChatEvent, initialChat, isWorking, pendingPermissions, resetChat } from './chat.ts'

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
