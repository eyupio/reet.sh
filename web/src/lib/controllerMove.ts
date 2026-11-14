// Moving a controller to a new machine or a new operator, as the Backups page and the shell read it.
//
// The controller decides everything that matters: what a fence refuses, whether a copy is the final one, whether an
// import is safe. This module only reads its answers into what a page shows, and checks what a person typed early
// enough to say so beside the field. It imports nothing, so a test runs it without a browser.

export type Fence = { fenced: boolean; since: number; movedTo: string }
export const NOT_FENCED: Fence = { fenced: false, since: 0, movedTo: '' }

/** What a copy says of its source, as a list or a run answers it. */
export type MarkedCopy = { id: string; takenAt: number; local: boolean; fenced?: boolean }

export type MoveFeatures = {
  /** The fence routes exist, so this controller can be told to stand still. */
  fence: boolean
  /** A copy records whether its source was fenced, and an import refuses one that was not. */
  copyOrigin: boolean
  /** An import takes the address the controller will answer at. */
  publicUrl: boolean
  /** A copy can be sealed with a password sent for that copy alone. */
  copyPassword: boolean
  /** The new address can be given or changed while the fence stands. */
  fenceAddress: boolean
  /** The fence reports which machines have followed it. */
  fenceMachines: boolean
}

/**
 * Which parts of a move this controller holds, from GET /api/version. A controller built before the list existed
 * sends no field at all, and that reads as none: a page must not offer a step the server would not understand.
 */
export function moveFeatures(features: unknown): MoveFeatures {
  const has = (name: string) => Array.isArray(features) && features.includes(name)
  return {
    fence: has('fence'), copyOrigin: has('copy-origin'), publicUrl: has('restore-public-url'), copyPassword: has('copy-password-request'),
    fenceAddress: has('fence-address'), fenceMachines: has('fence-machines'),
  }
}

export type FenceWatch = {
  get(): Fence
  set(next: Fence): void
  /** Calls fn whenever the state changes, and returns the call that stops it. */
  subscribe(fn: (fence: Fence) => void): () => void
}

/**
 * The fence as the open tab last heard it. Three places learn it (the shell's own read, the Backups page raising or
 * lifting it, and any write refused with 423), and one banner shows it, so they meet here rather than each asking.
 */
export function createFenceWatch(): FenceWatch {
  let current = NOT_FENCED
  const listeners = new Set<(fence: Fence) => void>()
  return {
    get: () => current,
    set(next) {
      const value = next.fenced ? { fenced: true, since: next.since, movedTo: next.movedTo } : NOT_FENCED
      if (value.fenced === current.fenced && value.since === current.since && value.movedTo === current.movedTo) return
      current = value
      for (const fn of listeners) fn(current)
    },
    subscribe(fn) {
      listeners.add(fn)
      return () => listeners.delete(fn)
    },
  }
}

export const fenceWatch = createFenceWatch()

/**
 * The fence a refused write describes. Only a 423 that names itself counts: another 423, or a body that is not the
 * controller's own, says nothing about a move and must not paint a banner.
 */
export function fenceFromRefusal(status: number, body: unknown): Fence | null {
  if (status !== 423 || !body || typeof body !== 'object') return null
  const b = body as Record<string, unknown>
  if (b.code !== 'controller_moved') return null
  return { fenced: true, since: typeof b.since === 'number' ? b.since : 0, movedTo: typeof b.movedTo === 'string' ? b.movedTo : '' }
}

/** The address as it is sent: trimmed, and without the trailing slash a pasted address often carries. */
export function cleanAddress(text: string): string {
  return text.trim().replace(/\/+$/, '')
}

/**
 * What is wrong with a controller address, in words for beside the field, or '' when it is empty or fine. The same
 * address ends up in `reet agent set-server` and in the saved addresses, both of which take a plain origin, so a path
 * or a query is caught here rather than three steps later on a machine.
 */
export function addressProblem(text: string): string {
  const value = cleanAddress(text)
  if (!value) return ''
  let url: URL
  try {
    url = new URL(value)
  } catch {
    return 'Use a full address, such as https://reet.example.com.'
  }
  if (url.protocol !== 'https:' && url.protocol !== 'http:') return 'The address starts with https:// or http://.'
  if (url.username || url.password) return 'Leave the user name and password out of the address.'
  if (url.pathname !== '/' || url.search || url.hash) return 'Use the address alone, with nothing after the host name.'
  return ''
}

export const COPY_PASSWORD_MIN = 12

/** The controller's own rule for a copy password, said before the request rather than after it. */
export function copyPasswordProblem(password: string): string {
  if (password.trim().length < COPY_PASSWORD_MIN) return `A copy password needs at least ${COPY_PASSWORD_MIN} characters.`
  return ''
}

/**
 * Why machines will not follow this address on their own, or '' when they will. A machine sends its credentials over
 * HTTPS only, except to a controller on the machine itself, so a plain http address elsewhere is fenced but not
 * followed.
 */
export function followProblem(text: string): string {
  const value = cleanAddress(text)
  if (!value || addressProblem(value)) return ''
  const url = new URL(value)
  if (url.protocol === 'https:') return ''
  const local = url.hostname === 'localhost' || url.hostname === '127.0.0.1' || url.hostname === '[::1]'
  return local ? '' : 'Machines only follow an https:// address on their own, so with this one each would need moving by hand.'
}

/** One enrolled machine as the fenced controller has heard from it. */
export type FenceMachine = {
  id: string; host: string; owner: string; lastSeen: number; toldAt: number; leftAt: number; leftTo: string; held: string
}

/** How long a machine turned away by the fence counts as still asking: the agent asks every five seconds. */
export const ASKING_WINDOW_MS = 2 * 60 * 1000

/**
 * Where one machine is in the move. moved: it said it now points at the new controller. held: it said it will not
 * follow on its own. waiting: the fence is turning it away now, so it knows the address and is waiting for the new
 * controller to answer. quiet: it was told, then stopped asking without saying it moved (switched off, or a build too
 * old to say). unseen: it has not asked since the fence went up, so it has not been told.
 */
export type MoveState = 'moved' | 'held' | 'waiting' | 'quiet' | 'unseen'

export function machineMoveState(m: FenceMachine, now: number): MoveState {
  if (m.leftAt > 0) return 'moved'
  if (m.held) return 'held'
  if (m.toldAt > 0) return now - m.toldAt <= ASKING_WINDOW_MS ? 'waiting' : 'quiet'
  return 'unseen'
}

/** What a state means for the owner, in a few words. */
export function moveStateLine(m: FenceMachine, now: number): string {
  switch (machineMoveState(m, now)) {
    case 'moved': return m.leftTo ? `Moved to ${m.leftTo}` : 'Moved'
    case 'held': return m.held === 'follow-off'
      ? 'Following is turned off on this machine; run reet agent set-server there'
      : 'Will not follow this address on its own; give an https:// address'
    case 'waiting': return 'Has the new address; waiting for the new controller to answer'
    case 'quiet': return 'Was given the new address, then went quiet; it carries on when it is back'
    case 'unseen': return 'Not heard from since the fence; it picks up the address when its agent next runs'
  }
}

export type MoveProgress = Record<MoveState, number> & { total: number; done: boolean }

/** How far the move has got across every machine. Done once every machine says it has moved. */
export function moveProgress(machines: FenceMachine[], now: number): MoveProgress {
  const out: MoveProgress = { moved: 0, held: 0, waiting: 0, quiet: 0, unseen: 0, total: machines.length, done: false }
  for (const m of machines) out[machineMoveState(m, now)]++
  out.done = out.total > 0 && out.moved === out.total
  return out
}

/**
 * What a fence means for the reader, to follow "This controller is fenced." in a banner. `when` formats the moment in
 * the reader's own settings.
 */
export function fenceSentence(fence: Fence, when: (ms: number) => string): string {
  if (!fence.fenced) return ''
  const since = fence.since ? ` since ${when(fence.since)}` : ''
  const to = fence.movedTo ? ` It has moved to ${fence.movedTo}.` : ''
  return `Every change has been refused here${since}.${to}`
}

/** What a copy says about the controller it was taken from. Absent is not the same as live: an older copy says nothing. */
export function copySource(item: { fenced?: boolean }): 'fenced' | 'live' | 'unknown' {
  if (item.fenced === true) return 'fenced'
  if (item.fenced === false) return 'live'
  return 'unknown'
}

/**
 * The copy to carry to the new controller: the newest one on this machine that says it was taken fenced, and taken
 * since this fence went up. A fenced copy from an earlier, lifted fence is older than what the controller did after
 * the lift, so it does not count.
 */
export function finalCopy<T extends MarkedCopy>(items: T[], fence: Fence): T | null {
  if (!fence.fenced) return null
  let best: T | null = null
  for (const item of items) {
    if (!item.local || item.fenced !== true || item.takenAt < fence.since) continue
    if (!best || item.takenAt > best.takenAt) best = item
  }
  return best
}

export type MoveReport = {
  publicUrl?: string; mailBaseRewritten?: boolean; syncOriginsRewritten?: number; mcpGrantsStale?: number; paused?: string[]
}

/** What an import did about the controller's address, in the report's own order. */
export function addressLines(report: MoveReport): string[] {
  if (!report.publicUrl) return ['Saved addresses were left as they were.']
  const origins = report.syncOriginsRewritten ?? 0
  const grants = report.mcpGrantsStale ?? 0
  return [
    `Saved addresses now point to ${report.publicUrl}.`,
    report.mailBaseRewritten ? 'The mail base URL was changed.' : 'No mail base URL was saved, so none was changed.',
    origins ? `The sync library origin was changed for ${origins} account(s).` : 'No sync library origin was saved, so none was changed.',
    grants ? `${grants} MCP grant(s) issued for the old address were removed; each client signs in again here.` : 'No MCP grant was issued for the old address.',
  ]
}

/** Whether a refused import is the one the owner answers by saying the old controller is stopped or gone. */
export function isLiveSourceRefusal(e: unknown): boolean {
  return !!e && typeof e === 'object' && (e as { code?: unknown }).code === 'copy_source_live'
}
