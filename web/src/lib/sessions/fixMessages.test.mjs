import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./fixMessages.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const f = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

test('the conflicts message names the base branch, defaulting to a generic one', () => {
  assert.equal(f.fixMessage('conflicts', { base: 'main' }), 'Resolve the merge conflicts with main on this branch, then push.')
  assert.equal(f.fixMessage('conflicts', {}), 'Resolve the merge conflicts with the base branch on this branch, then push.')
})

test('the checks message quotes each failing check', () => {
  assert.equal(f.fixMessage('checks', { checks: ['build', 'lint'] }), 'These checks are failing on the pull request: "build", "lint". Please find out why and fix them.')
  assert.equal(f.fixMessage('checks', { checks: ['build'] }), 'This check is failing on the pull request: "build". Please find out why and fix it.')
})

test('more than three checks are cut to three and counted', () => {
  const four = f.fixMessage('checks', { checks: ['a', 'b', 'c', 'd'] })
  assert.match(four, /"a", "b", "c" \(and 1 more\)/)
  const ten = f.fixMessage('checks', { checks: Array.from({ length: 10 }, (_, i) => `c${i}`) })
  assert.match(ten, /"c0", "c1", "c2" \(and 7 more\)/)
  assert.doesNotMatch(ten, /c3/)
})

test('control characters and newlines in a check name never reach the message', () => {
  const m = f.fixMessage('checks', { checks: ['bu\nild\r\t\u0000\u001b[31m', 'x\u2028y\u0085z'] })
  assert.doesNotMatch(m, /[\u0000-\u001f\u007f-\u009f\u2028\u2029]/)
  assert.match(m, /"bu ?ild/)
})

test('URLs in a check name are removed and no URL appears in any message', () => {
  const m = f.fixMessage('checks', { checks: ['deploy https://evil.example/x?a=1 now', 'http://a.test/', 'ok'] })
  assert.doesNotMatch(m, /https?:\/\//)
  assert.doesNotMatch(m, /evil\.example/)
  assert.match(m, /"deploy now"/)
})

test('a very long check name is cut at 60 characters with an ellipsis', () => {
  const m = f.fixMessage('checks', { checks: ['x'.repeat(200)] })
  assert.ok(m.includes(`"${'x'.repeat(59)}…"`), m)
  assert.ok(!m.includes('x'.repeat(60)))
})

test('every message is at most 2000 characters', () => {
  const huge = Array.from({ length: 500 }, () => 'y'.repeat(500))
  assert.ok(f.fixMessage('checks', { checks: huge }).length <= 2000)
  assert.ok(f.fixMessage('conflicts', { base: 'b'.repeat(5000) }).length <= 2000)
})

test('the threads message is singular for one and plural for several', () => {
  assert.equal(f.fixMessage('threads', { threads: 1 }), 'There is 1 unresolved review thread on the pull request. Please read it and address it.')
  assert.equal(f.fixMessage('threads', { threads: 3 }), 'There are 3 unresolved review threads on the pull request. Please read each one and address it.')
})

test('an empty draft is replaced by the message alone', () => {
  assert.equal(f.prefillText('', 'Fix it.'), 'Fix it.')
  assert.equal(f.prefillText('  \n ', 'Fix it.'), 'Fix it.')
})

test('an existing draft is kept and the message follows on a new line', () => {
  assert.equal(f.prefillText('Please also', 'Fix it.'), 'Please also\nFix it.')
})

test('a draft near the limit is never truncated: the result is null', () => {
  assert.equal(f.prefillText('d'.repeat(1990), 'Resolve the merge conflicts with main on this branch, then push.'), null)
  assert.equal(f.prefillText('d'.repeat(10), 'm'.repeat(10), 21), 'd'.repeat(10) + '\n' + 'm'.repeat(10))
  assert.equal(f.prefillText('d'.repeat(10), 'm'.repeat(10), 20), null)
})
