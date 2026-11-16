<script lang="ts">
  import type { Snippet } from 'svelte'
  import { initialFontSize, PHONE_TOUCH } from '../terminal/sizing'
  import { clamp, fits, layout, leaves, separators, setRatio, type Rect, type Split, type Tab } from './tree'

  // A desk tab's panes, each placed absolutely where the split tree puts it, with a separator on every split. The panes
  // are drawn in a fixed order (by id) so a swap, a move or a preset only changes where a pane sits, never remounts its
  // terminal. A zoomed pane fills the area and the others stay mounted, hidden; a phone shows the focused pane alone the
  // same way. A separator drag is drawn locally and saved once, on release, while the panes hold their sizes.
  let { tab, focused, zoomed, single = false, name, pane, onratio, onmove }: {
    tab: Tab
    focused: string
    /** The pane that fills the area, or '' for the split. */
    zoomed: string
    /** One pane at a time (a phone): the focused one fills the area; separators are not offered. */
    single?: boolean
    /** A pane's name, for the separators' labels. */
    name: (id: string) => string
    /** Draws one pane. `visible` is false while it is hidden by zoom; `hold` is true while a separator is dragged. */
    pane: Snippet<[string, { visible: boolean; hold: boolean }]>
    onratio: (path: string, ratio: number) => void
    onmove: (pane: string, onto: string, edge: Edge) => void
  } = $props()

  type Edge = 'left' | 'right' | 'top' | 'bottom'
  const GAP = 3
  // What a pane spends outside its terminal's grid: its border, header and the screen's padding (DeskPane, TerminalPane).
  const CHROME = { w: 2 + 28, h: 2 + 34 + 20 }
  const STEP = 0.02

  let area = $state<HTMLElement>()
  let width = $state(0), height = $state(0)
  // A drag in progress: the split's path and the ratio drawn now.
  let drag = $state<{ path: string; ratio: number; pointer: number; cell: { w: number; h: number } } | null>(null)
  const shown = $derived(drag ? setRatio(tab, drag.path, drag.ratio) : tab)
  const ids = $derived(leaves(tab.root))
  const order = $derived([...ids].sort())
  const solo = $derived(single ? (ids.includes(focused) ? focused : ids[0] ?? '') : ids.includes(zoomed) ? zoomed : '')
  const rects = $derived(layout(shown.root))
  const seps = $derived(solo ? [] : separators(shown.root))
  const whole: Rect = { x: 0, y: 0, w: 1, h: 1 }
  /** "Resize alpha and beta": the first pane on each side of the split at `path`. */
  function sepLabel(path: string): string {
    let n = tab.root
    for (const step of path) n = n && 'split' in n ? (step === 'a' ? n.a : n.b) : null
    if (!n || !('split' in n)) return 'Resize the panes'
    return `Resize ${name(leaves(n.a)[0])} and ${name(leaves(n.b)[0])}`
  }

  /** One terminal cell in pixels, for the terminal's font size as this browser has it. */
  function cell(): { w: number; h: number } {
    let size = 14
    try { size = initialFontSize(localStorage.getItem('reet-terminal-font'), matchMedia(PHONE_TOUCH).matches) } catch { /* the default size */ }
    const ctx = document.createElement('canvas').getContext('2d')
    if (!ctx) return { w: size * 0.6, h: size * 1.2 }
    ctx.font = `${size}px "DejaVu Sans Mono", "SFMono-Regular", Consolas, monospace`
    const m = ctx.measureText('W')
    const h = (m.fontBoundingBoxAscent ?? size * 0.95) + (m.fontBoundingBoxDescent ?? size * 0.25)
    return { w: m.width, h: Math.ceil(h) }
  }
  /** Whether a pane at this place still holds 20 × 8 cells once its own chrome is taken off. */
  function roomy(r: Rect, c = cell()): boolean {
    if (!width || !height) return true
    const inner = { x: 0, y: 0, w: Math.max(0, r.w * width - 2 * GAP - CHROME.w) / width, h: Math.max(0, r.h * height - 2 * GAP - CHROME.h) / height }
    return fits(inner, { w: width, h: height }, c)
  }
  /** Whether splitting `id` in two this way leaves both halves room. Unknown sizes (nothing drawn yet) allow it. */
  export function roomFor(id: string, dir: Split): boolean {
    const r = layout(tab.root).get(id)
    if (!r || !width) return true
    const half = dir === 'right' ? { ...r, w: r.w / 2 } : { ...r, h: r.h / 2 }
    return roomy(half)
  }
  /** A new ratio is refused when it squeezes a pane that had room below 20 × 8 cells, or squeezes one that already had none. */
  function allowed(path: string, ratio: number, c: { w: number; h: number }): boolean {
    const before = layout(tab.root), after = layout(setRatio(tab, path, ratio).root)
    for (const [id, r] of after) {
      const old = before.get(id)
      if (!old || (old.w === r.w && old.h === r.h) || roomy(r, c)) continue
      if (roomy(old, c) || r.w * r.h < old.w * old.h) return false
    }
    return true
  }
  /** Moves toward `want` from `from` and stops at the last ratio that is allowed. */
  function reach(path: string, from: number, want: number, c = cell()): number {
    want = clamp(want)
    if (allowed(path, want, c)) return want
    const dir = want > from ? 1 : -1
    let at = from
    for (let r = from + dir * 0.005; dir > 0 ? r <= want : r >= want; r += dir * 0.005) { if (!allowed(path, r, c)) break; at = r }
    return at
  }

  // Separators: pointer capture while dragging, the arrows in 2 % steps, Home and End to the limits, a double-click to halve.
  const ratioAt = (s: { split: Split; rect: Rect }, event: PointerEvent) => {
    const box = area!.getBoundingClientRect()
    return s.split === 'right' ? ((event.clientX - box.left) / box.width - s.rect.x) / s.rect.w : ((event.clientY - box.top) / box.height - s.rect.y) / s.rect.h
  }
  function down(s: { path: string; split: Split; rect: Rect; ratio: number }, event: PointerEvent) {
    if (event.button !== 0) return
    event.preventDefault()
    ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
    ;(event.currentTarget as HTMLElement).focus()
    // The cell is measured once a drag, not on every move.
    drag = { path: s.path, ratio: s.ratio, pointer: event.pointerId, cell: cell() }
  }
  function move(s: { path: string; split: Split; rect: Rect; ratio: number }, event: PointerEvent) {
    if (!drag || drag.path !== s.path || drag.pointer !== event.pointerId) return
    drag = { ...drag, ratio: reach(s.path, drag.ratio, ratioAt(s, event), drag.cell) }
  }
  function up(s: { path: string }, event: PointerEvent) {
    if (!drag || drag.path !== s.path || drag.pointer !== event.pointerId) return
    const { ratio } = drag
    drag = null
    if (Math.abs(ratio - (separators(tab.root).find(x => x.path === s.path)?.ratio ?? ratio)) > 1e-6) onratio(s.path, ratio)
  }
  function key(s: { path: string; split: Split; ratio: number }, event: KeyboardEvent) {
    const back = s.split === 'right' ? 'ArrowLeft' : 'ArrowUp', on = s.split === 'right' ? 'ArrowRight' : 'ArrowDown'
    let want: number
    if (event.key === back) want = s.ratio - STEP
    else if (event.key === on) want = s.ratio + STEP
    else if (event.key === 'Home') want = 0.1
    else if (event.key === 'End') want = 0.9
    else return
    event.preventDefault()
    // Rounded to the step, so ten presses from a half land on a whole number.
    const next = reach(s.path, s.ratio, Math.round(want * 1000) / 1000)
    if (Math.abs(next - s.ratio) > 1e-6) onratio(s.path, next)
  }
  function even(s: { path: string; ratio: number }) {
    const next = reach(s.path, s.ratio, 0.5)
    if (Math.abs(next - s.ratio) > 1e-6) onratio(s.path, next)
  }

  // Dragging a pane's header ([data-drag-pane]) onto another pane's edge moves it there; a preview shows the half it takes.
  let dragging = $state(''), drop = $state<{ onto: string; edge: Edge } | null>(null)
  function edgeAt(el: HTMLElement, event: DragEvent): Edge {
    const box = el.getBoundingClientRect()
    const fx = (event.clientX - box.left) / box.width, fy = (event.clientY - box.top) / box.height
    const near = [['left', fx], ['right', 1 - fx], ['top', fy], ['bottom', 1 - fy]] as [Edge, number][]
    return near.sort((a, b) => a[1] - b[1])[0][0]
  }
  function dragStart(event: DragEvent) {
    const from = (event.target as HTMLElement | null)?.closest<HTMLElement>('[data-drag-pane]')?.dataset.dragPane
    if (!from || solo || ids.length < 2) { if (from) event.preventDefault(); return }
    dragging = from
    event.dataTransfer?.setData('application/x-reet-desk-pane', from)
    if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move'
  }
  function dragOver(id: string, event: DragEvent) {
    if (!dragging || id === dragging) { if (dragging) drop = null; return }
    event.preventDefault()
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'move'
    const edge = edgeAt(event.currentTarget as HTMLElement, event)
    if (drop?.onto !== id || drop.edge !== edge) drop = { onto: id, edge }
  }
  function dropOn(id: string, event: DragEvent) {
    if (!dragging || id === dragging) return
    event.preventDefault()
    const edge = edgeAt(event.currentTarget as HTMLElement, event)
    const from = dragging
    dragging = ''; drop = null
    onmove(from, id, edge)
  }
  function dragEnd() { dragging = ''; drop = null }

  const place = (r: Rect) => `left:calc(${r.x * 100}% + ${GAP}px);top:calc(${r.y * 100}% + ${GAP}px);width:calc(${r.w * 100}% - ${2 * GAP}px);height:calc(${r.h * 100}% - ${2 * GAP}px)`
  function sepPlace(s: { split: Split; rect: Rect; ratio: number }) {
    const r = s.rect
    return s.split === 'right'
      ? `left:calc(${(r.x + r.w * s.ratio) * 100}% - 12px);top:calc(${r.y * 100}% + ${GAP}px);width:24px;height:calc(${r.h * 100}% - ${2 * GAP}px)`
      : `top:calc(${(r.y + r.h * s.ratio) * 100}% - 12px);left:calc(${r.x * 100}% + ${GAP}px);height:24px;width:calc(${r.w * 100}% - ${2 * GAP}px)`
  }
  const half: Record<Edge, string> = { left: 'left:0;top:0;bottom:0;width:50%', right: 'right:0;top:0;bottom:0;width:50%', top: 'left:0;right:0;top:0;height:50%', bottom: 'left:0;right:0;bottom:0;height:50%' }
</script>

<div class="split-area" class:dragging={!!drag} class:single bind:this={area} bind:clientWidth={width} bind:clientHeight={height} ondragstart={dragStart} ondragend={dragEnd} role="presentation">
  {#each order as id (id)}
    {@const hidden = !!solo && id !== solo}
    {@const r = solo ? whole : rects.get(id) ?? whole}
    <div class="slot" class:hidden data-slot={id} style={single ? 'inset:0' : place(r)} ondragover={event => dragOver(id, event)} ondragleave={event => { if (drop?.onto === id && !(event.currentTarget as HTMLElement).contains(event.relatedTarget as Node)) drop = null }} ondrop={event => dropOn(id, event)} role="presentation">
      {@render pane(id, { visible: !hidden, hold: !!drag })}
      {#if drop?.onto === id}<div class="preview" style={half[drop.edge]} aria-hidden="true"></div>{/if}
    </div>
  {/each}
  {#each seps as s (s.path)}
    <!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions (a focusable separator with a value is a widget: it is moved by pointer and by keys) -->
    <div class="sep" class:active={drag?.path === s.path} data-split={s.split} style={sepPlace(s)} role="separator" tabindex="0"
      aria-orientation={s.split === 'right' ? 'vertical' : 'horizontal'} aria-valuenow={Math.round(s.ratio * 100)} aria-valuemin={10} aria-valuemax={90}
      aria-label={sepLabel(s.path)}
      onpointerdown={event => down(s, event)} onpointermove={event => move(s, event)} onpointerup={event => up(s, event)} onpointercancel={event => up(s, event)}
      onkeydown={event => key(s, event)} ondblclick={() => even(s)}></div>
  {/each}
</div>

<style>
  .split-area { position: relative; flex: 1; min-width: 0; min-height: 0; margin: 3px; }
  .slot { position: absolute; display: flex; min-width: 0; min-height: 0; }
  .slot.hidden { display: none; }
  .single { margin: 0; }
  .split-area.dragging { cursor: grabbing; }
  .split-area.dragging .slot { pointer-events: none; }
  .preview { position: absolute; z-index: 6; border: 2px solid var(--accent-text); border-radius: 6px; background: color-mix(in srgb, var(--accent-text) 18%, transparent); pointer-events: none; }
  .sep { position: absolute; z-index: 4; touch-action: none; display: flex; align-items: center; justify-content: center; }
  /* A touch screen gets a 44 px hit area; the line drawn stays 2 px. */
  @media (pointer: coarse) {
    .sep[data-split='right'] { margin-left: -10px; width: 44px !important; }
    .sep[data-split='down'] { margin-top: -10px; height: 44px !important; }
  }
  .sep[data-split='right'] { cursor: col-resize; }
  .sep[data-split='down'] { cursor: row-resize; }
  .sep::after { content: ''; border-radius: 2px; background: transparent; transition: background-color 120ms; }
  .sep[data-split='right']::after { width: 2px; height: 100%; }
  .sep[data-split='down']::after { height: 2px; width: 100%; }
  .sep:hover::after, .sep.active::after { background: var(--accent-text); }
  .sep:focus-visible { outline: none; }
  .sep:focus-visible::after { background: var(--accent-text); box-shadow: 0 0 0 2px color-mix(in srgb, var(--accent-text) 35%, transparent); }
  @media (prefers-reduced-motion: reduce) { .sep::after { transition: none; } }
</style>
