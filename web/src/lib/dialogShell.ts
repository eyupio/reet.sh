// The shared dialog shell: what every modal <dialog> in the app does on a phone, a short screen or with the on-screen
// keyboard open. Each dialog keeps its own markup, frame, focus return, Escape handling and in-flight guard; it adds
// `use:shell` to its <dialog> and marks its regions, and dialog-shell.css does the rest. See docs/ui-verification.md
// ("Phones: dialogs").
//
// Regions, as attributes in the dialog's markup:
//  data-shell-head   a header that carries its own padding (compacted on a short screen)
//  data-shell-body   the part that scrolls
//  data-shell-foot   the footer with the main action (one row on a short screen)
//  data-shell-card   the visible card, for a dialog that is itself transparent around it
//  data-shell-aside  helper text that a short screen hides (eyebrows, ledes, reassurance lines)
//  data-shell-close  the close control (44 x 44 px on a touch screen)
//
// What this module does at run time:
//  - With the keyboard open on a browser that only shrinks the visual viewport (Safari, and Chrome on Android unless the
//    page asks for resizes-content), the dialog is fitted to the part of the screen above the keyboard: --shell-top,
//    --shell-height and --shell-bottom on the dialog, and data-shell-kb.
//  - data-shell-short when the usable height is 500 px or less (a sideways phone) or the keyboard is open, whether the
//    browser shrank only the visual viewport or resized the page itself (as the terminal asks for).
//  - The focused field is scrolled into the visible part of each scrolling region: below a pinned header, above a pinned
//    footer and above the keyboard, when it gains focus and again whenever the visible area changes size.
import type { Action } from 'svelte/action'

export type ShellOptions = { sheet?: 'full' | 'bottom' }

// The keyboard is taken to be open when the visual viewport is at least 15% shorter than the page at normal zoom, the
// same test as the terminal's (sizing.ts).
const KEYBOARD_RATIO = 0.85
const SHORT = 500

function scrollParents(el: HTMLElement, stop: HTMLElement): HTMLElement[] {
 const out: HTMLElement[] = []
 for (let n = el.parentElement; n; n = n.parentElement) {
  const s = getComputedStyle(n)
  if (/(auto|scroll)/.test(s.overflowY) && n.scrollHeight > n.clientHeight + 1) out.push(n)
  if (n === stop) break
 }
 return out
}

// Scrolls el into the part of each scrolling region that can be seen: inside the region, inside the visual viewport, and
// clear of any header or footer pinned (sticky) over the region.
export function reveal(el: Element | null, dialog: HTMLElement) {
 if (!(el instanceof HTMLElement) || el === dialog || !dialog.contains(el)) return
 const vv = window.visualViewport
 const viewTop = vv ? vv.offsetTop : 0, viewBottom = vv ? vv.offsetTop + vv.height : window.innerHeight
 for (const sc of scrollParents(el, dialog)) {
  const r = sc.getBoundingClientRect()
  let top = Math.max(r.top + sc.clientTop, viewTop), bottom = Math.min(r.top + sc.clientTop + sc.clientHeight, viewBottom)
  for (const pinned of sc.querySelectorAll<HTMLElement>('[data-shell-head],[data-shell-foot]')) {
   if (pinned.contains(el) || getComputedStyle(pinned).position !== 'sticky') continue
   const p = pinned.getBoundingClientRect()
   if (p.bottom <= top || p.top >= bottom) continue
   if (p.top - top < bottom - p.bottom) top = Math.max(top, p.bottom)
   else bottom = Math.min(bottom, p.top)
  }
  const box = el.getBoundingClientRect()
  const room = bottom - top - box.height
  const gap = Math.max(0, Math.min(8, room / 2))
  if (box.top < top + gap) sc.scrollTop -= top + gap - box.top
  else if (box.bottom > bottom - gap) sc.scrollTop += Math.min(box.bottom - (bottom - gap), box.top - (top + gap))
 }
}

export const shell: Action<HTMLDialogElement, ShellOptions | undefined> = (node, options) => {
 node.dataset.shell = options?.sheet ?? 'full'
 const vv = window.visualViewport
 let frame = 0, width = 0, tallest = 0, last = ''

 function measure() {
  frame = 0
  const inner = window.innerHeight
  const height = vv ? vv.height : inner, top = vv ? vv.offsetTop : 0
  // The keyboard over the page: only the visual viewport shrank, at normal zoom.
  const keyboard = !!vv && Math.abs(vv.scale - 1) < 0.01 && height < inner * KEYBOARD_RATIO
  // The keyboard resized the page itself: a touch screen shorter than the tallest seen at this width (turning the phone
  // changes both). A desktop window dragged shorter is not a keyboard.
  if (window.innerWidth !== width) { width = window.innerWidth; tallest = 0 }
  tallest = Math.max(tallest, inner)
  const resized = inner < tallest * KEYBOARD_RATIO && matchMedia('(pointer: coarse)').matches
  if (keyboard) {
   node.style.setProperty('--shell-top', `${top}px`)
   node.style.setProperty('--shell-height', `${height}px`)
   node.style.setProperty('--shell-bottom', `${Math.max(0, inner - top - height)}px`)
  } else for (const name of ['--shell-top', '--shell-height', '--shell-bottom']) node.style.removeProperty(name)
  node.toggleAttribute('data-shell-kb', keyboard)
  node.toggleAttribute('data-shell-short', keyboard || resized || inner <= SHORT)
  // Only when the visible area changed size: a visual viewport that merely scrolled leaves the person's place alone.
  const now = `${width}x${inner}:${height}`
  if (now !== last) { last = now; reveal(document.activeElement, node) }
 }
 const schedule = () => { if (!frame) frame = requestAnimationFrame(measure) }
 const focused = (event: FocusEvent) => { const target = event.target as Element; requestAnimationFrame(() => { if (document.activeElement === target) reveal(target, node) }) }

 measure()
 node.addEventListener('focusin', focused)
 window.addEventListener('resize', schedule)
 vv?.addEventListener('resize', schedule)
 vv?.addEventListener('scroll', schedule)
 return {
  update(next) { node.dataset.shell = next?.sheet ?? 'full' },
  destroy() {
   cancelAnimationFrame(frame)
   node.removeEventListener('focusin', focused)
   window.removeEventListener('resize', schedule)
   vv?.removeEventListener('resize', schedule)
   vv?.removeEventListener('scroll', schedule)
  },
 }
}
