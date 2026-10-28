export interface TerminalBufferLine {
  translateToString(trimRight?: boolean): string
}

export interface TerminalBuffer {
  readonly length: number
  readonly baseY: number
  readonly viewportY: number
  getLine(index: number): TerminalBufferLine | null | undefined
}

function count(value: number | undefined): number {
  return Number.isFinite(value) ? Math.max(0, Math.floor(value as number)) : 0
}

function lineText(buffer: TerminalBuffer, index: number): string {
  const line = buffer.getLine(index)
  return line ? line.translateToString(true) : ''
}

function read(buffer: TerminalBuffer, start: number, end: number): string {
  const length = count(buffer?.length)
  const from = Math.min(count(start), length)
  const to = Math.min(Math.max(count(end), from), length)
  const lines: string[] = []
  for (let index = from; index < to; index++) lines.push(lineText(buffer, index))
  return lines.join('\n')
}

export function screenText(buffer: TerminalBuffer, rows: number): string {
  const length = count(buffer?.length)
  const top = Math.min(count(buffer?.viewportY), length)
  const height = count(rows)
  if (!height || top >= length) return ''
  return read(buffer, top, Math.min(length, top + height))
}

export function lastLines(buffer: TerminalBuffer, n: number): string {
  const length = count(buffer?.length)
  const wanted = count(n)
  if (!length || !wanted) return ''
  return read(buffer, length - wanted, length)
}

export function allText(buffer: TerminalBuffer): string {
  return read(buffer, 0, count(buffer?.length))
}

const MAX_SELECT_LINES = 2000
const MAX_SELECT_BYTES = 256 * 1024

function utf8Length(text: string): number {
  return new TextEncoder().encode(text).length
}

// The longest prefix that fits, cut between code points so a multi-byte character is never halved.
function truncateUtf8(text: string, maxBytes: number): string {
  if (maxBytes <= 0) return ''
  let bytes = 0, end = 0
  for (const character of text) {
    const size = utf8Length(character)
    if (bytes + size > maxBytes) break
    bytes += size
    end += character.length
  }
  return text.slice(0, end)
}

// The sheet shows the recent buffer, capped so a huge scrollback can never be copied into the page at once. Over the byte
// cap only whole lines are dropped; the last line alone is truncated, at a code-point boundary.
export function selectionText(buffer: TerminalBuffer, maxLines = MAX_SELECT_LINES, maxBytes = MAX_SELECT_BYTES): string {
  const text = lastLines(buffer, maxLines)
  if (!text) return ''
  const cap = Number.isFinite(maxBytes) ? Math.max(0, Math.floor(maxBytes)) : Infinity
  if (utf8Length(text) <= cap) return text
  const lines = text.split('\n')
  let start = lines.length - 1
  let bytes = utf8Length(lines[start])
  while (start > 0) {
    const next = bytes + 1 + utf8Length(lines[start - 1])
    if (next > cap) break
    bytes = next
    start--
  }
  if (start === lines.length - 1 && bytes > cap) return truncateUtf8(lines[start], cap)
  return lines.slice(start).join('\n')
}
