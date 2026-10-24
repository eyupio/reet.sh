export type Shortcut = 'copy' | 'native-paste' | 'search' | 'terminal'

// Used only by xterm, never editable app fields. Native paste is left to the
// browser's paste event so a single shortcut cannot send clipboard text twice.
export function terminalShortcut(event: Pick<KeyboardEvent,'type'|'key'|'ctrlKey'|'metaKey'|'shiftKey'|'altKey'>, selected: boolean, mac: boolean): Shortcut {
  if (event.type !== 'keydown' || event.altKey) return 'terminal'
  const key = event.key.toLowerCase()
  if ((event.ctrlKey || event.metaKey) && event.shiftKey && key === 'f') return 'search'
  if ((event.ctrlKey || event.metaKey) && key === 'v') return 'native-paste'
  if (key === 'c' && (event.metaKey && !event.ctrlKey || event.ctrlKey && (event.shiftKey || selected && !mac))) return 'copy'
  return 'terminal'
}
