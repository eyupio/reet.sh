import { ApiError, UnauthorizedError, dismissContentPreview, getContentPins, getContentPreview, getContentPreviews, requestContentPreview, stopContentPreview, type ContentPin, type ContentPreview, type ContentPreviewCapability } from './api'
import { FALLBACK, SIGNED_OUT, asSeenFromPin, backoffDelay, describeFlowError, isSignedOut, pollDelay } from './contentReview'

// The non-visual half of the Check step: the checks the dialog holds and when each was read, the request keys, the
// watcher that reads a check again while it can change (one at a time, paused while the tab is hidden), and the actions
// that make, stop, dismiss and resume one. The shell owns the dialog, the pins and the messages; this reaches them only
// through the host it is given. Nothing here draws anything or writes to a machine.

export type CheckHost = {
  /** False once the dialog is gone: nothing may be started or applied after that. */
  alive(): boolean
  /** Whether the Check step is the one on screen. */
  inCheck(): boolean
  selectedPinId(): string
  selectedPin(): ContentPin | undefined
  /** Whether the shell holds this pin. */
  hasPin(pinId: string): boolean
  /** Make another pin's check the one on screen (resuming a check found on the machine). */
  select(pinId: string): void
  /** The shell's busy flag and message, shared with the pin actions. */
  busy: boolean
  error: string
  /** The checks could not be read; cleared by the capability read when it succeeds again. */
  checksDown: boolean
  /** Whether a check could be made now (connector ready, scope eligible, checks readable). */
  checkReady(): boolean
  /** The pin in words, for the message when it moved. */
  pinWords(pin: ContentPin): string
  reload(): Promise<boolean>
  leave(): void
  /** The sign-in is gone: the shell stops its own timers too. */
  signedOut(): void
  /** Aborted with the dialog. */
  signal: AbortSignal
}

export class CheckWatcher {
  previews = $state<ContentPreview[]>([])
  capabilities = $state<ContentPreviewCapability[]>([])
  // The browser time each response arrived, which is what freshness is judged from, never the server's timestamps.
  readAts = $state<Record<string, number>>({})
  resumeCandidate = $state<{ row: ContentPreview; at: number } | null>(null)
  watchFailed = $state(false)

  // A check belongs to one version of the pin: once the pin has moved on it is overtaken, whatever the server last said.
  check = $derived.by(() => {
    const pin = this.host.selectedPin()
    return pin ? (asSeenFromPin(this.latestFor(pin), pin) ?? undefined) : undefined
  })
  // A summary row for a captured check carries no capture, so it is not usable until it has been read in full.
  usable = $derived(!!this.check && !(this.check.state === 'captured' && !this.check.capture))
  connectorReady = $derived.by(() => {
    const pin = this.host.selectedPin()
    return !!pin && this.capabilities.find(c => c.agentId === pin.agentId)?.ready === true
  })

  private host: CheckHost
  private watchId = ''
  private watchGen = 0
  private watchWants = false
  private watchBusy = false
  // Reads of the watched check that failed in a row (backs the next one off), and checks the item read refused (404).
  private failures = 0
  private gone = new Set<string>()
  private watchTimer: ReturnType<typeof setTimeout> | undefined
  private watchAbort: AbortController | undefined
  private watchRead: (() => Promise<void>) | undefined
  private capTimer: ReturnType<typeof setTimeout> | undefined
  // Request keys stay in memory: one per pin version, kept after an uncertain create failure so a retry is the same
  // request, and dropped once the server has answered, so Check again is a new one.
  private requestKeys = new Map<string, string>()

  constructor(host: CheckHost) { this.host = host }

  latestFor = (pin: ContentPin): ContentPreview | undefined => this.previews.filter(p => p.targetId === pin.id).reduce<ContentPreview | undefined>((best, p) => (!best || p.createdAt > best.createdAt ? p : best), undefined)

  // The list is a summary: a row that has not changed keeps the full read already held for it, otherwise the summary
  // replaces it and is read in full before it is shown. Every row's arrival time is recorded.
  applyList(listed: { previews: ContentPreview[]; capabilities: ContentPreviewCapability[] }, arrived: number) {
    const held = new Map(this.previews.map(p => [p.id, p]))
    this.previews = listed.previews.map(row => {
      const old = held.get(row.id)
      return old?.capture && !row.capture && old.state === row.state && old.updatedAt === row.updatedAt && (old.reasons?.length ?? 0) === (row.reasons?.length ?? 0) ? old : row
    })
    this.capabilities = listed.capabilities
    const stamps = { ...this.readAts }
    for (const row of listed.previews) stamps[row.id] = arrived
    this.readAts = stamps
  }
  accept(p: ContentPreview, at = Date.now()) {
    this.previews = this.previews.some(x => x.id === p.id) ? this.previews.map(x => (x.id === p.id ? p : x)) : [...this.previews, p]
    this.readAts = { ...this.readAts, [p.id]: at }
  }
  forget(id: string) { this.previews = this.previews.filter(p => p.id !== id) }

  // Reading a check again and again while it can still change. One watcher at a time, fenced by the dialog's lifetime, the
  // selected pin and a generation, and cancelled with an AbortController; it stops on a final state and while the tab is
  // hidden, and the tab coming back reads at once.
  stopWatching() {
    clearTimeout(this.watchTimer); this.watchTimer = undefined
    this.watchAbort?.abort(); this.watchAbort = undefined
    this.watchGen++; this.watchId = ''; this.watchWants = false; this.watchBusy = false; this.watchRead = undefined; this.failures = 0
  }
  watch(id: string, immediate: boolean) {
    this.stopWatching()
    const host = this.host
    const gen = this.watchGen, pinId = host.selectedPinId()
    const abort = new AbortController()
    this.watchAbort = abort; this.watchId = id
    const fenced = () => !host.alive() || gen !== this.watchGen || pinId !== host.selectedPinId() || !host.inCheck()
    const schedule = (state: string) => {
      if (fenced()) return
      this.watchWants = pollDelay(state, false) !== null
      // A read that failed is tried again more and more slowly; one that worked at the normal rate.
      const delay = this.failures > 0 && !document.hidden && this.watchWants ? backoffDelay(this.failures) : pollDelay(state, document.hidden)
      if (delay !== null) this.watchTimer = setTimeout(() => void read(), delay)
    }
    const read = async () => {
      this.watchTimer = undefined
      if (fenced() || this.watchBusy || document.hidden) return
      this.watchBusy = true
      let state = this.previews.find(p => p.id === id)?.state ?? ''
      try {
        const p = await getContentPreview(id, abort.signal)
        if (fenced()) return
        this.accept(p); state = asSeenFromPin(p, host.selectedPin())?.state ?? p.state
        this.failures = 0
        if (this.watchFailed) { this.watchFailed = false; host.error = '' }
      } catch (e) {
        if (fenced() || abort.signal.aborted) return
        if (isSignedOut(e) || e instanceof UnauthorizedError) {
          // Nothing more can be read: stop, say so plainly, and do not ask again.
          this.stopWatching(); this.watchFailed = false; host.error = SIGNED_OUT; host.signedOut()
          return
        }
        if (e instanceof ApiError && e.status === 404) {
          // The list may keep a row the item read refuses: forget it, read the list once, and give up on that check if it returns.
          this.watchBusy = false
          this.forget(id)
          if (this.gone.has(id)) { this.stopWatching(); host.error = FALLBACK; return }
          this.gone.add(id)
          await host.reload()
          return
        }
        this.failures++
        host.error = describeFlowError(e); this.watchFailed = true
      } finally {
        if (gen === this.watchGen) this.watchBusy = false
      }
      schedule(state)
    }
    this.watchRead = read
    this.watchWants = true
    if (immediate) void read()
    else schedule(this.previews.find(p => p.id === id)?.state ?? '')
  }
  // Keep the watcher matched to what is on screen: nothing for a final check, a read at once for one not yet shown in full.
  syncWatch() {
    const row = this.check
    if (!this.host.alive() || !this.host.inCheck() || !this.host.selectedPin() || !row) { this.stopWatching(); return }
    if (!this.usable) { this.watch(row.id, true); return }
    if (pollDelay(row.state, false) === null) { this.stopWatching(); return }
    if (this.watchId !== row.id) this.watch(row.id, true)
  }
  retryWatch() {
    this.host.error = ''; this.watchFailed = false
    if (this.check) this.watch(this.check.id, true)
  }
  // While the connector has not recently reported in, the list is read now and then so the button comes back by itself.
  scheduleCaps() {
    clearTimeout(this.capTimer); this.capTimer = undefined
    if (!this.host.alive() || !this.host.inCheck() || this.connectorReady || document.hidden) return
    this.capTimer = setTimeout(async () => {
      this.capTimer = undefined
      if (!this.host.alive() || !this.host.inCheck()) return
      try {
        const listed = await getContentPreviews(this.host.signal)
        if (this.host.alive()) { this.applyList(listed, Date.now()); this.host.checksDown = false }
      } catch (e) {
        if (isSignedOut(e)) { this.host.error = SIGNED_OUT; this.host.signedOut(); return }
        /* the next try will do */
      }
      this.scheduleCaps()
    }, 10000)
  }
  // Coming back to the tab reads a check that can still change at once, and the connector's readiness again.
  visibilityChanged() {
    if (document.hidden) return
    this.scheduleCaps()
    if (this.watchWants && !this.watchBusy && this.watchRead) { clearTimeout(this.watchTimer); this.watchTimer = undefined; void this.watchRead() }
  }
  /** The person opened a pin's check (or left it). */
  opened() { this.resumeCandidate = null; this.watchFailed = false; this.syncWatch(); this.scheduleCaps() }
  left() { this.stopWatching(); clearTimeout(this.capTimer); this.capTimer = undefined; this.resumeCandidate = null; this.watchFailed = false }
  /** The dialog is closing or gone. */
  dispose() { this.stopWatching(); clearTimeout(this.capTimer); this.capTimer = undefined }

  private newKey(): string {
    const c = globalThis.crypto
    if (typeof c?.randomUUID === 'function') return `chk-${c.randomUUID()}`
    const bytes = c.getRandomValues(new Uint8Array(16))
    return `chk-${[...bytes].map(b => b.toString(16).padStart(2, '0')).join('')}`
  }
  private slotOf = (pin: ContentPin) => `${pin.id}:${pin.version}`

  async startCheck(): Promise<void> {
    const host = this.host
    const pin = host.selectedPin()
    if (host.busy || !pin || !host.checkReady()) return
    host.busy = true; host.error = ''; this.resumeCandidate = null
    const pinId = pin.id
    let slot = ''
    try {
      // The pin is read again first, so a check is never made for a pin that is not the one on screen.
      const fresh = (await getContentPins(host.signal)).targets.find(p => p.id === pinId)
      if (!host.alive() || host.selectedPinId() !== pinId) return
      if (!fresh || fresh.version !== pin.version) {
        await host.reload()
        if (!host.alive()) return
        const now = host.selectedPin()?.id === pinId ? host.selectedPin() : undefined
        if (!now) host.leave()
        host.error = !now ? `The pin for ${pin.name} was removed elsewhere, so no check was made.` : `The pin for ${pin.name} changed elsewhere, so no check was made. It is now ${host.pinWords(now)}. Check again to check it as it is now.`
        return
      }
      slot = this.slotOf(fresh)
      let key = this.requestKeys.get(slot)
      if (!key) { key = this.newKey(); this.requestKeys.set(slot, key) }
      const made = await requestContentPreview({ requestKey: key, targetId: fresh.id, expectedTargetVersion: fresh.version })
      this.requestKeys.delete(slot)
      if (!host.alive() || host.selectedPinId() !== pinId) return
      this.accept(made)
      this.watch(made.id, false)
    } catch (e) {
      if (host.alive() && host.selectedPinId() === pinId) await this.createFailed(e, pin, slot)
    } finally {
      if (host.alive()) host.busy = false
    }
  }
  private async createFailed(e: unknown, pin: ContentPin, slot: string) {
    const host = this.host
    host.error = describeFlowError(e)
    if (e instanceof ApiError) {
      // The server answered, so that key is spent. Every refusal to make a check is a 409, whether the machine is busy, the
      // pin moved or the connector has not reported in; only an active check on this connector makes it a busy answer.
      this.requestKeys.delete(slot)
      if (e.status === 409) await this.whenRefused(pin)
      return
    }
    if (e instanceof UnauthorizedError || slot === '') return
    // No reply arrived, so the check may exist: look for it by its key. If it cannot be found or the list cannot be read,
    // the key is kept and the next press sends the same request.
    const key = this.requestKeys.get(slot)
    try {
      const listed = await getContentPreviews(host.signal)
      const arrived = Date.now()
      if (!host.alive() || host.selectedPinId() !== pin.id) return
      const made = key ? listed.previews.find(p => p.requestKey === key) : undefined
      this.applyList(listed, arrived)
      if (made) { this.requestKeys.delete(slot); host.error = ''; this.syncWatch() }
    } catch { /* keep the key and the message */ }
  }
  private async whenRefused(pin: ContentPin) {
    const host = this.host
    try {
      const listed = await getContentPreviews(host.signal)
      const arrived = Date.now()
      if (!host.alive() || host.selectedPinId() !== pin.id) return
      this.capabilities = listed.capabilities
      const active = listed.previews.find(p => (p.state === 'queued' || p.state === 'capturing') && p.agentId === pin.agentId)
      // The shell's pins decide whether the active check is one of ours to resume.
      if (active && host.hasPin(active.targetId)) { this.resumeCandidate = { row: active, at: arrived }; host.error = `This machine is already checking ${active.name}.`; return }
      if (active) host.error = `A check for ${active.name} in another folder is running on this machine. Try again when it has finished.`
    } catch { /* the message already says what the server said */ }
    // Read everything again in case the pin moved, but keep what the server said.
    const said = host.error
    await host.reload()
    if (host.alive() && !host.error) host.error = said
  }
  resumeCheck() {
    const found = this.resumeCandidate
    if (this.host.busy || !found) return
    this.resumeCandidate = null; this.host.error = ''
    this.host.select(found.row.targetId)
    this.accept(found.row, found.at)
    this.watch(found.row.id, true)
  }
  async stopCheck(): Promise<void> {
    const host = this.host, current = this.check
    if (host.busy || !current) return
    host.busy = true; host.error = ''
    try {
      const stopped = await stopContentPreview(current.id)
      if (!host.alive()) return
      this.accept(stopped); this.syncWatch()
    } catch (e) {
      if (host.alive()) { host.error = describeFlowError(e); await host.reload() }
    } finally {
      if (host.alive()) host.busy = false
    }
  }
  async dismissCheck(): Promise<void> {
    const host = this.host, current = this.check
    if (host.busy || !current) return
    host.busy = true; host.error = ''
    try {
      await dismissContentPreview(current.id)
      if (!host.alive()) return
      this.forget(current.id)
      this.syncWatch()
    } catch (e) {
      if (host.alive()) { host.error = describeFlowError(e); await host.reload() }
    } finally {
      if (host.alive()) host.busy = false
    }
  }
}
