import { structuredAgent, terminalClosed, type Capabilities, type FolderProject, type LaunchRequest, type TerminalMachine, type TerminalSession } from './terminal/api'
import { groupSessions, sessionOwners, type Group, type GroupBy } from './home'
import { toolName } from './agentLaunch'
import { CATALOGUE } from './catalogue'
import { projectColour, type ProjectColour } from './projectColour'

// Pure logic for the Sessions page (/sessions): search, filters, groups and the worktree a session's menu can check and
// remove. No DOM, no fetch. Every session comes from the account-wide listing; nothing here is stored.

export type MachineName = { agentId: string; host: string }
/** The state filter's buckets. "needs" is the attention signal, which no record gives yet: it matches only the sessions
 *  the caller names (none today). */
export type StateFilter = 'needs' | 'running' | 'finished' | 'ended'
/** A session with no agent (a plain shell) is filtered as this. */
export const SHELL = 'shell'
/** `colour` is a palette id, or 'none' for sessions with no colour. */
export type SessionFilter = { state?: StateFilter | ''; agent?: string; machine?: string; yoloOnly?: boolean; worktreeOnly?: boolean; colour?: string }
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

/** A session's colour: the one chosen for it in the sessions panel (kept in this browser), else its project's, else none. */
export function sessionColourOf(s: Pick<TerminalSession, 'id' | 'projectId'>, saved: Record<string, string>, projects: Pick<FolderProject, 'id' | 'colour'>[]): ProjectColour {
  return projectColour(saved[s.id]) || projectColour(s.projectId ? projects.find(p => p.id === s.projectId)?.colour : '')
}

/** Every filter that is set must hold. `needing` is the ids the attention signal names (none today); `colourOf` gives a
 *  session's colour id ('' for none). */
export function filterSessions(sessions: TerminalSession[], f: SessionFilter, needing: ReadonlySet<string> = new Set(), colourOf: (s: TerminalSession) => string = () => ''): TerminalSession[] {
  return sessions.filter(s =>
    (!f.colour || (colourOf(s) || 'none') === f.colour) &&
    (!f.state || (f.state === 'needs' ? needing.has(s.id) : stateBucket(s) === f.state)) &&
    (!f.agent || (s.tool || SHELL) === f.agent) &&
    (!f.machine || s.agentId === f.machine) &&
    (!f.yoloOnly || s.yolo === true) &&
    (!f.worktreeOnly || !!s.worktreePath))
}

/** How many filters are on, for the Filters summary. */
export const filtersOn = (f: SessionFilter): number => [f.state, f.agent, f.machine, f.yoloOnly, f.worktreeOnly, f.colour].filter(Boolean).length

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

/** The launch that continues a closed session in the folder it ran in, with the same agent and model. The session's own
 *  dir is used, not the project's current folder: that may have changed since the session started. A project is named
 *  again only when its current folder is still the session's. */
export function continueLaunch(s: Pick<TerminalSession, 'state' | 'tool' | 'model' | 'effort' | 'dir' | 'projectId'>, projects: Pick<FolderProject, 'id' | 'dir'>[]): LaunchRequest | null {
  if (!s.tool || !terminalClosed(s.state)) return null
  const dir = s.dir || ''
  if (!dir) return null
  const project = s.projectId ? projects.find(p => p.id === s.projectId) : undefined
  const launch: LaunchRequest = { tool: s.tool, dir, resume: true }
  if (s.model) launch.model = s.model
  if (s.effort === 'low' || s.effort === 'medium' || s.effort === 'high') launch.effort = s.effort
  if (project && project.dir === dir) launch.projectId = project.id
  return launch
}

/** A closed structured chat reopens as a new row that asks the machine to load OpenCode's stored history. The old row
 *  stays in history. */
export function continueChatLaunch(s: Pick<TerminalSession, 'state' | 'tool' | 'structured' | 'acpSessionId' | 'dir' | 'projectId' | 'model'>): LaunchRequest | null {
  if (!s.structured || !s.acpSessionId || !terminalClosed(s.state) || s.tool !== 'opencode') return null
  const launch: LaunchRequest = { tool: 'opencode', structured: true, trust: true, resumeAcp: s.acpSessionId }
  if (s.model) launch.model = s.model
  if (s.dir) launch.dir = s.dir
  if (s.projectId) launch.projectId = s.projectId
  return launch
}

/** A chat can be reopened only on a machine that still advertises acp. */
export function canContinueChat(s: Pick<TerminalSession, 'state' | 'tool' | 'structured' | 'acpSessionId' | 'dir' | 'projectId' | 'model'>, machine: Pick<TerminalMachine, 'capabilities'> | undefined): boolean {
  return continueChatLaunch(s) !== null && structuredAgent(machine?.capabilities, s.tool)?.history === true
}

/** Whether a closed session can be continued: a reviewed resume command for its agent, a machine that negotiated
 *  resume-v1, and the folder the session ran in. */
export function canContinue(s: Pick<TerminalSession, 'state' | 'tool' | 'dir' | 'projectId'>, machine: Pick<TerminalMachine, 'capabilities'> | undefined, projects: Pick<FolderProject, 'id' | 'dir'>[]): boolean {
  return continueLaunch(s, projects) !== null && machine?.capabilities?.resume === true && CATALOGUE.find(agent => agent.id === s.tool)?.resume === true
}

// The session screen (/sessions?agent=…&session=…). Its tabs are the sessions opened in this browser tab since it loaded
// (R20): held in memory only, never stored, at most eight in the order they were opened. Closing a tab closes the view,
// never the session.
export const MAX_TABS = 8

/** The tabs after opening `id`: added at the end (the oldest dropped past eight), or left in place when already open. */
export function openTab(tabs: readonly string[], id: string): string[] {
  if (!id || tabs.includes(id)) return [...tabs]
  return [...tabs, id].slice(-MAX_TABS)
}

/** The tabs after closing `id`, and the session to show next: the current one, or (when the current tab closed) the tab now
 *  in its place, else the one before; '' when none is left. */
export function closeTab(tabs: readonly string[], id: string, current: string): { tabs: string[]; next: string } {
  const at = tabs.indexOf(id), rest = tabs.filter(t => t !== id)
  if (id !== current) return { tabs: rest, next: current }
  return { tabs: rest, next: rest[Math.min(Math.max(at, 0), rest.length - 1)] ?? '' }
}

const MINUTE = 60_000, HOUR = 60 * MINUTE, DAY = 24 * HOUR
const count = (n: number, one: string) => `${n} ${one}${n === 1 ? '' : 's'}`

/** "started 41 minutes ago", from the time since the session was created. */
export function startedAgo(ms: number): string {
  if (ms < MINUTE) return 'started just now'
  if (ms < HOUR) return `started ${count(Math.floor(ms / MINUTE), 'minute')} ago`
  if (ms < DAY) return `started ${count(Math.floor(ms / HOUR), 'hour')} ago`
  return `started ${count(Math.floor(ms / DAY), 'day')} ago`
}

/** The header's line under the title: `agent · model · started …` (a plain shell says Shell). */
export function headerMeta(s: Pick<TerminalSession, 'tool' | 'model'>, createdAt: number, now: number): string {
  return [s.tool ? toolName(s.tool) : 'Shell', s.model, startedAgo(Math.max(0, now - createdAt))].filter(Boolean).join(' · ')
}

/** Decisions in this session. A terminal agent asks in its own terminal; reet sees none of it, so it lists none. */
export function decisionsLine(s: Pick<TerminalSession, 'yolo' | 'structured'>): string {
  if (s.yolo) return s.structured ? 'YOLO: reet answers yes, this once, to each request.' : 'YOLO: the agent asked not to be prompted; the launch is audited.'
  return s.structured ? 'Asked in the chat: Allow once or Deny.' : 'Asked in the terminal.'
}

/**
 * The session header's reet TUI | OpenCode TUI switch: where the inactive button goes (`to`) and whether it can go now.
 * The first reason that holds is the one given: an agent with no reet TUI, a session that is not running, a session started
 * in the OpenCode TUI (no OpenCode session id for the reet TUI to load), a machine that has not negotiated session-mode-v1
 * (or has no chat history for this agent), a chat with no saved conversation yet (nothing for `opencode --session` to open),
 * then a busy chat. `busy` (a running turn, a waiting approval or question) only
 * holds a chat back: the OpenCode TUI's own state is not watched, as the controller does not watch it either.
 */
export function switchModeState(session: TerminalSession, caps: Capabilities | undefined, busy: boolean): { to: 'terminal' | 'chat'; enabled: boolean; why: string } {
  const to = session.structured ? 'terminal' : 'chat'
  const no = (why: string) => ({ to, enabled: false, why } as const)
  if (session.tool !== 'opencode') return no('The reet TUI is for a structured OpenCode session.')
  if (session.state !== 'running') return no('Only a running session can switch.')
  if (to === 'chat' && !session.acpSessionId) return no('This session started in the OpenCode TUI, so the reet TUI cannot pick up its conversation.')
  if (caps?.sessionMode !== true || structuredAgent(caps, session.tool)?.history !== true) return no('Update reet on this machine to switch between the reet TUI and the OpenCode TUI.')
  // `opencode --session` needs the id; the controller refuses with this same sentence (session_mode_no_conversation), and
  // as there, an old machine is named first.
  if (to === 'terminal' && !session.acpSessionId) return no('This session has no saved OpenCode conversation yet, so it cannot move to the OpenCode TUI.')
  if (session.structured && busy) return no('Wait for the agent to finish or answer what it is asking.')
  return { to, enabled: true, why: '' }
}

/** The context panel's changed files (R19): the first twenty paths the worktree status lists, the total, and how many more. */
export function changedList(status: { changed: number; paths: string[] }): { shown: string[]; total: number; more: number } {
  const shown = (status.paths ?? []).slice(0, 20)
  return { shown, total: status.changed, more: Math.max(0, status.changed - shown.length) }
}

/** Tab from a closed menu: focus goes on from the control that opened it (the next tab stop after it, or the opener itself
 *  for Shift+Tab), never to the page's start. The menu is drawn after the page, so the browser's own Tab would leave it there. */
export function tabFromMenu(opener: HTMLElement | null | undefined, backwards: boolean): void {
  if (!opener?.isConnected) return
  if (backwards) { opener.focus(); return }
  const stops = [...document.querySelectorAll<HTMLElement>('a[href], button:not(:disabled), input:not(:disabled):not([type=hidden]), select:not(:disabled), textarea:not(:disabled), summary, [tabindex]:not([tabindex="-1"])')]
    .filter(el => el.getClientRects().length && !el.closest('[inert], [role=menu]') && el.tabIndex >= 0)
  const after = stops.find(el => opener.compareDocumentPosition(el) & Node.DOCUMENT_POSITION_FOLLOWING && !opener.contains(el))
  ;(after ?? opener).focus()
}
