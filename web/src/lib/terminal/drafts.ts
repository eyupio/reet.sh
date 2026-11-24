// Unsent text kept in this browser so a reload, or a phone browser that discards the tab, does not lose it: the chat
// composer's draft, the terminal's compose box and the chat's browser-held queue of messages. localStorage only, text only
// (never images), one entry per session and signed-in account, never sent anywhere. Every read and write is allowed to
// fail: with storage off, nothing is kept and nothing else changes. The storage rules take the store as an argument so
// they are unit tested; the few lines at the end hold the timer and the account.

export const DRAFTS_KEY = 'reet.drafts.v1'
export const MAX_DRAFTS = 20
export const MAX_DRAFT_BYTES = 32 * 1024
export const DRAFT_DAYS = 14

export type Kept = { chat?: string; compose?: string; queue?: string[] }
type Entry = Kept & { u: number; s: string; at: number }
type Store = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>

const bytes = (text: string) => new TextEncoder().encode(text).length
const isText = (value: unknown): value is string => typeof value === 'string' && value !== ''

function read(store: Store, now: number): Entry[] {
  try {
    const parsed = JSON.parse(store.getItem(DRAFTS_KEY) ?? '[]')
    if (!Array.isArray(parsed)) return []
    return parsed.filter((e): e is Entry => !!e && typeof e.s === 'string' && Number.isInteger(e.u) && typeof e.at === 'number' && now - e.at < DRAFT_DAYS * 864e5)
  } catch { return [] }
}

/** What is kept for this account's session, or null. */
export function loadKept(store: Store, owner: number, session: string, now = Date.now()): Kept | null {
  const entry = read(store, now).find(e => e.u === owner && e.s === session)
  if (!entry) return null
  const kept: Kept = {}
  if (isText(entry.chat)) kept.chat = entry.chat
  if (isText(entry.compose)) kept.compose = entry.compose
  if (Array.isArray(entry.queue) && entry.queue.every(isText) && entry.queue.length) kept.queue = entry.queue
  return Object.keys(kept).length ? kept : null
}

/** Change what is kept for a session: an empty value in the patch forgets that field, and an entry with nothing left is removed. */
export function saveKept(store: Store, owner: number, session: string, patch: Kept, now = Date.now()) {
  try {
    const all = read(store, now)
    const old = all.find(e => e.u === owner && e.s === session)
    const entry: Entry = { ...old, ...patch, u: owner, s: session, at: now }
    for (const field of ['chat', 'compose', 'queue'] as const) if (!entry[field]?.length) delete entry[field]
    // An entry over its limit is dropped whole: a half of a draft restored as if it were all of it would be worse than none.
    const keep = Object.keys(entry).length > 3 && bytes(JSON.stringify(entry)) <= MAX_DRAFT_BYTES
    const rest = all.filter(e => e !== old)
    const next = (keep ? [...rest, entry] : rest).sort((a, b) => a.at - b.at).slice(-MAX_DRAFTS)
    if (next.length) store.setItem(DRAFTS_KEY, JSON.stringify(next)); else store.removeItem(DRAFTS_KEY)
  } catch { /* storage is off or full: nothing is kept */ }
}

export function clearKept(store: Store) {
  try { store.removeItem(DRAFTS_KEY) } catch { /* nothing to clear */ }
}

// The browser's side. Typing is saved after a short pause and whenever the page is hidden (the last moment a phone gives
// before it may discard the tab); emptying a field is saved at once, so a message just sent is never restored.
let owner: number | null = null
let pending = new Map<string, { owner: number; patch: Kept }>()
let timer: ReturnType<typeof setTimeout> | undefined

function store(): Store | null { try { return window.localStorage } catch { return null } }

/** Who is signed in (0 where there are no accounts). Without one, nothing is kept or restored. */
export function setDraftOwner(id: number | null) { owner = id }

export function restoreDraft(session: string): Kept | null {
  flushDrafts() // a pane closed a moment ago has not been written yet
  const s = store()
  return s && owner !== null ? loadKept(s, owner, session) : null
}

export function flushDrafts() {
  clearTimeout(timer); timer = undefined
  const s = store(), todo = pending
  pending = new Map()
  if (s) for (const [session, { owner: who, patch }] of todo) saveKept(s, who, session, patch)
}

export function keepDraft(session: string, patch: Kept) {
  if (owner === null) return
  const held = pending.get(session)
  pending.set(session, { owner, patch: { ...held?.patch, ...patch } })
  if (Object.values(patch).some(v => !v?.length)) flushDrafts()
  else timer ??= setTimeout(flushDrafts, 400)
}

/** Sign out: every draft goes, and nothing waiting to be written is written after. */
export function clearDrafts() {
  clearTimeout(timer); timer = undefined; pending = new Map()
  const s = store()
  if (s) clearKept(s)
}

if (typeof window !== 'undefined') {
  window.addEventListener('pagehide', flushDrafts)
  document.addEventListener('visibilitychange', () => { if (document.visibilityState === 'hidden') flushDrafts() })
}
