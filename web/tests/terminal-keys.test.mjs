import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = name => readFileSync(new URL(`../src/lib/terminal/${name}.ts`, import.meta.url), 'utf8')
const url = value => `data:text/javascript;base64,${Buffer.from(value).toString('base64')}`
const k = await import(url(ts.transpileModule(source('virtualKeys'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText))
const { withModifier, withModifiers, modifierCode, modifiers, barKeys, arrowKeys, keyGroups, noModifiers, tapModifier, afterKey, activeModifiers, repeats } = k

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

test('several modifiers join one key: xterm adds their values to one', () => {
  assert.equal(withModifiers('\x1b[A', []), '\x1b[A')
  assert.equal(withModifiers('\x1b[C', ['Ctrl', 'Shift']), '\x1b[1;6C')
  assert.equal(withModifiers('\x1b[D', ['Alt', 'Shift']), '\x1b[1;4D')
  assert.equal(withModifiers('\x1b[3~', ['Ctrl', 'Alt']), '\x1b[3;7~')
  assert.equal(withModifiers('\x1b[H', ['Shift', 'Alt', 'Ctrl']), '\x1b[1;8H')
  // The order they were pressed in does not matter.
  assert.equal(withModifiers('\x1b[C', ['Shift', 'Ctrl']), withModifiers('\x1b[C', ['Ctrl', 'Shift']))
  // A character: Shift capitalises, Ctrl makes the control code, Alt leads with escape.
  assert.equal(withModifiers('c', ['Ctrl', 'Alt']), '\x1b\x03')
  assert.equal(withModifiers('x', ['Alt', 'Shift']), '\x1bX')
  assert.equal(withModifiers('c', ['Ctrl', 'Shift']), '\x03')
  assert.equal(withModifiers('\r', ['Alt']), '\x1b\r')
  assert.equal(withModifiers('\t', ['Shift', 'Alt']), '\x1b\x1b[Z')
  // Text that is neither one character nor a key the table knows is sent as it is: a paste is never rewritten.
  assert.equal(withModifiers('ls -la', ['Ctrl', 'Shift']), 'ls -la')
  assert.equal(withModifiers('\x1b[?1;2c', ['Ctrl']), '\x1b[?1;2c')
})

test('a modifier is tapped on for one key, tapped again to lock, and again to let go', () => {
  let held = noModifiers()
  assert.deepEqual(activeModifiers(held), [])
  held = tapModifier(held, 'Ctrl')
  assert.equal(held.Ctrl, 'next')
  assert.deepEqual(activeModifiers(held), ['Ctrl'])
  // A key uses it up.
  assert.deepEqual(activeModifiers(afterKey(held)), [])
  // A second tap locks it: keys leave it on.
  held = tapModifier(held, 'Ctrl')
  assert.equal(held.Ctrl, 'locked')
  assert.deepEqual(activeModifiers(afterKey(afterKey(held))), ['Ctrl'])
  // A third tap lets go.
  held = tapModifier(held, 'Ctrl')
  assert.equal(held.Ctrl, 'off')
  // Each modifier keeps its own state, and a key uses up only the ones on for one key.
  held = tapModifier(tapModifier(tapModifier(noModifiers(), 'Ctrl'), 'Ctrl'), 'Alt')
  assert.deepEqual(held, { Ctrl: 'locked', Alt: 'next', Shift: 'off' })
  assert.deepEqual(activeModifiers(held), ['Ctrl', 'Alt'])
  assert.deepEqual(afterKey(held), { Ctrl: 'locked', Alt: 'off', Shift: 'off' })
  // Tapping never changes the state it was given.
  const before = noModifiers()
  tapModifier(before, 'Shift')
  assert.deepEqual(before, { Ctrl: 'off', Alt: 'off', Shift: 'off' })
  // With nothing on for one key, afterKey hands back the same state, so nothing is redrawn for each key typed.
  const locked = tapModifier(tapModifier(noModifiers(), 'Shift'), 'Shift')
  assert.equal(afterKey(locked), locked)
})

test('the bar leads with Esc, Tab and the modifiers, and the arrows are their own cluster', () => {
  assert.deepEqual(modifiers.map(key => key.name), ['Ctrl', 'Alt', 'Shift'])
  assert.deepEqual(barKeys.slice(0, 2).map(key => key.label), ['Esc', 'Tab'])
  assert.equal(barKeys.find(key => key.label === 'Esc').value, '\x1b')
  assert.equal(barKeys.find(key => key.label === '^C').value, '\x03')
  assert.deepEqual(arrowKeys.map(key => key.value), ['\x1b[D', '\x1b[A', '\x1b[B', '\x1b[C'])
  assert.deepEqual(arrowKeys.map(key => key.aria), ['Send Left arrow', 'Send Up arrow', 'Send Down arrow', 'Send Right arrow'])
})

test('the second row has every other key, in groups, each with a name of its own', () => {
  assert.deepEqual(keyGroups.map(group => group.id), ['edit', 'symbols', 'ctrl', 'fn'])
  const group = id => keyGroups.find(g => g.id === id).keys
  assert.equal(group('edit').find(key => key.label === 'Enter').value, '\r')
  assert.equal(group('edit').find(key => key.aria === 'Send Backspace').value, '\x7f')
  assert.equal(group('edit').find(key => key.label === 'Home').value, '\x1b[H')
  assert.equal(group('edit').find(key => key.aria === 'Send Page Down').value, '\x1b[6~')
  assert.equal(group('ctrl').find(key => key.aria === 'Send Ctrl+D').value, '\x04')
  // Ctrl+C is on the first row, so the group does not repeat it.
  assert.equal(group('ctrl').some(key => key.value === '\x03'), false)
  assert.equal(group('ctrl').find(key => key.aria === 'Send Ctrl+L').value, '\x0c')
  assert.deepEqual(group('fn').map(key => key.label), Array.from({ length: 12 }, (_, i) => `F${i + 1}`))
  assert.equal(group('fn')[4].value, '\x1b[15~')
  assert.equal(group('symbols').find(key => key.label === '{').value, '{')
  // The symbols on the first row are not repeated in the group.
  for (const first of ['/', '|', '-', '~']) assert.equal(group('symbols').some(key => key.value === first), false, first)
  const all = [...barKeys, ...arrowKeys, ...keyGroups.flatMap(g => g.keys)]
  for (const key of all) {
    assert.ok(key.label, 'every key has a label')
    assert.ok(key.label.length <= 5, `${key.label} fits a key cap`)
    assert.equal(typeof key.value, 'string')
    assert.ok(key.value.length > 0, `${key.label} sends something`)
  }
  // Within the second row no two keys read the same to a screen reader, and none sends what another does.
  const more = keyGroups.flatMap(g => g.keys)
  const names = more.map(key => key.aria ?? `Send ${key.label}`)
  assert.equal(new Set(names).size, names.length)
  assert.equal(new Set(more.map(key => key.value)).size, more.length)
})

test('only keys worth holding repeat', () => {
  for (const value of ['\x1b[A', '\x1b[B', '\x1b[C', '\x1b[D', '\x7f', '\x1b[3~', '\x1b[5~', '\x1b[6~']) assert.equal(repeats(value), true, JSON.stringify(value))
  for (const value of ['\x1b', '\t', '\r', '\x03', '\x04', '\x1bOP', '|', '\x1b[H']) assert.equal(repeats(value), false, JSON.stringify(value))
})
