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
}

/**
 * Which parts of a move this controller holds, from GET /api/version. A controller built before the list existed
 * sends no field at all, and that reads as none: a page must not offer a step the server would not understand.
 */
export function moveFeatures(features: unknown): MoveFeatures {
  const has = (name: string) => Array.isArray(features) && features.includes(name)
  return { fence: has('fence'), copyOrigin: has('copy-origin'), publicUrl: has('restore-public-url'), copyPassword: has('copy-password-request') }
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

/** What to put where the new address goes in a command while nobody has said what it is. */
export const NEW_ADDRESS_PLACEHOLDER = 'https://NEW-ADDRESS'

/** The command a machine runs to follow the controller to its new address. */
export function setServerCommand(address: string): string {
  return `reet agent set-server ${cleanAddress(address) || NEW_ADDRESS_PLACEHOLDER}`
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
