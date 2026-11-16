import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const url = (src) => `data:text/javascript;base64,${Buffer.from(src).toString('base64')}`
const transpile = (name, rewrite = s => s) => ts.transpileModule(rewrite(readFileSync(new URL(name, import.meta.url), 'utf8')), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const terminalStub = url(transpile('../terminal/api.ts'))
const apiUrl = url(transpile('./api.ts', s => s.replace("'../terminal/api'", `'${terminalStub}'`)))
const { StaleDesk } = await import(apiUrl)
const { apply, createAutosave } = await import(url(transpile('./autosave.ts', s => s
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
  assert.equal(apply(d, { kind: 'renameDesk', name: 'x'.repeat(81) }), null)
  assert.equal(apply(d, { kind: 'colourDesk', colour: 'pink' }), null)
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
  h.a.change({ kind: 'renameDesk', name: 'Kept' })
  await h.timers.tick(400)
  assert.equal(h.states.filter(s => s === 'stale').length, 1)
  assert.equal(h.saves.length, 1)
  assert.equal(h.saves[0].desk.name, 'Kept')
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
  h.a.change({ kind: 'renameDesk', name: 'Keep me' })
  await h.timers.tick(400)
  assert.equal(calls, 1)
  assert.equal(h.states.at(-1), 'unsaved')
  await h.timers.tick(999); assert.equal(calls, 1)
  await h.timers.tick(1); assert.equal(calls, 2)
  await h.timers.tick(1999); assert.equal(calls, 2)
  await h.timers.tick(1); assert.equal(calls, 3)
  await h.timers.tick(3999); assert.equal(calls, 3)
  await h.timers.tick(1); assert.equal(calls, 4)
  assert.equal(h.saves[0].desk.name, 'Keep me')
  assert.equal(h.states.at(-1), 'saved')
})

test('retry delay stops growing at 15 seconds', async () => {
  let calls = 0
  const h = harness(deskOf(1), async () => { calls++; throw new Error('offline') })
  await h.a.ready
  h.a.change({ kind: 'renameDesk', name: 'x' })
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
  h.a.change({ kind: 'renameDesk', name: 'Now' })
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
  a.change({ kind: 'renameDesk', name: 'Mine' })
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
