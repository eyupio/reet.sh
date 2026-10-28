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
