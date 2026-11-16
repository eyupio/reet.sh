import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const url = (src) => `data:text/javascript;base64,${Buffer.from(src).toString('base64')}`
const transpile = (name, rewrite = s => s) => ts.transpileModule(rewrite(readFileSync(new URL(name, import.meta.url), 'utf8')), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const terminalStub = url(transpile('../terminal/api.ts'))
const apiUrl = url(transpile('./api.ts', s => s.replace("'../terminal/api'", `'${terminalStub}'`)))
const { StaleDesk } = await import(apiUrl)
const { TerminalError } = await import(terminalStub)
const { apply, createAutosave, TOO_LARGE, MAX_DOC_BYTES } = await import(url(transpile('./autosave.ts', s => s
  .replace("'../projectColour'", `'${url(transpile('../projectColour.ts'))}'`)
  .replace("'../terminal/api'", `'${terminalStub}'`)
  .replace("'./api'", `'${apiUrl}'`)
  .replace("'./tree'", `'${url(transpile('./tree.ts'))}'`))))

const pane = (id, extra = {}) => ({ id, name: '', colour: '', host: 'm1', session: '', kind: 'shell', dir: '', dirAt: 0, start: '', ...extra })
const tabOf = (id, ...ids) => {
  const tab = { id, name: id, colour: '', root: null, zoomed: '', active: '', panes: {} }
  ids.forEach((p, i) => {
    tab.panes[p] = pane(p)
    tab.root = i === 0 ? { pane: p } : { split: 'right', ratio: 0.5, a: tab.root, b: { pane: p } }
  })
  return tab
}
const deskOf = (version, tabs = [tabOf('t1', 'a', 'b')]) => ({ id: 'd1', name: 'Desk', colour: '', template: false, position: 0, version, schema: 1, doc: { tabs, active: tabs[0]?.id ?? '' } })

/** A clock that only moves when told to. */
function clock() {
  let now = 0, next = 1
  const pending = new Map()
  return {
    setTimeout(fn, ms) { const id = next++; pending.set(id, { at: now + ms, fn }); return id },
    clearTimeout(id) { pending.delete(id) },
    async tick(ms) {
      const to = now + ms
      for (;;) {
        const due = [...pending.entries()].filter(([, t]) => t.at <= to).sort((a, b) => a[1].at - b[1].at)[0]
        if (!due) break
        pending.delete(due[0]); now = due[1].at; due[1].fn()
        await settle()
      }
      now = to
      await settle()
    },
    count: () => pending.size,
  }
}
const settle = async () => { for (let i = 0; i < 20; i++) await Promise.resolve() }

function harness(first, saveImpl) {
  const timers = clock()
  const states = [], saves = []
  let server = first
  const save = saveImpl ?? (async (desk, expect) => {
    saves.push({ desk, expect })
    server = { ...desk, version: expect + 1 }
    return server
  })
  const a = createAutosave({
    load: async () => server,
    save: async (d, e, k) => { const r = save(d, e, k); return r },
    onState: s => states.push(s), timers, channel: null, delay: 400,
  })
  return { a, timers, states, saves, setServer: d => { server = d }, getServer: () => server }
}

test('apply is pure and returns null when a change no longer applies', () => {
  const d = deskOf(1)
  const before = JSON.stringify(d)
  const r = apply(d, { kind: 'renameTab', tab: 't1', name: 'Build' })
  assert.equal(r.doc.tabs[0].name, 'Build')
  assert.equal(JSON.stringify(d), before)
  assert.equal(apply(d, { kind: 'renameTab', tab: 'gone', name: 'x' }), null)
  assert.equal(apply(d, { kind: 'renameTab', tab: 't1', name: 'x'.repeat(81) }), null)
  assert.equal(apply(d, { kind: 'colourTab', tab: 't1', colour: 'pink' }), null)
  assert.equal(apply(d, { kind: 'ratio', tab: 't1', path: 'ab', ratio: 0.3 }), null)
  assert.equal(apply(d, { kind: 'closePane', tab: 't1', pane: 'zzz' }), null)
  assert.equal(apply(d, { kind: 'addPane', tab: 't1', at: 'a', dir: 'down', pane: pane('c') }).doc.tabs[0].panes.c.id, 'c')
})

test('apply keeps the tab and pane limits', () => {
  const full = tabOf('t1', ...'abcdefghi'.split(''))
  assert.equal(apply(deskOf(1, [full]), { kind: 'addPane', tab: 't1', at: 'a', dir: 'down', pane: pane('z') }), null)
  const tabs = Array.from({ length: 20 }, (_, i) => tabOf('t' + i, 'a'))
  assert.equal(apply(deskOf(1, tabs), { kind: 'addTab', tab: tabOf('new', 'a') }), null)
  const one = apply(deskOf(1), { kind: 'addTab', tab: tabOf('t2', 'a'), index: 0 })
  assert.deepEqual(one.doc.tabs.map(t => t.id), ['t2', 't1'])
})

test('a rename whose name holds a hidden character does not apply', () => {
  const d = deskOf(1)
  for (const name of ['a\nb', 'a\u0007b', 'a b', 'a‮b', 'a⁦b', 'a​b', 'a‎b', 'a؜b', 'a\u0085b']) {
    assert.equal(apply(d, { kind: 'renameTab', tab: 't1', name }), null, JSON.stringify(name))
    assert.equal(apply(d, { kind: 'renamePane', tab: 't1', pane: 'a', name }), null, JSON.stringify(name))
  }
  assert.equal(apply(d, { kind: 'renameTab', tab: 't1', name: 'déploy 🚀' }).doc.tabs[0].name, 'déploy 🚀')
  assert.equal(apply(d, { kind: 'renamePane', tab: 't1', pane: 'a', name: 'logs' }).doc.tabs[0].panes.a.name, 'logs')
})

test('setSession binds a session to a pane, once across the desk', () => {
  const d = deskOf(1, [tabOf('t1', 'a', 'b'), tabOf('t2', 'c')])
  const bound = apply(d, { kind: 'setSession', tab: 't1', pane: 'a', session: 's9' })
  assert.equal(bound.doc.tabs[0].panes.a.session, 's9')
  assert.equal(d.doc.tabs[0].panes.a.session, '')
  // Binding the same session again to the same pane is fine; to another pane anywhere on the desk it is not.
  assert.equal(apply(bound, { kind: 'setSession', tab: 't1', pane: 'a', session: 's9' }).doc.tabs[0].panes.a.session, 's9')
  assert.equal(apply(bound, { kind: 'setSession', tab: 't1', pane: 'b', session: 's9' }), null)
  assert.equal(apply(bound, { kind: 'setSession', tab: 't2', pane: 'c', session: 's9' }), null)
  assert.equal(apply(bound, { kind: 'setSession', tab: 't1', pane: 'a', session: '' }).doc.tabs[0].panes.a.session, '')
  assert.equal(apply(d, { kind: 'setSession', tab: 't1', pane: 'gone', session: 's9' }), null)
})

test('setPlace gives a pane a machine and a folder, stamped newer', () => {
  const d = deskOf(1, [tabOf('t1', 'a')])
  d.doc.tabs[0].panes.a.dirAt = Date.now() + 60_000
  const moved = apply(d, { kind: 'setPlace', tab: 't1', pane: 'a', host: 'm2', dir: '/home/dev/new' })
  assert.equal(moved.doc.tabs[0].panes.a.host, 'm2')
  assert.equal(moved.doc.tabs[0].panes.a.dir, '/home/dev/new')
  assert.equal(moved.doc.tabs[0].panes.a.dirAt, d.doc.tabs[0].panes.a.dirAt + 1)
  assert.equal(d.doc.tabs[0].panes.a.host, 'm1')
  assert.equal(apply(d, { kind: 'setPlace', tab: 't1', pane: 'a', host: '', dir: '/x' }), null)
  assert.equal(apply(d, { kind: 'setPlace', tab: 't1', pane: 'zz', host: 'm2', dir: '/x' }), null)
})

test('setSession and setPlace are saved, and replayed onto a newer desk', async () => {
  const h = harness(deskOf(1))
  await h.a.ready
  h.a.change({ kind: 'setPlace', tab: 't1', pane: 'a', host: 'm2', dir: '/srv' })
  h.a.change({ kind: 'setSession', tab: 't1', pane: 'b', session: 's5' })
  const newer = deskOf(2, [tabOf('t1', 'a', 'b')])
  newer.name = 'Elsewhere'
  h.setServer(newer)
  await h.a.refresh()
  await h.a.flush()
  assert.equal(h.saves.length, 1)
  assert.equal(h.saves[0].expect, 2)
  assert.equal(h.saves[0].desk.name, 'Elsewhere')
  assert.equal(h.saves[0].desk.doc.tabs[0].panes.a.host, 'm2')
  assert.equal(h.saves[0].desk.doc.tabs[0].panes.a.dir, '/srv')
  assert.equal(h.saves[0].desk.doc.tabs[0].panes.b.session, 's5')
})

test('debounces three changes into one save', async () => {
  const h = harness(deskOf(3))
  await h.a.ready
  h.a.change({ kind: 'renameTab', tab: 't1', name: 'one' })
  await h.timers.tick(200)
  h.a.change({ kind: 'renameTab', tab: 't1', name: 'two' })
  await h.timers.tick(200)
  h.a.change({ kind: 'renameTab', tab: 't1', name: 'three' })
  await h.timers.tick(399)
  assert.equal(h.saves.length, 0)
  await h.timers.tick(1)
  assert.equal(h.saves.length, 1)
  assert.equal(h.saves[0].expect, 3)
  assert.equal(h.saves[0].desk.doc.tabs[0].name, 'three')
  assert.deepEqual(h.states, ['saved', 'unsaved', 'saving', 'saved'])
  assert.equal(h.a.desk().version, 4)
})

test('serialises saves', async () => {
  const gates = []
  let live = 0, peak = 0, version = 1
  const h = harness(deskOf(1), (desk, expect) => new Promise(resolve => {
    live++; peak = Math.max(peak, live)
    gates.push(() => { live--; version = expect + 1; resolve({ ...desk, version }) })
  }))
  await h.a.ready
  h.a.change({ kind: 'renameTab', tab: 't1', name: 'one' })
  await h.timers.tick(400)
  assert.equal(gates.length, 1)
  h.a.change({ kind: 'renameTab', tab: 't1', name: 'two' })
  await h.timers.tick(1000)
  assert.equal(gates.length, 1, 'no second save while one is in flight')
  gates[0]()
  await settle()
  await h.timers.tick(400)
  assert.equal(gates.length, 2)
  gates[1]()
  await settle()
  assert.equal(peak, 1)
  assert.equal(h.a.desk().doc.tabs[0].name, 'two')
  assert.equal(h.a.desk().version, 3)
  assert.equal(h.states.at(-1), 'saved')
})

test('merges a rename onto a newer doc', async () => {
  const newer = deskOf(5, [tabOf('t1', 'a', 'b')])
  newer.doc.tabs[0].panes.a = pane('a', { dir: '/srv/new', dirAt: 99 })
  let calls = 0
  const h = harness(deskOf(4), async (desk, expect) => {
    calls++
    if (calls === 1) { assert.equal(expect, 4); throw new StaleDesk(newer) }
    assert.equal(expect, 5)
    h.saves.push({ desk, expect })
    return { ...desk, version: 6 }
  })
  await h.a.ready
  h.a.change({ kind: 'renameTab', tab: 't1', name: 'Build' })
  await h.timers.tick(400)
  assert.equal(h.saves.length, 1)
  const merged = h.saves[0].desk
  assert.equal(merged.doc.tabs[0].name, 'Build')
  assert.equal(merged.doc.tabs[0].panes.a.dir, '/srv/new')
  assert.equal(merged.doc.tabs[0].panes.a.dirAt, 99)
  assert.equal(h.a.desk().version, 6)
  assert.equal(h.states.includes('stale'), false)
  assert.equal(h.states.at(-1), 'saved')
})

test('drops a ratio change for a split that is gone and reports stale', async () => {
  const newer = deskOf(5, [{ ...tabOf('t1', 'a'), name: 'solo' }])
  let calls = 0
  const h = harness(deskOf(4), async (desk, expect) => {
    calls++
    if (calls === 1) throw new StaleDesk(newer)
    h.saves.push({ desk, expect })
    return { ...desk, version: expect + 1 }
  })
  await h.a.ready
  h.a.change({ kind: 'ratio', tab: 't1', path: '', ratio: 0.3 })
  h.a.change({ kind: 'renameTab', tab: 't1', name: 'Kept' })
  await h.timers.tick(400)
  assert.equal(h.states.filter(s => s === 'stale').length, 1)
  assert.equal(h.saves.length, 1)
  assert.equal(h.saves[0].desk.doc.tabs[0].name, 'Kept')
  assert.equal(h.saves[0].desk.doc.tabs[0].root.pane, 'a')
  assert.equal(h.states.at(-1), 'stale')
})

test('retries a failed save and keeps the change', async () => {
  let calls = 0
  const stamps = []
  const h = harness(deskOf(1), async (desk, expect) => {
    calls++
    stamps.push(calls)
    if (calls <= 3) throw new Error('offline')
    h.saves.push({ desk, expect })
    return { ...desk, version: expect + 1 }
  })
  await h.a.ready
  h.a.change({ kind: 'renameTab', tab: 't1', name: 'Keep me' })
  await h.timers.tick(400)
  assert.equal(calls, 1)
  assert.equal(h.states.at(-1), 'unsaved')
  await h.timers.tick(999); assert.equal(calls, 1)
  await h.timers.tick(1); assert.equal(calls, 2)
  await h.timers.tick(1999); assert.equal(calls, 2)
  await h.timers.tick(1); assert.equal(calls, 3)
  await h.timers.tick(3999); assert.equal(calls, 3)
  await h.timers.tick(1); assert.equal(calls, 4)
  assert.equal(h.saves[0].desk.doc.tabs[0].name, 'Keep me')
  assert.equal(h.states.at(-1), 'saved')
})

test('retry delay stops growing at 15 seconds', async () => {
  let calls = 0
  const h = harness(deskOf(1), async () => { calls++; throw new Error('offline') })
  await h.a.ready
  h.a.change({ kind: 'renameTab', tab: 't1', name: 'x' })
  await h.timers.tick(400)
  for (const wait of [1000, 2000, 4000, 8000, 15000, 15000]) {
    const at = calls
    await h.timers.tick(wait - 1); assert.equal(calls, at)
    await h.timers.tick(1); assert.equal(calls, at + 1)
  }
})

test('refresh keeps queued changes', async () => {
  const newer = deskOf(7, [tabOf('t1', 'a', 'b')])
  newer.name = 'Renamed elsewhere'
  const h = harness(deskOf(6))
  await h.a.ready
  h.a.change({ kind: 'renameTab', tab: 't1', name: 'Local' })
  h.setServer(newer)
  await h.a.refresh()
  assert.equal(h.a.desk().name, 'Renamed elsewhere')
  assert.equal(h.a.desk().doc.tabs[0].name, 'Local')
  assert.equal(h.a.desk().version, 7)
  await h.timers.tick(400)
  assert.equal(h.saves[0].expect, 7)
  assert.equal(h.saves[0].desk.doc.tabs[0].name, 'Local')
  assert.equal(h.saves[0].desk.name, 'Renamed elsewhere')
})

test('flush saves at once and passes keepalive through', async () => {
  const seen = []
  const h = harness(deskOf(1), async (desk, expect, keepalive) => { seen.push(keepalive); return { ...desk, version: expect + 1 } })
  await h.a.ready
  h.a.change({ kind: 'renameTab', tab: 't1', name: 'Now' })
  await h.a.flush(true)
  assert.deepEqual(seen, [true])
  assert.equal(h.timers.count(), 0)
  assert.equal(h.states.at(-1), 'saved')
})

test('a message from another window refreshes, and our own saves are announced', async () => {
  const sent = []
  const chan = { postMessage: m => sent.push(m), onmessage: null, close() {} }
  let server = deskOf(1)
  const timers = clock()
  const a = createAutosave({ load: async () => server, save: async (d, e) => (server = { ...d, version: e + 1 }), onState() {}, timers, channel: chan })
  await a.ready
  server = { ...deskOf(2), name: 'Elsewhere' }
  chan.onmessage({ data: { id: 'd1', version: 2 } })
  await settle()
  assert.equal(a.desk().name, 'Elsewhere')
  a.change({ kind: 'renameTab', tab: 't1', name: 'Mine' })
  await timers.tick(400)
  assert.deepEqual(sent, [{ id: 'd1', version: 3 }])
  a.stop()
  assert.equal(chan.onmessage, null)
})

test('a save whose response was lost is not replayed', async () => {
  let calls = 0
  let applied = null
  const h = harness(deskOf(1), async (desk, expect) => {
    calls++
    h.saves.push({ desk, expect })
    if (calls === 1) {
      applied = { ...desk, version: expect + 1 }
      h.setServer(applied)
      throw new Error('response lost')
    }
    if (expect === applied.version) {
      applied = { ...desk, version: expect + 1 }
      h.setServer(applied)
      return applied
    }
    throw new StaleDesk(applied)
  })
  await h.a.ready
  h.a.change({ kind: 'swap', tab: 't1', a: 'a', b: 'b' })
  await h.timers.tick(400)
  assert.equal(calls, 1)
  assert.equal(h.states.at(-1), 'unsaved')
  await h.timers.tick(1000)
  assert.equal(calls, 2, 'the retry was sent once')
  await h.timers.tick(60000)
  assert.equal(calls, 2, 'no further save is sent')
  assert.deepEqual(h.a.desk().doc.tabs[0].root, applied.doc.tabs[0].root)
  assert.equal(h.a.desk().doc.tabs[0].root.a.pane, 'b', 'swapped exactly once')
  assert.equal(h.a.desk().doc.tabs[0].root.b.pane, 'a')
  assert.equal(h.a.desk().version, 2)
  assert.equal(h.states.at(-1), 'saved')
  assert.equal(h.states.includes('stale'), false)
})

test('a lost response followed by another window\'s edit still merges', async () => {
  let calls = 0
  let theirs = null
  const h = harness(deskOf(1), async (desk, expect) => {
    calls++
    h.saves.push({ desk, expect })
    if (calls === 1) throw new Error('offline')
    if (calls === 2) {
      theirs = { ...deskOf(2), name: 'Renamed elsewhere' }
      throw new StaleDesk(theirs)
    }
    return { ...desk, version: expect + 1 }
  })
  await h.a.ready
  h.a.change({ kind: 'swap', tab: 't1', a: 'a', b: 'b' })
  await h.timers.tick(400)
  await h.timers.tick(1000)
  assert.equal(calls, 3)
  const last = h.saves[2]
  assert.equal(last.expect, 2)
  assert.equal(last.desk.name, 'Renamed elsewhere')
  assert.equal(last.desk.doc.tabs[0].root.a.pane, 'b', 'the change is replayed onto the other copy')
  assert.equal(h.states.at(-1), 'saved')
})

test('a change made during a save whose response is lost is kept and nothing is replayed', async () => {
  let calls = 0
  let applied = null
  const h = harness(deskOf(1), async (desk, expect) => {
    calls++
    h.saves.push({ desk, expect })
    if (calls === 1) {
      applied = { ...desk, version: expect + 1 }
      h.setServer(applied)
      h.a.change({ kind: 'renameTab', tab: 't1', name: 'Build' })
      throw new Error('response lost')
    }
    if (expect === applied.version) {
      applied = { ...desk, version: expect + 1 }
      h.setServer(applied)
      return applied
    }
    throw new StaleDesk(applied)
  })
  await h.a.ready
  h.a.change({ kind: 'swap', tab: 't1', a: 'a', b: 'b' })
  await h.timers.tick(400)
  await h.timers.tick(60000)
  const d = h.a.desk()
  assert.equal(d.doc.tabs[0].root.a.pane, 'b', 'swapped exactly once')
  assert.equal(d.doc.tabs[0].root.b.pane, 'a')
  assert.equal(d.doc.tabs[0].name, 'Build')
  assert.equal(applied.doc.tabs[0].name, 'Build')
  assert.equal(h.states.at(-1), 'saved')
  assert.equal(h.states.includes('stale'), false)
})

test('movePaneToTab moves a pane in one step, and takes the tab it leaves when it was the last', () => {
  const d = deskOf(1, [tabOf('t1', 'a', 'b'), tabOf('t2', 'c'), tabOf('t3', 'e')])
  d.doc.tabs[1].active = 'c'
  const moved = apply(d, { kind: 'movePaneToTab', from: 't1', to: 't2', pane: 'b' })
  assert.deepEqual(Object.keys(moved.doc.tabs[0].panes), ['a'])
  assert.deepEqual(moved.doc.tabs[0].root, { pane: 'a' })
  assert.deepEqual(moved.doc.tabs[1].root, { split: 'right', ratio: 0.5, a: { pane: 'c' }, b: { pane: 'b' } })
  assert.equal(moved.doc.tabs[1].active, 'b')
  assert.equal(moved.doc.active, 't2')
  // The last pane of a tab takes the tab with it.
  const last = apply(d, { kind: 'movePaneToTab', from: 't3', to: 't2', pane: 'e' })
  assert.deepEqual(last.doc.tabs.map(t => t.id), ['t1', 't2'])
  assert.ok(last.doc.tabs[1].panes.e)
  // Nothing moves when either end is gone or full: the pane never leaves one tab without reaching the other.
  assert.equal(apply(d, { kind: 'movePaneToTab', from: 't1', to: 'gone', pane: 'b' }), null)
  assert.equal(apply(d, { kind: 'movePaneToTab', from: 't1', to: 't2', pane: 'zz' }), null)
  assert.equal(apply(d, { kind: 'movePaneToTab', from: 't1', to: 't1', pane: 'b' }), null)
  const full = deskOf(1, [tabOf('t1', 'a', 'b'), tabOf('t9', ...'cdefghijk'.split(''))])
  assert.equal(apply(full, { kind: 'movePaneToTab', from: 't1', to: 't9', pane: 'b' }), null)
})

test('a move whose target tab went in another window is dropped whole at the merge', async () => {
  const newer = deskOf(5, [tabOf('t1', 'a', 'b')])
  let calls = 0
  const h = harness(deskOf(4, [tabOf('t1', 'a', 'b'), tabOf('t2', 'c')]), async (desk, expect) => {
    calls++
    if (calls === 1) throw new StaleDesk(newer)
    h.saves.push({ desk, expect })
    return { ...desk, version: expect + 1 }
  })
  await h.a.ready
  h.a.change({ kind: 'movePaneToTab', from: 't1', to: 't2', pane: 'b' })
  await h.timers.tick(400)
  // The move no longer applies, so b stays where it was rather than leaving the desk.
  assert.deepEqual(Object.keys(h.a.desk().doc.tabs[0].panes).sort(), ['a', 'b'])
  assert.equal(h.states.at(-1), 'stale')
})

test('a save the server refuses for good is not retried: the queue is dropped and the desk read again', async () => {
  let calls = 0, loads = 0
  const refusals = []
  const timers = clock()
  const states = []
  let server = deskOf(1)
  const a = createAutosave({
    load: async () => { loads++; return server },
    save: async () => { calls++; throw new TerminalError(409, 'A session can be in only one pane.', 'desk_session_taken') },
    onState: s => states.push(s), onRefused: m => refusals.push(m), timers, channel: null, delay: 400,
  })
  await a.ready
  a.change({ kind: 'renameTab', tab: 't1', name: 'Mine' })
  a.change({ kind: 'setSession', tab: 't1', pane: 'a', session: 'taken' })
  await timers.tick(400)
  assert.equal(calls, 1)
  assert.deepEqual(refusals, ["reet couldn't save your last change: A session can be in only one pane. Showing the desk as it is saved."])
  assert.equal(states.at(-1), 'refused')
  assert.equal(a.desk().doc.tabs[0].name, 't1', 'the desk is shown as it is saved')
  assert.equal(a.desk().doc.tabs[0].panes.a.session, '')
  await settle()
  assert.equal(loads, 2, 'the desk was read again')
  await timers.tick(60000)
  assert.equal(calls, 1, 'nothing is retried')
  assert.equal(timers.count(), 0)
  // The next change is saved as usual and the state moves on.
  let saved = 0
  server = deskOf(1)
  const b = createAutosave({
    load: async () => server,
    save: async (desk, expect) => { saved++; if (saved === 1) throw new TerminalError(400, "That desk can't be saved: bad tree.", 'desk_invalid'); return { ...desk, version: expect + 1 } },
    onState: s => states.push(s), timers, channel: null, delay: 400,
  })
  await b.ready
  b.change({ kind: 'renameTab', tab: 't1', name: 'One' })
  await b.flush()
  assert.equal(states.at(-1), 'refused')
  b.change({ kind: 'renameTab', tab: 't1', name: 'Two' })
  await b.flush()
  assert.equal(saved, 2)
  assert.equal(b.desk().doc.tabs[0].name, 'Two')
  assert.equal(states.at(-1), 'saved')
})

test('a sign-in, a timeout, a rate limit and a server error are retried, not refused', async () => {
  for (const status of [401, 408, 429, 500, 503]) {
    let calls = 0
    const refusals = []
    const h = harness(deskOf(1), async (desk, expect) => { calls++; if (calls === 1) throw new TerminalError(status, 'later'); return { ...desk, version: expect + 1 } })
    await h.a.ready
    h.a.change({ kind: 'renameTab', tab: 't1', name: 'Kept' })
    await h.timers.tick(400)
    assert.equal(h.states.at(-1), 'unsaved', String(status))
    await h.timers.tick(1000)
    assert.equal(calls, 2, String(status))
    assert.equal(h.a.desk().doc.tabs[0].name, 'Kept')
    assert.deepEqual(refusals, [])
  }
})

test('a change that would take the document over 64 KiB is refused before it is made', async () => {
  const refusals = []
  const timers = clock()
  const saves = []
  // Twenty tabs of nine panes; each start-up command of 2000 characters takes the document nearer the limit.
  const tabs = Array.from({ length: 20 }, (_, i) => tabOf('t' + i, ...Array.from({ length: 9 }, (_, j) => `p${i}x${j}`)))
  const first = deskOf(1, tabs)
  const size = d => new TextEncoder().encode(JSON.stringify(d.doc)).length
  assert.ok(size(first) < MAX_DOC_BYTES / 2, String(size(first)))
  const a = createAutosave({ load: async () => first, save: async (d, e) => { saves.push(d); return { ...d, version: e + 1 } }, onState() {}, onRefused: m => refusals.push(m), timers, channel: null })
  await a.ready
  let refusedAt = -1
  for (let i = 0; i < 180 && refusedAt < 0; i++) {
    a.change({ kind: 'start', tab: 't' + Math.floor(i / 9), pane: `p${Math.floor(i / 9)}x${i % 9}`, start: 'y'.repeat(2000) })
    if (refusals.length) refusedAt = i
  }
  assert.ok(refusedAt > 0)
  assert.deepEqual(refusals, [TOO_LARGE])
  assert.equal(TOO_LARGE, 'This desk is as large as it can be. Remove a tab or shorten a start-up command.')
  assert.ok(size(a.desk()) <= MAX_DOC_BYTES)
  assert.ok(size(a.desk()) > MAX_DOC_BYTES - 2100, 'every change that fitted was made')
  const refusedPane = a.desk().doc.tabs[Math.floor(refusedAt / 9)].panes[`p${Math.floor(refusedAt / 9)}x${refusedAt % 9}`]
  assert.equal(refusedPane.start, '', 'the refused change was not made')
  // A change that makes it smaller is still made.
  a.change({ kind: 'closeTab', tab: 't19' })
  assert.equal(a.desk().doc.tabs.length, 19)
  await a.flush()
  assert.ok(size(saves.at(-1)) <= MAX_DOC_BYTES)
})

test('a retired autosave keeps retrying until its queue is saved, then stops', async () => {
  let calls = 0
  const h = harness(deskOf(1), async (desk, expect) => { calls++; if (calls <= 2) throw new Error('offline'); h.saves.push({ desk, expect }); return { ...desk, version: expect + 1 } })
  await h.a.ready
  h.a.change({ kind: 'renameTab', tab: 't1', name: 'Kept' })
  h.a.retire()
  await settle()
  assert.equal(calls, 1, 'sent at once')
  await h.timers.tick(1000)
  assert.equal(calls, 2)
  await h.timers.tick(2000)
  assert.equal(calls, 3)
  assert.equal(h.saves[0].desk.doc.tabs[0].name, 'Kept')
  assert.equal(h.timers.count(), 0, 'stopped once saved')
  // A retired autosave with nothing queued stops at once, and takes no more changes.
  const idle = harness(deskOf(1))
  await idle.a.ready
  idle.a.retire()
  idle.a.change({ kind: 'renameTab', tab: 't1', name: 'Late' })
  await idle.timers.tick(10000)
  assert.equal(idle.saves.length, 0)
})
