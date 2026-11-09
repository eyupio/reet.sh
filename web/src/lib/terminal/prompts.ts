// Pure logic for the prompt library: the backslash trigger, token resolution, search and validation.
// Prompts are plain text; skill tokens are replaced with the same ready-to-use text the skills picker inserts.
export type PromptContext = {
  project?: string
  branch?: string
  folder?: string
  sessionTitle?: string
  /** The moment used for {{date}}; defaults to now. */
  now?: number
}

export type PromptResolution = { text: string; unresolved: string[] }
export type PromptRecord = { title: string; description: string; body: string; category: string }
export type PromptWrite = PromptRecord & { tool: string; model: string; effort: string }

const MAX_TITLE = 80
const MAX_DESCRIPTION = 500
const MAX_CATEGORY = 40
const MAX_TOOL = 64
const MAX_MODEL = 200
const MAX_EFFORT = 64
export const MAX_PROMPT_BODY_BYTES = 8000
const TOKEN = /\{\{\s*([a-zA-Z][a-zA-Z0-9:_-]*)\s*\}\}/g

function localDate(now: number): string {
  const d = new Date(now)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

function contextValue(name: string, context: PromptContext): string | undefined {
  switch (name) {
    case 'project': return context.project
    case 'branch': return context.branch
    case 'folder': return context.folder
    case 'sessionTitle': return context.sessionTitle
    case 'date': return localDate(context.now ?? Date.now())
    default: return undefined
  }
}

/** Fill the variables a prompt may use. Skills are looked up by slug or id. An unknown
 *  or empty value is left exactly as written and reported, so nothing is silently lost. */
export function resolvePrompt(body: string, context: PromptContext = {}, skills: Record<string, string> = {}): PromptResolution {
  const unresolved: { token: string }[] = []
  const text = body.replace(TOKEN, (whole, raw: string) => {
    const token = raw.trim()
    if (token.startsWith('skill:')) {
      const key = token.slice('skill:'.length).trim()
      const skill = skills[key]
      if (skill && skill.trim()) return skill
      unresolved.push({ token: whole })
      return whole
    }
    const value = contextValue(token, context)
    if (value) return value
    unresolved.push({ token: whole })
    return whole
  })
  return { text, unresolved: [...new Set(unresolved.map(item => item.token))] }
}

/** The backslash token at the caret: start of the composer or after whitespace, and never
 *  across a space. Typing C:\Users\me does not open the picker. */
export function promptTrigger(draft: string, caret = draft.length): { start: number; query: string } | null {
  const at = Math.max(0, Math.min(Number.isFinite(caret) ? Math.floor(caret) : draft.length, draft.length))
  const before = draft.slice(0, at)
  const start = before.lastIndexOf('\\')
  if (start < 0 || (start > 0 && !/\s/.test(before[start - 1]))) return null
  const query = before.slice(start + 1)
  if (query !== '' && /[\s\\]/.test(query)) return null
  return { start, query }
}

/** Replace the trigger token with a prompt. The rest of the message is kept as typed. */
export function replacePromptTrigger(draft: string, trigger: { start: number; query: string }, text: string, limitBytes = MAX_PROMPT_BODY_BYTES):
  { draft: string; caret: number } | { error: string } {
  const end = Math.min(draft.length, trigger.start + 1 + trigger.query.length)
  const next = draft.slice(0, trigger.start) + text + draft.slice(end)
  if (new TextEncoder().encode(next).length > limitBytes) {
    return { error: `That prompt is too long for this box. Keep it under ${limitBytes.toLocaleString('en-GB')} bytes.` }
  }
  return { draft: next, caret: trigger.start + text.length }
}

/** Case-insensitive search over everything a person would recognise a prompt by. */
export function searchPrompts<T extends PromptRecord>(prompts: T[], query: string): T[] {
  const q = query.trim().toLowerCase()
  if (!q) return prompts
  return prompts.filter(prompt => `${prompt.title} ${prompt.description} ${prompt.category} ${prompt.body}`.toLowerCase().includes(q))
}

function controlFree(value: string, allowNewlines: boolean): boolean {
  for (const char of value) {
    const code = char.codePointAt(0)!
    if (code < 0x20 && !(allowNewlines && (code === 9 || code === 10 || code === 13))) return false
    if (code >= 0x7f && code <= 0x9f) return false
  }
  return true
}

function runes(value: string): number {
  return [...value].length
}

/** Field errors for the editor, keyed by the field whose label should point at them. */
export type PromptErrors = Partial<Record<'title' | 'description' | 'body' | 'category' | 'tool' | 'model' | 'effort', string>>

export function validatePrompt(prompt: PromptWrite): PromptErrors {
  const errors: PromptErrors = {}
  const title = prompt.title.trim()
  if (!title) errors.title = 'Give the prompt a title.'
  else if (runes(title) > MAX_TITLE) errors.title = `Keep the title to ${MAX_TITLE} characters.`
  else if (!controlFree(title, false)) errors.title = 'Use a title without control characters.'
  if (runes(prompt.description) > MAX_DESCRIPTION) errors.description = `Keep the description to ${MAX_DESCRIPTION} characters.`
  else if (!controlFree(prompt.description, true)) errors.description = 'Use a description without control characters.'
  const bytes = new TextEncoder().encode(prompt.body).length
  if (!prompt.body.trim()) errors.body = 'Write what the agent should do.'
  else if (bytes > MAX_PROMPT_BODY_BYTES) errors.body = `That prompt is too long. Keep it under ${MAX_PROMPT_BODY_BYTES.toLocaleString('en-GB')} bytes.`
  else if (!controlFree(prompt.body, true)) errors.body = 'Use text without terminal control characters.'
  if (runes(prompt.category) > MAX_CATEGORY) errors.category = `Keep the category to ${MAX_CATEGORY} characters.`
  else if (!controlFree(prompt.category, false)) errors.category = 'Use a category without control characters.'
  if (runes(prompt.tool) > MAX_TOOL) errors.tool = `Keep the agent name to ${MAX_TOOL} characters.`
  else if (!controlFree(prompt.tool, false)) errors.tool = 'Use an agent name without control characters.'
  if (runes(prompt.model) > MAX_MODEL) errors.model = `Keep the model to ${MAX_MODEL} characters.`
  else if (!controlFree(prompt.model, false)) errors.model = 'Use a model name without control characters.'
  if (runes(prompt.effort) > MAX_EFFORT) errors.effort = `Keep the effort to ${MAX_EFFORT} characters.`
  else if (!controlFree(prompt.effort, false)) errors.effort = 'Use an effort without control characters.'
  return errors
}

/** One line describing a prompt's optional launch suggestions, or '' when it has none. */
export function suggestionLine(prompt: Pick<PromptWrite, 'tool' | 'model' | 'effort'>, toolName: (id: string) => string = id => id): string {
  const parts = [prompt.tool ? toolName(prompt.tool) : '', prompt.model, prompt.effort].filter(Boolean)
  return parts.length ? `Suggests ${parts.join(' · ')}` : ''
}
