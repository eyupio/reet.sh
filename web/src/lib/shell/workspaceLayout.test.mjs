import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./workspaceLayout.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const p = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

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

test('a folded rail keeps its strip', () => {
  assert.equal(p.railTrack(true, 300, 1440), p.RAIL_STRIP)
  assert.equal(p.railTrack(false, 300, 1440), 300)
})

test('a stored width is brought into range, and a missing one keeps the fallback', () => {
  const clamp = width => p.clampRailWidth(width, 0)
  assert.equal(p.readWidth(null, p.RAIL_DEFAULT, clamp), p.RAIL_DEFAULT)
  assert.equal(p.readWidth('', p.RAIL_DEFAULT, clamp), p.RAIL_DEFAULT)
  assert.equal(p.readWidth('nonsense', p.RAIL_DEFAULT, clamp), p.RAIL_DEFAULT)
  assert.equal(p.readWidth('120', p.RAIL_DEFAULT, clamp), p.RAIL_MIN)
  assert.equal(p.readWidth('5000', p.RAIL_DEFAULT, clamp), p.RAIL_MAX)
  assert.equal(p.readWidth('264', p.RAIL_DEFAULT, clamp), 264)
  assert.equal(p.readCollapsed('1'), true)
  assert.equal(p.readCollapsed('0'), false)
  assert.equal(p.readCollapsed(null), false)
})

test('the separator steps 16 px and Home/End reach the ends', () => {
  assert.equal(p.keyWidth('ArrowRight', 300, 240, 460), 316)
  assert.equal(p.keyWidth('ArrowLeft', 300, 240, 460), 284)
  assert.equal(p.keyWidth('ArrowLeft', 250, 240, 460), 240)
  assert.equal(p.keyWidth('ArrowRight', 450, 240, 460), 460)
  assert.equal(p.keyWidth('Home', 300, 240, 460), 240)
  assert.equal(p.keyWidth('End', 300, 240, 460), 460)
  assert.equal(p.keyWidth('Enter', 300, 240, 460), null)
})
