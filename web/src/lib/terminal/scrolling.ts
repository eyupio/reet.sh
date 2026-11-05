/** Translate vertical touch drags into the same wheel events used by a mouse.
 * Tmux routes them to the app's history or its own copy mode. No terminal bytes
 * are invented here: xterm handles coordinates and the negotiated protocol.
 */
export function terminalScrolling(element: HTMLElement, options: {
  enabled: () => boolean
  mouseTracking: () => boolean
  earlier: () => void
}): () => void {
  let gesture: { id: number; x: number; y: number; last: number; vertical: boolean; target: Element } | undefined
  function wheel(event: WheelEvent) {
    // Ctrl+wheel is also used for trackpad pinch zoom. Leave it to the browser.
    if (event.ctrlKey || event.metaKey) { event.stopImmediatePropagation(); return }
    if (!options.enabled() || !options.mouseTracking()) {
      event.preventDefault(); event.stopImmediatePropagation()
      if (options.enabled() && event.deltaY < 0) options.earlier()
    }
  }
  function start(event: TouchEvent) {
    gesture = undefined
    if (!options.enabled() || event.touches.length !== 1) return
    const touch = event.touches[0], target = event.target
    if (!(target instanceof Element) || !target.closest('.xterm-screen')) return
    gesture = { id: touch.identifier, x: touch.clientX, y: touch.clientY, last: touch.clientY, vertical: false, target }
  }
  function move(event: TouchEvent) {
    if (!gesture || !options.enabled() || event.touches.length !== 1) { gesture = undefined; return }
    const touch = Array.from(event.touches).find(item => item.identifier === gesture!.id)
    if (!touch) { gesture = undefined; return }
    const dx = Math.abs(touch.clientX - gesture.x), dy = Math.abs(touch.clientY - gesture.y)
    if (!gesture.vertical) {
      if (Math.max(dx, dy) < 12) return
      if (dx >= dy) { gesture = undefined; return }
      gesture.vertical = true
    }
    event.preventDefault(); event.stopImmediatePropagation()
    const delta = gesture.last - touch.clientY
    const lines = Math.min(8, Math.floor(Math.abs(delta) / 24))
    if (!lines) return
    gesture.last = touch.clientY
    // Dispatch at the original touch position, inside the terminal grid.
    for (let i = 0; i < lines; i++) {
      gesture.target.dispatchEvent(new WheelEvent('wheel', {
        bubbles: true, cancelable: true, clientX: gesture.x, clientY: gesture.y,
        deltaMode: WheelEvent.DOM_DELTA_LINE, deltaY: Math.sign(delta),
      }))
      if (!options.mouseTracking()) break
    }
  }
  const end = () => { gesture = undefined }
  element.addEventListener('wheel', wheel, { capture: true, passive: false })
  element.addEventListener('touchstart', start, { capture: true, passive: true })
  element.addEventListener('touchmove', move, { capture: true, passive: false })
  element.addEventListener('touchend', end, true)
  element.addEventListener('touchcancel', end, true)
  return () => {
    end()
    element.removeEventListener('wheel', wheel, true)
    element.removeEventListener('touchstart', start, true)
    element.removeEventListener('touchmove', move, true)
    element.removeEventListener('touchend', end, true)
    element.removeEventListener('touchcancel', end, true)
  }
}
