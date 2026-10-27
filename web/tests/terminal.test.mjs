import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const transpile = name => ts.transpileModule(readFileSync(new URL(`../src/lib/terminal/${name}.ts`, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const url = source => `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`
const apiURL = url(transpile('api'))
const { TerminalConnection } = await import(url(transpile('connection').replace("from './api'", `from '${apiURL}'`)))
const { value, percent, fresh, uptime } = await import(url(transpile('health')))
const { readiness, cardReadiness, validTerminalTitle } = await import(apiURL)
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

test('a resize reports whether it reached the host, so an undelivered size is not treated as sent', async () => {
  const f = fixture()
  try {
    assert.equal(f.connection.resize(54, 30), false)
    const socket = await f.open()
    assert.equal(f.connection.resize(54, 30), false)
    socket.message(new Uint8Array([65]).buffer); f.renders.at(-1).done()
    assert.equal(f.connection.resize(54, 30), true)
    assert.deepEqual(JSON.parse(socket.sent.at(-1)), { type: 'resize', cols: 54, rows: 30 })
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

test('a machine card says plainly when the terminal list does not know the machine yet', () => {
  assert.equal(cardReadiness(undefined), "This machine hasn't connected for terminals yet.")
  assert.equal(cardReadiness({ready:true,capabilities:{osUser:'root',terminalControl:true,terminalEnabled:true,readiness:'ready'}}),'')
  assert.equal(cardReadiness({ready:false,capabilities:{osUser:'root',terminalControl:true,terminalEnabled:true,readiness:'ready'}}),'The machine is reconnecting. Existing shells will be kept.')
  assert.equal(readiness(undefined), 'Choose an enrolled machine.')
})

const { terminalShortcut } = await import(url(transpile('keyboard')))
test('terminal shortcuts preserve interrupts and editor keys while using native paste exactly once', () => {
  const key = (name, changes = {}) => ({ type:'keydown',key:name,ctrlKey:true,metaKey:false,shiftKey:false,altKey:false,...changes })
  assert.equal(terminalShortcut(key('c'),false,false),'terminal')
  assert.equal(terminalShortcut(key('c'),true,false),'copy')
  assert.equal(terminalShortcut(key('c'),true,true),'terminal')
  assert.equal(terminalShortcut(key('c',{shiftKey:true}),false,false),'copy')
  assert.equal(terminalShortcut(key('c',{ctrlKey:false,metaKey:true}),true,true),'copy')
  for (const mac of [true,false]) for (const shiftKey of [true,false]) assert.equal(terminalShortcut(key('v',{shiftKey}),false,mac),'native-paste')
  assert.equal(terminalShortcut(key('v',{ctrlKey:false,metaKey:true}),false,true),'native-paste')
  for (const name of ['x','a','d','z','Tab','Escape']) assert.equal(terminalShortcut(key(name),false,false),'terminal')
  assert.equal(terminalShortcut(key('v',{type:'keyup'}),false,false),'terminal')
  assert.equal(terminalShortcut(key('v',{altKey:true}),false,false),'terminal')
})
test('manual disconnect survives online/visibility recovery until explicitly reconnecting', async () => {
  const f=fixture()
  try {
    let socket=await f.open();socket.message(new Uint8Array([65]).buffer);f.renders.at(-1).done()
    f.connection.detach()
    const count=f.sockets.length
    f.connection.recover();f.connection.recover()
    assert.equal(f.sockets.length,count)
    assert.equal(f.states.at(-1),'detached')
    assert.equal(f.connection.text('do not replay'),false)
    socket=await f.open();socket.message(new Uint8Array([66]).buffer);f.renders.at(-1).done()
    assert.equal(f.states.at(-1),'live')
  } finally { f.cleanup() }
})

const { createTerminal, TerminalError } = await import(apiURL)
async function withFetch(reply, run) {
  const original = globalThis.fetch, requests = []
  globalThis.fetch = async (path, init) => { requests.push({ path, init }); return reply() }
  try { await run(requests) } finally { globalThis.fetch = original }
}
test('a plain create sends exactly the fields it always sent', async () => {
  await withFetch(() => new Response('{"id":"s"}', { status: 200 }), async requests => {
    await createTerminal('agent1', 'req1', 'Shell', 100, 30)
    await createTerminal('agent1', 'req1', 'Shell', 100, 30, undefined, {})
    await createTerminal('agent1', 'req1', 'Shell', 100, 30, undefined, { tool: '', model: '', dir: '', prompt: '' })
    for (const r of requests) assert.equal(r.init.body, '{"agentId":"agent1","requestId":"req1","title":"Shell","cols":100,"rows":30}')
  })
})
test('a launch create sends only the launch fields that are set', async () => {
  await withFetch(() => new Response('{"id":"s"}', { status: 200 }), async requests => {
    await createTerminal('a', 'r', 'Claude Code', 100, 30, undefined, { tool: 'claude' })
    await createTerminal('a', 'r', 'Claude Code · site', 100, 30, undefined, { tool: 'claude', model: 'opus', dir: '/home/me/site', prompt: 'fix it' })
    assert.deepEqual(JSON.parse(requests[0].init.body), { agentId: 'a', requestId: 'r', title: 'Claude Code', cols: 100, rows: 30, tool: 'claude' })
    assert.deepEqual(JSON.parse(requests[1].init.body), { agentId: 'a', requestId: 'r', title: 'Claude Code · site', cols: 100, rows: 30, tool: 'claude', model: 'opus', dir: '/home/me/site', prompt: 'fix it' })
  })
})
test('a refused create carries the status, the server sentence and its code', async () => {
  const refuse = (status, body) => () => new Response(JSON.stringify(body), { status })
  await withFetch(refuse(422, { error: "That folder isn't available on that machine.", code: 'folder_unavailable', message: "That folder isn't available on that machine.", field: 'dir' }), async () => {
    const e = await createTerminal('a', 'r', 'T', 100, 30, undefined, { tool: 'claude', dir: '/x' }).catch(e => e)
    assert.ok(e instanceof TerminalError)
    assert.equal(e.status, 422); assert.equal(e.code, 'folder_unavailable'); assert.equal(e.message, "That folder isn't available on that machine.")
  })
  await withFetch(refuse(409, { error: 'terminal session limit reached' }), async () => {
    const e = await createTerminal('a', 'r', 'T').catch(e => e)
    assert.equal(e.status, 409); assert.equal(e.code, undefined); assert.equal(e.message, 'terminal session limit reached')
  })
  await withFetch(() => new Response('not json', { status: 502 }), async () => {
    const e = await createTerminal('a', 'r', 'T').catch(e => e)
    assert.equal(e.status, 502); assert.equal(e.code, undefined); assert.equal(e.message, 'The terminal connection is unavailable.')
  })
  await withFetch(refuse(422, { error: 'x', code: 7 }), async () => {
    const e = await createTerminal('a', 'r', 'T').catch(e => e)
    assert.equal(e.code, undefined)
  })
})
test('agents-v2 create preserves selected options and omits false approvals', async () => {
  await withFetch(() => new Response('{"id":"s"}', { status: 200 }), async requests => {
    const launch = { tool:'claude', base:'main', effort:'high', worktree:'0123456789abcdef', project:'reet', yolo:false }
    await createTerminal('a','r','T',100,30,undefined,launch)
    await createTerminal('a','r2','T',100,30,undefined,{ ...launch, yolo:true })
    const body = JSON.parse(requests[0].init.body)
    assert.equal(body.base,'main'); assert.equal(body.effort,'high')
    assert.equal(body.worktree,launch.worktree); assert.equal(body.project,'reet')
    assert.equal('yolo' in body,false)
    assert.equal(JSON.parse(requests[1].init.body).yolo,true)
  })
})
