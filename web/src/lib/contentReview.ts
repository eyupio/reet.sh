// Shared connection identity and readable errors. Skill plan, pin and adoption helpers were retired.
const GENERIC_ERROR = 'Something went wrong. Try again.'
const NETWORK_ERROR = "Couldn't reach reet. Check your connection and try again."
// The only errors whose message is the server's own words: what req throws.
const SERVER_VOICED = new Set(['Error', 'ApiError', 'UnauthorizedError'])
// What fetch throws when no response arrives (Chromium, Firefox and Safari word it differently). Any other
// TypeError is a fault in our own code and must not be blamed on the connection.
const FETCH_FAILED = /^(Failed to fetch|NetworkError|Load failed|Network request failed)/i


const foldAscii = (s: string): string => s.replace(/[A-Z]/g, c => String.fromCharCode(c.charCodeAt(0) + 32))
export const sameHost = (a: string, b: string): boolean => foldAscii(a) === foldAscii(b)

export const SIGNED_OUT = "You've been signed out. Sign in again to continue."
export const isSignedOut = (e: unknown): boolean => typeof e === 'object' && e !== null && (e as { name?: unknown }).name === 'UnauthorizedError'

/** The server's own words when it gave some, otherwise a plain sentence. Never a stack, a status line or browser jargon. */
export function describeError(e: unknown): string {
  const name = typeof e === 'object' && e !== null && typeof (e as { name?: unknown }).name === 'string' ? (e as { name: string }).name : ''
  if (name === 'TypeError') return FETCH_FAILED.test(String((e as { message?: unknown }).message ?? '')) ? NETWORK_ERROR : GENERIC_ERROR
  // Only what req throws carries the server's words. Any other named error (SyntaxError from a page that is not JSON,
  // ReferenceError, DOMException, ...) is the browser's or our own, and is not for the person to read.
  if (name !== '' && !SERVER_VOICED.has(name)) return GENERIC_ERROR
  let raw: unknown = e
  if (typeof e === 'object' && e !== null) {
    const { error, message } = e as { error?: unknown; message?: unknown }
    raw = typeof error === 'string' && error.trim() !== '' ? error : message
  }
  if (typeof raw !== 'string') return GENERIC_ERROR
  const line = raw.split(/\r?\n/).map(l => l.trim()).find(l => l !== '') ?? ''
  if (line === '' || line.length > 300) return GENERIC_ERROR
  // The client's own "404 Not Found" when a refusal carried no text, a stringified error class, or a stack frame.
  if (/^\d{3}(\s|$)/.test(line) || /^[A-Za-z]*Error:/.test(line) || /(^|\s)at\s+\S.*:\d+:\d+\)?$/.test(line)) return GENERIC_ERROR
  return line
}
