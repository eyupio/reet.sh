import { displayPath, isWorking, pendingPermissions, pendingQuestions, toolTarget, type ChatItem, type ChatState } from './chat'

export type GafferSummary = {
  scope: 'turn' | 'session'
  handover: string
  completed: number
  failed: number
  stopped: number
  files: string[]
  commands: { key: string; target: string; status: string }[]
  attention: string[]
  next: string
  partial: boolean
}

/** A bounded plain-text excerpt of the agent's own handover, never a generated claim of success. */
function excerpt(text: string): string {
  const plain = text.replace(/```[\s\S]*?(?:```|$)/g, '')
    .replace(/!?\[([^\]]*)\]\([^)]*\)/g, '$1')
    .replace(/^\s*(?:#{1,6}\s+|[-*+]\s+|\d+[.)]\s+|>\s*)/gm, '')
    .replace(/[*`]/g, '').replace(/\s+/g, ' ').trim()
  if (plain.length <= 540) return plain
  const short = plain.slice(0, 539)
  const boundary = short.lastIndexOf(' ')
  return `${short.slice(0, boundary > 400 ? boundary : short.length)}…`
}

/** Live turn summaries cover the last prompt; a closed session covers all retained activity. */
export function gafferSummary(chat: ChatState, dir = '', replayed = true, ended = false): GafferSummary | null {
  const session = ended || chat.status.state === 'ended'
  if (!replayed || (!session && (chat.connection !== 'open' || isWorking(chat) || pendingPermissions(chat).length || pendingQuestions(chat).length))) return null
  if (!session && !['idle', 'error', 'cancelled'].includes(chat.status.state)) return null
  let promptAt = -1, lastActivity = -1
  for (let i = chat.items.length - 1; i >= 0; i--) {
    const item = chat.items[i]
    if (lastActivity < 0 && (item.kind === 'user' || item.kind === 'tool')) lastActivity = i
    if (item.kind === 'user') { promptAt = i; break }
  }
  if (!chat.items.length || (!session && promptAt < 0 && !chat.truncated)) return null
  const items = session ? chat.items : chat.items.slice(promptAt + 1)
  const tools = items.filter((item): item is Extract<ChatItem, { kind: 'tool' }> => item.kind === 'tool' && item.toolKind !== 'think')
  const completed = tools.filter(item => item.status === 'completed').length
  const failed = tools.filter(item => item.status === 'failed').length
  const stopped = tools.filter(item => !['completed', 'failed'].includes(item.status)).length
  const files = new Set<string>()
  for (const tool of tools) {
    if (tool.status !== 'completed' || !['edit', 'delete', 'move'].includes(tool.toolKind)) continue
    for (const path of [tool.path, ...tool.locations]) if (path) files.add(displayPath(path, dir))
  }
  // An earlier plan or a previous turn's answer must not become the final handover for unfinished work.
  const reply = chat.items.slice(lastActivity + 1).reverse().find(item => item.kind === 'agent' && item.text.trim())
  const handover = reply?.kind === 'agent' ? excerpt(reply.text) : ''
  const attention: string[] = []
  if (chat.status.state === 'error') attention.push(chat.status.detail || 'The agent stopped with an error. See the conversation for details.')
  if (chat.status.state === 'cancelled') attention.push('Work was stopped before the agent finished.')
  if (failed) attention.push(`${failed} tool call${failed === 1 ? '' : 's'} reported failure. Review the output for any unresolved issues.`)
  if (stopped) attention.push(`${stopped} tool call${stopped === 1 ? '' : 's'} ended without a completed result.`)
  return {
    scope: session ? 'session' : 'turn', handover, completed, failed, stopped, files: [...files],
    commands: tools.filter(tool => tool.toolKind === 'execute').map(tool => ({ key: tool.key, target: toolTarget(tool, dir), status: ['completed', 'failed'].includes(tool.status) ? tool.status : 'cancelled' })),
    attention, partial: chat.truncated,
    next: session ? 'Review the handover and any snags. Start a new session to carry on.'
      : attention.length ? 'Review the snags and partial work, then send your next instruction.'
      : 'Review the handover and changes, then tell us what’s next.',
  }
}
