import type { ModelChoice } from './PathwayPalette.svelte'

// Dragging on the pathway editor: what is picked up, where it can land, and a Svelte action for anything that can be picked
// up. The editor owns the drag's state and draws the ghost, the insertion bar and the hint; this file holds no state of its own.
export type DragItem = { type: 'stage'; kind: string } | { type: 'card'; from: number } | { type: 'model'; choice: ModelChoice } | { type: 'skill'; id: string } | { type: 'prompt'; text: string }
export type DropTarget = { type: 'slot'; at: number } | { type: 'card'; at: number } | null
export type DragState = { item: DragItem; x: number; y: number; target: DropTarget; why: string } | null
/** What the editor hands each draggable: told when a drag starts (with the node picked up), moves and ends. */
export type DragHooks = { onstart: (s: NonNullable<DragState>, node: HTMLElement) => void; onmove: (x: number, y: number) => void; onend: (drop: boolean) => void }

type Box = Pick<DOMRect, 'left' | 'right' | 'top' | 'bottom'>
const inside = (b: Box, x: number, y: number) => x >= b.left && x <= b.right && y >= b.top && y <= b.bottom

/** Which target the pointer is over, from the cards' rectangles and the line's direction. A stage block or a card is offered
 * the slot before the first card whose middle the pointer has not passed; a model, skill or prompt the card under the pointer. */
export function targetAt(item: DragItem, x: number, y: number, cards: Box[], line: Box, vertical: boolean): DropTarget {
  if (!inside(line, x, y)) return null
  if (item.type === 'stage' || item.type === 'card') {
    const along = vertical ? y : x
    return { type: 'slot', at: cards.filter(c => (vertical ? c.top + c.bottom : c.left + c.right) / 2 < along).length }
  }
  const at = cards.findIndex(c => inside(c, x, y))
  return at < 0 ? null : { type: 'card', at }
}

const HOLD_MS = 250, HOLD_SLOP = 8, MOVE_START = 6

/** Makes `node` something that can be picked up, by mouse, pen or touch. A mouse or pen picks it up after 6 px of movement;
 * touch after a 250 ms hold that moves less than 8 px, so a swipe still scrolls. Escape or a lost pointer cancels. A press
 * that never becomes a drag is an ordinary click; the click that ends a drag is swallowed. With no options it does nothing. */
export function draggable(node: HTMLElement, opts: (DragHooks & { item: () => DragItem | null }) | null) {
  if (!opts) return { destroy() {} }
  const o = opts
  // idle; pending (pressed, not yet picked up); live (dragging); done (cancelled, waiting for the pointer to come up).
  let phase: 'idle' | 'pending' | 'live' | 'done' = 'idle'
  let id = 0, touch = false, x0 = 0, y0 = 0, x = 0, y = 0, timer = 0, swallow = false, swallowing = 0, touchAction = ''
  node.style.setProperty('-webkit-touch-callout', 'none')

  function begin() {
    const item = o.item()
    if (!item) { reset(); return }
    phase = 'live'
    try { node.setPointerCapture(id) } catch { /* the pointer is already gone: pointerup or cancel follows */ }
    touchAction = node.style.touchAction
    node.style.touchAction = 'none'
    o.onstart({ item, x, y, target: null, why: '' }, node)
    node.dataset.dragging = ''
    addEventListener('keydown', key, true)
    addEventListener('blur', away)
    document.addEventListener('visibilitychange', away)
  }
  function finish(drop: boolean) {
    phase = 'done'
    node.style.touchAction = touchAction
    delete node.dataset.dragging
    removeEventListener('keydown', key, true)
    removeEventListener('blur', away)
    document.removeEventListener('visibilitychange', away)
    swallow = true
    o.onend(drop)
  }
  function reset() {
    clearTimeout(timer)
    phase = 'idle'
    removeEventListener('pointermove', move)
    removeEventListener('pointerup', up)
    removeEventListener('pointercancel', cancel)
  }

  function down(event: PointerEvent) {
    swallow = false; clearTimeout(swallowing)
    if (phase !== 'idle' || !event.isPrimary || (event.pointerType === 'mouse' && event.button !== 0)) return
    phase = 'pending'; id = event.pointerId; touch = event.pointerType === 'touch'
    x0 = x = event.clientX; y0 = y = event.clientY
    if (touch) timer = window.setTimeout(() => { if (phase === 'pending') begin() }, HOLD_MS)
    addEventListener('pointermove', move)
    addEventListener('pointerup', up)
    addEventListener('pointercancel', cancel)
  }
  function move(event: PointerEvent) {
    if (event.pointerId !== id) return
    x = event.clientX; y = event.clientY
    const far = Math.hypot(x - x0, y - y0)
    if (phase === 'pending') {
      if (touch) { if (far > HOLD_SLOP) reset(); return }
      if (far < MOVE_START) return
      begin()
    }
    if (phase === 'live') o.onmove(x, y)
  }
  function up(event: PointerEvent) {
    if (event.pointerId !== id) return
    if (phase === 'live') finish(true)
    reset()
    setTimeout(() => { swallow = false })
  }
  function cancel(event: PointerEvent) {
    if (event.pointerId !== id) return
    if (phase === 'live') finish(false)
    reset()
    swallow = false
  }
  function key(event: KeyboardEvent) {
    if (event.key !== 'Escape' || phase !== 'live') return
    event.preventDefault(); event.stopPropagation()
    finish(false)
  }
  // A pointer lost mid-drag (the node taken away, capture released) is a cancel.
  // The window losing focus (alt-tab with the button held) may send no pointerup or cancel: cancel the drag now. The button
  // may still be down, so the click its release makes is swallowed too, until the next press or half a second has passed.
  function stop() { finish(false); reset(); clearTimeout(swallowing); swallowing = window.setTimeout(() => { swallow = false }, 500) }
  function away(event: Event) { if (phase === 'live' && (event.type === 'blur' || document.visibilityState === 'hidden')) stop() }
  const lost = () => { if (phase === 'live') stop() }
  const click = (event: MouseEvent) => { if (swallow) { event.preventDefault(); event.stopImmediatePropagation() } }
  // Once a touch drag has started, the finger moving must not scroll the page or the line.
  const touchmove = (event: TouchEvent) => { if (phase === 'live' || phase === 'done') event.preventDefault() }
  const menu = (event: Event) => { if (phase !== 'idle') event.preventDefault() }

  node.addEventListener('pointerdown', down)
  node.addEventListener('lostpointercapture', lost)
  node.addEventListener('click', click, true)
  node.addEventListener('touchmove', touchmove, { passive: false })
  node.addEventListener('contextmenu', menu)
  return {
    destroy() {
      if (phase === 'live') finish(false)
      reset()
      clearTimeout(swallowing)
      node.removeEventListener('pointerdown', down)
      node.removeEventListener('lostpointercapture', lost)
      node.removeEventListener('click', click, true)
      node.removeEventListener('touchmove', touchmove)
      node.removeEventListener('contextmenu', menu)
    },
  }
}
