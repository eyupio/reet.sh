// The chat reducer turns the controller's bounded SSE events into the items a
// person reads. It is pure so the streaming, approval and reconnect cases can be
// tested without a browser or a live OpenCode process.

export type ChatEvent = {
  seq: number
  kind: 'user' | 'agent' | 'tool' | 'notice' | 'status' | 'permission' | 'permission_resolved' | 'permission_cancelled' | 'question' | 'question_resolved' | 'question_cancelled'
  messageId?: string
  toolCallId?: string
  permissionId?: string
  questionId?: string
  question?: ChatQuestion
  title?: string
  toolKind?: string
  status?: string
  state?: string
  text?: string
  command?: string
  path?: string
  diff?: string
  additions?: number
  deletions?: number
  durationMs?: number
  locations?: string[]
  allow?: boolean
  bad?: boolean
  truncated?: boolean
  reason?: string
  detail?: string
}

// A typed agent question, as the controller sends it. The machine validates every answer against the question it
// holds; the checks below only give the person feedback before they send.
export type ChatQuestionOption = { value: string; label: string; description?: string }
export type ChatQuestionField = {
  key: string
  title?: string
  description?: string
  kind: 'text' | 'single' | 'multi'
  required?: boolean
  options?: ChatQuestionOption[]
  minItems?: number
  maxItems?: number
  maxLength?: number
}
export type ChatQuestion = { message: string; fields: ChatQuestionField[] }
export type QuestionDecision = 'answered' | 'declined' | 'cancelled'

export type ChatItem =
  | { kind: 'user' | 'agent' | 'notice'; key: string; text: string; bad?: boolean }
  | { kind: 'tool'; key: string; toolCallId: string; title: string; toolKind: string; status: string; text: string; command: string; path: string; diff: string; additions: number; deletions: number; durationMs: number; locations: string[] }
  | { kind: 'permission'; key: string; permissionId: string; title: string; toolKind: string; command: string; path: string; diff: string; locations: string[]; truncated: boolean; decision?: 'allowed' | 'denied' | 'cancelled'; reason?: string }
  | { kind: 'question'; key: string; questionId: string; toolCallId: string; question: ChatQuestion; decision?: QuestionDecision; reason?: string }

export type ChatState = {
  items: ChatItem[]
  status: { state: string; detail: string }
  lastSeq: number
  truncated: boolean
  connection: 'connecting' | 'open' | 'reconnecting'
}

export const initialChat = (): ChatState => ({ items: [], status: { state: '', detail: '' }, lastSeq: 0, truncated: false, connection: 'connecting' })

export function resetChat(truncated: boolean): ChatState {
  return { ...initialChat(), truncated }
}

// The log the browser keeps is bounded, like the controller's ring: a long chat, or a continued chat replaying its
// history, must not grow the list (and the DOM) until the tab stops responding. Older items are dropped first and the
// log says so; an approval still waiting for an answer is never dropped, because it is the one thing the person must act on.
export const MAX_ITEMS = 400
export const MAX_TEXT = 256 * 1024
/** A single message stops growing at this size, so one very long answer cannot make every later update re-lay it out at length. */
export const MAX_MESSAGE_TEXT = 256 * 1024

const workingStates = new Set(['working', 'waiting', 'starting'])

export function isWorking(state: ChatState): boolean {
  return workingStates.has(state.status.state)
}

export function pendingPermissions(state: ChatState): Extract<ChatItem, { kind: 'permission' }>[] {
  return state.items.filter((item): item is Extract<ChatItem, { kind: 'permission' }> => item.kind === 'permission' && !item.decision)
}

export function pendingQuestions(state: ChatState): Extract<ChatItem, { kind: 'question' }>[] {
  return state.items.filter((item): item is Extract<ChatItem, { kind: 'question' }> => item.kind === 'question' && !item.decision)
}

/** Once a turn has stopped nothing it started is still running. A tool left pending or in progress would keep a busy state on a chat that says it
 *  stopped, so it is shown as stopped; the agent's own later report of how it ended still replaces this. */
function stopTools(items: ChatItem[]): ChatItem[] {
  if (!items.some(item => item.kind === 'tool' && (item.status === 'pending' || item.status === 'in_progress'))) return items
  return items.map(item => item.kind === 'tool' && (item.status === 'pending' || item.status === 'in_progress') ? { ...item, status: 'cancelled' } : item)
}

function cancelPending(items: ChatItem[], reason: string): ChatItem[] {
  return items.map(item => (item.kind === 'permission' || item.kind === 'question') && !item.decision ? { ...item, decision: 'cancelled' as const, reason } : item)
}

function itemChars(item: ChatItem): number {
  if (item.kind === 'tool') return item.title.length + item.text.length + item.locations.join('').length
  if (item.kind === 'permission') return item.title.length + item.command.length + item.path.length + item.diff.length + item.locations.join('').length
  if (item.kind === 'question') return questionChars(item.question)
  return item.text.length
}

/** Trim the oldest items until the log fits its limits, keeping a pending approval reachable. */
export function trimChat(items: ChatItem[], truncated: boolean): { items: ChatItem[]; truncated: boolean } {
  let total = 0
  for (const item of items) total += itemChars(item)
  let cut = 0
  while (cut < items.length && (items.length - cut > MAX_ITEMS || total > MAX_TEXT)) {
    const item = items[cut]
    if ((item.kind === 'permission' || item.kind === 'question') && !item.decision) break
    total -= itemChars(item); cut++; truncated = true
  }
  return cut ? { items: items.slice(cut), truncated } : { items, truncated }
}

export function applyChatEvent(state: ChatState, event: ChatEvent): ChatState {
  if (!Number.isFinite(event.seq) || event.seq <= state.lastSeq) return state
  const next = reduceChatEvent(state, event)
  if (next === state) return state
  const bounded = trimChat(next.items, next.truncated)
  return bounded.items === next.items && bounded.truncated === next.truncated ? next : { ...next, ...bounded }
}

function reduceChatEvent(state: ChatState, event: ChatEvent): ChatState {
  const next = { ...state, lastSeq: event.seq }
  switch (event.kind) {
    case 'user':
      return { ...next, items: [...state.items, { kind: 'user', key: `u${event.seq}`, text: event.text ?? '' }] }
    case 'agent': {
      if (!event.messageId || !event.text) return next
      let at = -1
      for (let i = state.items.length - 1; i >= 0; i--) { const item = state.items[i]; if (item.kind === 'agent' && item.key === `a${event.messageId}`) { at = i; break } }
      if (at < 0) return { ...next, items: [...state.items, { kind: 'agent', key: `a${event.messageId}`, text: event.text }] }
      const items = state.items.slice()
      const item = items[at]
      if (item.kind === 'agent') {
        const room = MAX_MESSAGE_TEXT - item.text.length
        items[at] = { ...item, text: item.text + event.text.slice(0, Math.max(0, room)) }
        if (event.text.length > Math.max(0, room)) return { ...next, items, truncated: true }
      }
      return { ...next, items }
    }
    case 'tool': {
      if (!event.toolCallId) return next
      const at = state.items.findIndex(item => item.kind === 'tool' && item.toolCallId === event.toolCallId)
      const tool: Extract<ChatItem, { kind: 'tool' }> = {
        kind: 'tool', key: `t${event.toolCallId}`, toolCallId: event.toolCallId,
        title: event.title ?? '', toolKind: event.toolKind ?? '', status: event.status ?? '',
        text: event.text ?? '', command: event.command ?? '', path: event.path ?? '', diff: event.diff ?? '',
        additions: clampCount(event.additions), deletions: clampCount(event.deletions),
        durationMs: clampDuration(event.durationMs), locations: event.locations ?? [],
      }
      if (at < 0) {
        // A new tool after thinking means the reasoning phase is over; the thought is not still running.
        const items = tool.toolKind !== 'think' ? settleThoughts(state.items) : state.items
        return { ...next, items: [...items, tool] }
      }
      const items = tool.toolKind !== 'think' ? settleThoughts(state.items) : state.items.slice()
      const old = items[at]
      if (old.kind === 'tool') {
        // A thought streams as deltas; a tool's content is its latest state.
        const thought = tool.toolKind === 'think' && old.toolKind === 'think'
        const text = thought ? appendCapped(old.text, tool.text) : (tool.text || old.text)
        items[at] = {
          ...old, title: tool.title || old.title, toolKind: tool.toolKind || old.toolKind, status: tool.status || old.status,
          text, command: tool.command || old.command, path: tool.path || old.path, diff: tool.diff || old.diff,
          additions: tool.additions || old.additions, deletions: tool.deletions || old.deletions,
          durationMs: tool.durationMs || old.durationMs, locations: tool.locations.length ? tool.locations : old.locations,
        }
        if (thought && text.length !== old.text.length + tool.text.length) return { ...next, items, truncated: true }
      }
      return { ...next, items }
    }
    case 'notice':
      return { ...next, items: [...state.items, { kind: 'notice', key: `n${event.seq}`, text: event.text ?? '', bad: event.bad }] }
    case 'status': {
      const status = { state: event.state ?? '', detail: event.detail ?? '' }
      const stopped = status.state === 'cancelled' || status.state === 'idle' || status.state === 'error' || status.state === 'ended'
      // Thinking belongs to the working phase: once the turn waits, finishes or stops, the thought is finished.
      const settled = status.state !== 'working' && status.state !== 'starting' ? settleThoughts(state.items) : state.items
      if (!stopped) return settled === state.items ? { ...next, status } : { ...next, status, items: settled }
      return { ...next, status, items: stopTools(cancelPending(settled, status.state === 'ended' ? 'ended' : status.state)) }
    }
    case 'permission': {
      if (!event.permissionId) return next
      const at = state.items.findIndex(item => item.kind === 'permission' && item.permissionId === event.permissionId)
      const prompt: Extract<ChatItem, { kind: 'permission' }> = {
        kind: 'permission', key: `p${event.permissionId}`, permissionId: event.permissionId,
        title: event.title ?? 'OpenCode tool call', toolKind: event.toolKind ?? '',
        command: event.command ?? '', path: event.path ?? '', diff: event.diff ?? '',
        locations: event.locations ?? [], truncated: !!event.truncated,
      }
      if (at >= 0) {
        const items = state.items.slice()
        items[at] = { ...prompt, decision: (state.items[at] as Extract<ChatItem, { kind: 'permission' }>).decision }
        return { ...next, items }
      }
      return { ...next, items: [...state.items, prompt] }
    }
    case 'permission_resolved': {
      if (!event.permissionId) return next
      return { ...next, items: state.items.map(item => item.kind === 'permission' && item.permissionId === event.permissionId ? { ...item, decision: event.allow ? 'allowed' : 'denied' } : item) }
    }
    case 'permission_cancelled': {
      if (!event.permissionId) return next
      return { ...next, items: state.items.map(item => item.kind === 'permission' && item.permissionId === event.permissionId ? { ...item, decision: 'cancelled', reason: event.reason } : item) }
    }
    case 'question': {
      if (!event.questionId) return next
      const question = readQuestion(event.question)
      if (!question) return next
      const at = state.items.findIndex(item => item.kind === 'question' && item.questionId === event.questionId)
      const prompt: Extract<ChatItem, { kind: 'question' }> = {
        kind: 'question', key: `q${event.questionId}`, questionId: event.questionId, toolCallId: event.toolCallId ?? '', question,
      }
      if (at >= 0) {
        const items = state.items.slice()
        const old = state.items[at] as Extract<ChatItem, { kind: 'question' }>
        items[at] = { ...prompt, decision: old.decision, reason: old.reason }
        return { ...next, items }
      }
      return { ...next, items: [...state.items, prompt] }
    }
    case 'question_resolved': {
      if (!event.questionId) return next
      const decision: QuestionDecision = event.reason === 'decline' ? 'declined' : 'answered'
      return { ...next, items: state.items.map(item => item.kind === 'question' && item.questionId === event.questionId ? { ...item, decision } : item) }
    }
    case 'question_cancelled': {
      if (!event.questionId) return next
      return { ...next, items: state.items.map(item => item.kind === 'question' && item.questionId === event.questionId ? { ...item, decision: 'cancelled', reason: event.reason } : item) }
    }
    default:
      return next
  }
}

// The controller validates a question before it sends one, but a malformed event must not break the whole chat view.
// Anything that is not a well-formed form is ignored, so the person is never shown a card they cannot answer.
const MAX_FIELDS = 8, MAX_OPTIONS = 12
export function readQuestion(raw: unknown): ChatQuestion | null {
  if (!raw || typeof raw !== 'object') return null
  const q = raw as Partial<ChatQuestion>
  if (typeof q.message !== 'string' || !q.message || !Array.isArray(q.fields) || !q.fields.length || q.fields.length > MAX_FIELDS) return null
  const keys = new Set<string>()
  const fields: ChatQuestionField[] = []
  for (const f of q.fields as Partial<ChatQuestionField>[]) {
    if (!f || typeof f.key !== 'string' || !f.key || keys.has(f.key)) return null
    if (f.kind !== 'text' && f.kind !== 'single' && f.kind !== 'multi') return null
    keys.add(f.key)
    const field: ChatQuestionField = { key: f.key, kind: f.kind, required: !!f.required }
    if (typeof f.title === 'string' && f.title) field.title = f.title
    if (typeof f.description === 'string' && f.description) field.description = f.description
    if (f.kind === 'text') {
      field.maxLength = Number.isInteger(f.maxLength) && f.maxLength! > 0 ? f.maxLength : 2000
    } else {
      if (!Array.isArray(f.options) || !f.options.length || f.options.length > MAX_OPTIONS) return null
      const seen = new Set<string>()
      const options: ChatQuestionOption[] = []
      for (const o of f.options as Partial<ChatQuestionOption>[]) {
        if (!o || typeof o.value !== 'string' || !o.value || seen.has(o.value) || typeof o.label !== 'string' || !o.label) return null
        seen.add(o.value)
        options.push(o.description ? { value: o.value, label: o.label, description: String(o.description) } : { value: o.value, label: o.label })
      }
      field.options = options
      if (f.kind === 'multi') {
        field.minItems = Number.isInteger(f.minItems) && f.minItems! > 0 ? f.minItems : 0
        field.maxItems = Number.isInteger(f.maxItems) && f.maxItems! > 0 ? Math.min(f.maxItems!, options.length) : options.length
      }
    }
    fields.push(field)
  }
  return { message: q.message, fields }
}

function questionChars(q: ChatQuestion): number {
  let total = q.message.length
  for (const f of q.fields) {
    total += f.key.length + (f.title?.length ?? 0) + (f.description?.length ?? 0)
    for (const o of f.options ?? []) total += o.value.length + o.label.length + (o.description?.length ?? 0)
  }
  return total
}

export type QuestionDraft = Record<string, string | string[]>
export type QuestionAnswers = Record<string, string | string[]>
export type QuestionCheck = { ok: boolean; errors: Record<string, string>; answers: QuestionAnswers }

const MAX_TEXT_BYTES = 2000
// Field keys come from the agent, so a key such as "__proto__" or "constructor" must be an ordinary name: dictionaries are
// built without a prototype, and reads take only own properties.
function dict<T>(): Record<string, T> { return Object.create(null) as Record<string, T> }
/** The value stored under an agent-provided key, or undefined when that key was never set on this object itself. */
export function own<T>(record: Record<string, T> | undefined, key: string): T | undefined {
  return record && Object.prototype.hasOwnProperty.call(record, key) ? record[key] : undefined
}
function byteLength(text: string): number { return new TextEncoder().encode(text).length }

/** The label a field is shown with: its title, else its key. */
export function fieldLabel(field: ChatQuestionField): string { return field.title || field.key }

/**
 * Check a person's draft against the question and build the answers to send. This is feedback only: the machine
 * checks every answer again and is the authority. An optional field left empty is left out, as the machine does.
 */
export function checkQuestionDraft(question: ChatQuestion, draft: QuestionDraft): QuestionCheck {
  const errors = dict<string>()
  const answers: QuestionAnswers = dict<string | string[]>()
  for (const field of question.fields) {
    const value = own(draft, field.key)
    if (field.kind === 'text') {
      const text = typeof value === 'string' ? value : ''
      if (!text.trim()) { if (field.required) errors[field.key] = 'Write an answer.'; continue }
      const limit = Math.min(field.maxLength ?? MAX_TEXT_BYTES, MAX_TEXT_BYTES)
      if (byteLength(text) > limit) { errors[field.key] = `Shorten this to ${limit} bytes or fewer.`; continue }
      answers[field.key] = text
    } else if (field.kind === 'single') {
      const choice = typeof value === 'string' ? value : ''
      if (!choice) { if (field.required) errors[field.key] = 'Choose one.'; continue }
      if (!field.options?.some(o => o.value === choice)) { errors[field.key] = 'Choose one of the options.'; continue }
      answers[field.key] = choice
    } else {
      const choices = Array.isArray(value) ? value : []
      const min = field.required ? Math.max(1, field.minItems ?? 0) : (field.minItems ?? 0)
      const max = field.maxItems ?? field.options?.length ?? 0
      if (!choices.length && !field.required) continue // left empty, so it is not answered, as the machine reads it
      if (choices.length < min) { errors[field.key] = `Choose at least ${min}.`; continue }
      if (choices.length > max) { errors[field.key] = `Choose at most ${max}.`; continue }
      if (new Set(choices).size !== choices.length || choices.some(c => !field.options?.some(o => o.value === c))) { errors[field.key] = 'Choose from the options.'; continue }
      answers[field.key] = choices
    }
  }
  return { ok: Object.keys(errors).length === 0, errors, answers }
}

/** A draft with every field empty, in the shape the form binds to. */
export function emptyDraft(question: ChatQuestion): QuestionDraft {
  const draft: QuestionDraft = dict<string | string[]>()
  for (const f of question.fields) draft[f.key] = f.kind === 'multi' ? [] : ''
  return draft
}

/** The answers a person gave, with option labels in place of values, for the transcript. */
export function summarizeAnswers(question: ChatQuestion, answers: QuestionAnswers): { key: string; label: string; text: string }[] {
  const out: { key: string; label: string; text: string }[] = []
  for (const f of question.fields) {
    const value = own(answers, f.key)
    if (value === undefined) continue
    const labelOf = (v: string) => f.options?.find(o => o.value === v)?.label ?? v
    out.push({ key: f.key, label: fieldLabel(f), text: Array.isArray(value) ? value.map(labelOf).join(', ') : labelOf(value) })
  }
  return out
}

function clampCount(value: number | undefined): number {
  return Number.isFinite(value) && value! > 0 ? Math.min(Math.floor(value!), 1 << 20) : 0
}

function clampDuration(value: number | undefined): number {
  return Number.isFinite(value) && value! >= 0 ? Math.min(Math.floor(value!), 24 * 60 * 60 * 1000) : 0
}

function settleThoughts(items: ChatItem[]): ChatItem[] {
  if (!items.some(item => item.kind === 'tool' && item.toolKind === 'think' && item.status === 'in_progress')) return items
  return items.map(item => item.kind === 'tool' && item.toolKind === 'think' && item.status === 'in_progress' ? { ...item, status: 'completed' } : item)
}

function appendCapped(text: string, add: string): string {
  const room = MAX_MESSAGE_TEXT - text.length
  return room <= 0 ? text : text + add.slice(0, room)
}

const TOOL_LABELS: Record<string, string> = {
  execute: 'Bash', read: 'Read', edit: 'Edit', delete: 'Delete', move: 'Move',
  search: 'Search', fetch: 'Fetch', think: 'Think', switch_mode: 'Mode', other: 'Tool',
}

/** The short all-caps word a tool row leads with: Bash, Read, Edit… */
/** Why a closed chat is closed, in words. A chat's live history is kept only in the machine's connector process, so what the person can do
 *  next depends on how it ended. `name` is the tool's product name. */
export function endedReason(state: string, name: string): string {
  if (state === 'lost') return `reet lost contact with this ${name} chat on the machine, for example when the connection closed or the reet service there restarted. Its live history was held in memory and is gone, so start a new chat to carry on.`
  if (state === 'ended') return `You ended this ${name} chat. Its live history is not kept after a chat ends.`
  // An exited chat is one that stopped on its own or that the person ended: the state does not say which, so it does not guess.
  return `This ${name} chat has ended. Its live history is not kept after the machine connection closes.`
}

export function toolLabel(kind: string): string {
  return TOOL_LABELS[kind] ?? (kind ? kind.charAt(0).toUpperCase() + kind.slice(1) : 'Tool')
}

export type ToolTone = 'good' | 'warn' | 'bad' | 'muted' | 'busy'
export type ToolState = { word: string; tone: ToolTone }

/** The tool's own state in the row's right-hand column. */
export function toolState(status: string): ToolState {
  switch (status) {
    case 'completed': return { word: 'Done', tone: 'good' }
    case 'failed': return { word: 'Failed', tone: 'bad' }
    case 'cancelled': return { word: 'Stopped', tone: 'muted' }
    case 'in_progress': return { word: 'Running', tone: 'busy' }
    case 'pending': return { word: 'Pending', tone: 'warn' }
    default: return { word: status || 'Done', tone: 'muted' }
  }
}

/** A path relative to the folder the session works in, as the agent names it. */
export function displayPath(path: string, dir?: string): string {
  const clean = path.replace(/\/+$/, '')
  if (!clean) return ''
  const base = (dir ?? '').replace(/\/+$/, '')
  if (base) {
    if (clean === base) return clean.split('/').pop() || clean
    if (clean.startsWith(base + '/')) return clean.slice(base.length + 1)
  }
  return clean
}

/** The target a row names: the command for Bash, else the file path, else the title. */
export function toolTarget(item: { toolKind: string; title: string; command: string; path: string; locations: string[] }, dir?: string): string {
  if (item.toolKind === 'execute') return item.command || item.title || '(no command)'
  if (item.toolKind === 'search' || item.toolKind === 'fetch') return item.title || item.command || displayPath(item.path, dir) || '(no query)'
  if (item.path) return displayPath(item.path, dir)
  if (item.locations.length) return displayPath(item.locations[0], dir)
  return item.title || '(no target)'
}

/** The last meaningful line of a thought, for the row's one-line preview. */
export function thoughtPreview(text: string, max = 120): string {
  const last = text.trimEnd().split('\n').map(line => line.trim()).filter(Boolean).at(-1) ?? ''
  return last.length > max ? `${last.slice(0, Math.max(0, max - 1))}…` : last
}

/** "1 ms", "179 ms", "1.2s", "1m 3s" — short enough for the row's meta column. */
export function formatDuration(ms: number): string {
  if (!Number.isFinite(ms) || ms < 0) return ''
  if (ms < 1000) return `${Math.round(ms)} ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(ms < 10_000 ? 1 : 0)}s`
  const seconds = Math.floor(ms / 1000)
  return `${Math.floor(seconds / 60)}m ${seconds % 60}s`
}

/** Added and removed lines in a unified diff, ignoring the file headers. */
export function diffCounts(diff: string): { additions: number; deletions: number } {
  let additions = 0, deletions = 0
  for (const line of diff.split('\n')) {
    if (line.startsWith('+++') || line.startsWith('---')) continue
    if (line.startsWith('+')) additions++
    else if (line.startsWith('-')) deletions++
  }
  return { additions, deletions }
}

export type CodeSegment = { kind: 'text'; text: string } | { kind: 'code'; text: string; lang: string }

/** Split a message into prose and fenced code so code is shown as code, not as a wall of wrapped text. */
export function textSegments(text: string): CodeSegment[] {
  const out: CodeSegment[] = []
  const fence = /```([^\n`]*)\n([\s\S]*?)```/g
  let at = 0, match: RegExpExecArray | null
  while ((match = fence.exec(text))) {
    if (match.index > at) out.push({ kind: 'text', text: text.slice(at, match.index) })
    out.push({ kind: 'code', lang: match[1].trim().split(/\s+/)[0] ?? '', text: match[2].replace(/\n$/, '') })
    at = fence.lastIndex
  }
  if (at < text.length) out.push({ kind: 'text', text: text.slice(at) })
  return out
}

export type InlineSegment = { kind: 'text' | 'code' | 'strong'; text: string }

/** Split a prose line into plain text, `inline code` and **bold**, without interpreting anything as HTML. */
export function inlineSegments(text: string): InlineSegment[] {
  const out: InlineSegment[] = []
  const pattern = /`([^`\n]+)`|\*\*([^*\n]+)\*\*/g
  let at = 0, match: RegExpExecArray | null
  while ((match = pattern.exec(text))) {
    if (match.index > at) out.push({ kind: 'text', text: text.slice(at, match.index) })
    if (match[1] !== undefined) out.push({ kind: 'code', text: match[1] })
    else out.push({ kind: 'strong', text: match[2] })
    at = pattern.lastIndex
  }
  if (at < text.length) out.push({ kind: 'text', text: text.slice(at) })
  return out
}

export type ProseBlock =
  | { kind: 'p'; text: string }
  | { kind: 'h'; level: 1 | 2 | 3; text: string }
  | { kind: 'ul'; items: string[] }
  | { kind: 'ol'; items: string[] }
  | { kind: 'quote'; text: string }

/** The small subset of markdown agent answers use: headings, bullet or numbered lists, quotes and paragraphs. */
export function proseBlocks(text: string): ProseBlock[] {
  const out: ProseBlock[] = []
  let paragraph: string[] = []
  let list: { kind: 'ul' | 'ol'; items: string[] } | null = null
  const flushParagraph = () => { if (paragraph.length) { out.push({ kind: 'p', text: paragraph.join('\n') }); paragraph = [] } }
  const flushList = () => {
    if (!list) return
    if (list.kind === 'ul') out.push({ kind: 'ul', items: list.items })
    else out.push({ kind: 'ol', items: list.items })
    list = null
  }
  for (const line of text.replace(/\r\n/g, '\n').split('\n')) {
    if (!line.trim()) { flushParagraph(); flushList(); continue }
    const heading = line.match(/^(#{1,3})\s+(.*)$/)
    if (heading) { flushParagraph(); flushList(); out.push({ kind: 'h', level: heading[1].length as 1 | 2 | 3, text: heading[2] }); continue }
    const bullet = line.match(/^\s*[-*]\s+(.*)$/)
    const ordered = line.match(/^\s*\d+[.)]\s+(.*)$/)
    if (bullet || ordered) {
      flushParagraph()
      const kind = bullet ? 'ul' : 'ol'
      if (!list || list.kind !== kind) { flushList(); list = { kind, items: [] } }
      list.items.push((bullet ?? ordered)![1])
      continue
    }
    const quote = line.match(/^\s*>\s?(.*)$/)
    if (quote) { flushParagraph(); flushList(); out.push({ kind: 'quote', text: quote[1] }); continue }
    flushList()
    paragraph.push(line)
  }
  flushParagraph(); flushList()
  return out
}

/**
 * Messages kept while the agent works, sent one at a time when its turn finishes. The queue lives only in the
 * open chat: it is never stored, and an item's payload (its attachments) belongs to the person's session.
 */
export type QueuedPrompt<T = unknown> = { id: string; text: string; payload: T }
export const MAX_QUEUED_PROMPTS = 10

/** Add a message to the end of the queue. A full queue leaves it untouched and says so. */
export function enqueuePrompt<T>(queue: readonly QueuedPrompt<T>[], item: QueuedPrompt<T>, max = MAX_QUEUED_PROMPTS): { queue: QueuedPrompt<T>[]; full: boolean } {
  if (queue.length >= max) return { queue: [...queue], full: true }
  return { queue: [...queue, item], full: false }
}

export function dropQueuedPrompt<T>(queue: readonly QueuedPrompt<T>[], id: string): QueuedPrompt<T>[] {
  return queue.filter(item => item.id !== id)
}

/** Change a queued message's text in place; an empty message is refused, so the queue never holds nothing to send. */
export function editQueuedPrompt<T>(queue: readonly QueuedPrompt<T>[], id: string, text: string): QueuedPrompt<T>[] {
  if (!text.trim()) return [...queue]
  return queue.map(item => item.id === id ? { ...item, text } : item)
}

/** Put an item back at the front (a retry), keeping every other item's order. */
export function promoteQueuedPrompt<T>(queue: readonly QueuedPrompt<T>[], id: string): QueuedPrompt<T>[] {
  const item = queue.find(entry => entry.id === id)
  return item ? [item, ...queue.filter(entry => entry.id !== id)] : [...queue]
}
