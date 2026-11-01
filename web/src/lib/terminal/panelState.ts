// Pure logic for the session panel's cleaner view: which sessions need the person, which sections are folded, and how many
// projects show before "Show more". Kept free of Svelte so node:test can run it.
import type { TerminalSession } from './api'

export type NeedKind = 'approval' | 'question'

/** How many projects the panel lists before the rest fold under "Show N more". */
export const PROJECT_CAP = 5

/**
 * What a session is waiting on, or null. This is the bell's rule for a waiting card (home.ts `needsYou`): the session is
 * waiting and has changed since the person last saw it, so opening or answering it clears both together. An approval is
 * named first when an approval and a question both wait, as the bell does.
 */
export function needsInput(session: Pick<TerminalSession, 'attention' | 'changedSinceSeen' | 'pendingPermission' | 'pendingQuestion'>): NeedKind | null {
  if (session.attention !== 'waiting' || !session.changedSinceSeen) return null
  if (session.pendingPermission?.permissionId) return 'approval'
  if (session.pendingQuestion?.questionId) return 'question'
  return 'approval'
}

/** The row's status line: the state, or what the session is waiting for. */
export function statusLine(session: Pick<TerminalSession, 'state' | 'attention' | 'changedSinceSeen' | 'pendingPermission' | 'pendingQuestion'>): string {
  const need = needsInput(session)
  if (need) return `Needs you · ${need}`
  return session.state.replaceAll('_', ' ')
}

/** How many of the sessions need the person: the badge on a project row or a section header. */
export function needCount(sessions: readonly Pick<TerminalSession, 'attention' | 'changedSinceSeen' | 'pendingPermission' | 'pendingQuestion'>[]): number {
  let n = 0
  for (const s of sessions) if (needsInput(s)) n++
  return n
}

/** Section keys the person folded, as saved in the browser: only strings survive, deduplicated and bounded. */
export function sanitiseCollapsed(saved: unknown): string[] {
  if (!Array.isArray(saved)) return []
  return [...new Set(saved.filter((v): v is string => typeof v === 'string' && v.length > 0 && v.length < 200))].slice(0, 500)
}

/** Fold or unfold one section; returns a new list. */
export function toggleCollapsed(collapsed: readonly string[], key: string): string[] {
  return collapsed.includes(key) ? collapsed.filter(k => k !== key) : [...collapsed, key]
}

export type ProjectLike = { id: string }

/**
 * The projects the panel lists. Projects with live sessions, a session waiting on the person and the current project always
 * show; the rest fill the cap in the order given, and the remainder fold under "Show N more". `expanded` lists them all.
 */
export function visibleProjects<P extends ProjectLike>(
  projects: readonly P[],
  running: ReadonlyMap<string, number>,
  waiting: ReadonlySet<string>,
  current: string,
  expanded: boolean,
  cap: number = PROJECT_CAP,
): { shown: P[]; hidden: number } {
  if (expanded || projects.length <= cap) return { shown: [...projects], hidden: 0 }
  const must = (p: P) => (running.get(p.id) ?? 0) > 0 || waiting.has(p.id) || p.id === current
  let room = cap - projects.filter(must).length
  const shown = projects.filter(p => {
    if (must(p)) return true
    if (room > 0) { room--; return true }
    return false
  })
  return { shown, hidden: projects.length - shown.length }
}
