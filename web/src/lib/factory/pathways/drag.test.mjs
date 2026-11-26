import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const compile = file => ts.transpileModule(readFileSync(new URL(file, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { targetAt } = await import(asURL(compile('./drag.ts')))

// Rectangles as getBoundingClientRect gives them.
const rect = (x, y, width, height) => ({ x, y, left: x, top: y, width, height, right: x + width, bottom: y + height })
// Three cards 176 wide with 40 between, inside a line from 0 to 800 across and 100 to 332 down.
const line = rect(0, 100, 800, 232)
const cards = [0, 1, 2].map(n => rect(32 + n * 216, 150, 176, 132))
const stage = { type: 'stage', kind: 'design' }
const skill = { type: 'skill', id: 'test-first' }

test('a stage block or a card is offered the slot whose gap the pointer is nearest', () => {
  assert.deepEqual(targetAt(stage, 20, 200, cards, line, false), { type: 'slot', at: 0 })
  assert.deepEqual(targetAt(stage, 100, 200, cards, line, false), { type: 'slot', at: 0 }) // the first card's first half
  assert.deepEqual(targetAt(stage, 160, 200, cards, line, false), { type: 'slot', at: 1 }) // its second half
  assert.deepEqual(targetAt(stage, 444, 200, cards, line, false), { type: 'slot', at: 2 }) // between cards 1 and 2
  assert.deepEqual(targetAt(stage, 790, 200, cards, line, false), { type: 'slot', at: 3 }) // past the last
  assert.deepEqual(targetAt({ type: 'card', from: 0 }, 444, 120, cards, line, false), { type: 'slot', at: 2 }) // above the cards, still in the line
})

test('a model, skill or prompt is offered the card under the pointer, and nothing between cards', () => {
  assert.deepEqual(targetAt(skill, 300, 200, cards, line, false), { type: 'card', at: 1 })
  assert.deepEqual(targetAt({ type: 'model', choice: 'other' }, 40, 160, cards, line, false), { type: 'card', at: 0 })
  assert.deepEqual(targetAt({ type: 'prompt', text: 'x' }, 600, 280, cards, line, false), { type: 'card', at: 2 })
  assert.equal(targetAt(skill, 228, 200, cards, line, false), null)
})

test('outside the line nothing is offered', () => {
  assert.equal(targetAt(stage, 400, 90, cards, line, false), null)
  assert.equal(targetAt(stage, 810, 200, cards, line, false), null)
  assert.equal(targetAt(skill, 300, 400, cards, line, false), null)
})

test('top to bottom, the slots run down the line', () => {
  const tall = rect(0, 0, 390, 600)
  const rows = [0, 1, 2].map(n => rect(16, 16 + n * 80, 358, 66))
  assert.deepEqual(targetAt(stage, 200, 10, rows, tall, true), { type: 'slot', at: 0 })
  assert.deepEqual(targetAt({ type: 'card', from: 0 }, 200, 170, rows, tall, true), { type: 'slot', at: 2 }) // between rows 1 and 2
  assert.deepEqual(targetAt(stage, 200, 500, rows, tall, true), { type: 'slot', at: 3 })
  assert.deepEqual(targetAt(skill, 200, 110, rows, tall, true), { type: 'card', at: 1 })
  assert.equal(targetAt(skill, 200, 90, rows, tall, true), null)
  assert.equal(targetAt(stage, 200, 610, rows, tall, true), null)
})

test('an empty line offers its only slot', () => {
  assert.deepEqual(targetAt(stage, 400, 200, [], line, false), { type: 'slot', at: 0 })
  assert.equal(targetAt(skill, 400, 200, [], line, false), null)
})
