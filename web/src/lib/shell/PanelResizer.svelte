<script lang="ts">
  import { KEY_STEP } from './workspaceLayout'

  // The drag handle on a left panel's right edge: pointer drags and the keyboard both set its width. The page owns the
  // value; this only reports the width it asks for, and calls oncommit when a drag or a key ends so the choice is stored.
  let { value, min, max, resetValue, label, mode = 'fixed', position = '', onchange, oncommit }: {
    value: number
    min: number
    max: number
    /** Double-click, and the width a person gets back, restored. */
    resetValue: number
    /** What a screen reader hears for the separator. */
    label: string
    /** 'fixed' for the window-edge sidebar, 'absolute' for the rail inside Home's column. */
    mode?: 'fixed' | 'absolute'
    /** The offsets that place it, such as `left: calc(... - 4px)` or `right: -4px`. */
    position?: string
    onchange: (width: number) => void
    oncommit: (width: number) => void
  } = $props()

  let dragging = $state(false)
  let startX = 0
  let startWidth = 0

  function down(event: PointerEvent) {
    if (event.button !== 0) return
    dragging = true
    startX = event.clientX
    startWidth = value
    ;(event.currentTarget as HTMLElement).setPointerCapture?.(event.pointerId)
    event.preventDefault()
  }

  function widthAt(clientX: number) {
    return Math.min(max, Math.max(min, Math.round(startWidth + clientX - startX)))
  }

  function move(event: PointerEvent) {
    if (!dragging) return
    onchange(widthAt(event.clientX))
  }

  function up(event: PointerEvent) {
    if (!dragging) return
    dragging = false
    ;(event.currentTarget as HTMLElement).releasePointerCapture?.(event.pointerId)
    oncommit(widthAt(event.clientX))
  }

  function key(event: KeyboardEvent) {
    const step = event.shiftKey ? KEY_STEP * 4 : KEY_STEP
    const next = event.key === 'ArrowRight' ? Math.min(max, value + step)
      : event.key === 'ArrowLeft' ? Math.max(min, value - step)
      : event.key === 'Home' ? min
      : event.key === 'End' ? max
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
<div class="panel-resizer" class:absolute={mode === 'absolute'} class:dragging
  style={position} role="separator" aria-orientation="vertical" aria-label={label}
  aria-valuenow={value} aria-valuemin={min} aria-valuemax={max} aria-valuetext={`${value} pixels`} tabindex="0"
  onpointerdown={down} onpointermove={move} onpointerup={up} onpointercancel={up} onlostpointercapture={up}
  ondblclick={reset} onkeydown={key}>
  <span class="grip" aria-hidden="true"></span>
</div>

<style>
  .panel-resizer { position: fixed; top: 0; bottom: 0; width: 9px; z-index: 25; cursor: col-resize; touch-action: none; background: transparent; }
  .panel-resizer.absolute { position: absolute; }
  .grip { position: absolute; top: 0; bottom: 0; left: 4px; width: 1px; background: transparent; transition: background .15s; }
  .panel-resizer:hover .grip, .panel-resizer:focus-visible .grip, .panel-resizer.dragging .grip { background: var(--accent); }
  .panel-resizer:focus-visible { outline: none; }
  .panel-resizer:focus-visible .grip { box-shadow: 0 0 0 1px var(--accent); width: 2px; }
</style>
