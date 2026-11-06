// Keep a floating entry point clear of growing composers and fixed action docks.
// The regions opt in; navigation and streamed content can replace them at any time.
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
      const r = region.getBoundingClientRect()
      if (r.width && r.height && r.bottom > viewTop && r.top < viewBottom && r.left < box.right && r.right > box.left) {
        bottom = Math.max(bottom, innerHeight - r.top + 12)
      }
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
