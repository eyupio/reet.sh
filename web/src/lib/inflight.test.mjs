import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { createIndicator, SHOW_AFTER_MS, SHOW_FOR_AT_LEAST_MS } = await import(`data:text/javascript;base64,${Buffer.from(compile('./inflight.ts')).toString('base64')}`)

// A clock the test advances by hand, so the delays are checked exactly rather than slept through.
function fakeClock() {
  let now = 0, id = 0
  const timers = new Map()
  return {
    setTimeout(fn, ms) { timers.set(++id, { at: now + ms, fn }); return id },
    clearTimeout(h) { timers.delete(h) },
    advance(ms) {
      now += ms
      for (const [h, t] of [...timers].sort((a, b) => a[1].at - b[1].at)) {
        if (t.at <= now) { timers.delete(h); t.fn() }
      }
    },
  }
}

test('a request that answers at once never shows the bar', () => {
  const clock = fakeClock(), ind = createIndicator(clock)
  const seen = []; ind.subscribe(v => seen.push(v))
  const end = ind.begin()
  clock.advance(SHOW_AFTER_MS - 1)
  end()
  clock.advance(10_000)
  assert.equal(ind.visible(), false)
  assert.deepEqual(seen, [])
})

test('a slow request shows the bar after the delay and hides it when the last request ends', () => {
  const clock = fakeClock(), ind = createIndicator(clock)
  const seen = []; ind.subscribe(v => seen.push(v))
  const first = ind.begin()
  clock.advance(SHOW_AFTER_MS)
  assert.equal(ind.visible(), true)
  const second = ind.begin()
  clock.advance(SHOW_FOR_AT_LEAST_MS)
  first()
  assert.equal(ind.visible(), true, 'one request is still outstanding')
  second()
  assert.equal(ind.visible(), false)
  assert.deepEqual(seen, [true, false])
})

test('once shown the bar stays for the minimum so a burst of short requests reads as one wait', () => {
  const clock = fakeClock(), ind = createIndicator(clock)
  const end = ind.begin()
  clock.advance(SHOW_AFTER_MS)
  end()
  assert.equal(ind.visible(), true, 'hidden the instant it appeared would be a flicker')
  clock.advance(SHOW_FOR_AT_LEAST_MS - 1)
  assert.equal(ind.visible(), true)
  clock.advance(1)
  assert.equal(ind.visible(), false)
})

test('ending a request twice does not let the count go negative', () => {
  const clock = fakeClock(), ind = createIndicator(clock)
  const end = ind.begin()
  end(); end()
  assert.equal(ind.pending(), 0)
  ind.begin()
  clock.advance(SHOW_AFTER_MS)
  assert.equal(ind.visible(), true, 'a later request still counts')
})

test('the app reports every request through api.ts to the indicator', () => {
  const api = readFileSync(new URL('./api.ts', import.meta.url), 'utf8')
  assert.match(api, /inflight\.begin\(\)/, 'req() must begin an outstanding request')
  assert.match(api, /finally\s*{\s*end\(\)/, 'and end it however the request finishes')
})
