import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./contextLayout.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const p = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

test('context panel keeps the terminal at least 60 %', () => {
  for (const container of [1100, 1200, 1440, 1920]) {
    for (const requested of [0, 300, 360, 520, 900]) {
      const width = p.clampContextWidth(requested, container)
      assert.ok(container - width >= container * 0.6 - 0.5, `${requested} in ${container} left the terminal ${container - width}`)
    }
  }
  // 40 % of 1200 is 480: the panel stops there however far it is dragged.
  assert.equal(p.clampContextWidth(520, 1200), 480)
})

test('width clamps 300..520', () => {
  assert.equal(p.clampContextWidth(100, 1920), 300)
  assert.equal(p.clampContextWidth(299.4, 1920), 300)
  assert.equal(p.clampContextWidth(360.4, 1920), 360)
  assert.equal(p.clampContextWidth(521, 1920), 520)
  assert.equal(p.clampContextWidth(9999, 1920), 520)
  assert.equal(p.clampContextWidth(Number.NaN, 1920), p.CONTEXT_DEFAULT)
  assert.equal(p.clampContextWidth(500, 0), 500)
  assert.equal(p.CONTEXT_DEFAULT, 360)
})

test('Left widens, Right narrows, Home and End reach the ends', () => {
  assert.equal(p.contextKeyWidth('ArrowLeft', 360, 1920), 376)
  assert.equal(p.contextKeyWidth('ArrowRight', 360, 1920), 344)
  assert.equal(p.contextKeyWidth('Home', 360, 1920), 300)
  assert.equal(p.contextKeyWidth('End', 360, 1920), 520)
  // The 40 % share wins in a narrower workspace.
  assert.equal(p.contextKeyWidth('End', 360, 1200), 480)
  assert.equal(p.contextKeyWidth('ArrowLeft', 300, 1920), 316)
  assert.equal(p.contextKeyWidth('ArrowRight', 300, 1920), 300)
  assert.equal(p.contextKeyWidth('Enter', 360, 1920), null)
})

test('saved layout reads back, with unreadable values falling back', () => {
  assert.deepEqual(p.readContextLayout(null), { open: undefined, width: undefined })
  assert.deepEqual(p.readContextLayout('not json'), { open: undefined, width: undefined })
  assert.deepEqual(p.readContextLayout('[]'), { open: undefined, width: undefined })
  assert.deepEqual(p.readContextLayout('{"open":false,"width":420}'), { open: false, width: 420 })
  // A saved width outside today's range is brought into it.
  assert.deepEqual(p.readContextLayout('{"width":900}'), { open: undefined, width: 520 })
  assert.deepEqual(p.readContextLayout('{"width":100}'), { open: undefined, width: 300 })
  assert.deepEqual(p.contextLayoutRecord(true, 360.4), { open: true, width: 360 })
  assert.deepEqual(p.contextLayoutRecord(false, 900), { open: false, width: 520 })
})
