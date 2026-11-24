import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./enrolCode.ts', import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const { enrolCountdown, enrolCodeProblem } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

const now = 1_800_000_000_000
const at = (secondsLeft) => now / 1000 + secondsLeft

test('the countdown reads minutes and seconds, rounding up so it never shows 0:00 while valid', () => {
  assert.deepEqual(enrolCountdown(at(900), now), { expired: false, text: 'Expires in 15:00' })
  assert.deepEqual(enrolCountdown(at(65), now), { expired: false, text: 'Expires in 1:05' })
  assert.deepEqual(enrolCountdown(at(0.2), now), { expired: false, text: 'Expires in 0:01' })
})

test('the countdown says when the code has run out', () => {
  for (const left of [0, -1, -3600]) assert.deepEqual(enrolCountdown(at(left), now), { expired: true, text: 'This code has run out.' })
})

test('a bad expiry counts as run out, not as a code that never expires', () => {
  for (const bad of [NaN, undefined, null, 'soon']) assert.equal(enrolCountdown(bad, now).expired, true)
})

test('errors use the server sentence for the known refusals', () => {
  const full = 'you have 20 unused enrolment codes; use one or wait for them to expire'
  assert.equal(enrolCodeProblem({ status: 429, message: full }), full)
  assert.equal(enrolCodeProblem({ status: 429, message: 'too many enrolment codes made; try again later' }), 'too many enrolment codes made; try again later')
  assert.equal(enrolCodeProblem({ status: 403, message: 'this account is read-only' }), 'this account is read-only')
})

test('a controller without accounts is told to use its configured token', () => {
  assert.match(enrolCodeProblem({ status: 404, message: '404 Not Found' }), /does not use accounts/)
})

test('anything else is a plain sentence, never a raw status line or a network error name', () => {
  assert.equal(enrolCodeProblem({ status: 500, message: '500 Internal Server Error' }), 'Could not make an enrolment code. Try again.')
  assert.equal(enrolCodeProblem(new TypeError('Failed to fetch')), 'Could not reach reet.sh. Check the connection and try again.')
  assert.equal(enrolCodeProblem(undefined), 'Could not reach reet.sh. Check the connection and try again.')
})
