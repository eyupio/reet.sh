import { test } from 'node:test'
import assert from 'node:assert/strict'
import { applyChatEvent, initialChat, MAX_ITEMS, MAX_TEXT, pendingPermissions, pendingQuestions } from './chat.ts'

test('waiting cards do not pin the output after them', () => {
  let state = initialChat()
  state = applyChatEvent(state, { seq: 1, kind: 'permission', permissionId: 'p1', command: 'echo review' })
  state = applyChatEvent(state, { seq: 2, kind: 'question', questionId: 'q1', question: { message: 'Which?', fields: [{ key: 'answer', kind: 'text' }] } })
  for (let seq = 3; seq < MAX_ITEMS * 4; seq++) {
    state = applyChatEvent(state, { seq, kind: 'notice', text: 'x'.repeat(1024) })
  }
  assert.equal(pendingPermissions(state).length, 1)
  assert.equal(pendingQuestions(state).length, 1)
  assert.ok(state.items.length <= MAX_ITEMS)
  assert.ok(state.items.filter(item => item.kind === 'notice').reduce((n, item) => n + item.text.length, 0) <= MAX_TEXT)
  assert.equal(state.truncated, true)
  assert.equal(state.items.at(-1).key, `n${MAX_ITEMS * 4 - 1}`)
})

test('tool commands, paths and diffs count towards the text budget', () => {
  for (const field of ['command', 'path', 'diff']) {
    let state = initialChat()
    for (let seq = 1; seq <= 100; seq++) {
      state = applyChatEvent(state, { seq, kind: 'tool', toolCallId: `c${seq}`, [field]: 'x'.repeat(8192) })
    }
    const chars = state.items.reduce((n, item) => n + (item[field]?.length ?? 0), 0)
    assert.ok(chars <= MAX_TEXT, `${field} retained ${chars} characters`)
    assert.equal(state.truncated, true)
  }
})
