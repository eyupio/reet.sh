// Desk autosave. Edits arrive as Changes; each is applied to the local desk at once, queued, and saved after a short
// pause, one save at a time. When a save loses to a newer server copy the queued changes are replayed onto it, and any
// that no longer apply are dropped.
import { projectColour, type ProjectColour } from '../projectColour'
import { StaleDesk, type Desk } from './api'
import { MAX_PANES, MAX_TABS, closePane, movePane, preset, setRatio, splitPane, swapPanes, type Pane, type Split, type Tab } from './tree'

export type Change =
  | { kind: 'renameTab'; tab: string; name: string }
  | { kind: 'colourTab'; tab: string; colour: ProjectColour }
  | { kind: 'renamePane'; tab: string; pane: string; name: string }
  | { kind: 'colourPane'; tab: string; pane: string; colour: ProjectColour }
  | { kind: 'ratio'; tab: string; path: string; ratio: number }
  | { kind: 'addTab'; tab: Tab; index?: number }
  | { kind: 'addPane'; tab: string; at: string; dir: Split; pane: Pane }
  | { kind: 'closePane'; tab: string; pane: string }
  | { kind: 'closeTab'; tab: string }
  | { kind: 'reorderTabs'; ids: string[] }
  | { kind: 'preset'; tab: string; name: 'side' | 'stack' | 'grid' | 'main' | 'even' }
  | { kind: 'swap'; tab: string; a: string; b: string }
  | { kind: 'move'; tab: string; pane: string; onto: string; edge: 'left' | 'right' | 'top' | 'bottom' }
  | { kind: 'zoom'; tab: string; pane: string }
  | { kind: 'active'; tab: string; pane?: string }
  | { kind: 'start'; tab: string; pane: string; start: string }
  | { kind: 'setSession'; tab: string; pane: string; session: string }
  | { kind: 'setPlace'; tab: string; pane: string; host: string; dir: string }
  | { kind: 'renameDesk'; name: string }
  | { kind: 'colourDesk'; colour: ProjectColour }

export type SaveState = 'saved' | 'saving' | 'unsaved' | 'stale'

const MAX_NAME = 80
const MAX_START = 2000
const nameOk = (s: unknown) => typeof s === 'string' && [...s].length <= MAX_NAME
const colourOk = (c: unknown) => typeof c === 'string' && projectColour(c) === c

/** A desk's content as a string for comparing, leaving out what the server stamps or other windows move: version,
 *  position, timestamps and each pane's folder. */
function content(desk: Desk): string {
  const tabs = desk.doc.tabs.map(t => ({
    ...t,
    panes: Object.fromEntries(Object.entries(t.panes).map(([id, p]) => {
      const { dir: _dir, dirAt: _dirAt, ...rest } = p
      return [id, rest]
    })),
  }))
  const sort = (v: unknown): unknown => Array.isArray(v) ? v.map(sort)
    : v && typeof v === 'object' ? Object.fromEntries(Object.entries(v).sort(([a], [b]) => (a < b ? -1 : 1)).map(([k, x]) => [k, sort(x)]))
    : v
  return JSON.stringify(sort({ name: desk.name, colour: desk.colour, doc: { ...desk.doc, tabs } }))
}

/** The change applied to a copy of the desk, or null when it no longer applies: the tab, pane or split it names is
 *  gone, a limit would be passed, or a value is invalid. The argument is never modified. */
export function apply(desk: Desk, change: Change): Desk | null {
  try {
    return applyChange(desk, change)
  } catch {
    return null
  }
}

function applyChange(desk: Desk, c: Change): Desk | null {
  const doc = desk.doc
  const withDoc = (tabs: Tab[], active = doc.active): Desk => ({ ...desk, doc: { ...doc, tabs, active } })
  const index = (id: string) => doc.tabs.findIndex(t => t.id === id)
  const withTab = (id: string, edit: (t: Tab) => Tab | null): Desk | null => {
    const i = index(id)
    if (i < 0) return null
    const next = edit(doc.tabs[i])
    if (!next) return null
    const tabs = doc.tabs.slice()
    tabs[i] = next
    return withDoc(tabs)
  }
  const withPane = (tab: string, pane: string, edit: (t: Tab, p: Pane) => Tab | null) =>
    withTab(tab, t => (t.panes[pane] ? edit(t, t.panes[pane]) : null))
  const editPane = (t: Tab, p: Pane, patch: Partial<Pane>): Tab => ({ ...t, panes: { ...t.panes, [p.id]: { ...p, ...patch } } })

  switch (c.kind) {
    case 'renameDesk':
      return nameOk(c.name) ? { ...desk, name: c.name } : null
    case 'colourDesk':
      return colourOk(c.colour) ? { ...desk, colour: c.colour } : null
    case 'renameTab':
      return nameOk(c.name) ? withTab(c.tab, t => ({ ...t, name: c.name })) : null
    case 'colourTab':
      return colourOk(c.colour) ? withTab(c.tab, t => ({ ...t, colour: c.colour })) : null
    case 'renamePane':
      return nameOk(c.name) ? withPane(c.tab, c.pane, (t, p) => editPane(t, p, { name: c.name })) : null
    case 'colourPane':
      return colourOk(c.colour) ? withPane(c.tab, c.pane, (t, p) => editPane(t, p, { colour: c.colour })) : null
    case 'start':
      return typeof c.start === 'string' && [...c.start].length <= MAX_START ? withPane(c.tab, c.pane, (t, p) => editPane(t, p, { start: c.start })) : null
    case 'setSession':
      // A session is in one pane only: one this desk already holds elsewhere is not bound a second time.
      if (typeof c.session !== 'string' || (c.session && doc.tabs.some(t => Object.values(t.panes).some(p => p.session === c.session && !(t.id === c.tab && p.id === c.pane))))) return null
      return withPane(c.tab, c.pane, (t, p) => editPane(t, p, { session: c.session }))
    case 'setPlace':
      // The folder is stamped newer than the one it replaces, so it wins over the machine's last report.
      return typeof c.host === 'string' && c.host && typeof c.dir === 'string'
        ? withPane(c.tab, c.pane, (t, p) => editPane(t, p, { host: c.host, dir: c.dir, dirAt: Math.max(Date.now(), p.dirAt + 1) }))
        : null
    case 'ratio':
      return withTab(c.tab, t => setRatio(t, c.path, c.ratio))
    case 'addTab': {
      if (doc.tabs.length >= MAX_TABS || index(c.tab.id) >= 0 || !c.tab.id || !nameOk(c.tab.name) || !colourOk(c.tab.colour)) return null
      const tabs = doc.tabs.slice()
      const at = c.index === undefined ? tabs.length : Math.min(Math.max(0, c.index), tabs.length)
      tabs.splice(at, 0, c.tab)
      return withDoc(tabs, doc.active || c.tab.id)
    }
    case 'closeTab': {
      const i = index(c.tab)
      if (i < 0) return null
      const tabs = doc.tabs.filter(t => t.id !== c.tab)
      const active = doc.active === c.tab ? (tabs[Math.min(i, tabs.length - 1)]?.id ?? '') : doc.active
      return withDoc(tabs, active)
    }
    case 'reorderTabs': {
      const have = new Set(doc.tabs.map(t => t.id))
      const seen = new Set<string>()
      // Ids that are gone are skipped and tabs the list never heard of keep their relative order at the end.
      const ordered: Tab[] = []
      for (const id of c.ids) if (have.has(id) && !seen.has(id)) { seen.add(id); ordered.push(doc.tabs.find(t => t.id === id)!) }
      for (const t of doc.tabs) if (!seen.has(t.id)) ordered.push(t)
      return withDoc(ordered)
    }
    case 'addPane': {
      if (!nameOk(c.pane.name) || !colourOk(c.pane.colour)) return null
      return withTab(c.tab, t => (Object.keys(t.panes).length >= MAX_PANES ? null : splitPane(t, c.at, c.dir, c.pane)))
    }
    case 'closePane':
      return withTab(c.tab, t => closePane(t, c.pane))
    case 'preset':
      return withTab(c.tab, t => preset(t, c.name))
    case 'swap':
      return withTab(c.tab, t => swapPanes(t, c.a, c.b))
    case 'move':
      return withTab(c.tab, t => movePane(t, c.pane, c.onto, c.edge))
    case 'zoom':
      return withTab(c.tab, t => (c.pane === '' || t.panes[c.pane] ? { ...t, zoomed: c.pane } : null))
    case 'active': {
      if (index(c.tab) < 0) return null
      const next = withTab(c.tab, t => {
        if (c.pane === undefined) return t
        return t.panes[c.pane] ? { ...t, active: c.pane } : null
      })
      return next && { ...next, doc: { ...next.doc, active: c.tab } }
    }
  }
  return null
}

type ChannelLike = { postMessage(message: unknown): void; onmessage: ((e: { data: unknown }) => void) | null; close(): void }
type Timers = { setTimeout(fn: () => void, ms: number): unknown; clearTimeout(handle: unknown): void }

export type AutosaveOptions = {
  load(): Promise<Desk>
  save(desk: Desk, expect: number, keepalive?: boolean): Promise<Desk>
  onState(s: SaveState): void
  /** Called whenever the local desk changes because of something other than the caller's own change (a refresh, a
   *  merge onto a newer copy, a saved result). */
  onDesk?(desk: Desk): void
  delay?: number
  timers?: Timers
  /** Defaults to a BroadcastChannel('reet-desks') when the browser has one. */
  channel?: ChannelLike | null
}

export type Autosave = {
  desk(): Desk
  change(c: Change): void
  flush(keepalive?: boolean): Promise<void>
  refresh(): Promise<void>
  stop(): void
  /** Settles when the first load has. */
  ready: Promise<void>
}

const RETRY_FIRST = 1000
const RETRY_MAX = 15000

export function createAutosave(opts: AutosaveOptions): Autosave {
  const delay = opts.delay ?? 400
  const timers: Timers = opts.timers ?? { setTimeout: (fn, ms) => setTimeout(fn, ms), clearTimeout: h => clearTimeout(h as ReturnType<typeof setTimeout>) }
  const channel: ChannelLike | null = opts.channel !== undefined ? opts.channel
    : typeof BroadcastChannel === 'function' ? (new BroadcastChannel('reet-desks') as unknown as ChannelLike) : null

  let server: Desk | null = null // the last copy received from the server
  let local: Desk | null = null  // server plus every queued change
  let queue: Change[] = []       // changes not yet saved, oldest first (includes those in flight)
  let sent = 0                   // how many of the queue's first changes the save in flight carries
  // Saves that failed without a definite answer (the response may have been lost after the server committed them):
  // the desk each sent and how many of the queue's first changes it carried. Cleared by any definite answer.
  let attempts: { desk: Desk; sent: number }[] = []
  let running: Promise<void> | null = null
  let timer: unknown = null
  let timerIsRetry = false
  let failures = 0
  let staleNote = false
  let stopped = false
  let last: SaveState | '' = ''

  const emit = (s: SaveState) => {
    if (s === 'saved' && staleNote) s = 'stale'
    if (s === last) return
    last = s
    opts.onState(s)
  }
  const clearTimer = () => { if (timer !== null) { timers.clearTimeout(timer); timer = null } }
  const arm = (ms: number, retry: boolean) => {
    clearTimer()
    timerIsRetry = retry
    timer = timers.setTimeout(() => { timer = null; timerIsRetry = false; void run() }, ms)
  }

  /** Replays changes onto a desk; returns the result and whether any were dropped. */
  const replay = (onto: Desk, changes: Change[]) => {
    let desk = onto, kept: Change[] = [], dropped = false
    for (const c of changes) {
      const next = apply(desk, c)
      if (next) { desk = next; kept.push(c) } else dropped = true
    }
    return { desk, kept, dropped }
  }

  /** Takes a newer server copy and puts the queue back on top of it. */
  const rebase = (copy: Desk) => {
    attempts = []
    const r = replay(copy, queue)
    server = copy
    queue = r.kept
    sent = 0
    local = r.desk
    if (r.dropped) staleNote = true
    opts.onDesk?.(local)
  }

  function run(keepalive?: boolean): Promise<void> {
    if (running) return running
    if (stopped || !server || !local || queue.length === 0) return Promise.resolve()
    clearTimer()
    sent = queue.length
    const sentDesk = local // what this save carries; later changes move `local` on
    emit('saving')
    const p = (async () => {
      try {
        const saved = await opts.save(local!, server!.version, keepalive)
        const rest = queue.slice(sent)
        attempts = []
        failures = 0
        queue = rest
        sent = 0
        const r = replay(saved, queue)
        server = saved
        queue = r.kept
        local = r.desk
        opts.onDesk?.(local)
        try { channel?.postMessage({ id: saved.id, version: saved.version }) } catch { /* the channel is only a hint */ }
        if (queue.length) arm(delay, false)
        emit(queue.length ? 'unsaved' : 'saved')
      } catch (e) {
        if (e instanceof StaleDesk) {
          // A save that failed without an answer may have landed: if the server copy is that save's desk, its changes
          // are already in it and must not be replayed (a swap would undo itself).
          const key = content(e.desk)
          const landed = attempts.filter(a => content(a.desk) === key).pop()
          if (landed) queue = queue.slice(landed.sent)
          rebase(e.desk)
          if (queue.length) { emit('saving'); running = null; await run(keepalive); return }
          emit('saved')
          return
        }
        if (attempts.at(-1)?.desk !== sentDesk) attempts.push({ desk: sentDesk!, sent })
        if (attempts.length > 8) attempts = attempts.slice(-8)
        sent = 0
        failures++
        emit('unsaved')
        if (!stopped) arm(Math.min(RETRY_MAX, RETRY_FIRST * 2 ** (failures - 1)), true)
      }
    })()
    running = p.finally(() => { if (running === wrapped) running = null })
    const wrapped = running
    return wrapped
  }

  const refresh = async (): Promise<void> => {
    const copy = await opts.load()
    if (running) await running
    if (server && copy.version <= server.version) return
    if (!server) { server = copy; local = copy; opts.onDesk?.(copy); emit('saved'); return }
    rebase(copy)
    if (queue.length && !timer && !running) arm(delay, false)
    emit(queue.length ? 'unsaved' : 'saved')
  }

  if (channel) {
    channel.onmessage = e => {
      const m = e.data as { id?: unknown; version?: unknown } | null
      if (!m || typeof m.version !== 'number' || !server || m.id !== server.id || m.version <= server.version) return
      refresh().catch(() => { /* the next message or save will try again */ })
    }
  }

  const ready = refresh()
  ready.catch(() => { /* reported to whoever awaits ready */ })

  return {
    ready,
    desk() {
      if (!local) throw new Error('desk not loaded')
      return local
    },
    change(c) {
      if (stopped) return
      if (!local) throw new Error('desk not loaded')
      const next = apply(local, c)
      if (!next) return
      local = next
      queue.push(c)
      staleNote = false
      emit('unsaved')
      if (!running && !(timerIsRetry && timer !== null)) arm(delay, false)
    },
    async flush(keepalive) {
      clearTimer()
      for (;;) {
        if (running) await running
        if (!queue.length || stopped) return
        const before = failures
        await run(keepalive)
        if (failures > before) return
      }
    },
    refresh,
    stop() {
      stopped = true
      clearTimer()
      if (channel) { channel.onmessage = null; try { channel.close() } catch { /* already closed */ } }
    },
  }
}
