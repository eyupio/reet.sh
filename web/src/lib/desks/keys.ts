// The desk's keys: Alt+Shift chords only, read by physical key (`code`) so a keyboard layout or Shift's symbol does not
// change them. None shadows a shell key: a shell sees Alt+Shift+letter as Esc and a capital, which nothing binds by
// default, and Ctrl or Meta with them is left alone. Alt+Shift+M (the Sessions terminal's maximise) is not a desk key;
// a desk pane zooms instead.

export type DeskKey = 'splitRight' | 'splitDown' | 'focusLeft' | 'focusRight' | 'focusUp' | 'focusDown' | 'zoom' | 'closePane' | 'newTab' | 'prevTab' | 'nextTab' | 'commandBar' | { tab: number }

const CODES: Record<string, Exclude<DeskKey, { tab: number }>> = {
  KeyD: 'splitRight', KeyE: 'splitDown',
  ArrowLeft: 'focusLeft', ArrowRight: 'focusRight', ArrowUp: 'focusUp', ArrowDown: 'focusDown',
  KeyZ: 'zoom', KeyW: 'closePane', KeyT: 'newTab', BracketLeft: 'prevTab', BracketRight: 'nextTab', KeyK: 'commandBar',
}

/** The desk action for a key press, or null for anything that is not exactly Alt+Shift and one of the chords. */
export function deskKey(e: { altKey: boolean; shiftKey: boolean; ctrlKey: boolean; metaKey: boolean; code: string }): DeskKey | null {
  if (!e.altKey || !e.shiftKey || e.ctrlKey || e.metaKey) return null
  const digit = /^Digit([1-9])$/.exec(e.code)
  if (digit) return { tab: Number(digit[1]) }
  return CODES[e.code] ?? null
}

/** What the desk menu's Keys entry lists, in this order. */
export const DESK_KEYS: { keys: string; label: string }[] = [
  { keys: 'Alt+Shift+D', label: 'Split right' },
  { keys: 'Alt+Shift+E', label: 'Split down' },
  { keys: 'Alt+Shift+arrows', label: 'Move focus to the next pane' },
  { keys: 'Alt+Shift+Z', label: 'Zoom the pane, or show them all' },
  { keys: 'Alt+Shift+W', label: 'Close the pane' },
  { keys: 'Alt+Shift+T', label: 'New tab' },
  { keys: 'Alt+Shift+[ and ]', label: 'Previous and next tab' },
  { keys: 'Alt+Shift+1…9', label: 'Tab by number' },
  { keys: 'Alt+Shift+K', label: 'Command bar' },
]
