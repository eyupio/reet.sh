import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./sectionMemory.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const m = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

const first = { waiting: true, working: false }
const store = value => ({ getItem: () => value })

test('nothing stored gives the first state', () => {
  assert.deepEqual(m.readSections(store(null), 'k', first), first)
})
test('stored values are merged over the first state', () => {
  assert.deepEqual(m.readSections(store('{"working":true}'), 'k', first), { waiting: true, working: true })
})
test('unreadable JSON or a non-object gives the first state', () => {
  for (const bad of ['{nope', '7', '"x"', 'null', '[true]']) assert.deepEqual(m.readSections(store(bad), 'k', first), first)
})
test('a storage that throws gives the first state and never throws', () => {
  const boom = { getItem() { throw new Error('blocked') }, setItem() { throw new Error('full') } }
  assert.deepEqual(m.readSections(boom, 'k', first), first)
  assert.doesNotThrow(() => m.writeSections(boom, 'k', first))
})
test('undefined storage gives the first state and writes nothing', () => {
  assert.deepEqual(m.readSections(undefined, 'k', first), first)
  assert.doesNotThrow(() => m.writeSections(undefined, 'k', first))
})
test('a write can be read back', () => {
  const data = {}
  const s = { getItem: k => data[k] ?? null, setItem: (k, v) => { data[k] = v } }
  m.writeSections(s, 'reet.factory.sections.u1', { waiting: false, working: true })
  assert.deepEqual(m.readSections(s, 'reet.factory.sections.u1', first), { waiting: false, working: true })
})
