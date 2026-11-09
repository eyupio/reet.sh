import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./workspaceLayout.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const p = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

test('the sidebar clamps to 192..360 and defaults to 228', () => {
  assert.equal(p.SIDEBAR_MIN, 192)
  assert.equal(p.SIDEBAR_DEFAULT, 228)
  assert.equal(p.clampSidebarWidth(100), 192)
  assert.equal(p.clampSidebarWidth(227.6), 228)
  assert.equal(p.clampSidebarWidth(300.4), 300)
  assert.equal(p.clampSidebarWidth(9000), 360)
  assert.equal(p.clampSidebarWidth(Number.NaN), 228)
})

test('Home\'s panel clamps to 260..460 and starts at 300', () => {
  assert.equal(p.HOME_SIDEBAR_MIN, 260)
  assert.equal(p.HOME_SIDEBAR_MAX, 460)
  assert.equal(p.HOME_SIDEBAR_DEFAULT, 300)
  const home = width => p.clampSidebarWidth(width, p.HOME_SIDEBAR_MIN, p.HOME_SIDEBAR_MAX, p.HOME_SIDEBAR_DEFAULT)
  assert.equal(home(100), 260)
  assert.equal(home(300), 300)
  assert.equal(home(9000), 460)
  assert.equal(home(Number.NaN), 300)
})

test('the rail clamps to 240..460 and keeps the board at least 60 %', () => {
  assert.equal(p.RAIL_DEFAULT, 301)
  assert.equal(p.clampRailWidth(100, 1440), 240)
  assert.equal(p.clampRailWidth(300.6, 1440), 301)
  assert.equal(p.clampRailWidth(9000, 2000), 460)
  // 40 % of 700 is 280: the rail stops there however far it is dragged.
  assert.equal(p.clampRailWidth(460, 700), 280)
  assert.equal(p.clampRailWidth(Number.NaN, 1440), 301)
  // An unmeasured window only has the absolute limits.
  assert.equal(p.clampRailWidth(450, 0), 450)
})

test('a folded panel takes no room; the rail keeps its strip', () => {
  assert.equal(p.sidebarTrack(true, 300), 0)
  assert.equal(p.sidebarTrack(false, 300), 300)
  assert.equal(p.railTrack(true, 300, 1440), p.RAIL_STRIP)
  assert.equal(p.railTrack(false, 300, 1440), 300)
})

test('a stored width is brought into range, and a missing one keeps the fallback', () => {
  assert.equal(p.readWidth(null, 228, p.clampSidebarWidth), 228)
  assert.equal(p.readWidth('', 228, p.clampSidebarWidth), 228)
  assert.equal(p.readWidth('nonsense', 228, p.clampSidebarWidth), 228)
  assert.equal(p.readWidth('120', 228, p.clampSidebarWidth), 192)
  assert.equal(p.readWidth('5000', 228, p.clampSidebarWidth), 360)
  assert.equal(p.readWidth('264', 228, p.clampSidebarWidth), 264)
  assert.equal(p.readCollapsed('1'), true)
  assert.equal(p.readCollapsed('0'), false)
  assert.equal(p.readCollapsed(null), false)
})

test('the separator steps 16 px and Home/End reach the ends', () => {
  assert.equal(p.keyWidth('ArrowRight', 300, 192, 360), 316)
  assert.equal(p.keyWidth('ArrowLeft', 300, 192, 360), 284)
  assert.equal(p.keyWidth('ArrowLeft', 200, 192, 360), 192)
  assert.equal(p.keyWidth('ArrowRight', 350, 192, 360), 360)
  assert.equal(p.keyWidth('Home', 300, 192, 360), 192)
  assert.equal(p.keyWidth('End', 300, 192, 360), 360)
  assert.equal(p.keyWidth('Enter', 300, 192, 360), null)
})
