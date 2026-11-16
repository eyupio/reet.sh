// The desk split tree: the TypeScript twin of internal/desk/tree.go. Every
// function returns a new Tab and leaves its argument alone. A shared fixture
// (fixtures/presets.json) is checked by both this file's test and the Go test
// TestPresetsMatchWeb, so the two cannot drift.
import type { ProjectColour } from '../projectColour'

export type Split = 'right' | 'down'
export type Node = { pane: string } | { split: Split; ratio: number; a: Node; b: Node }
export type Pane = { id: string; name: string; colour: ProjectColour; host: string; session: string; kind: 'shell' | 'agent'; dir: string; dirAt: number; start: string }
export type Tab = { id: string; name: string; colour: ProjectColour; root: Node | null; zoomed: string; active: string; panes: Record<string, Pane> }
export type Doc = { tabs: Tab[]; active: string }
/** A rectangle in fractions of the tab, 0..1. */
export type Rect = { x: number; y: number; w: number; h: number }

export const MAX_TABS = 20
export const MAX_PANES = 9
export const MIN_RATIO = 0.1
export const MAX_RATIO = 0.9
const MIN_COLS = 20
const MIN_ROWS = 8

const isLeaf = (n: Node): n is { pane: string } => !('split' in n)

/** Limits a split ratio to 0.1..0.9; NaN becomes 0.5. */
export function clamp(r: number): number {
  if (Number.isNaN(r)) return 0.5
  return Math.min(MAX_RATIO, Math.max(MIN_RATIO, r))
}

/** The pane ids of a tree, left to right and top to bottom. */
export function leaves(n: Node | null): string[] {
  if (!n) return []
  return isLeaf(n) ? [n.pane] : [...leaves(n.a), ...leaves(n.b)]
}

function split(n: { split: Split; ratio: number }, r: Rect): [Rect, Rect] {
  if (n.split === 'right') {
    const aw = r.w * n.ratio
    return [{ x: r.x, y: r.y, w: aw, h: r.h }, { x: r.x + aw, y: r.y, w: r.w - aw, h: r.h }]
  }
  const ah = r.h * n.ratio
  return [{ x: r.x, y: r.y, w: r.w, h: ah }, { x: r.x, y: r.y + ah, w: r.w, h: r.h - ah }]
}

/** Where each pane sits, from the ratios only. */
export function layout(root: Node | null): Map<string, Rect> {
  const out = new Map<string, Rect>()
  const walk = (n: Node, r: Rect) => {
    if (isLeaf(n)) { out.set(n.pane, r); return }
    const [ra, rb] = split(n, r)
    walk(n.a, ra)
    walk(n.b, rb)
  }
  if (root) walk(root, { x: 0, y: 0, w: 1, h: 1 })
  return out
}

/** One draggable divider per split. `path` is the route from the root ("a"/"b" per step, "" for the root) and is what
 *  setRatio takes; `rect` is the whole area the split divides, so a drag can turn a pointer position into a ratio. */
export function separators(root: Node | null): { path: string; split: Split; rect: Rect; ratio: number }[] {
  const out: { path: string; split: Split; rect: Rect; ratio: number }[] = []
  const walk = (n: Node, r: Rect, path: string) => {
    if (isLeaf(n)) return
    out.push({ path, split: n.split, rect: r, ratio: n.ratio })
    const [ra, rb] = split(n, r)
    walk(n.a, ra, path + 'a')
    walk(n.b, rb, path + 'b')
  }
  if (root) walk(root, { x: 0, y: 0, w: 1, h: 1 }, '')
  return out
}

function cloneNode(n: Node | null): Node | null {
  if (!n) return null
  return isLeaf(n) ? { pane: n.pane } : { split: n.split, ratio: n.ratio, a: cloneNode(n.a)!, b: cloneNode(n.b)! }
}

function cloneTab(t: Tab): Tab {
  return { ...t, root: cloneNode(t.root), panes: { ...t.panes } }
}

function findLeaf(n: Node | null, pane: string): (Node & { pane: string }) | null {
  if (!n) return null
  if (isLeaf(n)) return n.pane === pane ? n : null
  return findLeaf(n.a, pane) ?? findLeaf(n.b, pane)
}

/** Replaces a leaf in place with another node (the object keeps its identity so the parent still points at it). */
function overwrite(target: Node, next: Node) {
  for (const k of Object.keys(target)) delete (target as Record<string, unknown>)[k]
  Object.assign(target, next)
}

/** Puts a new leaf next to `pane` as the b child at ratio 0.5 and lists `add` in the panes. Throws 'full' at 9 panes. */
export function splitPane(tab: Tab, pane: string, dir: Split, add: Pane): Tab {
  if (dir !== 'right' && dir !== 'down') throw new Error(`split ${dir}`)
  const ids = leaves(tab.root)
  if (ids.length >= MAX_PANES) throw new Error('full')
  if (!add.id) throw new Error('new pane needs an id')
  if (ids.includes(add.id)) throw new Error(`pane ${add.id} already in the tab`)
  const next = cloneTab(tab)
  const n = findLeaf(next.root, pane)
  if (!n) throw new Error(`pane ${pane} not in the tab`)
  overwrite(n, { split: dir, ratio: 0.5, a: { pane }, b: { pane: add.id } })
  next.panes[add.id] = add
  return next
}

/** The tree without the leaf, and the sibling that took its space (null when the leaf was the whole tree). */
function remove(n: Node, pane: string): { tree: Node | null; sibling: Node | null } {
  if (isLeaf(n)) return { tree: null, sibling: null }
  if (isLeaf(n.a) && n.a.pane === pane) return { tree: n.b, sibling: n.b }
  if (isLeaf(n.b) && n.b.pane === pane) return { tree: n.a, sibling: n.a }
  if (findLeaf(n.a, pane)) {
    const r = remove(n.a, pane)
    n.a = r.tree!
    return { tree: n, sibling: r.sibling }
  }
  const r = remove(n.b, pane)
  n.b = r.tree!
  return { tree: n, sibling: r.sibling }
}

/** Removes a pane; its sibling takes the space. Closing the last pane leaves root null. The pane leaves `panes`,
 *  `zoomed` clears if it was that pane and `active` moves to the sibling's first pane. */
export function closePane(tab: Tab, pane: string): Tab {
  const next = cloneTab(tab)
  if (!findLeaf(next.root, pane)) throw new Error(`pane ${pane} not in the tab`)
  const r = remove(next.root!, pane)
  next.root = r.tree
  delete next.panes[pane]
  if (next.zoomed === pane) next.zoomed = ''
  if (next.active === pane) next.active = r.sibling ? leaves(r.sibling)[0] : ''
  return next
}

/** Exchanges the places of two panes in the tree. */
export function swapPanes(tab: Tab, a: string, b: string): Tab {
  const next = cloneTab(tab)
  const na = findLeaf(next.root, a), nb = findLeaf(next.root, b)
  if (!na || !nb) throw new Error('pane not in the tab')
  na.pane = b
  nb.pane = a
  return next
}

/** Lifts `pane` out and drops it on the given edge of `onto`, splitting that pane in half. */
export function movePane(tab: Tab, pane: string, onto: string, edge: 'left' | 'right' | 'top' | 'bottom'): Tab {
  if (pane === onto) return tab
  const next = cloneTab(tab)
  if (!findLeaf(next.root, pane) || !findLeaf(next.root, onto)) throw new Error('pane not in the tab')
  next.root = remove(next.root!, pane).tree
  const target = findLeaf(next.root, onto)!
  const dir: Split = edge === 'left' || edge === 'right' ? 'right' : 'down'
  const first = edge === 'left' || edge === 'top'
  overwrite(target, { split: dir, ratio: 0.5, a: { pane: first ? pane : onto }, b: { pane: first ? onto : pane } })
  return next
}

/** Sets the ratio of the split at `path` (see separators), clamped to 0.1..0.9. */
export function setRatio(tab: Tab, path: string, ratio: number): Tab {
  const next = cloneTab(tab)
  let n = next.root
  for (const step of path) {
    if (!n || isLeaf(n)) throw new Error(`no split at ${path}`)
    n = step === 'a' ? n.a : n.b
  }
  if (!n || isLeaf(n)) throw new Error(`no split at ${path}`)
  n.ratio = clamp(ratio)
  return next
}

const leafOf = (id: string): Node => ({ pane: id })

function chainNodes(dir: Split, nodes: Node[]): Node {
  if (nodes.length === 1) return nodes[0]
  return { split: dir, ratio: 1 / nodes.length, a: nodes[0], b: chainNodes(dir, nodes.slice(1)) }
}

const chain = (dir: Split, ids: string[]) => chainNodes(dir, ids.map(leafOf))

function evenTree(n: Node) {
  if (isLeaf(n)) return
  const a = leaves(n.a).length, b = leaves(n.b).length
  n.ratio = clamp(a / (a + b))
  evenTree(n.a)
  evenTree(n.b)
}

/** Rebuilds the tree from the panes in their present order: side (columns), stack (rows), grid (ceil(sqrt n) columns,
 *  each a stack), main (first pane left at 0.6, the rest stacked) or even (keep the shape, equal shares). */
export function preset(tab: Tab, name: 'side' | 'stack' | 'grid' | 'main' | 'even'): Tab {
  if (!['side', 'stack', 'grid', 'main', 'even'].includes(name)) throw new Error(`preset ${name}`)
  const next = cloneTab(tab)
  const ids = leaves(next.root)
  if (ids.length === 0) return next
  switch (name) {
    case 'side': next.root = chain('right', ids); break
    case 'stack': next.root = chain('down', ids); break
    case 'main':
      next.root = ids.length === 1 ? leafOf(ids[0]) : { split: 'right', ratio: 0.6, a: leafOf(ids[0]), b: chain('down', ids.slice(1)) }
      break
    case 'grid': {
      const cols = Math.ceil(Math.sqrt(ids.length))
      const base = Math.floor(ids.length / cols), extra = ids.length % cols
      const nodes: Node[] = []
      for (let c = 0, at = 0; c < cols; c++) {
        const n = base + (c < extra ? 1 : 0)
        if (n > 0) nodes.push(chain('down', ids.slice(at, at + n)))
        at += n
      }
      next.root = chainNodes('right', nodes)
      break
    }
    case 'even': evenTree(next.root!); break
  }
  return next
}

/** The pane next to `pane` in a direction, by where the panes sit: the nearest one that overlaps along the other axis
 *  (the most overlap wins a tie, then reading order). Null when there is none. */
export function neighbour(root: Node, pane: string, dir: 'left' | 'right' | 'up' | 'down'): string | null {
  const rects = layout(root)
  const cur = rects.get(pane)
  if (!cur) return null
  const eps = 1e-9
  const overlap = (a0: number, a1: number, b0: number, b1: number) => Math.min(a1, b1) - Math.max(a0, b0)
  let best: { id: string; gap: number; over: number } | null = null
  for (const [id, r] of rects) {
    if (id === pane) continue
    let gap: number, over: number
    if (dir === 'right') { gap = r.x - (cur.x + cur.w); over = overlap(cur.y, cur.y + cur.h, r.y, r.y + r.h) }
    else if (dir === 'left') { gap = cur.x - (r.x + r.w); over = overlap(cur.y, cur.y + cur.h, r.y, r.y + r.h) }
    else if (dir === 'down') { gap = r.y - (cur.y + cur.h); over = overlap(cur.x, cur.x + cur.w, r.x, r.x + r.w) }
    else { gap = cur.y - (r.y + r.h); over = overlap(cur.x, cur.x + cur.w, r.x, r.x + r.w) }
    if (gap < -eps || over <= eps) continue
    if (!best || gap < best.gap - eps || (Math.abs(gap - best.gap) <= eps && over > best.over + eps)) best = { id, gap, over }
  }
  return best ? best.id : null
}

/** Whether a pane of this rect, in a tab of `px` pixels, holds at least 20 x 8 cells. */
export function fits(rect: Rect, px: { w: number; h: number }, cell: { w: number; h: number }): boolean {
  if (!(cell.w > 0) || !(cell.h > 0)) return false
  return Math.floor((rect.w * px.w) / cell.w + 1e-9) >= MIN_COLS && Math.floor((rect.h * px.h) / cell.h + 1e-9) >= MIN_ROWS
}

/** A fresh id for a tab or pane. */
export function newId(): string {
  const b = new Uint8Array(8)
  crypto.getRandomValues(b)
  return Array.from(b, x => x.toString(16).padStart(2, '0')).join('')
}
