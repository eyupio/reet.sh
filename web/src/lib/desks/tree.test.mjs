import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const source = ts.transpileModule(readFileSync(new URL('./tree.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const t = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)
const fixture = JSON.parse(readFileSync(new URL('./fixtures/presets.json', import.meta.url), 'utf8'))

const pane = id => ({ id, name: '', colour: '', host: 'm1', session: '', kind: 'shell', dir: '', dirAt: 0, start: '' })
const leaf = id => ({ pane: id })
/** The same left-leaning chain as tabOf in the Go tests. */
const tabOf = (...ids) => {
  const tab = { id: 't', name: '', colour: '', root: null, zoomed: '', active: '', panes: {} }
  ids.forEach((id, i) => {
    tab.panes[id] = pane(id)
    tab.root = i === 0 ? leaf(id) : { split: 'right', ratio: 0.5, a: tab.root, b: leaf(id) }
  })
  return tab
}

test('split puts the new leaf as b at 0.5 and leaves the input alone', () => {
  const tab = tabOf('a')
  const before = JSON.stringify(tab)
  const next = t.splitPane(tab, 'a', 'down', pane('b'))
  assert.deepEqual(next.root, { split: 'down', ratio: 0.5, a: leaf('a'), b: leaf('b') })
  assert.ok(next.panes.b)
  assert.equal(JSON.stringify(tab), before)
})

test('split refuses at nine panes, an unknown pane and a repeated id', () => {
  const full = tabOf('p1', 'p2', 'p3', 'p4', 'p5', 'p6', 'p7', 'p8', 'p9')
  assert.throws(() => t.splitPane(full, 'p1', 'right', pane('x')), /full/)
  const tab = tabOf('a', 'b')
  assert.throws(() => t.splitPane(tab, 'zz', 'right', pane('x')))
  assert.throws(() => t.splitPane(tab, 'a', 'right', pane('b')))
})

test('close gives the space to the sibling and moves active and zoomed', () => {
  const tab = { ...tabOf('a', 'b', 'c'), active: 'c', zoomed: 'c' }
  const next = t.closePane(tab, 'c')
  assert.deepEqual(next.root, { split: 'right', ratio: 0.5, a: leaf('a'), b: leaf('b') })
  assert.equal(next.zoomed, '')
  assert.equal(next.active, 'a')
  assert.equal(next.panes.c, undefined)
  const keep = t.closePane({ ...tabOf('a', 'b'), active: 'a', zoomed: 'a' }, 'b')
  assert.equal(keep.active, 'a')
  assert.equal(keep.zoomed, 'a')
})

test('close the last pane leaves no root', () => {
  const next = t.closePane({ ...tabOf('a'), active: 'a' }, 'a')
  assert.equal(next.root, null)
  assert.equal(next.active, '')
  assert.deepEqual(next.panes, {})
})

test('swap exchanges places', () => {
  const next = t.swapPanes(tabOf('a', 'b'), 'a', 'b')
  assert.deepEqual(t.leaves(next.root), ['b', 'a'])
})

test('move drops a pane on an edge of another', () => {
  const left = t.movePane(tabOf('a', 'b', 'c'), 'c', 'a', 'left')
  assert.deepEqual(left.root, { split: 'right', ratio: 0.5, a: { split: 'right', ratio: 0.5, a: leaf('c'), b: leaf('a') }, b: leaf('b') })
  const bottom = t.movePane(tabOf('a', 'b'), 'a', 'b', 'bottom')
  assert.deepEqual(bottom.root, { split: 'down', ratio: 0.5, a: leaf('b'), b: leaf('a') })
  const top = t.movePane(tabOf('a', 'b'), 'a', 'b', 'top')
  assert.deepEqual(top.root, { split: 'down', ratio: 0.5, a: leaf('a'), b: leaf('b') })
  const same = tabOf('a', 'b')
  assert.equal(t.movePane(same, 'a', 'a', 'left'), same)
})

test('layout of a 2 + 1', () => {
  const root = { split: 'right', ratio: 0.5, a: leaf('l'), b: { split: 'down', ratio: 0.5, a: leaf('tr'), b: leaf('br') } }
  const rects = t.layout(root)
  assert.deepEqual(rects.get('l'), { x: 0, y: 0, w: 0.5, h: 1 })
  assert.deepEqual(rects.get('tr'), { x: 0.5, y: 0, w: 0.5, h: 0.5 })
  assert.deepEqual(rects.get('br'), { x: 0.5, y: 0.5, w: 0.5, h: 0.5 })
  assert.equal(t.layout(null).size, 0)
  const seps = t.separators(root)
  assert.deepEqual(seps.map(s => [s.path, s.split]), [['', 'right'], ['b', 'down']])
  assert.deepEqual(seps[1].rect, { x: 0.5, y: 0, w: 0.5, h: 1 })
})

test('neighbour right of the left pane', () => {
  const root = { split: 'right', ratio: 0.5, a: leaf('l'), b: { split: 'down', ratio: 0.5, a: leaf('tr'), b: leaf('br') } }
  assert.equal(t.neighbour(root, 'l', 'right'), 'tr')
  assert.equal(t.neighbour(root, 'br', 'left'), 'l')
  assert.equal(t.neighbour(root, 'tr', 'down'), 'br')
  assert.equal(t.neighbour(root, 'br', 'up'), 'tr')
  assert.equal(t.neighbour(root, 'l', 'left'), null)
  assert.equal(t.neighbour(root, 'l', 'up'), null)
})

test('setRatio clamps 0.95 to 0.9', () => {
  const tab = tabOf('a', 'b')
  assert.equal(t.setRatio(tab, '', 0.95).root.ratio, 0.9)
  assert.equal(t.setRatio(tab, '', 0.01).root.ratio, 0.1)
  assert.equal(t.setRatio(tab, '', NaN).root.ratio, 0.5)
  assert.equal(tab.root.ratio, 0.5)
  assert.throws(() => t.setRatio(tab, 'a', 0.3))
})

test('fits refuses a 19-column pane', () => {
  const cell = { w: 10, h: 20 }
  assert.equal(t.fits({ x: 0, y: 0, w: 1, h: 1 }, { w: 190, h: 400 }, cell), false)
  assert.equal(t.fits({ x: 0, y: 0, w: 1, h: 1 }, { w: 200, h: 160 }, cell), true)
  assert.equal(t.fits({ x: 0, y: 0, w: 1, h: 1 }, { w: 200, h: 140 }, cell), false)
  assert.equal(t.fits({ x: 0, y: 0, w: 0.5, h: 1 }, { w: 400, h: 400 }, cell), true)
})

test('presets keep every pane and never mutate', () => {
  for (const name of ['side', 'stack', 'grid', 'main', 'even']) {
    const tab = tabOf('a', 'b', 'c', 'd', 'e')
    const before = JSON.stringify(tab)
    const next = t.preset(tab, name)
    assert.deepEqual([...t.leaves(next.root)].sort(), ['a', 'b', 'c', 'd', 'e'], name)
    assert.equal(JSON.stringify(tab), before)
  }
  assert.throws(() => t.preset(tabOf('a'), 'nope'))
  assert.equal(t.preset({ ...tabOf('a'), root: null, panes: {} }, 'grid').root, null)
})

test('presets match the shared fixture the Go tree also reads', () => {
  assert.deepEqual(Object.keys(fixture).sort(), ['even', 'grid', 'main', 'side', 'stack'])
  for (const [name, byCount] of Object.entries(fixture)) {
    for (let n = 1; n <= 9; n++) {
      const ids = Array.from({ length: n }, (_, i) => `p${i + 1}`)
      assert.deepEqual(t.preset(tabOf(...ids), name).root, byCount[String(n)], `${name} with ${n}`)
    }
  }
})

test('newId is distinct', () => {
  assert.notEqual(t.newId(), t.newId())
})
