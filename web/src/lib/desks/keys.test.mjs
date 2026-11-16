import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./keys.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const k = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

const chord = (code, over = {}) => ({ altKey: true, shiftKey: true, ctrlKey: false, metaKey: false, code, ...over })

test('each Alt+Shift chord names its action', () => {
  const want = {
    KeyD: 'splitRight', KeyE: 'splitDown', ArrowLeft: 'focusLeft', ArrowRight: 'focusRight', ArrowUp: 'focusUp', ArrowDown: 'focusDown',
    KeyZ: 'zoom', KeyW: 'closePane', KeyT: 'newTab', BracketLeft: 'prevTab', BracketRight: 'nextTab',
  }
  for (const [code, action] of Object.entries(want)) assert.equal(k.deskKey(chord(code)), action, code)
})

test('Alt+Shift+1 to 9 pick a tab by number, from 1', () => {
  for (let n = 1; n <= 9; n++) assert.deepEqual(k.deskKey(chord(`Digit${n}`)), { tab: n })
  assert.equal(k.deskKey(chord('Digit0')), null)
})

test('anything that is not exactly Alt+Shift is left to the shell', () => {
  assert.equal(k.deskKey({ altKey: false, shiftKey: false, ctrlKey: false, metaKey: false, code: 'KeyD' }), null, 'plain d')
  assert.equal(k.deskKey(chord('KeyD', { altKey: false, ctrlKey: true })), null, 'Ctrl+Shift+D')
  assert.equal(k.deskKey(chord('KeyD', { shiftKey: false })), null, 'Alt+D')
  assert.equal(k.deskKey(chord('KeyD', { ctrlKey: true })), null, 'Ctrl+Alt+Shift+D')
  assert.equal(k.deskKey(chord('KeyD', { metaKey: true })), null, 'Meta+Alt+Shift+D')
  assert.equal(k.deskKey(chord('KeyM')), null, 'Alt+Shift+M is not a desk key')
  assert.equal(k.deskKey(chord('KeyQ')), null)
})

test('the Keys list names every chord once', () => {
  const labels = k.DESK_KEYS.map(item => item.keys)
  assert.equal(new Set(labels).size, labels.length)
  assert.ok(k.DESK_KEYS.some(item => item.keys === 'Alt+Shift+D' && item.label === 'Split right'))
})
