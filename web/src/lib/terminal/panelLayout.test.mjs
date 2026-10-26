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
  // 960 px and wider always docks; a tablet docks while Keep open is on, which it is unless the person turned it off.
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
  // Turned off on a tablet, the full panel is a drawer, so it starts closed; the mini rail never covers anything.
  assert.equal(p.panelMode(834, 'full', false), 'closed')
  assert.equal(p.panelMode(834, 'mini', false), 'mini')
})

test('a tablet docks by default: no saved Keep open means on, and a first width under 960 px is 240', () => {
  const fresh = p.readLayout(null)
  assert.equal(fresh.pinned, true)
  assert.equal(p.panelMode(834, fresh.mode, fresh.pinned), 'full')
  assert.equal(p.panelDocks(834, fresh.pinned), true)
  assert.equal(p.panelDocks(768, p.readLayout('{"open":true,"width":300}').pinned), true)
  // A person who turned Keep open off keeps the drawer.
  assert.equal(p.panelDocks(834, p.readLayout('{"open":true,"mode":"full","width":280,"pinned":false}').pinned), false)
  assert.equal(p.defaultPanelWidth(834), 240)
  assert.equal(p.defaultPanelWidth(959), 240)
  assert.equal(p.defaultPanelWidth(960), 280)
  assert.equal(p.defaultPanelWidth(1360), 280)
  assert.deepEqual(p.defaultLayout(834), { open: true, mode: 'full', width: 240, pinned: true })
  assert.deepEqual(p.defaultLayout(1360), { open: true, mode: 'full', width: 280, pinned: true })
})

test('expanding the rail on a tablet or desktop keeps the panel docked (Keep open turns on)', () => {
  assert.deepEqual(p.changeMode('mini', 'full', false, false), { mode: 'full', pinned: true })
  assert.deepEqual(p.changeMode('mini', 'full', true, false), { mode: 'full', pinned: true })
  assert.deepEqual(p.changeMode('full', 'mini', false, false), { mode: 'mini', pinned: false })
  assert.deepEqual(p.changeMode('closed', 'full', false, false), { mode: 'full', pinned: false })
  // A phone has no rail and no docking: nothing to keep open.
  assert.deepEqual(p.changeMode('mini', 'full', false, true), { mode: 'full', pinned: false })
})

test('phone never docks', () => {
  for (const width of [320, 390, 599]) {
    assert.equal(p.panelDocks(width, true), false)
    assert.equal(p.panelMode(width, 'full', true), 'closed')
    assert.equal(p.panelMode(width, 'mini', true), 'closed')
    assert.equal(p.isPhone(width, 1194), true)
  }
  // A phone is told by its smallest screen side: held sideways it is wider than 600 px and still a phone.
  assert.equal(p.isPhone(844, 390), true)
  assert.equal(p.isPhone(932, 430), true)
  assert.equal(p.panelDocks(844, true, true), false)
  assert.equal(p.panelMode(844, 'mini', true, true), 'closed')
  // A small tablet held sideways with tall browser chrome (834 × 480 page) is not: its screen's short side is 834.
  assert.equal(p.isPhone(834, 834), false)
  assert.equal(p.isPhone(1112, 834), false)
  assert.equal(p.isPhone(834, 600), false)
  assert.equal(p.isPhone(834, 599), true)
})

test('old saved layout still loads', () => {
  // An old layout has no Keep open field: it counts as on, so a tablet docks.
  assert.deepEqual(p.readLayout('{"open":true,"width":300}'), { mode: 'full', width: 300, pinned: true })
  assert.deepEqual(p.readLayout('{"open":false,"width":220}'), { mode: 'closed', width: 240, pinned: true })
  assert.deepEqual(p.readLayout('{"open":true,"width":9000}'), { mode: 'full', width: 480, pinned: true })
  // Nothing (or nothing readable) saved: no mode and no width, so the screen's defaults apply.
  assert.deepEqual(p.readLayout(null), { mode: undefined, width: undefined, pinned: true })
  assert.deepEqual(p.readLayout('not json'), { mode: undefined, width: undefined, pinned: true })
  assert.deepEqual(p.readLayout('[1,2]'), { mode: undefined, width: undefined, pinned: true })
  assert.deepEqual(p.readLayout('null'), { mode: undefined, width: undefined, pinned: true })
  assert.deepEqual(p.readLayout('{"open":"yes","width":"wide","mode":"huge","pinned":1}'), { mode: undefined, width: undefined, pinned: true })
  assert.deepEqual(p.readLayout('{"open":true,"mode":"full","width":280,"pinned":false}'), { mode: 'full', width: 280, pinned: false })
  assert.deepEqual(p.readLayout('{"open":true,"mode":"mini","width":350,"pinned":true}'), { mode: 'mini', width: 350, pinned: true })
  // What is saved is only these four fields, and it reads back the same.
  const record = p.layoutRecord('mini', 333.3, true)
  assert.deepEqual(Object.keys(record).sort(), ['mode', 'open', 'pinned', 'width'])
  assert.deepEqual(p.readLayout(JSON.stringify(record)), { mode: 'mini', width: 333, pinned: true })
  assert.equal(p.layoutRecord('full', 280, false).open, true)
  assert.equal(p.layoutRecord('mini', 280, false).open, false)
})
