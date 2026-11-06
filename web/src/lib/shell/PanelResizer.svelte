<script lang="ts">
  import { KEY_STEP } from './workspaceLayout'

  // The one resize handle used by every side panel in the web UI: a full-height rule at the panel's edge that reports the
  // width it asks for. The page owns the value; this only reports it, and calls oncommit when a drag or a key ends so the
  // choice is stored. `side` says which side the panel is on, so a right-hand panel widens as the pointer moves left.
  let { value, min, max, resetValue, label, mode = 'fixed', side = 'left', position = '', minText, controls, title = 'Drag to resize · double-click resets', extraClass = '', snap, onchange, oncommit, onresizing }: {
    value: number
    min: number
    max: number
    /** Double-click, and the width a person gets back, restored. */
    resetValue: number
    /** What a screen reader hears for the separator. */
    label: string
    /** 'fixed' for the window-edge sidebar, 'absolute' for a panel inside a positioned column, 'inline' for a grid track. */
    mode?: 'fixed' | 'absolute' | 'inline'
    /** 'left' for a panel whose edge is its right side, 'right' for a panel on the screen's right. */
    side?: 'left' | 'right'
    /** The offsets that place it, such as `left: calc(... - 4px)` or a grid placement. */
    position?: string
    /** The words for the smallest width, such as "Mini rail", when it is more than a number of pixels. */
    minText?: string
    /** The id of the panel this separator controls. */
    controls?: string
    title?: string
    /** Extra classes, for a page that still names its own separator. */
    extraClass?: string
    /** A width below `below` becomes `value` (a mini rail); a widening key from there restores `restore`. */
    snap?: { value: number; below: number; restore: number }
    onchange: (width: number) => void
    oncommit: (width: number) => void
    /** A drag has begun (true) or ended (false), so the page can hold a live size until it settles. */
    onresizing?: (active: boolean) => void
  } = $props()

  let dragging = $state(false)
  let startX = 0
  let startWidth = 0

  const clamp = (width: number) => Math.min(max, Math.max(min, Math.round(width)))
  // Below the snap point the panel is its mini form, whatever width the pointer asks for.
  const settle = (width: number) => (snap && width < snap.below ? snap.value : clamp(width))
  // Moving the pointer towards the panel widens it: to the right for a left panel, to the left for a right panel.
  const towards = () => (side === 'right' ? -1 : 1)

  function down(event: PointerEvent) {
    if (event.pointerType === 'mouse' && event.button !== 0) return
    dragging = true
    startX = event.clientX
    startWidth = value
    ;(event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId)
    ;(event.currentTarget as HTMLElement).focus({ preventScroll: true })
    onresizing?.(true)
    event.preventDefault()
  }

  function widthAt(clientX: number) {
    return settle(startWidth + (clientX - startX) * towards())
  }

  function move(event: PointerEvent) {
    if (!dragging) return
    onchange(widthAt(event.clientX))
  }

  function up(event: PointerEvent) {
    if (!dragging) return
    dragging = false
    ;(event.currentTarget as HTMLElement).releasePointerCapture?.(event.pointerId)
    onresizing?.(false)
    oncommit(widthAt(event.clientX))
  }

  // A key that widens (1) or narrows (-1), keeping a snap rail as the floor and the way back out of it.
  function stepped(direction: 1 | -1, step: number) {
    if (!snap) return clamp(value + direction * step)
    if (direction > 0) return value <= min ? clamp(snap.restore) : clamp(Math.max(snap.restore, value + step))
    return value <= snap.restore ? snap.value : clamp(Math.max(snap.restore, value - step))
  }

  function key(event: KeyboardEvent) {
    const step = event.shiftKey ? KEY_STEP * 4 : KEY_STEP
    const widen = event.key === (side === 'right' ? 'ArrowLeft' : 'ArrowRight')
    const narrow = event.key === (side === 'right' ? 'ArrowRight' : 'ArrowLeft')
    const next = event.key === 'Home' ? min
      : event.key === 'End' ? max
      : widen ? stepped(1, step)
      : narrow ? stepped(-1, step)
      : null
    if (next === null) return
    event.preventDefault()
    onchange(next)
    oncommit(next)
  }

  function reset() {
    onchange(resetValue)
    oncommit(resetValue)
  }
</script>

<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
<div class="panel-resizer {extraClass}" class:absolute={mode === 'absolute'} class:inline={mode === 'inline'}
  class:right={side === 'right'} class:at-min={!!snap && value <= min} class:dragging
  style={position} role="separator" aria-orientation="vertical" aria-label={label} aria-controls={controls}
  aria-valuenow={value} aria-valuemin={min} aria-valuemax={max}
  aria-valuetext={minText && value <= min ? minText : `${value} pixels`} tabindex="0" title={title}
  onpointerdown={down} onpointermove={move} onpointerup={up} onpointercancel={up} onlostpointercapture={up}
  ondblclick={reset} onkeydown={key}>
  <span class="grip" aria-hidden="true"></span>
</div>

<style>
  /* The handle reaches over the panel's scrollbar and the neighbour's edge, so the rule stays grabbable where they meet. */
  .panel-resizer { position: fixed; top: 0; bottom: 0; width: 9px; z-index: 25; cursor: col-resize; touch-action: none; background: transparent; }
  .panel-resizer.absolute { position: absolute; }
  /* In a grid track the margins widen the hit area over both sides while the grid keeps its 8 px column. */
  .panel-resizer.inline { position: relative; top: auto; bottom: auto; width: auto; margin: 0 -8px; z-index: 3; }
  .grip { position: absolute; top: 0; bottom: 0; left: 4px; width: 1px; background: transparent; transition: background .15s; pointer-events: none; }
  .panel-resizer.inline .grip { left: 50%; transform: translateX(-50%); }
  .panel-resizer:hover .grip, .panel-resizer:focus-visible .grip, .panel-resizer.dragging .grip { background: var(--accent); }
  .panel-resizer:focus-visible { outline: 2px solid var(--accent); outline-offset: -1px; }
  .panel-resizer:focus-visible .grip { background: var(--accent); width: 2px; }
  /* A touch tablet gets a 44 × 88 px grip on the panel side; the mini rail keeps only its 24 px track so it covers no button. */
  @media (pointer: coarse) {
    .panel-resizer.inline:not(.at-min)::before { content: ''; position: absolute; top: 50%; right: calc(50% - 4px); width: 44px; height: 88px; transform: translateY(-50%); }
    .panel-resizer.inline.right:not(.at-min)::before { right: auto; left: calc(50% - 4px); }
  }
</style>
