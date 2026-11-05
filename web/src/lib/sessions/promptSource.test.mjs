import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./promptSource.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const p = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

const chat = (items, truncated = false) => ({ items, truncated })

test('the stored excerpt wins over the chat', () => {
  const v = p.promptView({ promptExcerpt: 'stored', promptMore: true }, chat([{ kind: 'user', key: 'a', text: 'from chat' }]))
  assert.deepEqual(v, { kind: 'stored', excerpt: 'stored', more: true })
  assert.deepEqual(p.promptView({ promptExcerpt: 'stored' }), { kind: 'stored', excerpt: 'stored', more: false })
})

test('without an excerpt the first user item of a complete chat is the prompt', () => {
  const v = p.promptView({}, chat([{ kind: 'agent', key: 'a', text: 'hi' }, { kind: 'user', key: 'b', text: 'first' }, { kind: 'user', key: 'c', text: 'second' }]))
  assert.deepEqual(v, { kind: 'chat', text: 'first' })
})

test('a truncated chat cannot say what the first message was', () => {
  const v = p.promptView({}, chat([{ kind: 'user', key: 'b', text: 'not the first' }], true))
  assert.deepEqual(v, { kind: 'none', why: 'unloaded' })
})

test('no stored excerpt and no usable chat reads as an old session', () => {
  assert.deepEqual(p.promptView({}), { kind: 'none', why: 'old' })
  assert.deepEqual(p.promptView({}, chat([{ kind: 'agent', key: 'a', text: 'hi' }])), { kind: 'none', why: 'old' })
  assert.deepEqual(p.promptView({ promptExcerpt: '' }, chat([])), { kind: 'none', why: 'old' })
})
