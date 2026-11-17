// Keep a floating entry point clear of growing composers and fixed action docks.
// The regions opt in; navigation and streamed content can replace them at any time.
// A region marked data-floating-clearance="avoid" (a row of buttons in a list, an action bar) is not a dock the entry
// point rises above wherever it is in view: the entry point stays where it rests unless it would cover such a region,
// and then moves up the screen to the lowest place that covers none of them.
export function keepLauncherClear(node: HTMLElement) {
  let frame = 0
  const regions = new Set<HTMLElement>()
  const size = new ResizeObserver(schedule)
  const changes = new MutationObserver(schedule)
  function measure() {
    frame = 0
    const current = new Set(document.querySelectorAll<HTMLElement>('[data-floating-clearance]'))
    for (const region of regions) if (!current.has(region)) { size.unobserve(region); regions.delete(region) }
    for (const region of current) if (!regions.has(region)) { regions.add(region); size.observe(region) }
    node.style.removeProperty('--launcher-clearance')
    const box = node.getBoundingClientRect()
    const viewport = window.visualViewport
    const viewTop = viewport && Math.abs(viewport.scale - 1) < .01 ? viewport.offsetTop : 0
    const viewBottom = viewport && Math.abs(viewport.scale - 1) < .01 ? Math.min(innerHeight, viewTop + viewport.height) : innerHeight
    let bottom = Math.max(innerHeight - box.bottom, innerHeight - viewBottom + 20)
    for (const region of regions) {
      if (region.dataset.floatingClearance === 'avoid') continue
      const r = region.getBoundingClientRect()
      if (r.width && r.height && r.bottom > viewTop && r.top < viewBottom && r.left < box.right && r.right > box.left) {
        bottom = Math.max(bottom, innerHeight - r.top + 12)
      }
    }
    // Regions to keep off: climb above the highest one the button would cover, until it covers none. If that would
    // leave the screen (a list with no gap tall enough), it stays where the other regions put it.
    const avoid = [...regions].filter(region => region.dataset.floatingClearance === 'avoid').map(region => region.getBoundingClientRect())
      .filter(r => r.width && r.height && r.left < box.right + 8 && r.right > box.left - 8)
    const height = box.height
    let place = bottom
    for (let step = 0; step <= avoid.length; step++) {
      const top = innerHeight - place - height, end = innerHeight - place
      const covered = avoid.filter(r => r.top < end + 8 && r.bottom > top - 8)
      if (!covered.length) { bottom = place; break }
      place = innerHeight - Math.min(...covered.map(r => r.top)) + 12
      if (innerHeight - place - height < viewTop + 8) break
    }
    node.style.setProperty('--launcher-clearance', `${bottom}px`)
  }
  function schedule() { if (!frame) frame = requestAnimationFrame(measure) }
  changes.observe(document.body, {childList:true, subtree:true, attributes:true, attributeFilter:['data-floating-clearance']})
  window.addEventListener('resize', schedule)
  document.addEventListener('scroll', schedule, true)
  document.addEventListener('transitionend', schedule, true)
  window.visualViewport?.addEventListener('resize', schedule)
  window.visualViewport?.addEventListener('scroll', schedule)
  schedule()
  return {destroy() {
    changes.disconnect(); size.disconnect(); cancelAnimationFrame(frame)
    window.removeEventListener('resize', schedule)
    document.removeEventListener('scroll', schedule, true)
    document.removeEventListener('transitionend', schedule, true)
    window.visualViewport?.removeEventListener('resize', schedule)
    window.visualViewport?.removeEventListener('scroll', schedule)
    node.style.removeProperty('--launcher-clearance')
  }}
}

// Keep the desktop companion beside the conversation when the workspace is resized.
// The phone dialog continues to use its full-screen sheet.
export function alignToConversation(node: HTMLElement, dock: HTMLElement | null) {
  let current = dock, frame = 0
  const size = new ResizeObserver(schedule)
  function measure() {
    frame = 0
    const pane = current?.closest('.chat-pane')
    if (!pane || !pane.getClientRects().length) { node.style.removeProperty('--gaffer-right'); return }
    const right = innerWidth - pane.getBoundingClientRect().right + 12
    node.style.setProperty('--gaffer-right', `${Math.max(16, Math.min(right, innerWidth - node.getBoundingClientRect().width - 16))}px`)
  }
  function schedule() { if (!frame) frame = requestAnimationFrame(measure) }
  function observe() { size.disconnect(); size.observe(node); const pane = current?.closest('.chat-pane'); if (pane) size.observe(pane); schedule() }
  observe()
  window.addEventListener('resize', schedule)
  document.addEventListener('scroll', schedule, true)
  return {
    update(next: HTMLElement | null) { current = next; observe() },
    destroy() { size.disconnect(); cancelAnimationFrame(frame); window.removeEventListener('resize', schedule); document.removeEventListener('scroll', schedule, true); node.style.removeProperty('--gaffer-right') },
  }
}
