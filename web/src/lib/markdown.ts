import { Lexer, type Token, type Tokens } from 'marked'

export type MdSpan =
  | { kind: 'text' | 'code'; text: string }
  | { kind: 'strong' | 'em' | 'del'; spans: MdSpan[] }
  | { kind: 'link'; href: string; spans: MdSpan[] }
  | { kind: 'br' }

export type MdAlign = 'left' | 'center' | 'right' | null

export type MdBlock =
  | { kind: 'p'; spans: MdSpan[] }
  /** Inline content that sits straight in its list item, as a tight list has it. */
  | { kind: 'text'; spans: MdSpan[] }
  | { kind: 'h'; level: 1 | 2 | 3 | 4 | 5 | 6; spans: MdSpan[] }
  | { kind: 'code'; lang: string; text: string }
  | { kind: 'quote'; blocks: MdBlock[] }
  | { kind: 'list'; ordered: boolean; start: number; items: { checked: boolean | null; blocks: MdBlock[] }[] }
  | { kind: 'table'; head: MdSpan[][] | null; align: MdAlign[]; rows: MdSpan[][][] }
  | { kind: 'rule' }

const options = { gfm: true, breaks: true }
const ENTITIES: Record<string, string> = { amp: '&', lt: '<', gt: '>', quot: '"', apos: "'", '#39': "'", nbsp: ' ' }
const decode = (text: string) => text.replace(/&(amp|lt|gt|quot|apos|#39|nbsp);/g, (_, name: string) => ENTITIES[name])
const safeHref = (href: string) => /^https?:\/\//i.test(href)

function pushText(out: MdSpan[], text: string) {
  if (!text) return
  const last = out[out.length - 1]
  if (last?.kind === 'text') last.text += text
  else out.push({ kind: 'text', text })
}

/**
 * Inline tokens as spans. Nothing is ever treated as HTML: a tag in the message is shown as the text it is. Only
 * http and https links survive as links, so a crafted message cannot smuggle in a `javascript:` or `data:` target;
 * any other link keeps its words and loses its destination. An image is shown as a link to it, never loaded.
 */
function spans(tokens: Token[] | undefined, out: MdSpan[] = []): MdSpan[] {
  for (const token of tokens ?? []) {
    switch (token.type) {
      case 'strong': case 'em': case 'del': out.push({ kind: token.type, spans: spans(token.tokens) }); break
      case 'codespan': out.push({ kind: 'code', text: decode(token.text) }); break
      case 'br': out.push({ kind: 'br' }); break
      case 'link':
        if (safeHref(token.href)) out.push({ kind: 'link', href: token.href, spans: spans(token.tokens) })
        else spans(token.tokens, out)
        break
      case 'image':
        if (safeHref(token.href)) out.push({ kind: 'link', href: token.href, spans: [{ kind: 'text', text: token.text || token.href }] })
        else pushText(out, token.text)
        break
      case 'text':
        if (token.tokens?.length) spans(token.tokens, out)
        else pushText(out, decode(token.text))
        break
      case 'escape': pushText(out, decode(token.text)); break
      case 'checkbox': break
      default: pushText(out, token.raw ?? '')
    }
  }
  return out
}

/** The cells of a `| a | b |` line, or null when the line is not one. A pipe inside `code`, or written `\|`, stays in its cell. */
function tableCells(line: string): string[] | null {
  const row = line.trim()
  if (row.length < 2 || row[0] !== '|' || !row.endsWith('|') || row.endsWith('\\|')) return null
  const cells: string[] = []
  let cell = '', code = false
  for (let i = 1; i < row.length; i++) {
    const ch = row[i]
    if (ch === '\\' && row[i + 1] === '|') { cell += '|'; i++; continue }
    if (ch === '`' && (code || row.indexOf('`', i + 1) !== -1)) code = !code
    if (ch === '|' && (!code || i === row.length - 1)) { cells.push(cell.trim()); cell = ''; continue }
    cell += ch
  }
  return cells
}

/**
 * Table rows whose header is gone. A long answer can lose its first lines to the history bound, and markdown alone
 * would then show the rest of the table as a paragraph full of pipes.
 */
function headlessTable(text: string): MdBlock | null {
  const rows: string[][] = []
  for (const line of text.split('\n')) {
    const cells = tableCells(line)
    if (!cells) return null
    if (!cells.every(cell => /^:?-+:?$/.test(cell))) rows.push(cells)
  }
  if (rows.length < 2) return null
  const width = Math.max(...rows.map(row => row.length))
  return { kind: 'table', head: null, align: Array<MdAlign>(width).fill(null), rows: rows.map(row => Array.from({ length: width }, (_, i) => spans(Lexer.lexInline(row[i] ?? '', options)))) }
}

function blocks(tokens: Token[]): MdBlock[] {
  const out: MdBlock[] = []
  for (const token of tokens) {
    switch (token.type) {
      case 'space': case 'def': break
      case 'heading': out.push({ kind: 'h', level: Math.min(6, Math.max(1, token.depth)) as 1 | 2 | 3 | 4 | 5 | 6, spans: spans(token.tokens) }); break
      case 'code': out.push({ kind: 'code', lang: (token.lang ?? '').trim().split(/\s+/)[0] ?? '', text: token.text }); break
      case 'blockquote': out.push({ kind: 'quote', blocks: blocks(token.tokens ?? []) }); break
      case 'hr': out.push({ kind: 'rule' }); break
      case 'list': {
        const list = token as Tokens.List
        out.push({ kind: 'list', ordered: list.ordered, start: typeof list.start === 'number' ? list.start : 1, items: list.items.map(item => ({ checked: item.task ? !!item.checked : null, blocks: blocks(item.tokens) })) })
        break
      }
      case 'table': {
        const table = token as Tokens.Table
        out.push({ kind: 'table', head: table.header.map(cell => spans(cell.tokens)), align: table.align, rows: table.rows.map(row => row.map(cell => spans(cell.tokens))) })
        break
      }
      case 'paragraph': out.push(headlessTable(token.text) ?? { kind: 'p', spans: spans(token.tokens) }); break
      case 'text': out.push({ kind: 'text', spans: spans(token.tokens ?? [token]) }); break
      case 'checkbox': break
      default: if (token.raw?.trim()) out.push({ kind: 'p', spans: [{ kind: 'text', text: token.raw.trim() }] })
    }
  }
  return out
}

/**
 * Markdown (CommonMark with GitHub's tables, task lists, strikethrough and bare links) as blocks a component renders
 * element by element. A line break in the message stays a line break, as agents write their answers expecting.
 */
export function markdownBlocks(text: string): MdBlock[] {
  return blocks(new Lexer(options).lex(text.replace(/\r\n/g, '\n')))
}
