import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = name => readFileSync(new URL(`../src/lib/terminal/${name}.ts`, import.meta.url), 'utf8')
const url = value => `data:text/javascript;base64,${Buffer.from(value).toString('base64')}`
const { withModifier, modifierCode, modifiers, stripKeys, comboKeys, navigationKeys, functionKeys, symbolKeys } = await import(url(ts.transpileModule(source('virtualKeys'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText))

test('a modifier joins the next key the way xterm sends it', () => {
  assert.equal(withModifier('c', null), 'c')
  assert.equal(withModifier('c', 'Ctrl'), '\x03')
  assert.equal(withModifier('C', 'Ctrl'), '\x03')
  assert.equal(withModifier('1', 'Ctrl'), '1')
  assert.equal(withModifier('x', 'Alt'), '\x1bx')
  assert.equal(withModifier('a', 'Shift'), 'A')
  assert.equal(withModifier('\t', 'Shift'), '\x1b[Z')
  assert.equal(withModifier('\x1b[A', 'Ctrl'), '\x1b[1;5A')
  assert.equal(withModifier('\x1b[D', 'Shift'), '\x1b[1;2D')
  assert.equal(withModifier('\x1b[5~', 'Alt'), '\x1b[5;3~')
  assert.equal(withModifier('\x1bOP', 'Shift'), '\x1b[1;2P')
  assert.deepEqual([modifierCode('Shift'), modifierCode('Alt'), modifierCode('Ctrl')], [2, 3, 5])
})

test('the keys cover navigation, combinations, functions and symbols without duplicate labels', () => {
  assert.deepEqual(modifiers.map(key => key.name), ['Ctrl', 'Alt', 'Shift'])
  assert.deepEqual(stripKeys.slice(2, 6).map(key => key.value), ['\x1b[A', '\x1b[B', '\x1b[D', '\x1b[C'])
  assert.equal(stripKeys.find(key => key.label === 'Esc').value, '\x1b')
  assert.equal(stripKeys.find(key => key.label === 'Enter').value, '\r')
  assert.equal(stripKeys.find(key => key.label === 'Backspace').value, '\x7f')
  assert.equal(comboKeys.find(key => key.label === 'Ctrl+C').value, '\x03')
  assert.equal(comboKeys.find(key => key.label === 'Ctrl+D').value, '\x04')
  assert.equal(navigationKeys.find(key => key.label === 'Home').value, '\x1b[H')
  assert.deepEqual(functionKeys.map(key => key.label), Array.from({ length: 12 }, (_, i) => `F${i + 1}`))
  assert.equal(symbolKeys.find(key => key.label === '|').value, '|')
  const all = [...stripKeys, ...comboKeys, ...navigationKeys, ...functionKeys, ...symbolKeys]
  for (const key of all) {
    assert.ok(key.label, 'every key has a label')
    assert.equal(typeof key.value, 'string')
    assert.ok(key.value.length > 0, `${key.label} sends something`)
  }
  assert.equal(new Set(all.map(key => key.label)).size, all.length)
})
