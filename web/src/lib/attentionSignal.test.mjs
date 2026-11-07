import { readFileSync } from 'node:fs'
import { test, mock, afterEach } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = readFileSync(new URL('./attentionSignal.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { signalAttentionChanged, onAttentionChanged, SETTLE_MS } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)

// The trailing settle call is a timer: run it on a mocked clock, and let each test drain its own so none leaks into the next.
mock.timers.enable({ apis: ['setTimeout'] })
afterEach(() => { mock.timers.tick(SETTLE_MS) })

test('a handler is called once per signal', () => {
  let n = 0
  const off = onAttentionChanged(() => { n++ })
  signalAttentionChanged()
  assert.equal(n, 1)
  off()
})

test('two signals call the handler twice', () => {
  let n = 0
  const off = onAttentionChanged(() => { n++ })
  signalAttentionChanged()
  signalAttentionChanged()
  assert.equal(n, 2)
  off()
})

test('unsubscribing stops the calls', () => {
  let n = 0
  const off = onAttentionChanged(() => { n++ })
  signalAttentionChanged()
  off()
  signalAttentionChanged()
  assert.equal(n, 1)
})

test('a throwing handler does not stop the others', () => {
  let n = 0
  const offBad = onAttentionChanged(() => { throw new Error('boom') })
  const offGood = onAttentionChanged(() => { n++ })
  assert.doesNotThrow(() => signalAttentionChanged())
  assert.equal(n, 1)
  offBad()
  offGood()
})

test('subscribing the same function twice calls it once', () => {
  let n = 0
  const handler = () => { n++ }
  const off1 = onAttentionChanged(handler)
  const off2 = onAttentionChanged(handler)
  signalAttentionChanged()
  assert.equal(n, 1)
  off1()
  off2()
})

test('with no subscribers a signal is harmless', () => {
  assert.doesNotThrow(() => signalAttentionChanged())
})

test('the settle delay is one second', () => {
  assert.equal(SETTLE_MS, 1000)
})

test('a signal calls the handlers now and once more after the settle delay', () => {
  let n = 0
  const off = onAttentionChanged(() => { n++ })
  signalAttentionChanged()
  assert.equal(n, 1)
  mock.timers.tick(SETTLE_MS - 1)
  assert.equal(n, 1)
  mock.timers.tick(1)
  assert.equal(n, 2)
  mock.timers.tick(10 * SETTLE_MS)
  assert.equal(n, 2)
  off()
})

test('signals inside the window each call now, and share one trailing call', () => {
  let n = 0
  const off = onAttentionChanged(() => { n++ })
  signalAttentionChanged()
  mock.timers.tick(400)
  signalAttentionChanged()
  assert.equal(n, 2)
  mock.timers.tick(SETTLE_MS)
  assert.equal(n, 3)
  mock.timers.tick(10 * SETTLE_MS)
  assert.equal(n, 3)
  off()
})

test('a later signal moves the trailing call, so it is never closer than the settle delay to the latest one', () => {
  let n = 0
  const off = onAttentionChanged(() => { n++ })
  signalAttentionChanged()
  mock.timers.tick(800)
  signalAttentionChanged()
  assert.equal(n, 2)
  mock.timers.tick(SETTLE_MS - 1)
  assert.equal(n, 2)
  mock.timers.tick(1)
  assert.equal(n, 3)
  off()
})

test('a signal after the trailing call starts a new window', () => {
  let n = 0
  const off = onAttentionChanged(() => { n++ })
  signalAttentionChanged()
  mock.timers.tick(SETTLE_MS)
  assert.equal(n, 2)
  signalAttentionChanged()
  assert.equal(n, 3)
  mock.timers.tick(SETTLE_MS)
  assert.equal(n, 4)
  off()
})

test('a handler unsubscribed before the trailing call is not called by it', () => {
  let n = 0, kept = 0
  const off = onAttentionChanged(() => { n++ })
  const offKept = onAttentionChanged(() => { kept++ })
  signalAttentionChanged()
  off()
  mock.timers.tick(SETTLE_MS)
  assert.equal(n, 1)
  assert.equal(kept, 2)
  off()
  offKept()
})

test('a throwing handler does not stop the others on the trailing call either', () => {
  let n = 0
  const offBad = onAttentionChanged(() => { throw new Error('boom') })
  const offGood = onAttentionChanged(() => { n++ })
  signalAttentionChanged()
  assert.doesNotThrow(() => mock.timers.tick(SETTLE_MS))
  assert.equal(n, 2)
  offBad()
  offGood()
})

test('with no subscribers left the trailing call runs nothing', () => {
  let n = 0
  const off = onAttentionChanged(() => { n++ })
  signalAttentionChanged()
  off()
  assert.doesNotThrow(() => mock.timers.tick(SETTLE_MS))
  assert.equal(n, 1)
})

test('the trailing timer does not keep the process alive', () => {
  let unrefs = 0
  const real = globalThis.setTimeout
  globalThis.setTimeout = (...args) => { const t = real(...args); const u = t.unref?.bind(t); t.unref = () => { unrefs++; return u?.() ?? t }; return t }
  try {
    const off = onAttentionChanged(() => {})
    signalAttentionChanged()
    assert.equal(unrefs, 1)
    off()
  } finally { globalThis.setTimeout = real }
})
