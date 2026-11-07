import type { GafferTurn } from './api'

export type PendingGafferSend = { conversationId: string; id: string; revision: number; text?: string;relay?:boolean }
const receiptKey = (user: number, scope: string) => `reet-gaffer-pending:${user}:${scope}`
const idShape = /^[a-f0-9]{32}$/

// Only receipt metadata survives a reload. Drafts and messages are never
// copied into browser storage, and account/scope keys cannot share a receipt.
export function readGafferReceipt(user: number, scope: string): PendingGafferSend | null {
  try {
    const raw = sessionStorage.getItem(receiptKey(user, scope))
    if (!raw || raw.length > 512) return null
    const value = JSON.parse(raw)
    if (idShape.test(value.conversationId) && idShape.test(value.id) && Number.isSafeInteger(value.revision) && value.revision > 0)
      return { conversationId: value.conversationId, id: value.id, revision: value.revision,...(value.relay===true?{relay:true}:{}) }
  } catch { /* Storage is optional; the server still owns every admitted turn. */ }
  return null
}
export function writeGafferReceipt(user: number, scope: string, pending: PendingGafferSend | null): void {
  try {
    if (pending) sessionStorage.setItem(receiptKey(user, scope), JSON.stringify({ conversationId: pending.conversationId, id: pending.id, revision: pending.revision,...(pending.relay?{relay:true}:{}) }))
    else sessionStorage.removeItem(receiptKey(user, scope))
  } catch { /* Never persist message text as a fallback. */ }
}
export function gafferMessageId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16))
  return Array.from(bytes, b => b.toString(16).padStart(2, '0')).join('')
}
export const gafferMessageBytes = (text: string): number => new TextEncoder().encode(text).length
// Enter sends the message; Shift+Enter (or Alt+Enter) adds a line. A composing
// IME candidate never sends, so choosing a word cannot post half a sentence.
export function gafferSendKey(e: Pick<KeyboardEvent, 'key' | 'shiftKey' | 'altKey' | 'isComposing'>): boolean {
  return e.key === 'Enter' && !e.isComposing && !e.shiftKey && !e.altKey
}
export function newerGafferTurn(old: GafferTurn | undefined, next: GafferTurn): GafferTurn {
  if (!old) return next
  if (old.state !== 'running' && next.state === 'running') return old
  if (old.updatedAt > next.updatedAt) return old
  return next
}
export function gafferTurnLabel(turn: GafferTurn): string {
  switch (turn.state) {
    case 'running': return 'Gaffer is thinking'
    case 'completed': return turn.cut ? 'Reply limit reached' : 'Answered'
    case 'cancelled': return 'Answer cancelled'
    case 'interrupted': return 'Answer interrupted'
    default: return 'Answer needs attention'
  }
}
export function parseGafferTurn(raw: string, conversation: string, id: string): GafferTurn {
  if (raw.length > 600_000) throw new Error('Gaffer’s reply update was too large. Reload the saved conversation.')
  const t = JSON.parse(raw)
  if (t?.id !== id || t.conversationId !== conversation || typeof t.answer !== 'string' || typeof t.prompt !== 'string' ||
      !['running','completed','cancelled','interrupted','failed'].includes(t.state) || !Number.isFinite(t.updatedAt))
    throw new Error('Gaffer’s reply update did not match this conversation. Reload the saved conversation.')
  return t as GafferTurn
}
