import type { FolderProject, TerminalSession } from './terminal/api'
import { groupSessions, sessionOwners, type Group, type GroupBy } from './home'
import { toolName } from './agentLaunch'

// Pure logic for the Sessions page (/sessions): search, filters, groups and the worktree a session's menu can check and
// remove. No DOM, no fetch. Every session comes from the account-wide listing; nothing here is stored.

export type MachineName = { agentId: string; host: string }
/** The state filter's buckets. "needs" is the attention signal, which no record gives yet: it matches only the sessions
 *  the caller names (none today). */
export type StateFilter = 'needs' | 'running' | 'finished' | 'ended'
/** A session with no agent (a plain shell) is filtered as this. */
export const SHELL = 'shell'
export type SessionFilter = { state?: StateFilter | ''; agent?: string; machine?: string; yoloOnly?: boolean; worktreeOnly?: boolean }
export type SessionSort = 'recent' | 'name' | 'custom'
export type SessionGroup = Group & {
  /** The project's machine, shown beside a project group's name. */
  machine?: string
  /** Is the group a folder project's (drawn with the project's mark)? */
  project?: FolderProject
  /** Machines with sessions in this group that are not reporting now, by name: "studio is offline". */
  offline: string[]
}

const hostOf = (agentId: string, machines: MachineName[]): string => machines.find(m => m.agentId === agentId)?.host || agentId

/** Which state filter a session falls in: live ones (starting, running, ending) are running; one that exited by itself or
 *  whose connection was lost is finished; one the person ended is ended. */
export function stateBucket(s: Pick<TerminalSession, 'state'>): Exclude<StateFilter, 'needs'> {
  if (s.state === 'creating' || s.state === 'running' || s.state === 'end_requested') return 'running'
  if (s.state === 'ended') return 'ended'
  return 'finished'
}

/** The words a search looks in: title, agent (its name and id), model, branch, project and machine. */
function haystack(s: TerminalSession, projects: Pick<FolderProject, 'id' | 'name'>[], machines: MachineName[]): string {
  const project = s.projectId ? projects.find(p => p.id === s.projectId)?.name : ''
  return [s.title, s.tool ? toolName(s.tool) : '', s.tool, s.model, s.worktreeBranch, project, hostOf(s.agentId, machines)].filter(Boolean).join('\n').toLowerCase()
}

/** Sessions matching every word of the query in any of their words; an empty query matches all. */
export function searchSessions(sessions: TerminalSession[], projects: Pick<FolderProject, 'id' | 'name'>[], machines: MachineName[], q: string): TerminalSession[] {
  const terms = q.trim().toLowerCase().split(/\s+/).filter(Boolean)
  if (terms.length === 0) return sessions
  return sessions.filter(s => { const text = haystack(s, projects, machines); return terms.every(t => text.includes(t)) })
}

/** Every filter that is set must hold. `needing` is the ids the attention signal names (none today). */
export function filterSessions(sessions: TerminalSession[], f: SessionFilter, needing: ReadonlySet<string> = new Set()): TerminalSession[] {
  return sessions.filter(s =>
    (!f.state || (f.state === 'needs' ? needing.has(s.id) : stateBucket(s) === f.state)) &&
    (!f.agent || (s.tool || SHELL) === f.agent) &&
    (!f.machine || s.agentId === f.machine) &&
    (!f.yoloOnly || s.yolo === true) &&
    (!f.worktreeOnly || !!s.worktreePath))
}

/** How many filters are on, for the Filters summary. */
export const filtersOn = (f: SessionFilter): number => [f.state, f.agent, f.machine, f.yoloOnly, f.worktreeOnly].filter(Boolean).length

/** The agents the listing has, for the Agent filter: each tool id once with its name, plain shells last. */
export function agentChoicesOf(sessions: TerminalSession[]): { value: string; label: string }[] {
  const tools = [...new Set(sessions.map(s => s.tool).filter((t): t is string => !!t))].sort()
  const out = tools.map(t => ({ value: t, label: toolName(t) }))
  if (sessions.some(s => !s.tool)) out.push({ value: SHELL, label: 'Shell' })
  return out
}

/** Order within a group: newest change first, by title, or the custom order saved in this browser (unknown ids last). */
export function sortSessions(sessions: TerminalSession[], sort: SessionSort, order: readonly string[] = []): TerminalSession[] {
  const rank = new Map(order.map((id, i) => [id, i]))
  return [...sessions].sort((a, b) => {
    if (sort === 'name') return a.title.localeCompare(b.title) || a.id.localeCompare(b.id)
    if (sort === 'custom') return (rank.get(a.id) ?? Number.MAX_SAFE_INTEGER) - (rank.get(b.id) ?? Number.MAX_SAFE_INTEGER) || b.updatedAt - a.updatedAt || a.id.localeCompare(b.id)
    return b.updatedAt - a.updatedAt || a.id.localeCompare(b.id)
  })
}

/**
 * The page's groups, with Home's grouping: by project (sessions without one under "Other on <machine>"), machine or state.
 * Each group names the machines among its sessions that are not reporting (`offline`, by agent id).
 */
export function sessionGroups(sessions: TerminalSession[], projects: FolderProject[], machines: MachineName[], groupBy: GroupBy, offline: ReadonlySet<string>, sort: SessionSort = 'recent', order: readonly string[] = []): SessionGroup[] {
  const owners = sessionOwners(sessions, projects)
  return groupSessions(sessions, owners, machines, groupBy).map(g => {
    const project = g.key.startsWith('project:') ? projects.find(p => `project:${p.id}` === g.key) : undefined
    const label = groupBy === 'project' && g.key.startsWith('machine:') ? `Other on ${hostOf(g.key.slice('machine:'.length), machines)}` : g.label
    const hosts = [...new Set(g.sessions.filter(s => offline.has(s.agentId)).map(s => s.agentId))].map(id => hostOf(id, machines))
    return { ...g, label, project, machine: project ? project.machine || 'machine not enrolled' : undefined, offline: hosts, sessions: sortSessions(g.sessions, sort, order) }
  })
}

/** The sentence on a group whose machine is not reporting. */
export const offlineSentence = (host: string): string => `${host} is offline`

/** `agent · model · branch`: the agent's name (a plain shell says Shell), then the model and the worktree's branch when set. */
export function rowMeta(s: Pick<TerminalSession, 'tool' | 'model' | 'worktreeBranch'>): string {
  return [s.tool ? toolName(s.tool) : 'Shell', s.model, s.worktreeBranch].filter(Boolean).join(' · ')
}

/**
 * The run's worktree a session works in, as the worktree status and remove requests name it. The machine makes it at
 * `~/.reet/worktrees/<project>/<repo>/<run>` on the branch `reet/<project>-<run>`, so the path's last three parts are read
 * back only when the repository part is the one the machine reported and the branch (when listed) agrees. Anything else
 * offers nothing: reet never guesses a worktree.
 */
export function worktreeTargetOf(s: Pick<TerminalSession, 'agentId' | 'worktreePath' | 'worktreeRepo' | 'worktreeBranch'>): { agentId: string; project: string; repo: string; run: string } | null {
  if (!s.worktreePath || !s.worktreeRepo) return null
  const parts = s.worktreePath.split('/').filter(Boolean)
  if (parts.length < 5) return null
  const [run, repo, project, folder, dot] = parts.slice(-5).reverse()
  if (folder !== 'worktrees' || dot !== '.reet' || repo !== s.worktreeRepo || !run || !project) return null
  if (s.worktreeBranch && s.worktreeBranch !== `reet/${project}-${run}`) return null
  return { agentId: s.agentId, project, repo, run }
}
