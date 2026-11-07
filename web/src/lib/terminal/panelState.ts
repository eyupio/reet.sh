// Pure logic for the session panel's cleaner view: which sessions need the person, which sections are folded, and how many
// projects show before "Show more". Kept free of Svelte so node:test can run it.
import type { TerminalSession } from './api'
import { attentionRuleOf, isBlocked } from '../attention'

export type NeedKind = 'approval' | 'question'

/** How many projects the panel lists before the rest fold under "Show N more". */
export const PROJECT_CAP = 5

/**
 * What a session is waiting on, or null. This is the shared attention rule for a waiting card (attention.ts `isBlocked`):
 * the session is waiting and either changed since the person last saw it or still holds an approval or question, so a chat
 * the person opened but did not answer still reads as needing them. An approval is named first when an approval and a
 * question both wait, as the bell does.
 */
export function needsInput(session: Pick<TerminalSession, 'attention' | 'changedSinceSeen' | 'pendingPermission' | 'pendingQuestion'>): NeedKind | null {
  if (!isBlocked(session)) return null
  if (session.pendingPermission?.permissionId) return 'approval'
  if (session.pendingQuestion?.questionId) return 'question'
  return 'approval'
}

/**
 * The row's status line: what the session is waiting for, then what the agent last did, else the lifecycle state. The
 * agent's turn finishing leaves the session running, so attention says "finished", or "quiet" once the person has seen
 * it, where the state alone still reads "running".
 */
export function statusLine(session: Pick<TerminalSession, 'state' | 'attention' | 'changedSinceSeen' | 'pendingPermission' | 'pendingQuestion'>): string {
  const need = needsInput(session)
  if (need) return `Needs you · ${need}`
  if (session.attention === 'finished') return 'Finished'
  if (session.attention === 'failed') return session.state === 'lost' ? 'Connection lost' : 'Failed'
  if (session.attention === 'quiet') return 'Quiet'
  return session.state.replaceAll('_', ' ')
}

/** The dot beside a row and on the mini rail: what needs you, then the agent's attention, else the lifecycle state. A
 *  quiet chat's turn has stopped even though its session is still open, so it is marked finished rather than running. */
export function sessionMark(session: Pick<TerminalSession, 'state' | 'attention' | 'attentionSource'>): 'needs' | 'running' | 'starting' | 'ending' | 'lost' | 'failed' | 'finished' {
  if (session.attention === 'finished') return 'finished'
  if (session.attention === 'failed') return session.state === 'lost' ? 'lost' : 'failed'
  if (session.attention === 'quiet' && session.attentionSource === 'chat') return 'finished'
  switch (session.state) {
    case 'running': return 'running'
    case 'creating': return 'starting'
    case 'end_requested': return 'ending'
    case 'lost': return 'lost'
    default: return 'finished'
  }
}

/** How many of the sessions need the person under the shared attention rule (waiting, or an unseen failed or finished
 *  turn within a day): the badge on a project row or a section header. */
export function needCount(sessions: readonly Pick<TerminalSession, 'attention' | 'changedSinceSeen' | 'updatedAt' | 'pendingPermission' | 'pendingQuestion'>[], now: number = Date.now()): number {
  let n = 0
  for (const s of sessions) if (attentionRuleOf(s, now) !== null) n++
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

/** The group key of a session that belongs to no project. */
export const NO_PROJECT = '__no_project__'

/** The project-tree group a session sits under: its project, else the "No project" group. */
export function projectGroupKey(session: { projectId?: string | null }): string {
  return session.projectId || NO_PROJECT
}

/** Project groups in the order the projects are listed; a project no longer listed follows, and "No project" is last. */
export function orderProjectGroups<G extends { key: string }>(groups: readonly G[], projectIds: readonly string[]): G[] {
  const rank = (key: string) => (key === NO_PROJECT ? Number.MAX_SAFE_INTEGER : projectIds.includes(key) ? projectIds.indexOf(key) : projectIds.length)
  return [...groups].sort((a, b) => rank(a.key) - rank(b.key))
}
