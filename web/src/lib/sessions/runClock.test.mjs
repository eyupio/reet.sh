process.env.TZ = 'UTC'

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./runClock.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const c = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

const at = (iso) => Date.parse(iso)
const now = at('2026-11-05T15:14:00Z')

test('lengths read as under a minute, minutes, hours and minutes, or days and hours', () => {
  assert.equal(c.formatLength(0), 'under a minute')
  assert.equal(c.formatLength(59_999), 'under a minute')
  assert.equal(c.formatLength(60_000), '1m')
  assert.equal(c.formatLength(420_000), '7m')
  assert.equal(c.formatLength(3_600_000), '1h 0m')
  assert.equal(c.formatLength(4_320_000), '1h 12m')
  assert.equal(c.formatLength(183_600_000), '2d 3h')
})

test('a negative or missing length reads as under a minute', () => {
  assert.equal(c.formatLength(-5), 'under a minute')
  assert.equal(c.formatLength(Number.NaN), 'under a minute')
})

test('a start today shows the clock time and an earlier day adds the date', () => {
  assert.equal(c.startLabel(at('2026-11-05T14:02:00Z'), now, 'en-GB'), '14:02')
  assert.equal(c.startLabel(at('2026-11-04T14:02:00Z'), now, 'en-GB'), '4 Nov, 14:02')
})

test('a live session shows when it started and how long it has run', () => {
  const s = { state: 'running', createdAt: at('2026-11-05T14:02:00Z') }
  assert.equal(c.runLine(s, now, 'en-GB'), 'Started 14:02 · running 1h 12m')
  assert.equal(c.runLine({ ...s, state: 'creating' }, now, 'en-GB'), 'Started 14:02 · running 1h 12m')
  assert.equal(c.runLine({ ...s, state: 'end_requested' }, now, 'en-GB'), 'Started 14:02 · running 1h 12m')
})

test('a finished session shows how long it ran', () => {
  const s = { state: 'ended', createdAt: at('2026-11-05T14:02:00Z'), completedAt: at('2026-11-05T15:14:00Z') }
  assert.equal(c.runLine(s, now, 'en-GB'), 'Ran 1h 12m')
})

test('a finished session without a completion time shows its start and no length', () => {
  const s = { state: 'lost', createdAt: at('2026-11-05T14:02:00Z') }
  assert.equal(c.runLine(s, now, 'en-GB'), 'Started 14:02')
  assert.equal(c.runLine({ ...s, state: 'exited', completedAt: undefined }, now, 'en-GB'), 'Started 14:02')
})

test('a session that finished before it started reads as under a minute, never negative', () => {
  const s = { state: 'exited', createdAt: at('2026-11-05T14:02:00Z'), completedAt: at('2026-11-05T14:00:00Z') }
  assert.equal(c.runLine(s, now, 'en-GB'), 'Ran under a minute')
})
