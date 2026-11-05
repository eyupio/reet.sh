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
    let bottom = window.innerHeight - box.bottom
    for (const region of regions) {
      const r = region.getBoundingClientRect()
      if (r.width && r.height && r.bottom > 0 && r.top < innerHeight && r.left < box.right && r.right > box.left) {
        bottom = Math.max(bottom, innerHeight - r.top + 12)
      }
    }
    node.style.setProperty('--launcher-clearance', `${bottom}px`)
  }
  function schedule() { if (!frame) frame = requestAnimationFrame(measure) }
  changes.observe(document.body, {childList:true, subtree:true})
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
