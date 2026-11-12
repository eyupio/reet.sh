// The on-screen keys for a terminal without a hardware keyboard: the compact
// row under the screen, the expanded key pad, and how a selected modifier joins
// the next key. Sequences follow xterm's terminal conventions.
import type { TerminalIconName } from './icons'

export type Modifier = 'Ctrl' | 'Alt' | 'Shift'
export type VirtualKey = { label:string; value:string; icon?:TerminalIconName; title?:string; aria?:string; caption?:boolean }

export const modifiers: { name:Modifier; icon:TerminalIconName; title:string }[] = [
  { name:'Ctrl', icon:'ctrl', title:'Ctrl for the next key' },
  { name:'Alt', icon:'alt', title:'Alt for the next key' },
  { name:'Shift', icon:'shift', title:'Shift for the next key' },
]

// The row that stays beside the terminal: navigation first, since a phone's
// own keyboard has no arrow keys.
export const stripKeys: VirtualKey[] = [
  { label:'Esc', value:'\x1b', icon:'escape', title:'Escape', caption:true },
  { label:'Tab', value:'\t', icon:'tab', title:'Tab', caption:true },
  { label:'Up', value:'\x1b[A', icon:'arrowUp', aria:'Send Up arrow', title:'Arrow up' },
  { label:'Down', value:'\x1b[B', icon:'arrowDown', aria:'Send Down arrow', title:'Arrow down' },
  { label:'Left', value:'\x1b[D', icon:'arrowLeft', aria:'Send Left arrow', title:'Arrow left' },
  { label:'Right', value:'\x1b[C', icon:'arrowRight', aria:'Send Right arrow', title:'Arrow right' },
  { label:'Enter', value:'\r', icon:'enter', title:'Enter', caption:true },
  { label:'Backspace', value:'\x7f', icon:'backspace', aria:'Send Backspace', title:'Backspace', caption:true },
]

// Common control combinations, as a keyboard sends them.
export const comboKeys: VirtualKey[] = [
  { label:'Ctrl+C', value:'\x03', title:'Interrupt the running command' },
  { label:'Ctrl+D', value:'\x04', title:'End input / log out' },
  { label:'Ctrl+Z', value:'\x1a', title:'Suspend the running command' },
  { label:'Ctrl+L', value:'\x0c', title:'Clear the screen' },
  { label:'Ctrl+R', value:'\x12', title:'Search shell history' },
  { label:'Ctrl+A', value:'\x01', title:'Start of the line' },
  { label:'Ctrl+E', value:'\x05', title:'End of the line' },
  { label:'Ctrl+K', value:'\x0b', title:'Delete to the end of the line' },
  { label:'Ctrl+U', value:'\x15', title:'Delete to the start of the line' },
  { label:'Ctrl+W', value:'\x17', title:'Delete the previous word' },
]

export const navigationKeys: VirtualKey[] = [
  { label:'Home', value:'\x1b[H', icon:'home', title:'Home', caption:true },
  { label:'End', value:'\x1b[F', icon:'end', title:'End', caption:true },
  { label:'PgUp', value:'\x1b[5~', icon:'pageUp', aria:'Send Page Up', title:'Page up', caption:true },
  { label:'PgDn', value:'\x1b[6~', icon:'pageDown', aria:'Send Page Down', title:'Page down', caption:true },
  { label:'Ins', value:'\x1b[2~', icon:'insert', aria:'Send Insert', title:'Insert', caption:true },
  { label:'Del', value:'\x1b[3~', icon:'delete', aria:'Send Delete', title:'Delete', caption:true },
]

export const functionKeys: VirtualKey[] = [
  ['F1','\x1bOP'],['F2','\x1bOQ'],['F3','\x1bOR'],['F4','\x1bOS'],
  ['F5','\x1b[15~'],['F6','\x1b[17~'],['F7','\x1b[18~'],['F8','\x1b[19~'],
  ['F9','\x1b[20~'],['F10','\x1b[21~'],['F11','\x1b[23~'],['F12','\x1b[24~'],
].map(([label,value])=>({label,value,title:label}))

// Characters that a phone keyboard buries behind a symbol page.
export const symbolKeys: VirtualKey[] = ['|','~','/','\\','-','_','=','+','*','&',';',':','"',"'",'(',')','[',']','{','}','<','>'].map(label=>({label,value:label,title:`Type ${label}`}))

// The modifier form of a cursor or editing key (xterm: CSI 1 ; modifier final).
const edited: Record<string, [string,string]> = {
  '\x1b[A':['1','A'], '\x1b[B':['1','B'], '\x1b[C':['1','C'], '\x1b[D':['1','D'],
  '\x1b[H':['1','H'], '\x1b[F':['1','F'],
  '\x1b[2~':['2','~'], '\x1b[3~':['3','~'], '\x1b[5~':['5','~'], '\x1b[6~':['6','~'],
  '\x1bOP':['1','P'], '\x1bOQ':['1','Q'], '\x1bOR':['1','R'], '\x1bOS':['1','S'],
  '\x1b[15~':['15','~'], '\x1b[17~':['17','~'], '\x1b[18~':['18','~'], '\x1b[19~':['19','~'],
  '\x1b[20~':['20','~'], '\x1b[21~':['21','~'], '\x1b[23~':['23','~'], '\x1b[24~':['24','~'],
}

/** xterm's modifier parameter: Shift 2, Alt 3, Ctrl 5. */
export const modifierCode = (modifier: Modifier): number => modifier === 'Shift' ? 2 : modifier === 'Alt' ? 3 : 5

/**
 * The bytes a selected modifier sends with a key. A cursor or editing key gets
 * its xterm modifier form; a printable character gets Ctrl's control code, Alt's
 * leading escape, or Shift's capital. Unknown sequences are sent unchanged.
 */
export function withModifier(value: string, modifier: Modifier | null): string {
  if (!modifier) return value
  if (modifier === 'Shift' && value === '\t') return '\x1b[Z'
  const editedKey = edited[value]
  if (editedKey) { const [number, final] = editedKey; return `\x1b[${number};${modifierCode(modifier)}${final}` }
  if (value.length !== 1) return value
  if (modifier === 'Ctrl') return /^[a-zA-Z@[\]\\^_]$/.test(value) ? String.fromCharCode(value.toUpperCase().charCodeAt(0) & 31) : value
  if (modifier === 'Alt') return `\x1b${value}`
  return value.toUpperCase()
}
