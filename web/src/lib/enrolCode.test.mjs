import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./enrolCode.ts', import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const { enrolCountdown, enrolCodeProblem, reenrolDone } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

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

test('each refusal is the card\'s own sentence, whatever words the server used', () => {
  assert.equal(enrolCodeProblem({ status: 429, message: 'too many enrolment codes made; try again later' }), 'You have made too many codes. Try again later.')
  assert.equal(enrolCodeProblem({ status: 429, message: 'you have 20 unused enrolment codes; use one or wait for them to expire' }), 'You have 20 unused codes. Wait for one to run out (each lasts 15 minutes), or use one.')
  assert.equal(enrolCodeProblem({ status: 403, message: 'this account is read-only' }), 'Your account can look but not add machines.')
  assert.equal(enrolCodeProblem({ status: 401, message: 'missing or wrong token' }), 'Sign in again to make a code.')
  assert.equal(enrolCodeProblem({ name: 'UnauthorizedError', message: 'x' }), 'Sign in again to make a code.')
})

test('anything else is one plain sentence, never a raw status line or a network error name', () => {
  const plain = 'The code could not be made. Try again.'
  for (const e of [{ status: 500, message: '500 Internal Server Error' }, { status: 404, message: 'not found' }, { status: 502 }, new TypeError('Failed to fetch'), undefined]) assert.equal(enrolCodeProblem(e), plain)
})

test('a machine enrolled again is done only after it was seen on a personal token and then is not', () => {
  assert.equal(reenrolDone(undefined, true), false)
  assert.equal(reenrolDone({ legacyToken: true }, true), false)
  // Online and already clean at the first read proves nothing.
  assert.equal(reenrolDone({ legacyToken: false }, false), false)
  assert.equal(reenrolDone({}, false), false)
  assert.equal(reenrolDone({ legacyToken: false }, true), true)
  assert.equal(reenrolDone({}, true), true)
})
