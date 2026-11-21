import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./chatLoaders.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { loadModels, MODEL_LIST_UNAVAILABLE, pollPlans } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)
const wait = ms => new Promise(resolve => setTimeout(resolve, ms))

test('the plan is read at once and again on every interval, until it is retired', async () => {
  const answers = []
  let reads = 0
  const stop = pollPlans(async () => ++reads, 20, answer => answers.push(answer))
  assert.equal(reads, 1, 'the first read is not delayed')
  await wait(70)
  stop()
  const seen = reads
  assert.ok(seen >= 3, `read again on the interval, got ${seen}`)
  await wait(60)
  assert.equal(reads, seen, 'a retired poll reads no more')
  assert.deepEqual(answers, Array.from({ length: answers.length }, (_, i) => i + 1))
})

test('an answer that lands after the poll was retired is dropped', async () => {
  const answers = []
  let release
  const stop = pollPlans(() => new Promise(resolve => { release = resolve }), 1000, answer => answers.push(answer))
  stop()
  release('late')
  await wait(10)
  assert.deepEqual(answers, [])
})

function loaders(overrides = {}) {
  const log = { machine: undefined, favourites: undefined, notes: [], signals: [] }
  const stop = loadModels({
    machineModels: async signal => { log.signals.push(signal); return { models: ['a'] } },
    favourites: async () => ({ favourites: [{ tool: 'opencode', model: 'a', agentDefault: true }] }),
    onMachine: machine => { log.machine = machine },
    onFavourites: favourites => { log.favourites = favourites },
    onNote: note => log.notes.push(note),
    ...overrides,
  })
  return { log, stop }
}

test('models and favourites are both handed on when they arrive', async () => {
  const { log } = loaders()
  await wait(10)
  assert.deepEqual(log.machine, { models: ['a'] })
  assert.equal(log.favourites.length, 1)
  assert.deepEqual(log.notes, [])
})

test('a failed models read leaves the note and a failed favourites read leaves nothing', async () => {
  const { log } = loaders({ machineModels: async () => { throw new Error('down') }, favourites: async () => { throw new Error('down') } })
  await wait(10)
  assert.deepEqual(log.notes, [MODEL_LIST_UNAVAILABLE])
  assert.equal(log.machine, undefined)
  assert.equal(log.favourites, undefined)
})

test('retiring aborts the reads and nothing that lands afterwards is handed on', async () => {
  let release, releaseFavourites, fail
  const { log, stop } = loaders({
    machineModels: signal => new Promise(resolve => { release = resolve }),
    favourites: () => new Promise(resolve => { releaseFavourites = resolve }),
  })
  stop()
  release({ models: ['late'] }); releaseFavourites({ favourites: [] })
  await wait(10)
  assert.equal(log.machine, undefined)
  assert.equal(log.favourites, undefined)
  const failing = loaders({ machineModels: signal => new Promise((_, reject) => { fail = reject }) })
  failing.stop()
  fail(new Error('aborted'))
  await wait(10)
  assert.deepEqual(failing.log.notes, [], 'an aborted read does not leave a note')
})

test('retiring aborts the signal the read was given', () => {
  const { log, stop } = loaders()
  assert.equal(log.signals[0].aborted, false)
  stop()
  assert.equal(log.signals[0].aborted, true)
})
