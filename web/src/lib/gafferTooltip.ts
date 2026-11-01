/** Shared explanation for the native browser interface; stored `reet`/`chat` values stay unchanged. */
export const GAFFER_DESCRIPTION = "Reet's native AI agent control interface. Chat with your agent, follow its work, manage tool calls and approvals, and control sessions without using the agent's original terminal UI."

let nextId = 0
let closeActive: (() => void) | undefined

/** HTML popover in the top layer, including inside modal dialogs. Never intercepts selection/clicks. */
export function gafferTooltip(node: HTMLElement, enabled = true) {
  const tip = document.createElement('div')
  tip.id = `gaffer-help-${++nextId}`
  tip.setAttribute('role', 'tooltip')
  tip.setAttribute('popover', 'manual')
  // It is an explanation, not a control: pointer-events none keeps it from covering the button or menu it explains,
  // which on a narrow layout sits exactly where the tip is placed.
  tip.style.cssText = 'position:fixed;inset:auto;margin:0;box-sizing:border-box;width:320px;max-width:calc(100vw - 16px);padding:12px 16px;border:1px solid var(--line-strong);border-radius:8px;background:var(--card);color:var(--ink);box-shadow:var(--shadow-dock);font:14px/1.5 system-ui;z-index:10000;pointer-events:none;'
  const title = document.createElement('strong')
  title.textContent = 'Gaffer'
  const description = document.createElement('div')
  description.textContent = GAFFER_DESCRIPTION
  tip.append(title, description)
  // Keep the accessible description present even while the visual popover is closed.
  const accessible = document.createElement('span')
  accessible.id = `${tip.id}-description`
  accessible.textContent = GAFFER_DESCRIPTION
  accessible.style.cssText = 'position:absolute;width:1px;height:1px;overflow:hidden;clip-path:inset(50%);white-space:nowrap;'
  const parent = node.closest('dialog') ?? document.body
  parent.append(tip, accessible)
  let visible = false, hovering = false, focused = false, dismissed = false
  let timer: ReturnType<typeof setTimeout> | undefined
  const original = node.getAttribute('aria-describedby')
  function describe() {
    const ids = (node.getAttribute('aria-describedby') ?? '').split(/\s+/).filter(id => id && id !== accessible.id)
    if (enabled) ids.push(accessible.id)
    const value = ids.join(' ')
    if (node.getAttribute('aria-describedby') !== (value || null)) {
      if (value) node.setAttribute('aria-describedby', value)
      else node.removeAttribute('aria-describedby')
    }
  }
  function position() {
    if (!visible) return
    const r = node.getBoundingClientRect(), t = tip.getBoundingClientRect()
    tip.style.left = `${Math.max(8, Math.min(r.left, innerWidth - t.width - 8))}px`
    const below = r.bottom + 8
    tip.style.top = `${Math.max(8, below + t.height <= innerHeight - 8 ? below : r.top - t.height - 8)}px`
  }
  function hide() {
    clearTimeout(timer)
    if (visible) { tip.hidePopover(); visible = false }
    if (closeActive === hide) closeActive = undefined
  }
  function show() {
    clearTimeout(timer)
    if (!enabled || dismissed || !node.isConnected || visible) return
    closeActive?.()
    closeActive = hide
    tip.showPopover()
    visible = true
    position()
  }
  function leave() {
    hovering = false
    if (!focused) timer = setTimeout(hide, 100)
  }
  function enter() { hovering = true; dismissed = false; show() }
  function focus() { focused = true; dismissed = false; show() }
  function blur() { focused = false; if (!hovering) hide() }
  function escape(event: KeyboardEvent) {
    if (event.key === 'Escape' && visible) {
      event.preventDefault(); event.stopPropagation(); dismissed = true; hide()
    }
  }
  function outside(event: PointerEvent) {
    if (!node.contains(event.target as Node) && !tip.contains(event.target as Node)) hide()
  }
  // Touch opens on pointerdown as well as focus, without cancelling the normal click.
  function touch(event: PointerEvent) { if (event.pointerType === 'touch') { dismissed = false; show() } }
  node.addEventListener('pointerenter', enter)
  node.addEventListener('pointerleave', leave)
  node.addEventListener('focus', focus)
  node.addEventListener('blur', blur)
  node.addEventListener('pointerdown', touch)
  tip.addEventListener('pointerenter', enter)
  tip.addEventListener('pointerleave', leave)
  document.addEventListener('keydown', escape, true)
  document.addEventListener('pointerdown', outside)
  window.addEventListener('resize', position)
  window.addEventListener('scroll', position, true)
  describe()
  // Svelte may update an existing disabled-reason description as session state changes.
  const observer = new MutationObserver(describe)
  observer.observe(node, { attributes: true, attributeFilter: ['aria-describedby'] })
  return {
    update(value: boolean) { enabled = value; describe(); if (!enabled) hide() },
    destroy() {
      observer.disconnect()
      hide()
      node.removeEventListener('pointerenter', enter)
      node.removeEventListener('pointerleave', leave)
      node.removeEventListener('focus', focus)
      node.removeEventListener('blur', blur)
      node.removeEventListener('pointerdown', touch)
      document.removeEventListener('keydown', escape, true)
      document.removeEventListener('pointerdown', outside)
      window.removeEventListener('resize', position)
      window.removeEventListener('scroll', position, true)
      const ids = (node.getAttribute('aria-describedby') ?? '').split(/\s+/).filter(id => id && id !== accessible.id)
      if (ids.length) node.setAttribute('aria-describedby', ids.join(' '))
      else if (original === '') node.setAttribute('aria-describedby', '')
      else node.removeAttribute('aria-describedby')
      tip.remove(); accessible.remove()
    },
  }
}
