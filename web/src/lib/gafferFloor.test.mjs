import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = readFileSync(new URL('./gafferFloor.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const f = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)

const fakeStorage = () => {
  const data = new Map()
  return { getItem: key => (data.has(key) ? data.get(key) : null), setItem: (key, value) => data.set(key, String(value)), data }
}
const session = (id, extra = {}) => ({ id, agentId: 'a1', title: `Session ${id}`, state: 'running', structured: true, ...extra })
const goal = (sessionId, state = 'active') => ({ sessionId, state })

test('only running structured chats without a saved goal are offered', () => {
  const sessions = [session('one'), session('two', { structured: false }), session('three', { state: 'exited' }), session('four'), session('five')]
  const offered = f.floorCandidates(sessions, [goal('four'), goal('five', 'completed')])
  assert.deepEqual(offered.map(item => item.id), ['one'])
})

test('room counts only active goals against the account limit', () => {
  assert.equal(f.floorRoom([]), 10)
  assert.equal(f.floorRoom([goal('a'), goal('b', 'paused'), goal('c', 'needs_user'), goal('d', 'completed')]), 9)
  assert.equal(f.floorRoom(Array.from({ length: 12 }, (_, n) => goal(`s${n}`))), 0)
})

test('a tick for a session that left the floor is ignored and order follows the floor', () => {
  const candidates = [session('one'), session('two'), session('three')]
  assert.deepEqual(f.floorChosen(candidates, new Set(['three', 'gone', 'one'])).map(item => item.id), ['one', 'three'])
})

test('the review says why it cannot start', () => {
  const ready = { chosen: 2, room: 5, text: 'Raise the PR. Do not merge.', provider: true }
  assert.equal(f.floorBlocker(ready), '')
  assert.match(f.floorBlocker({ ...ready, provider: false }), /default Gaffer provider/)
  assert.match(f.floorBlocker({ ...ready, chosen: 0 }), /at least one session/)
  assert.match(f.floorBlocker({ ...ready, chosen: 7 }), /room for 5 more\. Untick 2\./)
  assert.match(f.floorBlocker({ ...ready, room: 0 }), /already following 10 sessions/)
  assert.match(f.floorBlocker({ ...ready, text: '  ' }), /Write the goal/)
  assert.match(f.floorBlocker({ ...ready, text: 'é'.repeat(1001) }), /over 2000 bytes/)
  assert.equal(f.floorBlocker({ ...ready, text: 'é'.repeat(1000) }), '')
})

test('the summary never claims a session that was not started', () => {
  const ok = { id: 'a', title: 'A', ok: true, note: '' }, bad = { id: 'b', title: 'B', ok: false, note: 'No worker channel.' }
  assert.equal(f.floorSummary([ok]), 'Reet. Gaffer is looking after 1 session.')
  assert.equal(f.floorSummary([ok, ok]), 'Reet. Gaffer is looking after 2 sessions.')
  assert.match(f.floorSummary([bad]), /was not started/)
  assert.match(f.floorSummary([bad, bad]), /None of the 2 sessions/)
  assert.equal(f.floorSummary([ok, bad, ok]), '2 started, 1 not started. The ones not started were left unchanged.')
})

test('saved defaults round-trip per account and are clamped to the goal limits', () => {
  const area = fakeStorage()
  assert.deepEqual(f.readFloorDefaults(7, 'Fallback goal', area), { text: 'Fallback goal', maxReviews: 12, hours: 24 })
  f.writeFloorDefaults(7, { text: '  Ship it. Do not merge.  ', maxReviews: 6, hours: 48 }, area)
  assert.deepEqual(f.readFloorDefaults(7, 'Fallback goal', area), { text: 'Ship it. Do not merge.', maxReviews: 6, hours: 48 })
  assert.deepEqual(f.readFloorDefaults(8, 'Fallback goal', area), { text: 'Fallback goal', maxReviews: 12, hours: 24 })
  assert.deepEqual(f.parseFloorDefaults(JSON.stringify({ text: 'x', maxReviews: 900, hours: 0 }), 'F'), { text: 'x', maxReviews: 48, hours: 1 })
  assert.deepEqual(f.parseFloorDefaults(JSON.stringify({ text: 'é'.repeat(1001), maxReviews: 'many', hours: null }), 'F'), { text: 'F', maxReviews: 12, hours: 24 })
  assert.deepEqual(f.parseFloorDefaults('{not json', 'F'), { text: 'F', maxReviews: 12, hours: 24 })
  assert.deepEqual(f.parseFloorDefaults('null', 'F'), { text: 'F', maxReviews: 12, hours: 24 })
})

test('blocked storage leaves the review usable', () => {
  const blocked = { getItem: () => { throw new Error('blocked') }, setItem: () => { throw new Error('blocked') } }
  assert.deepEqual(f.readFloorDefaults(1, 'F', blocked), { text: 'F', maxReviews: 12, hours: 24 })
  assert.doesNotThrow(() => f.writeFloorDefaults(1, { text: 'x', maxReviews: 1, hours: 1 }, blocked))
})
