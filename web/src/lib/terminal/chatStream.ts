// The chat's live stream. One EventSource per pane; its events are batched into the reducer, and a replay settles one batch
// after the controller's reset. The pane keeps the reactive state and hands this module the way to read and write it.
import { applyChatEvent, resetChat, type ChatEvent, type ChatState } from '../chat'

/** A stream's events wait this long, or until BATCH_MAX have arrived, before they are applied together. */
export const BATCH_MS = 16
export const BATCH_MAX = 1000

/** Whether a reset says the controller dropped earlier messages. A reset with no readable body is a plain one. */
export function resetTruncated(data: string): boolean {
  try { return !!JSON.parse(data)?.truncated } catch { return false }
}

/** One event from the stream, or undefined when it cannot be read. */
export function parseChatEvent(data: string): ChatEvent | undefined {
  try { return JSON.parse(data) as ChatEvent } catch { return undefined }
}

export type ChatStreamOptions = {
  url: string
  /** The pane's chat, read fresh each time. */
  chat: () => ChatState
  setChat: (next: ChatState) => void
  /** Whether the chat has replayed once what arrived with the reset: until then, and while it reconnects, it is false. */
  setReplayed: (replayed: boolean) => void
  /** The log changed; the pane follows its new height. */
  changed: () => void
  /** Opens the stream. The browser's EventSource unless a test brings its own. */
  open?: (url: string) => EventSource
}

export function createChatStream(options: ChatStreamOptions) {
  const { chat, setChat, setReplayed, changed } = options
  let source: EventSource | undefined
  let alive = true
  // Events are batched before they reach the reducer: a fast stream, or a continued chat replaying its whole history,
  // arrives as many events in a burst, and applying and drawing each one separately is what used to stall the tab.
  let pendingEvents: ChatEvent[] = []
  let flushTimer: ReturnType<typeof setTimeout> | undefined
  let settleTimer: ReturnType<typeof setTimeout> | undefined
  function stopSettle() { if (settleTimer !== undefined) { clearTimeout(settleTimer); settleTimer = undefined } }
  function stopFlush() {
    if (flushTimer !== undefined) { clearTimeout(flushTimer); flushTimer = undefined }
  }
  function closeStream() { source?.close(); source = undefined }
  function flushEvents() {
    stopFlush()
    if (!pendingEvents.length) return
    const events = pendingEvents
    pendingEvents = []
    const before = chat()
    let next = before
    for (const event of events) next = applyChatEvent(next, event)
    if (next === before) return
    setChat(next)
    changed()
  }
  function queueEvent(event: ChatEvent) {
    pendingEvents.push(event)
    // A backlog is applied in one pass rather than one render per event.
    if (pendingEvents.length >= BATCH_MAX) flushEvents()
    else if (flushTimer === undefined) flushTimer = setTimeout(flushEvents, BATCH_MS)
  }
  // Whether this chat's word on being busy can be trusted: the controller writes a stream's reset and its replay together, so
  // the chat has replayed once what arrived with the reset has been applied (settle, one batch later).
  function readReset(event: MessageEvent) {
    const truncated = resetTruncated(event.data)
    pendingEvents = []; stopFlush()
    setChat({ ...resetChat(truncated), connection: 'open' })
    setReplayed(false); stopSettle()
    settleTimer = setTimeout(() => { settleTimer = undefined; flushEvents(); setReplayed(true) }, BATCH_MS)
    changed()
  }
  function readEvent(event: MessageEvent) {
    const parsed = parseChatEvent(event.data)
    if (parsed !== undefined) queueEvent(parsed)
  }
  return {
    start() {
      const stream = options.open ? options.open(options.url) : new EventSource(options.url)
      source = stream
      stream.addEventListener('reset', readReset as EventListener)
      stream.addEventListener('chat', readEvent as EventListener)
      stream.onopen = () => { setChat({ ...chat(), connection: 'open' }) }
      // A dropped stream says nothing about being busy until its next reset.
      stream.onerror = () => { if (alive) { setChat({ ...chat(), connection: 'reconnecting' }); stopSettle(); setReplayed(false) } }
    },
    /** Drops what is waiting and closes the stream; nothing it holds fires afterwards. */
    stop() { alive = false; pendingEvents = []; stopFlush(); stopSettle(); closeStream() },
  }
}
