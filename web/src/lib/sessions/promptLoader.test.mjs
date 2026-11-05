import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL(process.env.PROMPT_LOADER_SOURCE || './promptLoader.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { createPromptLoader } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

// A fetch the test settles by hand, so each answer arrives exactly when the scenario says.
function held() {
  const calls = []
  const fetchPrompt = (id, signal) => new Promise((resolve, reject) => calls.push({ id, signal, resolve, reject }))
  return { calls, fetchPrompt }
}
function make(fetchPrompt, id = { current: 'one' }, isMissing) {
  const seen = []
  const loader = createPromptLoader(fetchPrompt, () => id.current, o => seen.push(o.kind), isMissing)
  return { loader, seen, id }
}
const tick = () => new Promise(done => setTimeout(done, 0))

test('an answer for a session the person has left is dropped', async () => {
  const f = held(), { loader, id } = make(f.fetchPrompt)
  const run = loader.load()
  id.current = 'two'
  f.calls[0].resolve('the first session prompt')
  await run
  assert.equal(loader.outcome.kind, 'loading')
})

test('a failure for a session the person has left is dropped', async () => {
  const f = held(), { loader, id } = make(f.fetchPrompt)
  const run = loader.load()
  id.current = 'two'
  f.calls[0].reject(new Error('offline'))
  await run
  assert.equal(loader.outcome.kind, 'loading')
})

test('an aborted read drops a late answer', async () => {
  const f = held(), { loader } = make(f.fetchPrompt)
  const run = loader.load()
  loader.abort()
  f.calls[0].resolve('too late')
  await run
  assert.equal(loader.outcome.kind, 'idle')
})

test('an aborted read is never reported as a failure', async () => {
  const f = held(), { loader, seen } = make(f.fetchPrompt)
  const run = loader.load()
  loader.abort()
  f.calls[0].reject(new DOMException('aborted', 'AbortError'))
  await run
  assert.equal(loader.outcome.kind, 'idle')
  assert.equal(seen.includes('failed'), false)
})

test('an empty prompt or a missing one reads as not kept', async () => {
  const empty = make(async () => '')
  await empty.loader.load()
  assert.equal(empty.loader.outcome.kind, 'none')
  const missing = make(async () => { throw Object.assign(new Error('gone'), { status: 404 }) }, undefined, e => e.status === 404)
  await missing.loader.load()
  assert.equal(missing.loader.outcome.kind, 'none')
})

test('a failed read can be tried again', async () => {
  let n = 0
  const { loader } = make(async () => { if (++n === 1) throw new Error('offline'); return 'the prompt' })
  await loader.load()
  assert.equal(loader.outcome.kind, 'failed')
  await loader.load()
  assert.deepEqual(loader.outcome, { kind: 'ready', text: 'the prompt' })
})

test('a second load while one is in flight wins and the first answer is dropped', async () => {
  const f = held(), { loader } = make(f.fetchPrompt)
  const first = loader.load(), second = loader.load()
  assert.equal(f.calls[0].signal.aborted, true)
  assert.equal(f.calls[1].signal.aborted, false)
  f.calls[1].resolve('the new answer')
  await second
  f.calls[0].resolve('the old answer')
  await first
  await tick()
  assert.deepEqual(loader.outcome, { kind: 'ready', text: 'the new answer' })
})

test('the first read failing late does not spoil the second', async () => {
  const f = held(), { loader } = make(f.fetchPrompt)
  const first = loader.load(), second = loader.load()
  f.calls[1].resolve('the new answer')
  await second
  f.calls[0].reject(new Error('offline'))
  await first
  assert.deepEqual(loader.outcome, { kind: 'ready', text: 'the new answer' })
})
