import type { Machine } from './api'
import { CATALOGUE, agentChoices } from './catalogue'
import type { TerminalMachine } from './terminal/api'

// What the "Start an agent" dialog checks before it asks a machine to start an agent CLI, and the exact sentences it
// shows when the machine or the controller refuses. Pure: no DOM, no fetch. The limits match the controller and the
// connector (docs/terminal-launch.md), which both check again; nothing here says an agent is signed in or working.

export const MAX_PROMPT_BYTES = 2000
export const TOOLS: { id: string; name: string }[] = [
  { id: 'claude', name: 'Claude Code' },
  { id: 'codex', name: 'Codex' },
  { id: 'opencode', name: 'OpenCode' },
  { id: 'gemini', name: 'Gemini CLI' },
]
// These take the prompt as an operand and could read a one-word prompt as a subcommand.
const NEEDS_TWO_WORDS = new Set(['claude', 'codex', 'cline'])
const MODEL = /^[A-Za-z0-9][A-Za-z0-9._:/@+-]{0,99}$/
const SOMETHING_CHANGED = 'Something changed. Check again.'

const nameOf = (tool: string): string => CATALOGUE.find(agent => agent.id === tool)?.name ?? tool

/** The display name for a tool id; an unknown id is shown as given, and a missing one as "an agent". */
export const toolName = (tool: string | undefined): string => (tool ? nameOf(tool) : 'an agent')

const UPDATE_TO_LAUNCH = 'Update reet on this machine to start agents.'
export type ToolChoice = { id: string; name: string; available: boolean; reason: string }
/** Every catalogue agent, offered only when the machine takes launch requests and reports it installed. The catalogue and
 *  the machine's own gate decide: an old connector keeps the original four, an agents-v2 one may offer Cline too. */
export function toolChoices(machine: TerminalMachine | undefined): ToolChoice[] {
  if (!machine) return CATALOGUE.map(agent => ({ id: agent.id, name: agent.name, available: false, reason: UPDATE_TO_LAUNCH }))
  return agentChoices(machine).map(choice => ({
    id: choice.agent.id, name: choice.agent.name, available: choice.available, reason: choice.reason ?? '',
  }))
}

/** The Activity line for a `terminal-launch` audit event (machine, host, tool, model, dir; the prompt is never recorded). */
export function launchActivity(d: { tool?: string; model?: string; dir?: string; host?: string }): string {
  const model = d.model ? ` with ${d.model}` : ''
  return `Started ${toolName(d.tool)}${model} in ${d.dir || 'the home folder'} on ${d.host || 'a machine'}`
}

/** Hints only: the machine's agent decides whether a model name exists. */
export function modelSuggestions(tool: string): string[] { return [...(CATALOGUE.find(agent => agent.id === tool)?.models ?? [])] }

/** An empty model is fine (the agent picks its own). */
/** The value to send: validateModel looks at the trimmed text, so the caller must send this, not the raw input. */
export const cleanModel = (model: string): string => String(model ?? '').trim()

export function validateModel(model: string): string | null {
  const s = cleanModel(model)
  if (s === '') return null
  return MODEL.test(s) ? null : "That model name isn't one reet can pass on. Use letters, digits and . _ : / @ + -, up to 100 characters, starting with a letter or digit."
}

// C0 other than newline, DEL, C1, and the two Unicode line separators.
const badControl = (n: number): boolean => (n <= 0x1f && n !== 0x0a) || (n >= 0x7f && n <= 0x9f) || n === 0x2028 || n === 0x2029

/** An empty prompt is fine. The text is sent exactly as typed, so it is not trimmed. */
export function validatePrompt(tool: string, prompt: string): string | null {
  const s = String(prompt ?? '')
  if (s === '') return null
  for (const c of s) { const n = c.codePointAt(0)!; if (n >= 0xd800 && n <= 0xdfff) return 'Use a prompt that is valid text.' }
  if (new TextEncoder().encode(s).length > MAX_PROMPT_BYTES) return `That prompt is too long. Use at most ${MAX_PROMPT_BYTES} bytes; accented letters and emoji take more than one.`
  if ([...s].some(c => badControl(c.codePointAt(0)!))) return 'Use a prompt without control characters other than line breaks.'
  if (s.startsWith('-')) return 'Start the prompt with a word, not a dash.'
  if (NEEDS_TWO_WORDS.has(tool) && !/[ \n]/.test(s)) return 'Write the opening prompt as at least two words.'
  return null
}

// Control and format characters (Go's ValidTitle rejects both) and the two Unicode line separators.
const INVISIBLE = new RegExp('[\\p{Cc}\\p{Cf}\\u2028\\u2029]', 'u')
const hasLoneSurrogate = (s: string): boolean => [...s].some(c => { const n = c.codePointAt(0)!; return n >= 0xd800 && n <= 0xdfff })
export const MAX_DIR_BYTES = 4096

/** A folder to start in: absolute, at most 4,096 bytes, valid text, nothing invisible. Empty is not valid here: it means the home folder. */
export function validateDir(dir: string): string | null {
  const s = String(dir ?? '')
  if (!s.startsWith('/')) return 'Use a full path that starts with /.'
  if (hasLoneSurrogate(s)) return 'Use a folder path that is valid text.'
  if (new TextEncoder().encode(s).length > MAX_DIR_BYTES) return `That folder path is too long. Use at most ${MAX_DIR_BYTES} bytes; accented letters and emoji take more than one.`
  if (INVISIBLE.test(s)) return 'Use a folder path without control or invisible characters.'
  return null
}

export type FolderChoice = { path: string; label: string }
/** Home first (an empty path means the enrolled home), then the project folders the machine has reported, once each. Odd paths are left out, not rewritten. */
export function folderChoices(machine: { host: string } | undefined, snapshot: Machine[] | undefined): FolderChoice[] {
  const out: FolderChoice[] = [{ path: '', label: 'Home folder' }]
  if (!machine || !Array.isArray(snapshot)) return out
  const host = String(machine.host).toLowerCase()
  const seen = new Set<string>()
  for (const m of snapshot) {
    if (!m || typeof m.host !== 'string' || m.host.toLowerCase() !== host || !Array.isArray(m.groups)) continue
    for (const g of m.groups) {
      const raw = typeof g?.project === 'string' ? g.project : ''
      const path = raw.length > 1 ? raw.replace(/\/+$/, '') : raw
      if (path === '' || path === '/' || seen.has(path) || validateDir(path) !== null) continue
      if (path.split('/').slice(1).some(part => part === '' || part === '.' || part === '..')) continue
      seen.add(path)
      out.push({ path, label: path })
    }
  }
  return out
}

/** One text for the dialog: what will start, that nothing runs until Start, and (with a prompt) where the prompt can be seen. */
export function launchSentence(tool: string, dir: string, machine: string, hasPrompt: boolean): string {
  const start = `Starts ${nameOf(tool)} in ${dir ? dir : 'the home folder'} on ${machine}. Nothing runs until you press Start.`
  return hasPrompt ? `${start} The prompt is kept with the session record and is visible to other users of that machine in its process list.` : start
}

/** The session title: the agent, and the last part of the folder when one is chosen. At most 80 characters, no control or format characters. */
export function titleFor(tool: string, dir: string): string {
  const name = nameOf(tool)
  const last = String(dir ?? '').split('/').filter(Boolean).pop() ?? ''
  if (last === '' || INVISIBLE.test(last) || hasLoneSurrogate(last)) return name
  const title = `${name} \u00b7 ${last}`
  return Array.from(title).length <= 80 ? title : Array.from(title).slice(0, 80).join('')
}

const RUN_SENTENCES: Record<string, string> = {
  unknown_position: 'Choose repositories that are in this Project, each once.',
  not_failed: 'Only a repository that failed can be tried again.',
  terminals_disabled: "Remote terminals are switched off on this server, so agents can't be started.",
  unknown_outcome: "We didn't hear back, so the agent may have started. Check your sessions before retrying.",
  session_lost: 'That machine lost the session before it started. Start it again.',
  machine_refused: 'That machine refused to start the agent. Check the machine, then try again.',
  invalid_launch: "reet couldn't send that start request. Check the Project's settings and try again.",
  request_changed: 'That request changed. Start it again.',
  request_id_required: 'Starting a run needs a request id: 16 to 128 letters, digits, hyphens or underscores.',
  run_in_progress: 'This Project is still starting. Wait for it to finish, then start again.',
  internal_error: 'Something went wrong on our side. Check your sessions, then retry.',
  project_changed: 'Someone changed this Project. Reload it and try again.',
  run_not_found: "That run isn't on this account.",
  name_taken: 'Another Project on this account already has that name.',
  project_limit: 'This account already has 50 Projects, the most it can have. Delete one first.',
  member_limit: 'A Project can have at most 12 repositories.',
  duplicate_member: 'Each machine and folder can be in a Project only once.',
  no_members: 'Choose at least one repository to start.',
  member_not_pending: "That repository isn't waiting to start, or is already being started.",
  invalid_name: 'Give the Project a name of 1 to 80 characters, without line breaks or control characters.',
  worktree_busy: 'That machine is already working on a worktree. Try again in a minute.',
  worktree_unavailable: "That machine couldn't do that just now. Try again in a moment.",
  worktree_mismatch: "That machine's answer did not match the request. Check again.",
  worktree_unsupported: 'Update reet on that machine to start agents with worktrees.',
  worktree_changed: 'That worktree changed since you looked. Check again.',
  worktree_refused: 'That machine refused to make a worktree for this request. Start it again to see why.',
  // The reasons a machine gives for refusing a worktree create, status or remove.
  not_repo_root: "That folder isn't the top of a git repository.",
  repo_busy: 'That repository is in the middle of a merge, rebase or similar. Finish or abort it first.',
  worktree_exists: "A worktree or branch with this run's name is already on that machine, so nothing was started.",
  outside_home: 'The folder ~/.reet/worktrees on that machine is a link, belongs to another user or can be changed by others. Fix it, then try again.',
  not_registered: "Git doesn't list that folder as this run's worktree, so reet won't touch it.",
  has_changes: "This worktree has changes you haven't saved. Commit, stash or discard them in the worktree, then check again.",
  unreferenced_commits: 'Some commits in this worktree are not on any branch. Put them on a branch first, then check again.',
  nested_repository: 'This worktree contains another git repository. Move or remove it first.',
  hidden_changes: 'Git is not tracking some edits in this worktree. Review them before removing.',
  check_incomplete: "reet couldn't check all of this worktree. Remove it yourself.",
  // Refusals any Projects request can meet.
  invalid_version: "reet couldn't tell which version of this Project you're changing. Reload it and try again.",
  invalid_request: "reet couldn't read that request. Reload the page and try again.",
  too_large: 'That request is too large for a Project. Shorten the shared prompt or the folder paths.',
  invalid_project: "Something in that Project can't be saved. Check each field and try again.",
  member_state: "That repository can't do that in the state it is in now. Reload the run.",
  member_stale: SOMETHING_CHANGED,
  server_error: "reet couldn't do that just now. Try again.",
  accounts_required: 'Projects need an account signed in through the browser.',
  read_only: "This account is read-only, so it can't use Projects.",
  origin_refused: "That request didn't come from this reet's own address. Open reet from its usual address and try again.",
}

/** The plain sentence for a code only Projects, runs and worktrees answer with; '' for any other code. */
export const codeSentence = (code: string | undefined): string => (code && Object.hasOwn(RUN_SENTENCES, code) ? RUN_SENTENCES[code] : '')

/**
 * The sentence for a refused start. The server sends {error, message} and, for launch refusals only, a `code`. `message`
 * is the server's own text, used to tell apart refusals that share a status.
 */
export function refusalText(status: number, code?: string, message?: string): string {
  const c = code ?? ''
  const msg = (message ?? '').toLowerCase()
  if (status === 401) return "You've been signed out. Sign in again to continue."
  if (c === 'git_failed') return msg.startsWith("git couldn't") ? (message ?? '') : "Git couldn't make a worktree there."
  if (status === 404 && c === 'not_found') return msg.includes('worktree') ? "That worktree isn't on that machine. It may have been removed already." : "That Project isn't on this account."
  const known = codeSentence(c)
  if (known) return known
  if (status === 409 && c === 'launch_unsupported') return 'Update reet on that machine to start agents.'
  // The server words these: the update sentence when the machine lacks agents-v2, else that the agent has no such option.
  if (c === 'launch_option_unsupported') return msg.startsWith('update reet') ? 'Update reet on that machine to use that option.' : "That agent doesn't take that option."
  if (c === 'tool_unavailable') return "reet doesn't start this agent yet."
  if (status === 422 && c === 'tool_not_installed') return "That agent isn't installed on that machine."
  if (status === 422 && c === 'folder_unavailable') return "That folder isn't available on that machine."
  if (status === 422 && c === 'prompt_unsupported') return 'Write the opening prompt as at least two words.'
  if (status === 409 && c === '' && msg.includes('session limit')) return 'The terminal session limit has been reached. End a session and try again.'
  if (status === 409 && c === '') return 'That request changed. Start it again.'
  if (status === 503) return 'That machine is reconnecting. Try again in a moment.'
  if (status === 403 && msg.includes('disabled by the server')) return 'Browser terminals are switched off on this server.'
  return SOMETHING_CHANGED
}

/**
 * A create can answer 200 with a session the machine did not start: the broker records it as lost when the machine refused it
 * (local terminal consent switched off between the readiness read and the create, for instance). That is a definite outcome:
 * nothing is running, so the person may start again. An exited session did start, and is landed on so its exit status shows.
 */
export function notStartedText(state: string): string {
  return state === 'lost' || state === 'ended' ? "That machine didn't start the agent. Check its terminal settings on that machine and try again." : ''
}

/**
 * The machine the dialog opens on: the one you came from when it is ready; otherwise the first ready one, with a note saying
 * so. When nothing is ready the one you came from stays selected, so its own readiness sentence is shown.
 */
export function preselect(machines: { agentId: string; host: string; ready: boolean }[], initial: string): { agentId: string; note: string } {
  const from = machines.find(x => x.agentId === initial)
  if (from?.ready) return { agentId: from.agentId, note: '' }
  const ready = machines.find(x => x.ready)
  if (!ready) return { agentId: from?.agentId ?? '', note: '' }
  return { agentId: ready.agentId, note: from ? `${from.host} isn't ready, so ${ready.host} is selected.` : '' }
}

export type LaunchChoices = { key: string; model: string; folder: string; effort?: string }
/** A new machine or agent (the key) re-resolves the model and effort to the given defaults and clears the folder; the same
 *  key keeps every choice exactly. */
export function rekey(prev: LaunchChoices, key: string, defaults: { model?: string; folder?: string; effort?: string } = {}): LaunchChoices {
  return prev.key === key ? prev : { key, model: defaults.model ?? '', folder: defaults.folder ?? '', effort: defaults.effort ?? '' }
}

/** Which field a definite refusal is about, so focus can go there. */
export function refusalField(status: number, code?: string): '' | 'machine' | 'tool' | 'folder' | 'prompt' {
  if (status === 409 && code === 'launch_unsupported') return 'machine'
  if (status !== 422) return ''
  return code === 'folder_unavailable' ? 'folder' : code === 'tool_not_installed' ? 'tool' : code === 'prompt_unsupported' ? 'prompt' : ''
}
