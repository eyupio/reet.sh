import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const load = async file => {
  const source = ts.transpileModule(readFileSync(new URL(file, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
  return import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)
}
const g = await load('./typeInAll.ts')
const { pasteNeedsReview } = await load('../terminal/paste.ts')
const review = (text, bracketed) => pasteNeedsReview(text, { bracketed }).review

/** A fake pane: `up` is read on every canSend (a function flips it between two calls), `fails` makes send refuse. */
function fake(name, over = {}) {
  const r = { name, got: [], up: true, reading: false, bracketed: true, fails: false, ...over }
  r.api = {
    canSend: () => (typeof r.up === 'function' ? r.up() : r.up),
    scrollback: () => r.reading,
    bracketed: () => r.bracketed,
    send: (data, paste) => { if (r.fails) return false; r.got.push(paste ? `[paste]${data}` : data); return true },
  }
  return r
}
function tab(panes, over = {}) {
  const byId = Object.fromEntries(panes.map(p => [p.name, p]))
  return {
    mode: { tab: 't', shape: 's', set: panes.map(p => p.name) },
    now: { tab: 't', shape: 's', receivers: panes.map(p => p.name), present: id => !!byId[id], pane: id => byId[id]?.api, name: id => id, ...over },
  }
}

test('typing goes to every other pane of the set, not back to the source', () => {
  const [a, b, c] = ['alpha', 'beta', 'gamma'].map(n => fake(n))
  const { mode, now } = tab([a, b, c])
  assert.deepEqual(g.sendToAll(mode, now, 'alpha', 'x', false, review), { sent: ['beta', 'gamma'], stop: '' })
  assert.deepEqual(a.got, [])
  assert.deepEqual(b.got, ['x'])
  assert.deepEqual(c.got, ['x'])
})

test('a pane whose socket is going is caught at send time, even though it was up a moment before', () => {
  // Up for the first read (as the screen last drew it), down for every read after: the send-time check must see it.
  let reads = 0
  const a = fake('alpha'), b = fake('beta'), c = fake('gamma', { up: () => reads++ === 0 })
  const { mode, now } = tab([a, b, c])
  assert.equal(c.api.canSend(), true)
  const out = g.sendToAll(mode, now, 'alpha', '\r', false, review)
  assert.equal(out.stop, 'Stopped typing in all panes: gamma is no longer connected.')
  assert.deepEqual(out.sent, [])
  assert.deepEqual(b.got, [], 'no other pane gets the key')
  assert.deepEqual(c.got, [])
})

test('a send that fails partway stops and names the panes that did get it', () => {
  const a = fake('alpha'), b = fake('beta'), c = fake('gamma', { fails: true }), d = fake('delta')
  const { mode, now } = tab([a, b, c, d])
  const out = g.sendToAll(mode, now, 'alpha', 'ls\r', false, review)
  assert.deepEqual(out.sent, ['beta'])
  assert.equal(out.stop, 'Stopped typing in all panes: only alpha and beta received the last input.')
  assert.deepEqual(d.got, [], 'nothing after the failed pane')
})

test('the partial notice lists three names as a sentence, and the source alone', () => {
  assert.equal(g.stopPartial(['a', 'b', 'c']), 'Stopped typing in all panes: only a, b and c received the last input.')
  assert.equal(g.stopPartial(['a']), 'Stopped typing in all panes: only a received the last input.')
})

test('a multi-line paste goes nowhere else when one other pane lacks bracketed paste', () => {
  const a = fake('alpha'), b = fake('beta'), c = fake('gamma', { bracketed: false })
  const { mode, now } = tab([a, b, c])
  const out = g.sendToAll(mode, now, 'alpha', 'one\ntwo\nthree', true, review)
  assert.equal(out.stop, 'Stopped typing in all panes: gamma would run this paste line by line.')
  assert.deepEqual(b.got, [])
  assert.deepEqual(c.got, [])
})

test('a one-line paste still goes to a pane without bracketed paste, and a multi-line one to bracketed panes', () => {
  const a = fake('alpha', { bracketed: false }), b = fake('beta'), c = fake('gamma', { bracketed: false })
  const { mode, now } = tab([a, b, c])
  assert.equal(g.sendToAll(mode, now, 'alpha', 'echo hi', true, review).stop, '')
  assert.deepEqual(c.got, ['[paste]echo hi'])
  const d = fake('delta'), e = fake('epsilon')
  const two = tab([d, e])
  assert.equal(g.sendToAll(two.mode, two.now, 'delta', 'one\ntwo', true, review).stop, '')
  assert.deepEqual(e.got, ['[paste]one\ntwo'])
})

test('a pane reading earlier output stops it as the tab changing, not as a lost connection', () => {
  const a = fake('alpha'), b = fake('beta', { reading: true })
  const { mode, now } = tab([a, b], { receivers: ['alpha'] })
  const out = g.sendToAll(mode, now, 'alpha', 'x', false, review)
  assert.equal(out.stop, 'Stopped typing in all panes: the panes on this tab changed.')
  assert.deepEqual(b.got, [])
})

test('a changed tab, a changed shape, a pane gone or a new pane stops it with nothing sent', () => {
  const panes = () => ['alpha', 'beta'].map(n => fake(n))
  for (const over of [{ tab: 'other' }, { shape: 'zoomed' }, { receivers: ['alpha', 'beta', 'gamma'] }]) {
    const [a, b] = panes(), { mode, now } = tab([a, b], over)
    assert.deepEqual(g.sendToAll(mode, now, 'alpha', 'x', false, review), { sent: [], stop: g.STOP_CHANGED })
    assert.deepEqual(b.got, [])
  }
  const [a, b] = panes(), { mode, now } = tab([a, b], { present: id => id === 'alpha', receivers: ['alpha'] })
  assert.equal(g.sendToAll(mode, now, 'alpha', 'x', false, review).stop, g.STOP_CHANGED)
})

test('a pane still on the tab whose terminal is gone (its shell ended) is named', () => {
  const a = fake('alpha'), b = fake('beta')
  const { mode, now } = tab([a, b], { pane: id => (id === 'alpha' ? a.api : undefined), receivers: ['alpha'] })
  assert.equal(g.stopReason(mode, now), 'Stopped typing in all panes: beta is no longer connected.')
})

test('input from a pane outside the set is left alone', () => {
  const a = fake('alpha'), b = fake('beta')
  const { mode, now } = tab([a, b])
  assert.deepEqual(g.sendToAll(mode, now, 'other', 'x', false, review), { sent: [], stop: '' })
  assert.deepEqual(b.got, [])
})
