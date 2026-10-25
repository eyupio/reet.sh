import type { Machine } from './api'

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
const SUGGESTIONS: Record<string, string[]> = {
  claude: ['sonnet', 'opus', 'haiku'],
  codex: ['gpt-5-codex'],
  opencode: [],
  gemini: ['gemini-2.5-pro', 'gemini-2.5-flash'],
}
// These two take the prompt as an operand and could read a one-word prompt as a subcommand.
const NEEDS_TWO_WORDS = new Set(['claude', 'codex'])
const MODEL = /^[A-Za-z0-9][A-Za-z0-9._:/@+-]{0,99}$/
const SOMETHING_CHANGED = 'Something changed. Check again.'

const nameOf = (tool: string): string => TOOLS.find(t => t.id === tool)?.name ?? tool

/** The display name for a tool id; an unknown id is shown as given, and a missing one as "an agent". */
export const toolName = (tool: string | undefined): string => (tool ? nameOf(tool) : 'an agent')

const UPDATE_TO_LAUNCH = 'Update reet on this machine to start agents.'
export type ToolChoice = { id: string; name: string; available: boolean; reason: string }
/** Every built-in tool, offered only when the machine takes launch requests and reports the tool as installed. */
export function toolChoices(caps: { launch?: boolean; tools?: unknown } | undefined): ToolChoice[] {
  const launch = caps?.launch === true
  const tools = Array.isArray(caps?.tools) ? caps.tools : []
  return TOOLS.map(t => {
    const available = launch && tools.includes(t.id)
    return { id: t.id, name: t.name, available, reason: available ? '' : launch ? 'Not installed on this machine.' : UPDATE_TO_LAUNCH }
  })
}

/** The Activity line for a `terminal-launch` audit event (machine, host, tool, model, dir; the prompt is never recorded). */
export function launchActivity(d: { tool?: string; model?: string; dir?: string; host?: string }): string {
  const model = d.model ? ` with ${d.model}` : ''
  return `Started ${toolName(d.tool)}${model} in ${d.dir || 'the home folder'} on ${d.host || 'a machine'}`
}

/** Hints only: the machine's agent decides whether a model name exists. */
export function modelSuggestions(tool: string): string[] { return [...(SUGGESTIONS[tool] ?? [])] }

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

/**
 * The sentence for a refused start. The server sends {error, message} and, for launch refusals only, a `code`. `message`
 * is the server's own text, used to tell apart refusals that share a status.
 */
export function refusalText(status: number, code?: string, message?: string): string {
  const c = code ?? ''
  const msg = (message ?? '').toLowerCase()
  if (status === 409 && c === 'launch_unsupported') return 'Update reet on that machine to start agents.'
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

export type LaunchChoices = { key: string; model: string; folder: string }
/** A new machine or agent (the key) starts the model and folder afresh; the same key keeps them exactly. */
export function rekey(prev: LaunchChoices, key: string): LaunchChoices {
  return prev.key === key ? prev : { key, model: '', folder: '' }
}

/** Which field a definite refusal is about, so focus can go there. */
export function refusalField(status: number, code?: string): '' | 'machine' | 'tool' | 'folder' | 'prompt' {
  if (status === 409 && code === 'launch_unsupported') return 'machine'
  if (status !== 422) return ''
  return code === 'folder_unavailable' ? 'folder' : code === 'tool_not_installed' ? 'tool' : code === 'prompt_unsupported' ? 'prompt' : ''
}
