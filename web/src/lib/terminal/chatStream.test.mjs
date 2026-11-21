import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const asURL = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const chatModule = asURL(compile('../chat.ts'))
const { BATCH_MAX, createChatStream, parseChatEvent, resetTruncated } = await import(asURL(compile('./chatStream.ts').replace("from '../chat'", `from '${chatModule}'`)))
const { initialChat } = await import(chatModule)

const wait = ms => new Promise(resolve => setTimeout(resolve, ms))

class FakeEventSource {
  constructor(url) { this.url = url; this.listeners = {}; this.closed = false; this.onopen = null; this.onerror = null }
  addEventListener(type, listener) { this.listeners[type] = listener }
  close() { this.closed = true }
  emit(type, data) { this.listeners[type]?.({ data: typeof data === 'string' ? data : JSON.stringify(data) }) }
}

function pane() {
  const log = { chat: initialChat(), replayed: undefined, changes: 0, replayedWrites: [], source: undefined }
  const stream = createChatStream({
    url: '/chat',
    chat: () => log.chat,
    setChat: next => { log.chat = next },
    setReplayed: value => { log.replayed = value; log.replayedWrites.push(value) },
    changed: () => { log.changes++ },
    open: url => (log.source = new FakeEventSource(url)),
  })
  return { log, stream }
}
const agent = (seq, text) => ({ seq, kind: 'agent', messageId: 'm1', text })

test('a reset body says whether earlier messages were dropped, and a broken one is a plain reset', () => {
  assert.equal(resetTruncated('{"truncated":true}'), true)
  assert.equal(resetTruncated('{"truncated":false}'), false)
  assert.equal(resetTruncated('{}'), false)
  assert.equal(resetTruncated('not json'), false)
  assert.equal(resetTruncated('null'), false)
})

test('an event that cannot be read is skipped, and one that can is returned as sent', () => {
  assert.equal(parseChatEvent('{oops'), undefined)
  assert.deepEqual(parseChatEvent('{"seq":3,"kind":"agent"}'), { seq: 3, kind: 'agent' })
})

test('start opens the stream at its address and the chat stays connecting until it says otherwise', () => {
  const { log, stream } = pane()
  stream.start()
  assert.equal(log.source.url, '/chat')
  assert.equal(log.chat.connection, 'connecting')
  log.source.onopen()
  assert.equal(log.chat.connection, 'open')
  stream.stop()
})

test('events that arrive together are applied together, in order, with one redraw', async () => {
  const { log, stream } = pane()
  stream.start()
  log.source.emit('chat', agent(1, 'hel'))
  log.source.emit('chat', agent(2, 'lo'))
  log.source.emit('chat', '{broken')
  assert.equal(log.chat.items.length, 0, 'nothing is applied until the batch is due')
  await wait(50)
  assert.equal(log.chat.items.length, 1)
  assert.equal(log.chat.items[0].text, 'hello')
  assert.equal(log.chat.lastSeq, 2)
  assert.equal(log.changes, 1)
  stream.stop()
})

test('an event the chat has already seen changes nothing and does not redraw', async () => {
  const { log, stream } = pane()
  stream.start()
  log.source.emit('chat', agent(1, 'a'))
  await wait(50)
  const before = log.chat
  log.source.emit('chat', agent(1, 'a'))
  await wait(50)
  assert.equal(log.chat, before)
  assert.equal(log.changes, 1)
  stream.stop()
})

test('a backlog of a thousand events is applied at once rather than waiting for the timer', () => {
  const { log, stream } = pane()
  stream.start()
  for (let seq = 1; seq <= BATCH_MAX; seq++) log.source.emit('chat', { seq, kind: 'notice', text: `n${seq}` })
  assert.equal(log.chat.lastSeq, BATCH_MAX)
  assert.equal(log.changes, 1)
  stream.stop()
})

test('a reset starts the chat over and it counts as replayed one batch later, after its events are in', async () => {
  const { log, stream } = pane()
  stream.start()
  log.source.emit('chat', agent(1, 'stale'))
  log.source.emit('reset', { truncated: true })
  assert.equal(log.chat.items.length, 0, 'what was waiting before the reset is dropped')
  assert.equal(log.chat.truncated, true)
  assert.equal(log.chat.connection, 'open')
  assert.equal(log.replayed, false)
  log.source.emit('chat', agent(1, 'fresh'))
  await wait(50)
  assert.equal(log.chat.items[0].text, 'fresh')
  assert.equal(log.replayed, true)
  assert.deepEqual(log.replayedWrites, [false, true])
  stream.stop()
})

test('a second reset before the first settles leaves one settle, not two', async () => {
  const { log, stream } = pane()
  stream.start()
  log.source.emit('reset', {})
  log.source.emit('reset', {})
  await wait(50)
  assert.deepEqual(log.replayedWrites, [false, false, true])
  stream.stop()
})

test('a dropped stream reconnects, says nothing about being busy, and a late settle cannot say it again', async () => {
  const { log, stream } = pane()
  stream.start()
  log.source.emit('reset', {})
  log.source.onerror()
  assert.equal(log.chat.connection, 'reconnecting')
  assert.equal(log.replayed, false)
  await wait(50)
  assert.equal(log.replayed, false, 'the settle that was waiting was cancelled')
  log.source.onopen()
  assert.equal(log.chat.connection, 'open')
  stream.stop()
})

test('stop closes the stream and nothing it held fires afterwards', async () => {
  const { log, stream } = pane()
  stream.start()
  log.source.emit('chat', agent(1, 'never'))
  log.source.emit('reset', {})
  log.source.emit('chat', agent(1, 'never either'))
  stream.stop()
  assert.equal(log.source.closed, true)
  await wait(50)
  assert.equal(log.chat.items.length, 0)
  assert.deepEqual(log.replayedWrites, [false])
  const chat = log.chat
  log.source.onerror()
  assert.equal(log.chat, chat, 'a late error does not mark a closed pane as reconnecting')
})
