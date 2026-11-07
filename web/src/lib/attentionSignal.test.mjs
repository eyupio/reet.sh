import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = readFileSync(new URL('./attentionSignal.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { signalAttentionChanged, onAttentionChanged } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)

test('a handler is called once per signal', () => {
  let n = 0
  const off = onAttentionChanged(() => { n++ })
  signalAttentionChanged()
  assert.equal(n, 1)
  off()
})

test('two signals call the handler twice', () => {
  let n = 0
  const off = onAttentionChanged(() => { n++ })
  signalAttentionChanged()
  signalAttentionChanged()
  assert.equal(n, 2)
  off()
})

test('unsubscribing stops the calls', () => {
  let n = 0
  const off = onAttentionChanged(() => { n++ })
  signalAttentionChanged()
  off()
  signalAttentionChanged()
  assert.equal(n, 1)
})

test('a throwing handler does not stop the others', () => {
  let n = 0
  const offBad = onAttentionChanged(() => { throw new Error('boom') })
  const offGood = onAttentionChanged(() => { n++ })
  assert.doesNotThrow(() => signalAttentionChanged())
  assert.equal(n, 1)
  offBad()
  offGood()
})

test('subscribing the same function twice calls it once', () => {
  let n = 0
  const handler = () => { n++ }
  const off1 = onAttentionChanged(handler)
  const off2 = onAttentionChanged(handler)
  signalAttentionChanged()
  assert.equal(n, 1)
  off1()
  off2()
})

test('with no subscribers a signal is harmless', () => {
  assert.doesNotThrow(() => signalAttentionChanged())
})
