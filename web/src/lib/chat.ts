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
  locations?: string[]
  allow?: boolean
  bad?: boolean
  truncated?: boolean
  reason?: string
  detail?: string
}

export type ChatItem =
  | { kind: 'user' | 'agent' | 'notice'; key: string; text: string; bad?: boolean }
  | { kind: 'tool'; key: string; toolCallId: string; title: string; toolKind: string; status: string; text: string; locations: string[] }
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

export function applyChatEvent(state: ChatState, event: ChatEvent): ChatState {
  if (!Number.isFinite(event.seq) || event.seq <= state.lastSeq) return state
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
      if (item.kind === 'agent') items[at] = { ...item, text: item.text + event.text }
      return { ...next, items }
    }
    case 'tool': {
      if (!event.toolCallId) return next
      const at = state.items.findIndex(item => item.kind === 'tool' && item.toolCallId === event.toolCallId)
      const tool: Extract<ChatItem, { kind: 'tool' }> = {
        kind: 'tool', key: `t${event.toolCallId}`, toolCallId: event.toolCallId,
        title: event.title ?? '', toolKind: event.toolKind ?? '', status: event.status ?? '',
        text: event.text ?? '', locations: event.locations ?? [],
      }
      if (at < 0) return { ...next, items: [...state.items, tool] }
      const items = state.items.slice()
      const old = items[at]
      if (old.kind === 'tool') items[at] = { ...old, title: tool.title || old.title, toolKind: tool.toolKind || old.toolKind, status: tool.status || old.status, text: tool.text || old.text, locations: tool.locations.length ? tool.locations : old.locations }
      return { ...next, items }
    }
    case 'notice':
      return { ...next, items: [...state.items, { kind: 'notice', key: `n${event.seq}`, text: event.text ?? '', bad: event.bad }] }
    case 'status': {
      const status = { state: event.state ?? '', detail: event.detail ?? '' }
      const stopped = status.state === 'cancelled' || status.state === 'idle' || status.state === 'error' || status.state === 'ended'
      return { ...next, status, items: stopped ? cancelPending(state.items, status.state === 'ended' ? 'ended' : status.state) : state.items }
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
