// The on-screen keys for a terminal without a hardware keyboard: the row that
// stays beside the screen, the second row behind it, and how the modifiers a
// person has on join the next key. Sequences follow xterm's terminal
// conventions.
import type { TerminalIconName } from './icons'

export type Modifier = 'Ctrl' | 'Alt' | 'Shift'
/** A key's cap text, what it sends, and what it is called aloud when the cap alone would not say. */
export type VirtualKey = { label:string; value:string; icon?:TerminalIconName; title?:string; aria?:string }
/** One group of the second row: its name, the short cap its button shows, and its keys. */
export type KeyGroup = { id:string; title:string; tab:string; keys:VirtualKey[] }

export const modifiers: { name:Modifier; title:string }[] = [
  { name:'Ctrl', title:'Ctrl: tap for the next key, tap again to lock' },
  { name:'Alt', title:'Alt: tap for the next key, tap again to lock' },
  { name:'Shift', title:'Shift: tap for the next key, tap again to lock' },
]

// --- Which modifiers are on -------------------------------------------------

/** off; on for the next key only; or locked on until tapped again. */
export type ModifierState = 'off' | 'next' | 'locked'
export type Held = Record<Modifier, ModifierState>

export const noModifiers = (): Held => ({ Ctrl:'off', Alt:'off', Shift:'off' })

/** One tap: off, then on for the next key, then locked, then off again. */
export function tapModifier(held: Held, name: Modifier): Held {
  const next: ModifierState = held[name] === 'off' ? 'next' : held[name] === 'next' ? 'locked' : 'off'
  return { ...held, [name]: next }
}

/** The modifiers that join the next key, in a fixed order. */
export function activeModifiers(held: Held): Modifier[] {
  return modifiers.map(key => key.name).filter(name => held[name] !== 'off')
}

/** A key was sent: the modifiers that were on for one key are used up; locked ones stay. The same object comes back when nothing changes. */
export function afterKey(held: Held): Held {
  if (!modifiers.some(key => held[key.name] === 'next')) return held
  const next = { ...held }
  for (const key of modifiers) if (next[key.name] === 'next') next[key.name] = 'off'
  return next
}

// --- The keys ---------------------------------------------------------------

// The row that stays: what a phone's own keyboard lacks and a shell or an
// agent's interface needs most. The modifiers are drawn between Tab and ^C.
export const barKeys: VirtualKey[] = [
  { label:'Esc', value:'\x1b', title:'Escape' },
  { label:'Tab', value:'\t', title:'Tab' },
  { label:'^C', value:'\x03', aria:'Send Ctrl+C', title:'Interrupt · Ctrl+C' },
  { label:'/', value:'/', title:'Type /' },
  { label:'|', value:'|', title:'Type |' },
  { label:'-', value:'-', title:'Type -' },
  { label:'~', value:'~', title:'Type ~' },
]
/** How many of barKeys come before the modifiers. */
export const BAR_KEYS_BEFORE_MODIFIERS = 2

// The arrows keep their own place at the end of the row, always in view.
export const arrowKeys: VirtualKey[] = [
  { label:'←', value:'\x1b[D', icon:'arrowLeft', aria:'Send Left arrow', title:'Arrow left' },
  { label:'↑', value:'\x1b[A', icon:'arrowUp', aria:'Send Up arrow', title:'Arrow up' },
  { label:'↓', value:'\x1b[B', icon:'arrowDown', aria:'Send Down arrow', title:'Arrow down' },
  { label:'→', value:'\x1b[C', icon:'arrowRight', aria:'Send Right arrow', title:'Arrow right' },
]

const editKeys: VirtualKey[] = [
  { label:'Enter', value:'\r', title:'Enter' },
  { label:'Bksp', value:'\x7f', aria:'Send Backspace', title:'Backspace' },
  { label:'Del', value:'\x1b[3~', aria:'Send Delete', title:'Delete' },
  { label:'Home', value:'\x1b[H', title:'Home' },
  { label:'End', value:'\x1b[F', title:'End' },
  { label:'PgUp', value:'\x1b[5~', aria:'Send Page Up', title:'Page up' },
  { label:'PgDn', value:'\x1b[6~', aria:'Send Page Down', title:'Page down' },
  { label:'Ins', value:'\x1b[2~', aria:'Send Insert', title:'Insert' },
]

// Characters that a phone keyboard buries behind a symbol page. The four on the
// first row are not repeated here.
const symbolKeys: VirtualKey[] = ['\\','_','=','+','*','&',';',':','"',"'",'`','$','(',')','[',']','{','}','<','>','!','#','@','%','^','?'].map(label=>({label,value:label,title:`Type ${label}`}))

// Common control combinations, as a keyboard sends them. Ctrl+C is on the first row.
const controlKeys: VirtualKey[] = ([
  ['D','\x04','End input / log out'], ['Z','\x1a','Suspend the running command'], ['L','\x0c','Clear the screen'],
  ['R','\x12','Search shell history'], ['A','\x01','Start of the line'], ['E','\x05','End of the line'],
  ['K','\x0b','Delete to the end of the line'], ['U','\x15','Delete to the start of the line'], ['W','\x17','Delete the previous word'],
  ['B','\x02','Back a character, or the tmux prefix'], ['O','\x0f','Ctrl+O'], ['X','\x18','Ctrl+X'],
] as const).map(([letter,value,title])=>({label:`^${letter}`,value,aria:`Send Ctrl+${letter}`,title:title.startsWith('Ctrl+')?title:`${title} · Ctrl+${letter}`}))

const functionKeys: VirtualKey[] = [
  ['F1','\x1bOP'],['F2','\x1bOQ'],['F3','\x1bOR'],['F4','\x1bOS'],
  ['F5','\x1b[15~'],['F6','\x1b[17~'],['F7','\x1b[18~'],['F8','\x1b[19~'],
  ['F9','\x1b[20~'],['F10','\x1b[21~'],['F11','\x1b[23~'],['F12','\x1b[24~'],
].map(([label,value])=>({label,value,title:label}))

/** The second row, one group at a time. */
export const keyGroups: KeyGroup[] = [
  { id:'edit', title:'Editing and navigation', tab:'Edit', keys:editKeys },
  { id:'symbols', title:'Symbols', tab:'#+=', keys:symbolKeys },
  { id:'ctrl', title:'Ctrl combinations', tab:'^X', keys:controlKeys },
  { id:'fn', title:'Function keys', tab:'Fn', keys:functionKeys },
]

/** Keys that send again while held: moving and deleting. A key that acts once, such as Esc, Enter or Ctrl+C, never repeats. */
const repeating = new Set(['\x1b[A','\x1b[B','\x1b[C','\x1b[D','\x7f','\x1b[3~','\x1b[5~','\x1b[6~'])
export const repeats = (value: string): boolean => repeating.has(value)
/** How long a key is held before it repeats, and how often it then sends, in milliseconds. */
export const REPEAT_AFTER = 400
export const REPEAT_EVERY = 70

// --- Modifiers joining a key -------------------------------------------------

// The modifier form of a cursor or editing key (xterm: CSI 1 ; modifier final).
const edited: Record<string, [string,string]> = {
  '\x1b[A':['1','A'], '\x1b[B':['1','B'], '\x1b[C':['1','C'], '\x1b[D':['1','D'],
  '\x1b[H':['1','H'], '\x1b[F':['1','F'],
  '\x1b[2~':['2','~'], '\x1b[3~':['3','~'], '\x1b[5~':['5','~'], '\x1b[6~':['6','~'],
  '\x1bOP':['1','P'], '\x1bOQ':['1','Q'], '\x1bOR':['1','R'], '\x1bOS':['1','S'],
  '\x1b[15~':['15','~'], '\x1b[17~':['17','~'], '\x1b[18~':['18','~'], '\x1b[19~':['19','~'],
  '\x1b[20~':['20','~'], '\x1b[21~':['21','~'], '\x1b[23~':['23','~'], '\x1b[24~':['24','~'],
}

/** xterm's modifier parameter for one modifier: Shift 2, Alt 3, Ctrl 5. */
export const modifierCode = (modifier: Modifier): number => modifier === 'Shift' ? 2 : modifier === 'Alt' ? 3 : 5

/**
 * The bytes the given modifiers send with a key. A cursor or editing key gets
 * its xterm modifier form, whose parameter is one plus Shift's 1, Alt's 2 and
 * Ctrl's 4. A single character gets Shift's capital, then Ctrl's control code,
 * then Alt's leading escape. Anything else, such as pasted text or an answer
 * the terminal itself sends, is returned unchanged.
 */
export function withModifiers(value: string, held: readonly Modifier[]): string {
  if (!held.length) return value
  const shift = held.includes('Shift'), alt = held.includes('Alt'), ctrl = held.includes('Ctrl')
  const editedKey = edited[value]
  if (editedKey) { const [number, final] = editedKey; return `\x1b[${number};${1 + (shift ? 1 : 0) + (alt ? 2 : 0) + (ctrl ? 4 : 0)}${final}` }
  if (value.length !== 1) return value
  let out = value
  if (shift) out = out === '\t' ? '\x1b[Z' : out.toUpperCase()
  if (ctrl && /^[a-zA-Z@[\]\\^_]$/.test(out)) out = String.fromCharCode(out.toUpperCase().charCodeAt(0) & 31)
  return alt ? `\x1b${out}` : out
}

/** The bytes one selected modifier sends with a key. */
export function withModifier(value: string, modifier: Modifier | null): string {
  return withModifiers(value, modifier ? [modifier] : [])
}
