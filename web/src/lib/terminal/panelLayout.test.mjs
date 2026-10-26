import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./panelLayout.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const p = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

test('terminal keeps at least 60 %', () => {
  for (const container of [600, 700, 800, 834, 1000, 1200]) {
    for (const requested of [0, 240, 280, 400, 480, 900]) {
      const width = p.clampPanelWidth(requested, container)
      assert.ok(container - width >= container * 0.6 - 0.5, `${requested} in ${container} left the terminal ${container - width}`)
    }
  }
  // 40 % of 800 is 320: the panel stops there however far it is dragged.
  assert.equal(p.clampPanelWidth(480, 800), 320)
})

test('width clamps 240..480', () => {
  assert.equal(p.clampPanelWidth(100, 1400), 240)
  assert.equal(p.clampPanelWidth(239.6, 1400), 240)
  assert.equal(p.clampPanelWidth(300.4, 1400), 300)
  assert.equal(p.clampPanelWidth(481, 1400), 480)
  assert.equal(p.clampPanelWidth(9999, 1400), 480)
  assert.equal(p.clampPanelWidth(Number.NaN, 1400), p.PANEL_DEFAULT)
  // An unmeasured container (0) only has the absolute limits.
  assert.equal(p.clampPanelWidth(470, 0), 470)
  assert.equal(p.PANEL_DEFAULT, 280)
  assert.equal(p.MINI_WIDTH, 64)
})

test('mini below 160', () => {
  assert.deepEqual(p.dragWidth(159, 1400), { mode: 'mini', width: null })
  assert.deepEqual(p.dragWidth(40, 1400), { mode: 'mini', width: null })
  assert.deepEqual(p.dragWidth(160, 1400), { mode: 'full', width: 240 })
  assert.deepEqual(p.dragWidth(333, 1400), { mode: 'full', width: 333 })
  assert.deepEqual(p.dragWidth(700, 800), { mode: 'full', width: 320 })
  // The chevron cycles full → mini → closed → full; on a phone it only opens and closes.
  assert.equal(p.chevronNext('full', false), 'mini')
  assert.equal(p.chevronNext('mini', false), 'closed')
  assert.equal(p.chevronNext('closed', false), 'full')
  assert.equal(p.chevronNext('full', true), 'closed')
  assert.equal(p.chevronNext('closed', true), 'full')
})

test('keyboard steps 16 px, snaps to mini below the minimum and Home/End reach the ends', () => {
  assert.deepEqual(p.keyWidth('ArrowRight', 'full', 280, 1400), { mode: 'full', width: 296 })
  assert.deepEqual(p.keyWidth('ArrowLeft', 'full', 280, 1400), { mode: 'full', width: 264 })
  assert.deepEqual(p.keyWidth('ArrowLeft', 'full', 240, 1400), { mode: 'mini', width: 240 })
  assert.deepEqual(p.keyWidth('ArrowRight', 'mini', 300, 1400), { mode: 'full', width: 240 })
  assert.deepEqual(p.keyWidth('ArrowLeft', 'mini', 300, 1400), { mode: 'mini', width: 300 })
  assert.deepEqual(p.keyWidth('End', 'full', 280, 1400), { mode: 'full', width: 480 })
  assert.deepEqual(p.keyWidth('End', 'mini', 280, 800), { mode: 'full', width: 320 })
  assert.deepEqual(p.keyWidth('Home', 'full', 400, 1400), { mode: 'mini', width: 400 })
  assert.deepEqual(p.keyWidth('ArrowRight', 'full', 320, 800), { mode: 'full', width: 320 })
  assert.equal(p.keyWidth('Enter', 'full', 280, 1400), null)
})

test('pinned wide screen docks', () => {
  // 960 px and wider always docks; a tablet docks once pinned (Keep open).
  assert.equal(p.panelDocks(1360, false), true)
  assert.equal(p.panelDocks(960, false), true)
  assert.equal(p.panelDocks(834, false), false)
  assert.equal(p.panelDocks(834, true), true)
  assert.equal(p.panelDocks(600, true), true)
  assert.equal(p.panelMode(1360, undefined, false), 'full')
  assert.equal(p.panelMode(1360, 'mini', false), 'mini')
  assert.equal(p.panelMode(1360, 'closed', false), 'closed')
  assert.equal(p.panelMode(834, 'full', true), 'full')
  assert.equal(p.panelMode(834, undefined, true), 'full')
  assert.equal(p.panelMode(834, 'mini', true), 'mini')
  // Unpinned on a tablet the full panel is a drawer, so it starts closed as before; the mini rail never covers anything.
  assert.equal(p.panelMode(834, 'full', false), 'closed')
  assert.equal(p.panelMode(834, 'mini', false), 'mini')
})

test('phone never docks', () => {
  for (const width of [320, 390, 599]) {
    assert.equal(p.panelDocks(width, true), false)
    assert.equal(p.panelMode(width, 'full', true), 'closed')
    assert.equal(p.panelMode(width, 'mini', true), 'closed')
    assert.equal(p.isPhone(width, false), true)
  }
  // A phone held sideways is wider than 600 px but keeps the drawer.
  assert.equal(p.isPhone(844, true), true)
  assert.equal(p.panelDocks(844, true, true), false)
  assert.equal(p.panelMode(844, 'mini', true, true), 'closed')
  assert.equal(p.isPhone(834, false), false)
})

test('old saved layout still loads', () => {
  assert.deepEqual(p.readLayout('{"open":true,"width":300}'), { mode: 'full', width: 300, pinned: false })
  assert.deepEqual(p.readLayout('{"open":false,"width":220}'), { mode: 'closed', width: 240, pinned: false })
  assert.deepEqual(p.readLayout('{"open":true,"width":9000}'), { mode: 'full', width: 480, pinned: false })
  assert.deepEqual(p.readLayout(null), { mode: undefined, width: 280, pinned: false })
  assert.deepEqual(p.readLayout('not json'), { mode: undefined, width: 280, pinned: false })
  assert.deepEqual(p.readLayout('[1,2]'), { mode: undefined, width: 280, pinned: false })
  assert.deepEqual(p.readLayout('null'), { mode: undefined, width: 280, pinned: false })
  assert.deepEqual(p.readLayout('{"open":"yes","width":"wide","mode":"huge","pinned":1}'), { mode: undefined, width: 280, pinned: false })
  assert.deepEqual(p.readLayout('{"open":true,"mode":"mini","width":350,"pinned":true}'), { mode: 'mini', width: 350, pinned: true })
  // What is saved is only these four fields, and it reads back the same.
  const record = p.layoutRecord('mini', 333.3, true)
  assert.deepEqual(Object.keys(record).sort(), ['mode', 'open', 'pinned', 'width'])
  assert.deepEqual(p.readLayout(JSON.stringify(record)), { mode: 'mini', width: 333, pinned: true })
  assert.equal(p.layoutRecord('full', 280, false).open, true)
  assert.equal(p.layoutRecord('mini', 280, false).open, false)
})
