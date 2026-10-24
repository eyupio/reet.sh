import { terminalSessions, terminalTicket, TerminalError } from './api'

export type ConnectionState = 'connecting' | 'resynchronising' | 'live' | 'reconnecting' | 'detached' | 'taken_over' | 'ended' | 'lost' | 'access_denied'
export type Renderer = { write: (bytes: Uint8Array, reset: boolean, done: () => void) => void }
export type ConnectionOptions = { session: string; renderer: Renderer; dimensions: () => { cols: number; rows: number }; onstate: (state: ConnectionState, message: string) => void }
const frameLimit = 32 * 1024, queueLimit = 256 * 1024

// One connection controller per visible logical session. It never creates a
// shell, saves terminal bytes, queues disconnected input or retries keystrokes.
export class TerminalConnection {
  private options: ConnectionOptions
  private epoch = 0
  private socket: WebSocket | null = null
  private request: AbortController | null = null
  private retry: ReturnType<typeof setTimeout> | null = null
  private backoff = 1000
  private state: ConnectionState = 'detached'
  private disposed = false
  private manual = false
  private ready = false
  private reset = true
  private queue: { bytes: Uint8Array; epoch: number }[] = []
  private queued = 0
  private inFlight = 0

  constructor(options: ConnectionOptions) { this.options = options }
  private set(state: ConnectionState, message = '') { this.state = state; this.options.onstate(state, message) }
  private clear() {
    if (this.retry) clearTimeout(this.retry)
    this.retry = null
    this.request?.abort(); this.request = null
    const old = this.socket; this.socket = null
    old?.close()
    this.ready = false; this.queue = []; this.queued = 0
  }
  async connect(takeover = false) {
    if (this.disposed) return
    const epoch = ++this.epoch
    this.clear(); this.manual = false
    this.set(this.backoff > 1000 ? 'reconnecting' : 'connecting')
    const request = new AbortController(); this.request = request
    try {
      const sessions = await terminalSessions(request.signal)
      if (epoch !== this.epoch || this.disposed) return
      const session = sessions.find((item) => item.id === this.options.session)
      if (!session) { this.set('lost', 'This shell is no longer available. No replacement was created.'); return }
      if (session.state !== 'running') {
        if (session.state === 'creating') { this.schedule(epoch, 'Waiting for the original creation to be reconciled.'); return }
        if(session.state==='lost'){this.set('lost','The host can no longer find this shell. No replacement was created.');return}
        this.set('ended', session.state === 'end_requested' ? 'Ending when the machine reconnects.' : `This shell ${session.state === 'exited' ? 'exited' : 'ended'}.`); return
      }
      const size = this.options.dimensions()
      const ticket = await terminalTicket(this.options.session, size.cols, size.rows, takeover, request.signal)
      if (epoch !== this.epoch || this.disposed) return
      const url = new URL('/api/v2/terminal/socket', location.origin); url.protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
      const socket = new WebSocket(url); this.socket = socket; socket.binaryType = 'arraybuffer'
      socket.onopen = () => { if (epoch === this.epoch) socket.send(JSON.stringify({ ticket })) }
      socket.onmessage = (event) => {
        if (epoch !== this.epoch || this.disposed) return
        if (typeof event.data === 'string') {
          let message: { type?: string }
          try { message = JSON.parse(event.data) } catch { this.interrupt(epoch, 'Invalid stream response.'); return }
          if (message.type !== 'ready' || this.ready) { this.interrupt(epoch, 'Invalid stream response.'); return }
          this.ready = true; this.reset = true; this.set('resynchronising', 'Restoring the existing shell…'); return
        }
        if (!(event.data instanceof ArrayBuffer) || !this.ready) { this.interrupt(epoch, 'Invalid terminal frame.'); return }
        const bytes = new Uint8Array(event.data)
        if (!bytes.length || bytes.length > frameLimit || this.queued + this.inFlight + bytes.length > queueLimit) { this.interrupt(epoch, 'Output exceeded the display buffer. Reconnecting to resynchronise.'); return }
        this.queue.push({ bytes, epoch }); this.queued += bytes.length; this.drain()
      }
      socket.onclose = () => { if (epoch === this.epoch && !this.manual && !this.disposed) this.schedule(epoch, 'Connection interrupted. Your shell keeps running; input is paused.') }
      socket.onerror = () => { /* onclose supplies the recovery path */ }
    } catch (error) {
      if (epoch !== this.epoch || request.signal.aborted || this.disposed) return
      if (error instanceof TerminalError && (error.status === 401 || error.status === 403)) this.set('access_denied', 'Access is no longer available. Sign in again or check machine permissions.')
      else if (error instanceof TerminalError && error.status === 409) this.set('taken_over', 'This shell is open elsewhere. Choose Continue here to transfer control.')
      else this.schedule(epoch, 'The machine is reconnecting. Your existing shell is preserved.')
    }
  }
  private drain() {
    if (this.inFlight || this.disposed) return
    const item = this.queue.shift(); if (!item) return
    this.queued -= item.bytes.length; this.inFlight = item.bytes.length
    const reset = item.epoch === this.epoch && this.reset
    if (reset) this.reset = false
    this.options.renderer.write(item.bytes, reset, () => {
      this.inFlight = 0
      if (item.epoch === this.epoch && this.ready && !this.disposed && this.socket?.readyState === WebSocket.OPEN) { this.backoff = 1000; if(this.state!=='live')this.set('live') }
      this.drain()
    })
  }
  private interrupt(epoch: number, message: string) { if (epoch !== this.epoch) return; this.socket?.close(); this.schedule(epoch, message) }
  private schedule(epoch: number, message: string) {
    if (epoch !== this.epoch || this.retry || this.disposed || this.manual) return
    this.ready = false; this.queue = []; this.queued = 0
    this.set('reconnecting', message)
    const delay = this.backoff * (0.8 + Math.random() * 0.4); this.backoff = Math.min(15000, this.backoff * 2)
    this.retry = setTimeout(() => { this.retry = null; void this.connect() }, delay)
  }
  send(bytes: Uint8Array): boolean {
    const socket = this.socket
    if (this.state !== 'live' || !socket || socket.readyState !== WebSocket.OPEN || !bytes.length) return false
    if (bytes.length + socket.bufferedAmount > queueLimit) { this.interrupt(this.epoch, 'Input was too large to send safely. It was not queued or replayed.'); return false }
    try { for (let offset = 0; offset < bytes.length; offset += frameLimit) socket.send(new Uint8Array(bytes.subarray(offset, offset + frameLimit)).buffer); return true } catch { this.interrupt(this.epoch, 'Input delivery was interrupted. It will not be replayed.'); return false }
  }
  text(text: string): boolean { return this.send(new TextEncoder().encode(text)) }
  resize(cols: number, rows: number) { if (this.state === 'live' && this.socket?.readyState === WebSocket.OPEN) {try {this.socket.send(JSON.stringify({ type: 'resize', cols, rows }))}catch{this.interrupt(this.epoch,'Resize interrupted; restoring the existing shell.')}} }
  recover() { if (this.state === 'reconnecting' || this.state === 'connecting') { this.backoff = 1000; void this.connect() } }
  detach() { this.manual = true; ++this.epoch; this.clear(); this.set('detached', 'Disconnected from this browser. The shell keeps running on the machine.') }
  dispose() { this.disposed = true; this.manual = true; ++this.epoch; this.clear() }
}
