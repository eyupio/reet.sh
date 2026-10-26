import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('../src/lib/terminal/sizing.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const s = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

test('a stored font size wins; with nothing stored phones start at 11 px and other screens at 14 px', () => {
  assert.equal(s.initialFontSize(null, false), 14)
  assert.equal(s.initialFontSize(null, true), 11)
  assert.equal(s.initialFontSize('', true), 11)
  assert.equal(s.initialFontSize('16', true), 16)
  assert.equal(s.initialFontSize('10', false), 10)
  assert.equal(s.initialFontSize('22', true), 22)
  for (const bad of ['9', '23', 'large', '0', '-12']) {
    assert.equal(s.initialFontSize(bad, true), 11)
    assert.equal(s.initialFontSize(bad, false), 14)
  }
})

test('the size sent to the host never drops below 20 columns by 8 rows, nor above the protocol limits', () => {
  assert.deepEqual(s.hostSize(41, 1), { cols: 41, rows: 8 })
  assert.deepEqual(s.hostSize(2, 30), { cols: 20, rows: 30 })
  assert.deepEqual(s.hostSize(0, 0), { cols: 20, rows: 8 })
  assert.deepEqual(s.hostSize(55, 27), { cols: 55, rows: 27 })
  assert.deepEqual(s.hostSize(900, 900), { cols: 500, rows: 300 })
})

function timers() {
  let now = 0, next = 1
  const pending = new Map()
  return {
    set: (fn, ms) => { const id = next++; pending.set(id, { fn, at: now + ms }); return id },
    clear: id => { pending.delete(id) },
    advance(ms) {
      const until = now + ms
      for (;;) {
        const due = [...pending.entries()].filter(([, t]) => t.at <= until).sort((a, b) => a[1].at - b[1].at)[0]
        if (!due) break
        pending.delete(due[0]); now = due[1].at; due[1].fn()
      }
      now = until
    },
    get pending() { return pending.size },
  }
}
function sender(options = {}) {
  const clock = timers(), sent = []
  const accept = { value: true }
  const resize = new s.ResizeSender(size => { if (!accept.value) return false; sent.push(size); return true }, { delay: 150, gap: 300, setTimer: clock.set, clearTimer: clock.clear, ...options })
  return { clock, sent, resize, accept }
}

test('a burst of sizes, like a keyboard animation, sends only the final size once the burst settles', () => {
  const { clock, sent, resize } = sender()
  resize.known({ cols: 54, rows: 40 })
  for (let rows = 39; rows >= 20; rows--) { resize.request({ cols: 54, rows }); clock.advance(16) }
  assert.deepEqual(sent, [])
  // The last request was 16 ms ago.
  clock.advance(133)
  assert.deepEqual(sent, [])
  clock.advance(1)
  assert.deepEqual(sent, [{ cols: 54, rows: 20 }])
})

test('a size the host already has is never sent again', () => {
  const { clock, sent, resize } = sender()
  resize.known({ cols: 54, rows: 40 })
  resize.request({ cols: 54, rows: 40 }); clock.advance(500)
  assert.deepEqual(sent, [])
  resize.request({ cols: 54, rows: 30 }); clock.advance(500)
  resize.request({ cols: 54, rows: 30 }); clock.advance(500)
  resize.request({ cols: 54, rows: 31 }); resize.request({ cols: 54, rows: 30 }); clock.advance(500)
  assert.deepEqual(sent, [{ cols: 54, rows: 30 }])
})

test('the attachment ticket is the size the host starts with, so a reconnect at the same size sends nothing', () => {
  const { clock, sent, resize } = sender()
  resize.known({ cols: 54, rows: 30 })
  resize.request({ cols: 54, rows: 20 })
  resize.known({ cols: 54, rows: 30 })
  clock.advance(500)
  assert.deepEqual(sent, [])
})

test('a size the connection could not send is retried by the next request, not forgotten', () => {
  const { clock, sent, resize, accept } = sender()
  resize.known({ cols: 54, rows: 40 })
  accept.value = false
  resize.request({ cols: 54, rows: 20 }); clock.advance(500)
  accept.value = true
  resize.request({ cols: 54, rows: 20 }); clock.advance(500)
  assert.deepEqual(sent, [{ cols: 54, rows: 20 }])
})

test('a redraw nudge changes the height by one row and then restores the wanted size after a gap', () => {
  const { clock, sent, resize } = sender()
  resize.known({ cols: 54, rows: 30 })
  resize.nudge()
  assert.deepEqual(sent, [{ cols: 54, rows: 29 }])
  clock.advance(299)
  assert.equal(sent.length, 1)
  clock.advance(1)
  assert.deepEqual(sent, [{ cols: 54, rows: 29 }, { cols: 54, rows: 30 }])
  clock.advance(1000)
  assert.equal(sent.length, 2)
})

test('a size requested during a nudge is the size the nudge ends on', () => {
  const { clock, sent, resize } = sender()
  resize.known({ cols: 54, rows: 30 })
  resize.nudge()
  clock.advance(50)
  resize.request({ cols: 55, rows: 27 })
  // The nudge keeps its full gap, so tmux sees two separate size changes.
  clock.advance(249)
  assert.deepEqual(sent, [{ cols: 54, rows: 29 }])
  clock.advance(1)
  assert.deepEqual(sent, [{ cols: 54, rows: 29 }, { cols: 55, rows: 27 }])
  clock.advance(1000)
  assert.equal(sent.length, 2)
})

test('a nudge before any known size, or one the connection refuses, sends nothing more', () => {
  const { clock, sent, resize, accept } = sender()
  resize.nudge(); clock.advance(1000)
  assert.deepEqual(sent, [])
  resize.known({ cols: 54, rows: 30 })
  accept.value = false
  resize.nudge(); clock.advance(1000)
  assert.deepEqual(sent, [])
  assert.equal(clock.pending, 0)
})

test('dispose cancels a pending send', () => {
  const { clock, sent, resize } = sender()
  resize.known({ cols: 54, rows: 30 })
  resize.request({ cols: 54, rows: 20 })
  resize.dispose(); clock.advance(1000)
  assert.deepEqual(sent, [])
})

const reading = (innerHeight, visual, innerWidth = 390) => ({ innerWidth, innerHeight, visual: visual === undefined ? { height: innerHeight, offsetTop: 0, scale: 1 } : visual })

test('without a keyboard the usable height is the visual viewport', () => {
  const v = new s.ViewportTracker()
  assert.deepEqual(v.read(reading(631)), { height: 631, top: 0, shrink: 0 })
  assert.deepEqual(v.read(reading(631, null)), { height: 631, top: 0, shrink: 0 })
})

test('when the page keeps its layout height (keyboard overlays it) the visual viewport and its offset are used', () => {
  const v = new s.ViewportTracker()
  v.read(reading(844))
  const open = v.read(reading(844, { height: 464, offsetTop: 12, scale: 1 }))
  assert.equal(open.height, 464)
  assert.equal(open.top, 12)
  assert.ok(Math.abs(open.shrink - (1 - 464 / 844)) < 1e-9)
})

test('when the layout viewport itself shrinks (resizes-content) its height wins over an under-reported visual viewport', () => {
  const v = new s.ViewportTracker()
  v.read(reading(631))
  // The browser hid its bottom toolbar and resized the page for the keyboard; the visual viewport still subtracts the toolbar.
  const open = v.read(reading(449, { height: 358, offsetTop: 0, scale: 1 }))
  assert.equal(open.height, 449)
  assert.equal(open.top, 0)
  assert.ok(open.shrink > 0.25)
})

test('a small browser toolbar change is not a layout resize, and a new width starts a new baseline', () => {
  const v = new s.ViewportTracker()
  v.read(reading(700))
  assert.equal(v.read(reading(660, { height: 640, offsetTop: 0, scale: 1 })).height, 640)
  const turned = v.read(reading(390, undefined, 844))
  assert.equal(turned.shrink, 0)
})

test('the keyboard focus layout needs a phone, touch, typing in the terminal, an unzoomed page and a large shrink', () => {
  const yes = { coarse: true, narrow: true, editing: true, scale: 1 }
  assert.equal(s.keyboardLikelyOpen(0.45, yes), true)
  assert.equal(s.keyboardLikelyOpen(0.25, yes), true)
  assert.equal(s.keyboardLikelyOpen(0.2, yes), false)
  assert.equal(s.keyboardLikelyOpen(0.45, { ...yes, coarse: false }), false)
  assert.equal(s.keyboardLikelyOpen(0.45, { ...yes, narrow: false }), false)
  assert.equal(s.keyboardLikelyOpen(0.45, { ...yes, editing: false }), false)
  assert.equal(s.keyboardLikelyOpen(0.45, { ...yes, scale: 1.5 }), false)
})

test('overflow hints mark the edges that hide more controls', () => {
  assert.deepEqual(s.overflowEdges(0, 376, 376), { start: false, end: false })
  assert.deepEqual(s.overflowEdges(0, 587, 376), { start: false, end: true })
  assert.deepEqual(s.overflowEdges(100, 587, 376), { start: true, end: true })
  assert.deepEqual(s.overflowEdges(211, 587, 376), { start: true, end: false })
  assert.deepEqual(s.overflowEdges(210.4, 587, 376), { start: true, end: false })
})
