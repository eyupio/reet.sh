import { toolName, validateDir, validateModel, refusalText, codeSentence, TOOLS } from './agentLaunch'
import { readiness } from './terminal/api'
import type { TerminalMachine, TerminalSession } from './terminal/api'

// Pure logic for the Projects pages: names and members, the confirmation-sheet plan, the run summary and the worktree
// sentences. No DOM, no fetch. Nothing here says an agent is signed in, understands its prompt or is making progress:
// "started" means the machine started the program in tmux in that folder.

export const MAX_PROJECT_NAME = 80
export const MAX_MEMBERS = 12
export const MAX_PROJECTS = 50
export const MAX_SESSIONS_PER_MACHINE = 8
export const MAX_SESSIONS_PER_ACCOUNT = 32

export type ProjectMember = { host: string; dir: string; tool: string; model: string; position: number }
export type MemberInput = Pick<ProjectMember, 'host' | 'dir' | 'tool' | 'model'>
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
/** What a create or update sends. On an update an absent field keeps its saved value and "" clears a text field. */
export type ProjectInput = {
  name?: string; defaultTool?: string; defaultModel?: string; sharedPrompt?: string; useWorktrees?: boolean
  members?: MemberInput[]
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

// Same rule as the server: control and format characters and the two Unicode line separators are not plain text.
const INVISIBLE = new RegExp('[\\p{Cc}\\p{Cf}\\u2028\\u2029]', 'u')
const NAME_SENTENCE = `Give the Project a name of 1 to ${MAX_PROJECT_NAME} characters, without line breaks or control characters.`

/** A name is 1 to 80 characters after trimming, plain text. `others` are the account's other Project names; they compare ignoring case. */
export function validateProjectName(name: string, others: string[] = []): string | null {
  const s = String(name ?? '').trim()
  if (s === '' || Array.from(s).length > MAX_PROJECT_NAME || INVISIBLE.test(s)) return NAME_SENTENCE
  const lower = s.toLowerCase()
  if (others.some(o => String(o).trim().toLowerCase() === lower)) return 'Another Project on this account already has that name.'
  return null
}

/**
 * One repository of a Project: an enrolled machine, an absolute folder (checked as the Start an agent folder is), and
 * optional agent and model overrides. `snapshot` is accepted for symmetry with the folder choices but does not restrict
 * the folder: the machine decides whether a folder it did not report exists.
 */
export function validateMember(member: MemberInput, machines: TerminalMachine[], _snapshot?: unknown): string | null {
  const host = String(member?.host ?? '')
  if (host === '') return 'Choose a machine.'
  if (!machines.some(m => m.agentId === host)) return "That machine isn't enrolled on this account."
  if (String(member.dir ?? '') === '') return 'Choose a project folder.'
  const dirProblem = validateDir(member.dir)
  if (dirProblem) return dirProblem
  if (member.tool && !TOOLS.some(t => t.id === member.tool)) return 'Choose one of the agents reet can start.'
  return validateModel(member.model ?? '')
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
// The server compares hosts trimmed and in lower case, and folders cleaned.
const repositoryKey = (m: Pick<MemberInput, 'host' | 'dir'>): string => `${String(m.host ?? '').trim().toLowerCase()}\u0000${cleanPath(m.dir)}`

/** The list as a whole: at most 12 repositories, each machine and folder once. */
export function validateMembers(members: MemberInput[]): string | null {
  if (members.length > MAX_MEMBERS) return `A Project can have at most ${MAX_MEMBERS} repositories.`
  const seen = new Set<string>()
  for (const m of members) {
    const key = repositoryKey(m)
    if (seen.has(key)) return 'Each machine and folder can be in a Project only once.'
    seen.add(key)
  }
  return null
}

export type SkipCode = 'not_enrolled' | 'not_ready' | 'no_launch' | 'tool_not_installed' | 'no_worktree' | 'machine_session_limit' | 'account_session_limit'
export type PlannedMember = {
  position: number; member: ProjectMember; host: string; machine: string; dir: string
  tool: string; toolName: string; model: string; worktree: boolean
}
export type SkippedMember = { position: number; member: ProjectMember; code: SkipCode; reason: string }

const open = (s: Pick<TerminalSession, 'state'>): boolean => s.state === 'creating' || s.state === 'running' || s.state === 'end_requested'

/**
 * What the confirmation sheet lists. A member starts when its machine is enrolled and ready, takes launch requests (and
 * worktrees when the Project makes them), has the tool installed, and the run would not exceed 8 open sessions on that
 * machine or 32 on the account. `sessions` are the account's sessions now; each member planned to start counts as one
 * more, in order. A skipped member says why in plain words; `not_enrolled` is the one that offers Add a machine.
 */
export function planRun(project: Project, machines: TerminalMachine[], _snapshot: unknown, sessions: Pick<TerminalSession, 'agentId' | 'state'>[]): { start: PlannedMember[]; skipped: SkippedMember[] } {
  // Without the account's sessions the 8 and 32 limits could never apply, so the sheet would promise more than can start.
  if (!Array.isArray(sessions)) throw new Error("planRun needs the account's sessions.")
  const start: PlannedMember[] = [], skipped: SkippedMember[] = []
  const perMachine = new Map<string, number>()
  let account = 0
  for (const s of sessions) if (open(s)) { account++; perMachine.set(s.agentId, (perMachine.get(s.agentId) ?? 0) + 1) }
  for (const member of project.members) {
    const skip = (code: SkipCode, reason: string) => skipped.push({ position: member.position, member, code, reason })
    const machine = machines.find(m => m.agentId === member.host)
    const caps = machine?.capabilities
    const tool = member.tool || project.defaultTool
    if (!machine) { skip('not_enrolled', "That machine isn't enrolled on this account."); continue }
    // readiness() also says why when the machine is not connected right now.
    const notReady = readiness(machine)
    if (notReady !== '') { skip('not_ready', notReady); continue }
    if (caps?.launch !== true) { skip('no_launch', 'Update reet on that machine to start agents.'); continue }
    if (!Array.isArray(caps.tools) || !caps.tools.includes(tool)) { skip('tool_not_installed', "That agent isn't installed on that machine."); continue }
    if (project.useWorktrees && caps.worktree !== true) { skip('no_worktree', 'Update reet on that machine to start agents with worktrees.'); continue }
    if ((perMachine.get(member.host) ?? 0) >= MAX_SESSIONS_PER_MACHINE) { skip('machine_session_limit', `That machine already has ${MAX_SESSIONS_PER_MACHINE} sessions, the most it can have. End one, then start again.`); continue }
    if (account >= MAX_SESSIONS_PER_ACCOUNT) { skip('account_session_limit', `This account already has ${MAX_SESSIONS_PER_ACCOUNT} sessions, the most it can have. End one, then start again.`); continue }
    perMachine.set(member.host, (perMachine.get(member.host) ?? 0) + 1)
    account++
    start.push({ position: member.position, member, host: member.host, machine: machine.host, dir: member.dir, tool, toolName: toolName(tool), model: resolvedModel(project, member), worktree: project.useWorktrees })
  }
  return { start, skipped }
}

/**
 * The model a member starts with, as the server resolves it: its own, else the Project's default model only when it uses the
 * default agent (a default model belongs to the default agent), else the agent's own default (empty).
 */
export function resolvedModel(project: Pick<Project, 'defaultTool' | 'defaultModel'>, member: Pick<ProjectMember, 'tool' | 'model'>): string {
  if (member.model) return member.model
  const tool = member.tool || project.defaultTool
  return tool === project.defaultTool ? project.defaultModel : ''
}

/** The confirmation sheet's statement: what starts, how it is isolated, that nothing runs until Start, and where a prompt can be seen. */
export function confirmSentence(count: number, hasPrompt: boolean, useWorktrees: boolean): string {
  if (count === 0) return 'No repository can start. Nothing runs until you press Start.'
  const agents = `${count} ${count === 1 ? 'agent' : 'agents'}`
  const where = useWorktrees
    ? `${agents}, each in its own git worktree and branch under ~/.reet/worktrees on its machine. reet never removes a worktree by itself.`
    : `${agents} in the saved ${count === 1 ? 'folder' : 'folders'} itself, so any other session there shares that checkout.`
  const start = `Starts ${where} Nothing runs until you press Start.`
  return hasPrompt ? `${start} The prompt is kept with the session record and is visible to other users of that machine in its process list.` : start
}

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

/** A run's state in words, for the Projects list and the runs list. */
export function runStateLabel(state: string): string {
  return ({ running: 'Starting', complete: 'All started', partial: 'Some failed', failed: 'None started' } as Record<string, string>)[state] ?? state
}

export type ProjectForm = { name: string; defaultTool: string; defaultModel: string; sharedPrompt: string; useWorktrees: boolean; members: MemberInput[] }
/**
 * What a save sends. A create sends everything. An update sends the shared prompt only when the Project was read with it
 * (the single read) and the text changed: the list and save replies leave the prompt out, and sending "" would clear it.
 */
export function projectInput(editing: Pick<Project, 'sharedPrompt'> | null, form: ProjectForm): ProjectInput {
  const input: ProjectInput = { name: form.name.trim(), defaultTool: form.defaultTool, defaultModel: form.defaultModel.trim(), useWorktrees: form.useWorktrees, members: form.members }
  if (!editing || (editing.sharedPrompt !== undefined && editing.sharedPrompt !== form.sharedPrompt)) input.sharedPrompt = form.sharedPrompt
  return input
}
