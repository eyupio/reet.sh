import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const transpile = name => ts.transpileModule(readFileSync(new URL(`../src/lib/terminal/${name}.ts`, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const url = source => `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`
const apiURL = url(transpile('api'))
const { TerminalConnection } = await import(url(transpile('connection').replace("from './api'", `from '${apiURL}'`)))
const { value, percent, fresh, uptime } = await import(url(transpile('health')))
const { readiness, validTerminalTitle } = await import(apiURL)
test('session titles reject terminal controls, format controls and overlong names before creating a retry key',()=>{
  for(const title of ['', '   ', 'a'.repeat(81),'bad\x1btitle','bad\u202etitle','bad\u2028title'])assert.equal(validTerminalTitle(title),false)
  for(const title of ['Root build','Café 💥','a'.repeat(80)])assert.equal(validTerminalTitle(title),true)
})

function fixture() {
  const original = { fetch: globalThis.fetch, WebSocket: globalThis.WebSocket, location: globalThis.location }
  const sockets = [], renders = [], states = [], requests = []
  globalThis.location = { origin: 'https://reet.test', protocol: 'https:' }
  globalThis.fetch = async (path, init) => { requests.push({ path, init }); return new Response(JSON.stringify(path.endsWith('/attachments') ? { ticket: 'ticket0123456789abcdef' } : { sessions: [{ id: 'session0123456789abcdef', state: 'running' }] }), { status: 200 }) }
  class Socket {
    static OPEN = 1
    readyState = 0
    bufferedAmount = 0
    sent = []
    constructor(url) { this.url = url; sockets.push(this) }
    send(data) { if (this.readyState !== 1) throw new Error('closed'); this.sent.push(data) }
    close() { this.readyState = 3; this.onclose?.() }
    open() { this.readyState = 1; this.onopen?.() }
    message(data) { this.onmessage?.({ data }) }
  }
  globalThis.WebSocket = Socket
  const connection = new TerminalConnection({ session: 'session0123456789abcdef', dimensions: () => ({ cols: 80, rows: 24 }), renderer: { write: (bytes, reset, done) => renders.push({ bytes: new Uint8Array(bytes), reset, done }) }, onstate: state => states.push(state) })
  const open = async () => { await connection.connect(); const socket = sockets.at(-1); socket.open(); socket.message('{"type":"ready"}'); return socket }
  return { connection, sockets, renders, states, requests, open, cleanup: () => { connection.dispose(); Object.assign(globalThis, original) } }
}

test('terminal input waits for redraw and disconnected keystrokes are never replayed', async () => {
  const f = fixture()
  try {
    assert.equal(f.connection.text('offline'), false)
    const first = await f.open()
    assert.equal(f.connection.text('before redraw'), false)
    first.message(new Uint8Array([27, 91, 72]).buffer)
    assert.equal(f.connection.text('before rendered callback'), false)
    assert.equal(f.renders[0].reset, true)
    f.renders[0].done()
    assert.equal(f.states.at(-1), 'live')
    assert.equal(f.connection.text('sent once'), true)
    first.close()
    assert.equal(f.connection.text('never queued'), false)
    const second = await f.open()
    second.message(new Uint8Array([65]).buffer); f.renders.at(-1).done()
    assert.equal(second.sent.filter(item => item instanceof ArrayBuffer).length, 0)
    assert.match(second.url.href, /^wss:\/\/reet.test\/api\/v2\/terminal\/socket$/)
    assert.equal(second.url.search, '')
    for (const { init } of f.requests) { assert.equal(init.credentials, 'same-origin'); assert.equal(new Headers(init.headers).has('X-Reet-Token'), false); assert.equal(new Headers(init.headers).has('Authorization'), false) }
  } finally { f.cleanup() }
})

test('UTF-8 output remains bytes across frame boundaries and stale sockets are fenced', async () => {
  const f = fixture()
  try {
    const first = await f.open()
    first.message(new Uint8Array([0xf0, 0x9f]).buffer)
    first.message(new Uint8Array([0x92, 0xa5]).buffer)
    f.renders[0].done(); f.renders[1].done()
    assert.deepEqual([...f.renders[0].bytes, ...f.renders[1].bytes], [0xf0, 0x9f, 0x92, 0xa5])
    await f.open()
    const before = f.renders.length
    first.message(new Uint8Array([88]).buffer)
    assert.equal(f.renders.length, before)
  } finally { f.cleanup() }
})

test('slow renderer and oversized frames detach instead of growing an unbounded queue', async () => {
  const f = fixture()
  try {
    const socket = await f.open()
    for (let i = 0; i < 9; i++) socket.message(new Uint8Array(32768).buffer)
    assert.equal(f.states.at(-1), 'reconnecting')
    assert.equal(f.connection.text('blocked'), false)
    assert.equal(f.renders.length, 1)
  } finally { f.cleanup() }
})

test('health preserves missing readings and distinguishes last-known observations', () => {
  assert.equal(value({ value: null, reason: 'initial_sample' }), null)
  assert.equal(value({ value: 0 }), 0)
  assert.equal(percent({ value: 0 }, { value: 0 }), null)
  assert.equal(percent({ value: 100 }, { value: 25 }), 75)
  assert.equal(fresh({ connection: { connected: true, reportFresh: true, reportAt: 1000 } }, 92000), false)
  assert.equal(fresh({ connection: { connected: false, reportFresh: true, reportAt: 1000 } }, 2000), false)
  assert.equal(uptime(null), '—')
  assert.equal(readiness({ready:true,capabilities:{osUser:'root',terminalControl:true,terminalEnabled:true,readiness:'ready'}}),'')
})
