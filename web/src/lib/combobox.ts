// One searchable list for every dropdown in the app.
//
// A native <select> keeps its place in the page (so its styling, label, bindings and tests are unchanged), but its own popup
// is replaced: opening it shows a filterable listbox. An <input data-combo-list="id"> (a free-text field with suggestions
// read from the <datalist id>) gets the same list under it, and keeps accepting anything typed.
//
// Opt a select out with data-native. Multi-selects and list boxes (size > 1) are left alone.

type Item = { label: string; value: string; index: number; group: string; disabled: boolean; selected: boolean }
type Open = {
  anchor: HTMLSelectElement | HTMLInputElement
  root: HTMLElement
  filter: HTMLInputElement | null
  list: HTMLElement
  items: Item[]
  shown: Item[]
  active: number
  source: () => Item[]
  pick: (item: Item) => void
  cleanup: () => void
}

let current: Open | null = null
let uid = 0

const isSelect = (el: unknown): el is HTMLSelectElement => el instanceof HTMLSelectElement
const comboSelect = (el: unknown): el is HTMLSelectElement => isSelect(el) && !el.multiple && el.size <= 1 && !el.hasAttribute('data-native')
const comboInput = (el: unknown): el is HTMLInputElement => el instanceof HTMLInputElement && el.hasAttribute('data-combo-list')
const fine = () => matchMedia('(pointer: fine)').matches

function selectItems(sel: HTMLSelectElement): Item[] {
  const out: Item[] = []
  let index = 0
  for (const o of Array.from(sel.options)) {
    const group = o.parentElement instanceof HTMLOptGroupElement ? o.parentElement.label : ''
    out.push({ label: (o.label || o.textContent || '').trim(), value: o.value, index: index++, group, disabled: o.disabled || (o.parentElement instanceof HTMLOptGroupElement && o.parentElement.disabled), selected: o.selected })
  }
  return out
}

function datalistItems(input: HTMLInputElement): Item[] {
  const list = document.getElementById(input.getAttribute('data-combo-list') ?? '')
  const out: Item[] = []
  let index = 0
  for (const o of Array.from(list?.querySelectorAll('option') ?? [])) {
    const value = o.getAttribute('value') ?? o.textContent ?? ''
    if (!value) continue
    out.push({ label: o.label || value, value, index: index++, group: '', disabled: o.disabled, selected: value === input.value })
  }
  return out
}

function matching(items: Item[], text: string, allWhenExact: boolean): Item[] {
  const q = text.trim().toLowerCase()
  if (!q) return items
  if (allWhenExact && items.some(i => i.value.toLowerCase() === q)) return items
  return items.filter(i => i.label.toLowerCase().includes(q) || i.value.toLowerCase().includes(q))
}

function place(o: Open) {
  const r = o.anchor.getBoundingClientRect()
  const vw = document.documentElement.clientWidth, vh = document.documentElement.clientHeight
  const width = Math.min(Math.max(r.width, 240), vw - 16)
  const left = Math.max(8, Math.min(r.left, vw - width - 8))
  const below = vh - r.bottom - 12, above = r.top - 12
  const wanted = Math.min(340, o.root.scrollHeight || 340)
  const down = below >= Math.min(wanted, 220) || below >= above
  const max = Math.max(140, Math.min(340, down ? below : above))
  const s = o.root.style
  s.width = `${width}px`
  s.left = `${left}px`
  s.maxHeight = `${max}px`
  if (down) { s.top = `${r.bottom + 4}px`; s.bottom = 'auto' }
  else { s.bottom = `${vh - r.top + 4}px`; s.top = 'auto' }
}

function render(o: Open) {
  const id = o.list.id
  o.list.textContent = ''
  let group: string | null = null
  o.shown.forEach((item, i) => {
    if (item.group && item.group !== group) {
      const h = document.createElement('div')
      h.className = 'cbx-group'
      h.setAttribute('role', 'presentation')
      h.textContent = item.group
      o.list.append(h)
    }
    group = item.group
    const el = document.createElement('div')
    el.className = 'cbx-option'
    el.id = `${id}-${i}`
    el.setAttribute('role', 'option')
    el.setAttribute('aria-selected', String(item.selected))
    if (item.disabled) el.setAttribute('aria-disabled', 'true')
    el.dataset.i = String(i)
    el.textContent = item.label || ' '
    o.list.append(el)
  })
  if (o.shown.length === 0) {
    const p = document.createElement('p')
    p.className = 'cbx-empty'
    p.textContent = 'Nothing matches.'
    o.list.append(p)
  }
  setActive(o, o.active)
}

function setActive(o: Open, i: number) {
  const n = o.shown.length
  o.active = n === 0 ? -1 : Math.max(0, Math.min(n - 1, i))
  o.list.querySelectorAll('.cbx-option.active').forEach(e => e.classList.remove('active'))
  const el = o.active >= 0 ? o.list.querySelector<HTMLElement>(`[data-i="${o.active}"]`) : null
  el?.classList.add('active')
  const owner = o.filter ?? o.anchor
  if (el) { owner.setAttribute('aria-activedescendant', el.id); el.scrollIntoView({ block: 'nearest' }) }
  else owner.removeAttribute('aria-activedescendant')
}

/** Moves to the next enabled option in a direction, wrapping at the ends. */
function step(o: Open, from: number, dir: 1 | -1): number {
  const n = o.shown.length
  for (let k = 1; k <= n; k++) {
    const i = (from + dir * k + n * k) % n
    if (!o.shown[i].disabled) return i
  }
  return from
}

function close(returnFocus = true) {
  const o = current
  if (!o) return
  current = null
  o.cleanup()
  o.anchor.setAttribute('aria-expanded', 'false')
  o.anchor.removeAttribute('aria-activedescendant')
  try { o.root.hidePopover?.() } catch { /* not shown as a popover */ }
  o.root.remove()
  if (returnFocus && o.anchor.isConnected) o.anchor.focus({ preventScroll: true })
}

function choose(o: Open, i: number) {
  const item = o.shown[i]
  if (!item || item.disabled) return
  o.pick(item)
  close()
}

function open(anchor: HTMLSelectElement | HTMLInputElement, seed = '') {
  if (current?.anchor === anchor) return
  close(false)
  const inputMode = anchor instanceof HTMLInputElement
  const source = inputMode ? () => datalistItems(anchor) : () => selectItems(anchor as HTMLSelectElement)
  const id = `cbx-${++uid}`
  const root = document.createElement('div')
  root.className = 'cbx'
  root.setAttribute('popover', 'manual')
  const filter = inputMode ? null : document.createElement('input')
  if (filter) {
    filter.className = 'cbx-filter'
    filter.type = 'text'
    filter.autocomplete = 'off'
    filter.spellcheck = false
    filter.placeholder = 'Search…'
    filter.setAttribute('role', 'combobox')
    filter.setAttribute('aria-label', `Search ${anchor.getAttribute('aria-label') || anchor.labels?.[0]?.textContent?.trim() || 'options'}`)
    filter.setAttribute('aria-controls', `${id}-list`)
    filter.setAttribute('aria-expanded', 'true')
    filter.setAttribute('aria-autocomplete', 'list')
    filter.value = seed
    root.append(filter)
  }
  const list = document.createElement('div')
  list.className = 'cbx-list'
  list.id = `${id}-list`
  list.setAttribute('role', 'listbox')
  root.append(list)

  const o: Open = {
    anchor, root, filter, list, items: source(), shown: [], active: -1, source,
    pick: item => {
      if (anchor instanceof HTMLSelectElement) {
        if (anchor.selectedIndex === item.index) return
        anchor.selectedIndex = item.index
      } else {
        if (anchor.value === item.value) return
        anchor.value = item.value
      }
      anchor.dispatchEvent(new Event('input', { bubbles: true }))
      anchor.dispatchEvent(new Event('change', { bubbles: true }))
    },
    cleanup: () => {},
  }
  const refresh = (resetActive: boolean) => {
    o.items = o.source()
    o.shown = matching(o.items, inputMode ? anchor.value : filter!.value, inputMode)
    const selected = o.shown.findIndex(i => i.selected && !i.disabled)
    render(o)
    setActive(o, resetActive ? (inputMode ? (selected >= 0 && anchor.value ? selected : -1) : selected >= 0 && !filter!.value ? selected : step(o, -1, 1)) : o.active)
    if (inputMode && resetActive && !anchor.value) setActive(o, -1)
    place(o)
  }

  const host = anchor.closest('dialog') ?? document.body
  host.append(root)
  try { root.showPopover?.() } catch { root.classList.add('cbx-fixed') }
  anchor.setAttribute('aria-expanded', 'true')
  anchor.setAttribute('aria-controls', list.id)
  current = o
  refresh(true)

  const onOption = (e: Event) => {
    const el = (e.target as HTMLElement).closest<HTMLElement>('.cbx-option')
    return el ? Number(el.dataset.i) : -1
  }
  root.addEventListener('mousedown', e => { if (e.target !== filter) e.preventDefault() })
  list.addEventListener('click', e => { const i = onOption(e); if (i >= 0) choose(o, i) })
  list.addEventListener('mousemove', e => { const i = onOption(e); if (i >= 0 && i !== o.active && !o.shown[i].disabled) setActive(o, i) })

  const keys = (e: KeyboardEvent) => {
    const k = e.key
    if (k === 'Escape') { e.preventDefault(); e.stopPropagation(); close(); return }
    if (k === 'Tab') { close(false); return }
    if (k === 'ArrowDown') { e.preventDefault(); setActive(o, o.active < 0 ? step(o, -1, 1) : step(o, o.active, 1)); return }
    if (k === 'ArrowUp') { e.preventDefault(); setActive(o, o.active < 0 ? step(o, 0, -1) : step(o, o.active, -1)); return }
    if (!inputMode && k === 'Home') { e.preventDefault(); setActive(o, step(o, -1, 1)); return }
    if (!inputMode && k === 'End') { e.preventDefault(); setActive(o, step(o, o.shown.length, -1)); return }
    if (k === 'Enter') {
      if (o.active >= 0 && !o.shown[o.active]?.disabled) { e.preventDefault(); e.stopPropagation(); choose(o, o.active) }
      else if (!inputMode) e.preventDefault()
    }
  }
  const typed = () => refresh(true)
  const keyTarget: HTMLElement = filter ?? anchor
  keyTarget.addEventListener('keydown', keys, true)
  ;(filter ?? anchor).addEventListener('input', typed)
  const outside = (e: Event) => {
    const t = e.target as Node
    if (root.contains(t) || anchor.contains(t)) return
    close(false)
  }
  const reposition = () => { if (!anchor.isConnected) close(false); else place(o) }
  document.addEventListener('pointerdown', outside, true)
  addEventListener('resize', reposition)
  addEventListener('scroll', reposition, true)
  const watch = new MutationObserver(() => { if (!anchor.isConnected) close(false); else refresh(false) })
  if (!inputMode) watch.observe(anchor, { childList: true, subtree: true, characterData: true, attributes: true, attributeFilter: ['label', 'disabled', 'value'] })
  o.cleanup = () => {
    keyTarget.removeEventListener('keydown', keys, true)
    ;(filter ?? anchor).removeEventListener('input', typed)
    document.removeEventListener('pointerdown', outside, true)
    removeEventListener('resize', reposition)
    removeEventListener('scroll', reposition, true)
    watch.disconnect()
  }
  if (filter && fine()) filter.focus({ preventScroll: true })
  if (filter && !fine()) anchor.blur()
}

/** Installs the searchable list for every dropdown on the page, including ones drawn later. Safe to call more than once. */
export function installCombobox() {
  const flag = '__reetCombobox'
  const w = window as unknown as Record<string, unknown>
  if (w[flag]) return
  w[flag] = true

  // A mouse or finger on a select opens the list instead of the browser's own popup.
  document.addEventListener('mousedown', e => {
    if (e.button !== 0) return
    const t = e.target
    if (!comboSelect(t) || t.disabled) return
    e.preventDefault()
    t.focus({ preventScroll: true })
    if (current?.anchor === t) close(false)
    else open(t)
  }, true)

  document.addEventListener('keydown', e => {
    const t = e.target
    if (e.defaultPrevented || e.ctrlKey || e.metaKey) return
    if (comboSelect(t) && !t.disabled) {
      if (current?.anchor === t) return
      if (e.key === 'Enter' || e.key === ' ' || e.key === 'ArrowDown' || e.key === 'ArrowUp') { e.preventDefault(); open(t) }
      else if (e.key.length === 1 && !e.altKey) { e.preventDefault(); open(t, e.key) }
      return
    }
    if (comboInput(t) && !t.disabled && !t.readOnly && !current) {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') { e.preventDefault(); open(t) }
    }
  }, true)

  // A text field with suggestions shows them as it is clicked into, or typed in.
  document.addEventListener('mousedown', e => {
    const t = e.target
    if (!comboInput(t) || t.disabled || t.readOnly) return
    if (current?.anchor === t) { if (t.selectionStart === t.selectionEnd && (e as MouseEvent).offsetX > t.clientWidth - 28) close(false); return }
    queueMicrotask(() => { if (document.activeElement === t || t.matches(':focus-within')) open(t) })
  }, true)
  document.addEventListener('focusin', e => { const t = e.target; if (comboInput(t) && !t.readOnly && !t.disabled && fine() && current?.anchor !== t) open(t) })
  document.addEventListener('input', e => { const t = e.target; if (comboInput(t) && !current && !t.readOnly && !t.disabled) open(t) })
  document.addEventListener('focusout', e => {
    const t = e.target
    if (current?.anchor === t && comboInput(t) && !current.root.contains(e.relatedTarget as Node)) close(false)
  })
}
