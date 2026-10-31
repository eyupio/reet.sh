// The chat reducer turns the controller's bounded SSE events into the items a
// person reads. It is pure so the streaming, approval and reconnect cases can be
// tested without a browser or a live OpenCode process.

export type ChatEvent = {
  seq: number
  kind: 'user' | 'agent' | 'tool' | 'notice' | 'status' | 'permission' | 'permission_resolved' | 'permission_cancelled'
  messageId?: string
  toolCallId?: string
  permissionId?: string
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

export type ChatItem =
  | { kind: 'user' | 'agent' | 'notice'; key: string; text: string; bad?: boolean }
  | { kind: 'tool'; key: string; toolCallId: string; title: string; toolKind: string; status: string; text: string; command: string; path: string; diff: string; additions: number; deletions: number; durationMs: number; locations: string[] }
  | { kind: 'permission'; key: string; permissionId: string; title: string; toolKind: string; command: string; path: string; diff: string; locations: string[]; truncated: boolean; decision?: 'allowed' | 'denied' | 'cancelled'; reason?: string }

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

function cancelPending(items: ChatItem[], reason: string): ChatItem[] {
  return items.map(item => item.kind === 'permission' && !item.decision ? { ...item, decision: 'cancelled' as const, reason } : item)
}

function itemChars(item: ChatItem): number {
  if (item.kind === 'tool') return item.title.length + item.text.length + item.locations.join('').length
  if (item.kind === 'permission') return item.title.length + item.command.length + item.path.length + item.diff.length + item.locations.join('').length
  return item.text.length
}

/** Trim the oldest items until the log fits its limits, keeping a pending approval reachable. */
export function trimChat(items: ChatItem[], truncated: boolean): { items: ChatItem[]; truncated: boolean } {
  let total = 0
  for (const item of items) total += itemChars(item)
  let cut = 0
  while (cut < items.length && (items.length - cut > MAX_ITEMS || total > MAX_TEXT)) {
    const item = items[cut]
    if (item.kind === 'permission' && !item.decision) break
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
      return { ...next, status, items: cancelPending(settled, status.state === 'ended' ? 'ended' : status.state) }
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
    default:
      return next
  }
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
