// After an entry of a list leaves (answered, marked seen, a choice carried out), the focus would fall to the page's body.
// It goes instead to the entry now in its place, else the one before it, else to the list's own heading control.

const CONTROL = 'a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled])'

/** Whether the focus was lost: on the body, nowhere, or on an element no longer in the page. */
export function focusLost(doc: Document = document): boolean {
  const active = doc.activeElement
  return !active || active === doc.body || !active.isConnected
}

/** Moves the focus, only if it was lost, to the first control of `list`'s item at `index`, else of the item before it,
 *  else to `fallback`. */
export function keepFocusIn(list: HTMLElement | null | undefined, index: number, fallback?: HTMLElement | null): void {
  if (!focusLost()) return
  const items = list?.isConnected ? [...list.children] as HTMLElement[] : []
  for (const item of [items[index], items[index - 1]]) {
    const control = item?.querySelector<HTMLElement>(CONTROL)
    if (control) { control.focus(); return }
  }
  fallback?.focus()
}
