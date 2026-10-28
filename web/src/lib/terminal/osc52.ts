export const MAX_OSC52_BYTES = 1 << 20

export type Osc52Result =
  | { kind: 'write'; text: string; bytes: number }
  | { kind: 'ignore'; reason: 'query' | 'empty' | 'large' | 'invalid' }

const BASE64_RE = /^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/
const BASE64_VALUES = (() => {
  const values = new Int16Array(128).fill(-1)
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/'
  for (let index = 0; index < alphabet.length; index++) values[alphabet.charCodeAt(index)] = index
  return values
})()

function value(char: string): number {
  const code = char.charCodeAt(0)
  return code < 128 ? BASE64_VALUES[code] : -1
}

function decodeBase64(payload: string): Uint8Array | null {
  if (!BASE64_RE.test(payload)) return null
  const padding = payload.endsWith('==') ? 2 : payload.endsWith('=') ? 1 : 0
  const bytes = new Uint8Array((payload.length / 4) * 3 - padding)
  let written = 0
  for (let index = 0; index < payload.length; index += 4) {
    const a = value(payload[index]), b = value(payload[index + 1])
    const c = payload[index + 2] === '=' ? 0 : value(payload[index + 2])
    const d = payload[index + 3] === '=' ? 0 : value(payload[index + 3])
    if (a < 0 || b < 0 || c < 0 || d < 0) return null
    const triple = (a << 18) | (b << 12) | (c << 6) | d
    bytes[written++] = (triple >> 16) & 255
    if (written < bytes.length) bytes[written++] = (triple >> 8) & 255
    if (written < bytes.length) bytes[written++] = triple & 255
  }
  return bytes
}

export function parseOsc52(data: string, limit = MAX_OSC52_BYTES): Osc52Result {
  if (typeof data !== 'string') return { kind: 'ignore', reason: 'invalid' }
  if (data === '?') return { kind: 'ignore', reason: 'query' }
  const separator = data.indexOf(';')
  if (separator === -1 || data.slice(0, separator) !== 'c') return { kind: 'ignore', reason: 'invalid' }
  const payload = data.slice(separator + 1)
  if (payload.includes('?')) return { kind: 'ignore', reason: 'query' }
  if (!payload) return { kind: 'ignore', reason: 'empty' }
  const bytes = decodeBase64(payload)
  if (!bytes) return { kind: 'ignore', reason: 'invalid' }
  if (!bytes.length) return { kind: 'ignore', reason: 'empty' }
  const cap = Number.isFinite(limit) ? Math.max(0, Math.floor(limit)) : MAX_OSC52_BYTES
  if (bytes.length > cap) return { kind: 'ignore', reason: 'large' }
  return { kind: 'write', text: new TextDecoder('utf-8').decode(bytes), bytes: bytes.length }
}

export function formatCopySize(bytes: number): string {
  const count = Number.isFinite(bytes) ? Math.max(0, Math.floor(bytes)) : 0
  if (count < 1024) return `${count} B`
  if (count < MAX_OSC52_BYTES) return `${(count / 1024).toFixed(1)} KB`
  return `${(count / MAX_OSC52_BYTES).toFixed(1)} MB`
}
