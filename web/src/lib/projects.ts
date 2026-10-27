import { refusalText, codeSentence } from './agentLaunch'
import type { TerminalMachine, TerminalSession } from './terminal/api'

// Pure logic for the kept runs of saved Projects (read-only in Activity and the rail's Worktrees): the run summary, the
// member words, path cleaning and the worktree sentences. No DOM, no fetch. Nothing here says an agent is signed in, understands its prompt or is making progress:
// "started" means the machine started the program in tmux in that folder.

export type ProjectMember = { host: string; dir: string; tool: string; model: string; position: number }
export type RunSummaryRef = { id: string; state: string; startedAt: number }
export type Project = {
  id: string; name: string; defaultTool: string; defaultModel: string; useWorktrees: boolean
  hasSharedPrompt: boolean
  /** Sent only by the single-Project read, for the edit form. */
  sharedPrompt?: string
  members: ProjectMember[]
  createdAt: number; updatedAt: number
  /** The number to send back as If-Match when saving or deleting. */
  version: number
  lastRun?: RunSummaryRef
}
export type RunMemberState = 'pending' | 'started' | 'failed' | 'removed'
export type RunState = 'running' | 'complete' | 'partial' | 'failed'
export type RunMember = {
  position: number; host: string; dir: string; tool: string; model: string
  state: RunMemberState; code: string; sentence: string; sessionId: string
  worktreePath: string; worktreeRepo: string; worktreeBranch: string
  /** With host and worktreeRepo, what the worktree status and remove requests take. Empty for a run without worktrees. */
  worktreeRun: string; worktreeProject: string
  /** True when the run made no worktree: the agent works in the saved folder itself. */
  sharesCheckout: boolean
  attempt: number
}
export type ProjectRun = {
  id: string; projectId: string; projectName: string; state: RunState | string; startedAt: number
  useWorktrees: boolean; worktreeProject: string; members: RunMember[]
}
export type WorktreeTarget = { agentId: string; project: string; repo: string; run: string }
export type WorktreeStatus = {
  path: string; branch: string; changed: number; unpushed: number; paths: string[]
  unreferenced: number; hidden: number; nested: boolean; incomplete: boolean
}

/** Go's path.Clean: one slash between parts, no `.` parts, and each `..` removes the part before it (never above /). */
export function cleanPath(dir: string): string {
  const s = String(dir ?? '')
  if (s === '') return '.'
  const rooted = s.startsWith('/')
  const out: string[] = []
  for (const part of s.split('/')) {
    if (part === '' || part === '.') continue
    if (part === '..') {
      if (out.length > 0 && out[out.length - 1] !== '..') out.pop()
      else if (!rooted) out.push('..')
      continue
    }
    out.push(part)
  }
  const joined = (rooted ? '/' : '') + out.join('/')
  return joined === '' ? '.' : joined
}
const open = (s: Pick<TerminalSession, 'state'>): boolean => s.state === 'creating' || s.state === 'running' || s.state === 'end_requested'

export const STARTED_MEANS = "Started means the machine started the program in tmux in that folder. It doesn't mean the agent is signed in or making progress."

/** One line for the whole run, from its members. Removed counts as started: the agent did start, and only its worktree is gone. */
export function runSummary(run: Pick<ProjectRun, 'members'>): string {
  const n = run.members.length
  const started = run.members.filter(m => m.state === 'started' || m.state === 'removed').length
  const failed = run.members.filter(m => m.state === 'failed').length
  const pending = n - started - failed
  if (pending > 0) return `Starting: ${started} of ${n} started so far${failed ? `, ${failed} failed` : ''}.`
  if (failed === 0) return n === 1 ? 'Started.' : `All ${n} started.`
  if (started === 0) return n === 1 ? "That one didn't start." : `None of the ${n} started.`
  return `${started} of ${n} started, ${failed} failed.`
}

/** One member's state in words. A failed member shows the sentence the server gave for its code. */
export function memberLine(m: Pick<RunMember, 'state' | 'sentence' | 'code'>): string {
  switch (m.state) {
    case 'pending': return 'Waiting to start.'
    case 'started': return 'Started.'
    case 'removed': return 'Started. Its worktree has been removed.'
    default: return m.sentence || refusalText(0, m.code) || 'Something changed. Check again.'
  }
}

export const branchKept = (branch: string): string => `The branch ${branch} is kept.`

/**
 * The sentence for a refused worktree status, remove or create. The server sends the sentence for each reason, with
 * git's own first line as separate detail; for git_failed and a generic refusal its message says which request it was.
 */
export function worktreeRefusalText(status: number, code?: string, message?: string): string {
  const msg = (message ?? '').trim()
  if (status === 404 && code === 'not_found') return refusalText(status, code, 'worktree')
  if (code === 'worktree_refused' && /^That machine refused to (check|remove) the worktree\.$/.test(msg)) return msg
  return refusalText(status, code, message)
}

// Same rule as the server: control and format characters and the two Unicode line separators are not plain text.
const INVISIBLE = new RegExp('[\\p{Cc}\\p{Cf}\\u2028\\u2029]', 'u')

// Field refusals that name the repository they are about ("Repository 2: …"). The server's own sentence is the useful one.
const FIELD_CODES = new Set(['invalid_dir', 'unknown_machine', 'invalid_tool', 'invalid_model', 'invalid_prompt'])

/** The sentence for a refused Project save, delete, read or run start. */
export function projectRefusalText(status: number, code?: string, message?: string): string {
  const msg = String(message ?? '').trim()
  if (status === 422 && code && FIELD_CODES.has(code) && msg !== '' && msg.length <= 300 && !INVISIBLE.test(msg)) return msg
  return refusalText(status, code, message)
}

/**
 * What stops a worktree from being removed, in the spec's sentences. Uncommitted changes always block: they are shown, and
 * the person commits, stashes or discards them in the worktree and checks again. Only unpushed commits can be removed with,
 * because they stay on the branch.
 */
export function worktreeBlockers(s: WorktreeStatus): string[] {
  const out: string[] = []
  if (s.changed > 0) out.push(codeSentence('has_changes'))
  if (s.unreferenced > 0) out.push(codeSentence('unreferenced_commits'))
  if (s.hidden > 0) out.push(codeSentence('hidden_changes'))
  if (s.nested) out.push(codeSentence('nested_repository'))
  if (s.incomplete) out.push(codeSentence('check_incomplete'))
  return out
}

/** The confirmation for a remove, with the counts that were shown (and that the remove request repeats). */
export function removeConfirmText(s: WorktreeStatus): string {
  const unpushed = s.unpushed === 0 ? 'no unpushed commits' : `${s.unpushed} unpushed ${s.unpushed === 1 ? 'commit, which stays' : 'commits, which stay'} on the branch`
  return `Remove the worktree at ${s.path}? It has no uncommitted changes and ${unpushed}. Ignored files in it, such as .env or node_modules, are deleted with the folder. ${branchKept(s.branch)}`
}

/**
 * Whether a run member's worktree may be offered for removal: the member started and made a worktree, and its session has
 * ended (a session the list no longer has has ended too). While the sessions are not known (not read yet, or the last read
 * failed) it is never offered. reet never checks what a running agent is doing in the folder.
 */
export function removeOffer(m: RunMember, sessions: Pick<TerminalSession, 'id' | 'state'>[] | undefined, readFailed = false): { offer: boolean; reason: string } {
  if (m.state !== 'started' || !m.worktreeRun || !m.worktreeRepo || !m.worktreeProject) return { offer: false, reason: '' }
  // Unknown is not ended: removing the folder under a running agent would delete its work and its ignored files.
  if (readFailed) return { offer: false, reason: "Couldn't check whether its session has ended, so its worktree can't be removed from here just now. Reload the page to try again." }
  if (!sessions) return { offer: false, reason: 'Checking whether its session has ended…' }
  const session = sessions.find(s => s.id === m.sessionId)
  if (session && open(session)) return { offer: false, reason: 'End its session before removing its worktree.' }
  return { offer: true, reason: '' }
}

/** True while any member is still waiting to start: the run view keeps reading the run. */
export const runPending = (run: Pick<ProjectRun, 'members'>): boolean => run.members.some(m => m.state === 'pending')

/**
 * A machine's name for the words: its host once the machines have been read, its id until then (the names are only a
 * label, so a slow or failed machines read never holds a page back), and a plain phrase for one no longer enrolled.
 */
export function hostLabel(agentId: string, machines: Pick<TerminalMachine, 'agentId' | 'host'>[] | undefined): string {
  if (!machines) return agentId
  return machines.find(m => m.agentId === agentId)?.host ?? 'A machine no longer on this account'
}
