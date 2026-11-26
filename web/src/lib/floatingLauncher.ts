// Keep a floating entry point clear of growing composers and fixed action docks.
// The regions opt in; navigation and streamed content can replace them at any time.
// With `avoid`, the entry point also keeps off a region marked data-floating-clearance="avoid" (a row of buttons in a
// list, an approval): it stays where it rests unless it would cover such a region, and then moves to the nearest place
// above that covers none. A region marked "controls" (an action bar whose buttons sit on one side) only keeps it off the
// buttons themselves, so it may rest over the bar's empty side rather than climb above the whole bar. Without `avoid`
// such regions are ignored, so another floating thing (the sync dock) is not moved by the rows of a list.
export function keepLauncherClear(node: HTMLElement, options: { avoid?: boolean } = {}) {
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
    if (options.avoid) {
      const marked = [...regions].filter(region => region.dataset.floatingClearance === 'avoid')
      const bars = [...regions].filter(region => region.dataset.floatingClearance === 'controls')
      const column = (r: DOMRect, gap: number) => r.width && r.height && r.left < box.right + gap && r.right > box.left - gap
      const rest = bottom
      const controlsIn = (region: HTMLElement) => [...region.querySelectorAll<HTMLElement>('button, a[href], input, select, textarea')]
        .map(control => control.getBoundingClientRect()).filter(r => column(r, 4))
      bottom = clearOf(marked.map(region => region.getBoundingClientRect()).filter(r => column(r, 8)), 8, rest, box.height, viewTop, true)
      // No gap between the rows fits the button: keep it off the buttons themselves and let it cover text instead.
      bottom = clearOf(marked.flatMap(controlsIn), 4, bottom, box.height, viewTop, false)
      // An action bar's empty side is free floor: only its buttons count, so it need not rise above the whole bar.
      const barFloor = clearOf(bars.flatMap(controlsIn), 4, rest, box.height, viewTop, false)
      bottom = Math.max(bottom, barFloor)
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
  return {update(next: { avoid?: boolean } = {}) { options = next; schedule() }, destroy() {
    changes.disconnect(); size.disconnect(); cancelAnimationFrame(frame)
    window.removeEventListener('resize', schedule)
    document.removeEventListener('scroll', schedule, true)
    document.removeEventListener('transitionend', schedule, true)
    window.visualViewport?.removeEventListener('resize', schedule)
    window.visualViewport?.removeEventListener('scroll', schedule)
    node.style.removeProperty('--launcher-clearance')
  }}
}

// The nearest place (a distance from the foot of the screen, never lower than `rest`) where a button of this height keeps
// `gap` px off every obstacle and stays on screen. The places tried are the resting one and the ones just above and just
// below each obstacle. When there is none: with `exact` the resting place is returned unchanged for the next pass to
// refine; otherwise the place that covers the least of them wins.
function clearOf(obstacles: DOMRect[], gap: number, rest: number, height: number, viewTop: number, exact: boolean) {
  if (!obstacles.length) return rest
  const places = [rest]
  for (const r of obstacles) places.push(innerHeight - r.top + gap, innerHeight - r.bottom - gap - height)
  const covered = (place: number) => obstacles.reduce((sum, r) => {
    const top = innerHeight - place - height, end = innerHeight - place
    const across = Math.min(end + gap, r.bottom) - Math.max(top - gap, r.top)
    return sum + (across > 0 ? across * Math.max(1, r.width) : 0)
  }, 0)
  const onScreen = places.filter(place => place >= rest && innerHeight - place - height >= viewTop + 8).sort((a, b) => a - b)
  const free = onScreen.find(place => !covered(place))
  if (free !== undefined) return free
  if (exact) return rest
  return onScreen.reduce((best, place) => covered(place) < covered(best) ? place : best, rest)
}
