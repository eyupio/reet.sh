import type { TerminalSession } from './terminal/api'

/** An unseen finished or failed session needs the person this long after its last state change, then stops. */
export const ATTENTION_WINDOW_MS = 24 * 60 * 60_000

/** The agent holds a typed approval or question open for the person. */
export function hasPending(s: Pick<TerminalSession, 'pendingPermission' | 'pendingQuestion'>): boolean {
  return !!s.pendingPermission?.permissionId || !!s.pendingQuestion?.questionId
}

/** Waiting on the person: newly waiting and unseen, or still holding an approval or question. */
export function isBlocked(s: Pick<TerminalSession, 'attention' | 'changedSinceSeen' | 'pendingPermission' | 'pendingQuestion'>): boolean {
  return s.attention === 'waiting' && (!!s.changedSinceSeen || hasPending(s))
}

export type AttentionRule = 'waiting' | 'failed' | 'finished'

/** The one answer to "does this session need the person", and why; null when it does not. */
export function attentionRuleOf(s: Pick<TerminalSession, 'attention' | 'changedSinceSeen' | 'updatedAt' | 'pendingPermission' | 'pendingQuestion'>, now: number): AttentionRule | null {
  if (isBlocked(s)) return 'waiting'
  if (s.changedSinceSeen && now - s.updatedAt <= ATTENTION_WINDOW_MS) {
    if (s.attention === 'failed') return 'failed'
    if (s.attention === 'finished') return 'finished'
  }
  return null
}
