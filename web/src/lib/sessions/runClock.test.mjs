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

test('a run that crossed midnight keeps the date, so the line reads the same an hour after midnight as at noon', () => {
  // The browser fixtures start a session an hour before now; between 00:00 and 01:00 that start is yesterday evening.
  const justAfterMidnight = at('2026-11-09T00:49:00Z')
  const s = { state: 'running', createdAt: at('2026-11-08T23:49:00Z') }
  assert.equal(c.startLabel(s.createdAt, justAfterMidnight, 'en-GB'), '8 Nov, 23:49')
  assert.equal(c.runLine(s, justAfterMidnight, 'en-GB'), 'Started 8 Nov, 23:49 · running 1h 0m')
  assert.equal(c.runLine(s, justAfterMidnight, 'en-US'), 'Started Nov 8, 23:49 · running 1h 0m')
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

// A Gaffer chat keeps its agent process alive between turns, so the row stays `running` while the broker records
// `completedAt` when a turn goes idle and clears it when the next turn starts. The clock follows that signal, or it
// would keep counting for a chat the Overview already calls finished.
test('a chat whose turn has finished while its agent still runs shows how long it ran, and counts again when the next turn clears it', () => {
  const started = at('2026-11-05T14:02:00Z')
  const idle = { state: 'running', createdAt: started, completedAt: at('2026-11-05T14:12:00Z') }
  assert.equal(c.runLine(idle, now, 'en-GB'), 'Ran 10m')
  assert.equal(c.runLine({ ...idle, completedAt: 0 }, now, 'en-GB'), 'Started 14:02 · running 1h 12m')
  assert.equal(c.runLine({ ...idle, completedAt: undefined }, now, 'en-GB'), 'Started 14:02 · running 1h 12m')
  assert.equal(c.runLine({ state: 'creating', createdAt: started, completedAt: 0 }, now, 'en-GB'), 'Started 14:02 · running 1h 12m')
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

test('a session with no real start time shows no run line rather than an invented one', () => {
  for (const createdAt of [0, -5, Number.NaN, undefined, Number.POSITIVE_INFINITY]) {
    assert.equal(c.runLine({ state: 'running', createdAt }, now, 'en-GB'), '', `createdAt ${createdAt}`)
    assert.equal(c.runLine({ state: 'exited', createdAt, completedAt: now }, now, 'en-GB'), '', `finished, createdAt ${createdAt}`)
  }
})
