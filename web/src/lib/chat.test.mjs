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
