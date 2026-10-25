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

/** Hints only: the machine's agent decides whether a model name exists. */
export function modelSuggestions(tool: string): string[] { return [...(SUGGESTIONS[tool] ?? [])] }

/** An empty model is fine (the agent picks its own). */
export function validateModel(model: string): string | null {
  const s = String(model ?? '').trim()
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

export type FolderChoice = { path: string; label: string }
/** Home first (an empty path means the enrolled home), then the project folders the machine has reported, once each. */
export function folderChoices(machine: { host: string } | undefined, snapshot: Machine[] | undefined): FolderChoice[] {
  const out: FolderChoice[] = [{ path: '', label: 'Home folder' }]
  if (!machine || !Array.isArray(snapshot)) return out
  const host = String(machine.host).toLowerCase()
  const seen = new Set<string>()
  for (const m of snapshot) {
    if (!m || typeof m.host !== 'string' || m.host.toLowerCase() !== host || !Array.isArray(m.groups)) continue
    for (const g of m.groups) {
      const raw = typeof g?.project === 'string' ? g.project : ''
      if (!raw.startsWith('/')) continue
      const path = raw.length > 1 ? raw.replace(/\/+$/, '') : raw
      if (path === '' || path === '/' || seen.has(path)) continue
      seen.add(path)
      out.push({ path, label: path })
    }
  }
  return out
}

export function launchSentence(tool: string, dir: string, machine: string, hasPrompt: boolean): string {
  return `Start ${nameOf(tool)} in ${dir ? dir : 'the home folder'} on ${machine}${hasPrompt ? ' with your opening prompt' : ''}.`
}

/** The session title: the agent, and the last part of the folder when one is chosen. At most 80 characters, no controls. */
export function titleFor(tool: string, dir: string): string {
  const name = nameOf(tool)
  const last = String(dir ?? '').split('/').filter(Boolean).pop() ?? ''
  const odd = [...last].some(c => { const n = c.codePointAt(0)!; return badControl(n) || n === 0x0a || (n >= 0x200b && n <= 0x200f) || (n >= 0x202a && n <= 0x202e) || (n >= 0x2066 && n <= 0x2069) || n === 0xfeff })
  if (last === '' || odd) return name
  const title = `${name} · ${last}`
  return Array.from(title).length <= 80 ? title : Array.from(title).slice(0, 80).join('')
}

/**
 * The sentence for a refused start. `message` is the server's own text, used only to tell apart refusals that share a
 * status: the session limit and a changed request are both 409, and "disabled" is how the server says terminals are off.
 */
export function refusalText(status: number, code?: string, message?: string): string {
  const c = code ?? ''
  const msg = (message ?? '').toLowerCase()
  if (status === 409 && c === 'launch_unsupported') return 'Update reet on that machine to start agents.'
  if (status === 422 && c === 'tool_not_installed') return "That agent isn't installed on that machine."
  if (status === 422 && c === 'folder_unavailable') return "That folder isn't available on that machine."
  if (status === 422 && c === 'prompt_unsupported') return 'Write the opening prompt as at least two words.'
  if (status === 409 && (c === 'capacity' || c === 'session_limit' || msg.includes('session limit'))) return 'That machine already has the most sessions it allows.'
  if (status === 409 && c === '') return 'That request changed. Start it again.'
  if (status === 503) return 'That machine is reconnecting. Try again in a moment.'
  if (status === 403 && (c === 'terminal_disabled' || c === 'consent_off' || msg.includes('disabled'))) return 'Browser terminals are switched off on that machine.'
  return SOMETHING_CHANGED
}
