import type { ITheme } from '@xterm/xterm'
export type TerminalPalette = { id:string; name:string; detail:string; colors:ITheme }
const ansi={red:'#e06c75',green:'#98c379',yellow:'#e5c07b',blue:'#61afef',magenta:'#c678dd',cyan:'#56b6c2',white:'#dfe4eb',brightRed:'#f28b94',brightGreen:'#b3dc98',brightYellow:'#f1d697',brightBlue:'#91c8ff',brightMagenta:'#d9a0ef',brightCyan:'#85d9df',brightWhite:'#ffffff'}
export const terminalPalettes:TerminalPalette[]=[
 {id:'midnight',name:'Midnight',detail:'Cool, clear, focused',colors:{...ansi,background:'#101820',foreground:'#dce5ef',cursor:'#7dd3fc',selectionBackground:'#33485e',black:'#101820',brightBlack:'#7f91a7'}},
 {id:'graphite',name:'Graphite',detail:'A quiet engineering desk',colors:{...ansi,background:'#1c1e22',foreground:'#e4e4e7',cursor:'#fb923c',selectionBackground:'#454047',black:'#1c1e22',brightBlack:'#93939e'}},
 {id:'ember',name:'Ember',detail:'Warm charcoal and copper',colors:{...ansi,background:'#211b17',foreground:'#eee3d5',cursor:'#fdba74',selectionBackground:'#5a4031',black:'#211b17',brightBlack:'#a89787'}},
 {id:'paper',name:'Paper',detail:'Crisp ink, soft daylight',colors:{background:'#fbf9f5',foreground:'#292621',cursor:'#a94313',selectionBackground:'#e6d4bf',black:'#292621',red:'#aa263d',green:'#286137',yellow:'#80560a',blue:'#1c5c98',magenta:'#7d3690',cyan:'#146878',white:'#c5c0b8',brightBlack:'#756b61',brightRed:'#a72336',brightGreen:'#287042',brightYellow:'#93620b',brightBlue:'#176bad',brightMagenta:'#8b3ba1',brightCyan:'#167488',brightWhite:'#ffffff'}}
]
export type CursorShape='block'|'bar'|'underline'
export function paletteFor(id:string,dark:boolean):ITheme{
 const palette=terminalPalettes.find(item=>item.id===id)
 if(palette)return palette.colors
 return dark?terminalPalettes[2].colors:terminalPalettes[3].colors
}
