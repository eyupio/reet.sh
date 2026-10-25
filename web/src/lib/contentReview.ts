import type { AdoptionBreakdown, AdoptionOffer, ContentPin, ContentReceipt, ContentPinWrite, ContentPlan, ContentPreview, PrivateBundle, PrivateRevision, SyncAgent, SyncEnvironment } from './api'

// What is true about a pin, a check and an ownership record, in words. Pure: no DOM, no fetch, no clock of its own.
// Nothing here may say more than the machine has told us: an unknown code or state falls back to FALLBACK, and the
// word "verified" appears only to deny that an ownership record was.

export const OWNERSHIP_SENTENCE = 'Recorded by reet from a recent check, not verified on the machine.'
export const APPLY_NOTICE = "Applying changes isn't available yet."
export const FALLBACK = 'Something changed. Check again.'

// A captured read older than this has not been seen by the shell recently enough to adopt from.
const FRESH_READ_MS = 30_000
const SCAN_WINDOW_MS = 14 * 86_400_000
const GENERIC_ERROR = 'Something went wrong. Try again.'
const NETWORK_ERROR = "Couldn't reach reet. Check your connection and try again."
// The only errors whose message is the server's own words: what req throws.
const SERVER_VOICED = new Set(['Error', 'ApiError', 'UnauthorizedError'])
// What fetch throws when no response arrives (Chromium, Firefox and Safari word it differently). Any other
// TypeError is a fault in our own code and must not be blamed on the connection.
const FETCH_FAILED = /^(Failed to fetch|NetworkError|Load failed|Network request failed)/i

// The closed tables are copied from the Go side; contentReview.test.mjs enumerates them so a new server code fails
// there. Lookups use own properties only, so a code such as "constructor" can never reach an inherited member.
function lookup(table: Readonly<Record<string, string>>, code: unknown): string {
  return typeof code === 'string' && Object.prototype.hasOwnProperty.call(table, code) ? table[code] : FALLBACK
}

const STATE_LABELS = {
  queued: 'Waiting for the machine',
  capturing: 'Reading the machine',
  captured: 'Checked',
  failed: "Couldn't check",
  cancelled: 'Stopped',
  expired: 'Ran out of time',
  superseded: 'Out of date',
  stale: 'Too old',
} as const
export const checkStateLabel = (state: string): string => lookup(STATE_LABELS, state)

const FENCE_REASONS = {
  pin_changed: 'The pin changed after this check was made.',
  pin_removed: 'The pin was removed after this check was made.',
  connector_paused: 'The connector on this machine is paused.',
  connector_removed: 'The connector on this machine was removed.',
  process_changed: 'The connector restarted or stopped reporting for this folder since the check.',
  inventory_changed: 'The machine reported a newer scan after this check was made.',
  scope_removed: 'The machine no longer reports this folder.',
  receipt_changed: 'The ownership record for this skill changed after this check was made.',
  expired: 'This check ran out of time before it finished.',
  stale: 'This check is more than five minutes old.',
} as const
export const reasonText = (code: string): string => lookup(FENCE_REASONS, code)

const FAILURES = {
  capture_failed: "The machine couldn't read this skill folder.",
  scope_unavailable: "The connector can't use this folder for private skills.",
  plan_unavailable: "The folder was read, but the changes couldn't be worked out. If an ownership record is damaged, release it and check again.",
} as const
export const failureText = (code: string): string => lookup(FAILURES, code)

const PLAN_CODES = {
  adapter_unsupported: "This folder layout isn't supported for private skills yet.",
  root_shared: 'Another agent scope, or a link, reaches this same folder, so reet will not touch it.',
  root_unknown: "Couldn't tell whether another agent scope reaches this folder, so reet will not touch it.",
  generation_changed: 'The connector restarted after this folder was read.',
  inventory_changed: 'The machine reported a newer scan after this folder was read.',
  observation_unknown: "The folder couldn't be read completely.",
  observation_stale: 'The folder was read too long ago to rely on.',
  observation_invalid: "The files found don't make a valid skill folder.",
  adoption_required: 'Files already there are not recorded as owned by reet yet.',
  unmanaged_path: "This file is already there, but reet doesn't own it, so it will not be changed.",
  managed_target_missing: "An ownership record exists, but the folder isn't there now. Release the record to clear this.",
  local_modified: 'This file on the machine has changed since ownership was recorded.',
  both_modified: 'This file has changed on the machine and in the pinned revision.',
  local_deleted: 'This file is missing from the machine; it was there when ownership was recorded.',
  result_invalid: "The planned result wouldn't be a valid skill folder.",
  baseline_target_mismatch: 'The ownership record is for a different folder or layout from the one just read.',
  baseline_bundle_mismatch: 'The ownership record is for a different library skill from the one pinned.',
} as const
export const planCodeText = (code: string): string => lookup(PLAN_CODES, code)

const PRESERVED = {
  local_only: "Stays yours: this file isn't in the pinned revision, so reet leaves it alone.",
  additive_managed: 'Kept: the pinned revision no longer has this file, but this pin only adds and updates.',
} as const
export const preservedText = (reason: string): string => lookup(PRESERVED, reason)

/** The codes each closed table words, so a test can hold them against the Go sources. */
export const CLOSED_CODES = {
  states: Object.keys(STATE_LABELS),
  reasons: Object.keys(FENCE_REASONS),
  failures: Object.keys(FAILURES),
  planCodes: Object.keys(PLAN_CODES),
  preserved: Object.keys(PRESERVED),
} as const

/** Found, Not there yet or Couldn't tell. Empty until a check has read the machine. */
export function presenceLabel(p: ContentPreview | null | undefined): string {
  const presence = p?.capture?.presence
  if (!presence) return ''
  return presence === 'present' ? 'Found' : presence === 'absent' ? 'Not there yet' : "Couldn't tell"
}

/**
 * What to say about who else reaches the folder. Silence means "this scope has it to itself", so only the literal
 * "exclusive" is silent: a missing or unrecognised value reads as unknown, never as reassurance. Ask only once a
 * check has read the folder.
 */
export function ownershipNote(rootOwnership: string | undefined): string {
  if (rootOwnership === 'exclusive') return ''
  return rootOwnership === 'shared' ? planCodeText('root_shared') : planCodeText('root_unknown')
}

// The database folds ASCII letters only when it compares hosts (SQLite NOCASE), so this does too: toLowerCase() would
// also fold letters such as "Ë" and "K" (Kelvin sign) that the server treats as different.
const foldAscii = (s: string): string => s.replace(/[A-Z]/g, c => String.fromCharCode(c.charCodeAt(0) + 32))
const sameHost = (a: string, b: string): boolean => foldAscii(a) === foldAscii(b)

/**
 * The connector a pin or check for this scope goes through: one that is not paused, on the scope's host. When more than
 * one qualifies the one seen most recently is the live one. scopeEligibility is true exactly when this finds one.
 */
export function connectorFor(env: SyncEnvironment, agents: SyncAgent[] | null | undefined): SyncAgent | undefined {
  let best: SyncAgent | undefined
  for (const a of agents ?? []) if (!a.paused && sameHost(a.host, env.host) && (!best || a.lastSeen > best.lastSeen)) best = a
  return best
}

export type Eligibility = { ok: true } | { ok: false; reason: string }
const no = (reason: string): Eligibility => ({ ok: false, reason })

/**
 * Whether the Private skills action is offered for a reported scope, mirroring what the pin API will accept. Each
 * refusal has its own plain reason; the first rule that fails wins. The server still decides.
 */
export function scopeEligibility(env: SyncEnvironment, agents: SyncAgent[] | null | undefined, opts: { accounts: boolean; canWrite: boolean; now: number }): Eligibility {
  if (!opts.accounts) return no('Private skills need a signed-in account.')
  if (!opts.canWrite) return no("Your account can only view, so it can't pin or check private skills.")
  if (env.agent === 'codex') return no("This agent is scan-only, so private skills can't be pinned to it.")
  if (!['agents', 'claude', 'cursor', 'opencode'].includes(env.agent)) return no("This agent doesn't support private skills.")
  const project = env.project ?? ''
  if (project !== '') {
    if (env.agent !== 'claude' && env.agent !== 'agents') return no('Project folders support private skills for Claude Code and shared agents only.')
    if (!project.startsWith('/')) return no("This project's folder isn't an absolute path, so it can't be used for private skills.")
  }
  // Written so that a missing or non-numeric scan time fails rather than passes.
  if (!env.complete || !(env.scannedAt > 0) || !(opts.now - env.scannedAt <= SCAN_WINDOW_MS)) return no("This machine hasn't reported a complete scan of this folder in the last 14 days.")
  // Before the machines have loaded nothing is known about connectors; that is not the same as none being enrolled.
  if (!agents) return no('Machines are still loading.')
  if (!agents.some(a => sameHost(a.host, env.host))) return no("No connector is enrolled on this machine, so it can't be checked.")
  if (!connectorFor(env, agents)) return no('The connector on this machine is paused. Resume it to check this machine.')
  return { ok: true }
}

export type CheckKind = 'none' | 'working' | 'current' | 'refreshing' | 'stale' | 'blocked' | 'ended'
export type CheckView = { kind: CheckKind; headline: string; reasons: string[]; canAdopt: boolean; canCheckAgain: boolean }

/** A present folder in an exclusive root whose only obstacle is that nothing owns it yet, with the review hash adoption binds to. */
export function adoptable(p: ContentPreview): boolean {
  const capture = p.capture, conflicts = p.plan?.conflicts
  return typeof p.reviewHash === 'string' && p.reviewHash !== '' && !!capture && capture.presence === 'present' && capture.rootOwnership === 'exclusive' && capture.manifest?.complete === true &&
    !!conflicts && conflicts.length === 1 && conflicts[0].code === 'adoption_required'
}

function view(kind: CheckKind, headline: string, reasons: string[] = [], canAdopt = false, canCheckAgain = true): CheckView {
  return { kind, headline, reasons: [...new Set(reasons)], canAdopt, canCheckAgain }
}

/**
 * What a check means right now. readAt is the browser time its response arrived and now is the browser time of the
 * question, so the two clocks are never mixed: the server's own state and reasons decide what is out of date, and
 * a captured read the shell has not renewed within 30 seconds is only "refreshing" until it is read again.
 */
export function classifyCheck(p: ContentPreview | null, readAt: number, now: number): CheckView {
  if (!p) return view('none', 'Nothing has been checked yet.')
  const reasons = (p.reasons ?? []).map(reasonText)
  switch (p.state) {
    case 'queued': return view('working', 'Waiting for the machine to pick up the check.', [], false, false)
    case 'capturing': return view('working', 'Reading the folder on this machine.', [], false, false)
    case 'captured': {
      // A captured check that still carries reasons has been overtaken; never trust it.
      if (reasons.length > 0) return view('stale', 'This check is out of date.', reasons)
      // Fresh only for two usable times with the read not after the question and no more than 30 seconds before it.
      // A read "from the future" or at a time that is not a number means the clocks cannot be trusted: read again.
      if (!(Number.isFinite(readAt) && Number.isFinite(now) && now >= readAt && now - readAt <= FRESH_READ_MS)) return view('refreshing', 'Updating this result before it can be used.')
      const presence = p.capture?.presence
      const headline = presence === 'present' ? 'Found on this machine.' : presence === 'absent' ? 'Not there yet on this machine.' : "Couldn't tell what is on this machine."
      return view('current', headline, [], adoptable(p))
    }
    case 'superseded': return view('stale', 'This check is out of date.', reasons.length > 0 ? reasons : [FALLBACK])
    case 'stale': return view('stale', 'This check is out of date.', [...reasons, reasonText('stale')])
    case 'expired': return view('ended', 'This check ran out of time.', [reasonText('expired')])
    case 'cancelled': return view('ended', 'This check was stopped.')
    // True whichever failure follows (the machine may have been read and the changes still not worked out); the reason carries the detail.
    case 'failed': return view('blocked', "That check didn't finish.", [failureText(p.error ?? '')])
    default: return view('blocked', FALLBACK)
  }
}

export type PlanSummary = {
  kind: 'up_to_date' | 'changes' | 'conflicts' | 'adoption' | 'unknown'
  actions: ContentPlan['actions']
  conflicts: ContentPlan['conflicts']
  preserved: ContentPlan['preserved']
}

/** The planner's verdict in one of five kinds (the plan's conflicts are listed in the result). A plan that is neither ready nor explained is "unknown", never reassuring. */
export function summarisePlan(plan?: ContentPlan): PlanSummary {
  if (!plan) return { kind: 'unknown', actions: [], conflicts: [], preserved: [] }
  const actions = plan.actions ?? [], conflicts = plan.conflicts ?? [], preserved = plan.preserved ?? []
  let kind: PlanSummary['kind']
  if (conflicts.length === 1 && conflicts[0].code === 'adoption_required') kind = 'adoption'
  else if (conflicts.length > 0) kind = 'conflicts'
  else if (!plan.ready) kind = 'unknown'
  else kind = actions.length > 0 ? 'changes' : 'up_to_date'
  return { kind, actions: kind === 'changes' ? actions : [], conflicts, preserved }
}

/**
 * A check is for one version of one pin. When the pin has moved on since (its version differs from the one the check was
 * made for) the check is overtaken whatever the server last said, so it is read as superseded by the pin. A check that has
 * already ended (failed, cancelled, expired) says what it said.
 */
export function asSeenFromPin(check: ContentPreview | null | undefined, pin: Pick<ContentPin, 'version'> | null | undefined): ContentPreview | null | undefined {
  if (!check || !pin || check.targetVersion === pin.version) return check
  if (check.state === 'failed' || check.state === 'cancelled' || check.state === 'expired') return check
  const reasons = check.reasons ?? []
  return { ...check, state: 'superseded', reasons: reasons.includes('pin_changed') ? reasons : [...reasons, 'pin_changed'] }
}

/** The connector's own issue codes and paths are not for the person to read; this is all that is said about them. */
export const CONNECTOR_PROBLEM = 'The connector reported a problem reading this folder.'

/**
 * The planner's verdict for a check that is being shown. "Up to date" is only ever said of a folder that was found: a ready
 * plan with nothing to do for a folder that could not be read is unknown, never reassurance.
 */
export function planVerdict(check: Pick<ContentPreview, 'capture' | 'plan'>): PlanSummary {
  const summary = summarisePlan(check.plan)
  return summary.kind === 'up_to_date' && check.capture?.presence !== 'present' ? { ...summary, kind: 'unknown' } : summary
}

/** The offer's candidates exactly as the server ordered them (best match first, then newest). */
export function candidatesFor(offer?: Partial<AdoptionOffer> | null): AdoptionOffer['candidates'] {
  return [...(offer?.candidates ?? [])]
}

/** A revision is chosen for the person only when the offer holds exactly one; otherwise they choose. */
export function preselect(offer?: Partial<AdoptionOffer> | null): string {
  const candidates = candidatesFor(offer)
  return candidates.length === 1 ? candidates[0].revisionId : ''
}

export type BreakdownRow = { path: string; state: 'claimed' | 'stays_yours' | 'library_only' | 'differs'; bytes: number; executable: boolean; digest: string }

/**
 * One row per path, in the server's order. A path that differs between the machine and the library arrives twice
 * (stays_yours and library_only) and becomes one "differs" row describing the machine's copy, the one that stays
 * yours. A status that is not recognised is never read as claimed.
 */
export function groupBreakdown(b: AdoptionBreakdown): BreakdownRow[] {
  const rows: BreakdownRow[] = []
  const unpaired = new Map<string, number>()
  for (const f of b.files ?? []) {
    const state = f.status === 'claimed' ? 'claimed' : f.status === 'library_only' ? 'library_only' : 'stays_yours'
    const row: BreakdownRow = { path: f.path, state, bytes: f.bytes, executable: f.executable, digest: f.digest }
    const at = unpaired.get(f.path)
    if (at !== undefined) {
      const other = rows[at]
      if ((other.state === 'stays_yours' && state === 'library_only') || (other.state === 'library_only' && state === 'stays_yours')) {
        rows[at] = { ...(state === 'stays_yours' ? row : other), state: 'differs' }
        unpaired.delete(f.path)
        continue
      }
    }
    rows.push(row)
    if (state === 'claimed') unpaired.delete(f.path)
    else unpaired.set(f.path, rows.length - 1)
  }
  return rows
}

export type ClaimSummary = { claimed: number; claimedBytes: number; staysYours: number; differs: number; libraryOnly: number; text: string }

const files = (n: number) => `${n} ${n === 1 ? 'file' : 'files'}`

/** Counts of the rows the person sees, and the same in a sentence. */
export function claimSummary(b: AdoptionBreakdown): ClaimSummary {
  const s: ClaimSummary = { claimed: 0, claimedBytes: 0, staysYours: 0, differs: 0, libraryOnly: 0, text: '' }
  for (const row of groupBreakdown(b)) {
    if (row.state === 'claimed') { s.claimed++; s.claimedBytes += row.bytes }
    else if (row.state === 'differs') s.differs++
    else if (row.state === 'stays_yours') s.staysYours++
    else s.libraryOnly++
  }
  const parts = [s.claimed === 0 ? 'No files would be recorded as owned by reet.' : `${files(s.claimed)} would be recorded as owned by reet.`]
  if (s.differs > 0) parts.push(`${files(s.differs)} ${s.differs === 1 ? 'differs' : 'differ'} from this revision and stay${s.differs === 1 ? 's' : ''} yours.`)
  if (s.staysYours > 0) parts.push(`${files(s.staysYours)} ${s.staysYours === 1 ? 'is' : 'are'} only on this machine and stay${s.staysYours === 1 ? 's' : ''} yours.`)
  if (s.libraryOnly > 0) parts.push(`${files(s.libraryOnly)} ${s.libraryOnly === 1 ? 'is' : 'are'} only in the library.`)
  s.text = parts.join(' ')
  return s
}

/** The row words of the breakdown table. Exactly these four, whatever the server's status. */
export const BREAKDOWN_LABELS = { claimed: 'Claimed', stays_yours: 'Stays yours', library_only: 'Only in the library', differs: 'Differs from this revision' } as const

/** A candidate revision in a sentence of counts. The server only says bytes or mode differ, never why. */
export function candidateCounts(c: { matched: number; observedOnly: number; revisionOnly: number }): string {
  return `${files(c.matched)} ${c.matched === 1 ? 'matches' : 'match'} · ${files(c.observedOnly)} on this machine not claimed · ${files(c.revisionOnly)} in the library not matched`
}

/** Why a breakdown could not be shown, in plain words. The server's shared 409 text is never repeated. */
export function breakdownNote(e: unknown): string {
  const status = typeof e === 'object' && e !== null ? (e as { status?: unknown }).status : undefined
  return status === 409 || status === 404 ? "This revision can't be compared with the folder any more. Check again." : describeError(e)
}

/**
 * What to tell the person when recording ownership was refused. The shared 409 text is never shown: the status and what
 * the check says now decide. owned is true when this scope now holds an ownership record for the skill; reasons are the
 * fence reasons of the check read again (already in words).
 */
export const RECEIPT_LIMIT = 1024
export type AdoptRefusalContext = {
  /** This scope now holds a record for the skill. */
  owned: boolean
  /** The fence reasons of the check read again, in words. */
  reasons: string[]
  /** The check read again is still current and adoptable (false when it could not be read). */
  adoptable: boolean
  /** How many ownership records the account holds (the list is account-wide). */
  records: number
}
export function adoptRefusal(status: number, name: string, ctx: AdoptRefusalContext): string {
  if (status === 403) return "Your account can only view, so it can't record ownership."
  if (status === 409 || status === 404) {
    if (ctx.owned) return `${name} already has an ownership record in this folder. Nothing was recorded. Release that record first if you want to record ownership again.`
    if (ctx.reasons.length > 0) return `Nothing was recorded. ${ctx.reasons.join(' ')} Check again.`
    if (!ctx.adoptable) return 'Nothing was recorded. Something changed since the check was made. Check again.'
    // The check still stands and nothing explains the refusal: do not blame the check.
    if (ctx.records >= RECEIPT_LIMIT) return `Nothing was recorded. This account already holds the most ownership records it can (${RECEIPT_LIMIT.toLocaleString('en-GB')}). Release a record you no longer need, then try again.`
    return "Nothing was recorded. reet couldn't record ownership this time. Try again; if it keeps happening, release a record you no longer need."
  }
  return ''
}

/** What changed under a release that was refused: the record as it was shown, and as it is now (undefined once gone). */
export function releaseNote(before: Pick<ContentReceipt, 'version'>, after: Pick<ContentReceipt, 'version'> | undefined, name: string): string {
  if (!after) return `The ownership record for ${name} was already released elsewhere, so there was nothing to release.`
  if (after.version !== before.version) return `The ownership record for ${name} changed elsewhere. Nothing was released. Review it, then release it again if you still want to.`
  return ''
}

/** A record's revision as a phrase: "revision <date>" from a list that has been read, otherwise "an older revision"; never the start of an id. */
export function receiptRevision(id: string, list: PrivateRevision[] | undefined, head: string | undefined): string {
  const label = list ? revisionLabels(list, head).get(id) : undefined
  return label ? `revision ${label}` : 'an older revision'
}

/** Whether a listed record is the one an adoption of this check and revision made (the way a lost reply is reconciled). */
export const madeByAdoption = (r: ContentReceipt, previewId: string, revisionId: string): boolean => r.damaged !== true && r.evidence?.previewId === previewId && r.revisionId === revisionId

/** How long to wait before reading a check again, or null to stop. Never while the tab is hidden. */
export function pollDelay(state: string, hidden: boolean): number | null {
  if (hidden) return null
  if (state === 'queued' || state === 'capturing') return 3000
  // The server's stale or superseded verdict should reach an open Adopt step within seconds.
  if (state === 'captured') return 10000
  return null
}

/** The server's own words when it gave some, otherwise a plain sentence. Never a stack, a status line or browser jargon. */
export function describeError(e: unknown): string {
  const name = typeof e === 'object' && e !== null && typeof (e as { name?: unknown }).name === 'string' ? (e as { name: string }).name : ''
  if (name === 'TypeError') return FETCH_FAILED.test(String((e as { message?: unknown }).message ?? '')) ? NETWORK_ERROR : GENERIC_ERROR
  // Only what req throws carries the server's words. Any other named error (SyntaxError from a page that is not JSON,
  // ReferenceError, DOMException, ...) is the browser's or our own, and is not for the person to read.
  if (name !== '' && !SERVER_VOICED.has(name)) return GENERIC_ERROR
  let raw: unknown = e
  if (typeof e === 'object' && e !== null) {
    const { error, message } = e as { error?: unknown; message?: unknown }
    raw = typeof error === 'string' && error.trim() !== '' ? error : message
  }
  if (typeof raw !== 'string') return GENERIC_ERROR
  const line = raw.split(/\r?\n/).map(l => l.trim()).find(l => l !== '') ?? ''
  if (line === '' || line.length > 300) return GENERIC_ERROR
  // The client's own "404 Not Found" when a refusal carried no text, a stringified error class, or a stack frame.
  if (/^\d{3}(\s|$)/.test(line) || /^[A-Za-z]*Error:/.test(line) || /(^|\s)at\s+\S.*:\d+:\d+\)?$/.test(line)) return GENERIC_ERROR
  return line
}

// ---------------------------------------------------------------------------------------------------------------
// Pins: choosing a library skill and revision, the mode words, and what changed when a write is refused.

/** What the Pin step sends: the shell adds the scope and the connector, which only it knows. */
export type PinChoice = Omit<ContentPinWrite, 'environmentId' | 'agentId'>

/** The first eight characters of an id, enough to tell revisions apart on screen. */
export const shortId = (id: string): string => id.slice(0, 8)

const MODE_LABELS = {
  additive: 'Add and update',
  managed: 'Also remove files reet owns that a newer revision drops',
} as const
export const MODES: readonly { value: ContentPinWrite['mode']; label: string }[] = (Object.keys(MODE_LABELS) as ContentPinWrite['mode'][]).map(value => ({ value, label: MODE_LABELS[value] }))
/** A mode this page has no words for is said to be unknown; it is never shown as a raw code. */
export const modeLabel = (mode: string): string => (Object.prototype.hasOwnProperty.call(MODE_LABELS, mode) ? MODE_LABELS[mode as keyof typeof MODE_LABELS] : "A mode this page doesn't recognise")

/** Whether a pin already says what is about to be saved: the same connector, library skill, revision and mode. */
export function samePin(pin: ContentPin | undefined, want: { agentId: string; bundleId: string; revisionId: string; mode: string }): boolean {
  return !!pin && pin.agentId === want.agentId && pin.bundleId === want.bundleId && pin.revisionId === want.revisionId && pin.mode === want.mode
}

/** A library skill is named by its name; two with the same name also carry the start of their id so they can be told apart. */
export function bundleLabel(b: PrivateBundle, bundles: PrivateBundle[]): string {
  return bundles.some(o => o !== b && o.name === b.name) ? `${b.name} · ${shortId(b.id)}` : b.name
}

const stampOf = (ms: number): string => new Date(ms).toLocaleString()

/**
 * What to call each revision on screen: when it was made, with the start of its id added only to tell two apart that
 * share a time, and "Newest" on the bundle's head. The format is a parameter so a test can be exact.
 */
export function revisionLabels(revisions: PrivateRevision[], head: string | undefined, stamp: (ms: number) => string = stampOf): Map<string, string> {
  const stamps = revisions.map(r => stamp(r.createdAt))
  const labels = new Map<string, string>()
  revisions.forEach((r, i) => {
    const same = stamps.filter(x => x === stamps[i]).length > 1
    labels.set(r.id, `${stamps[i]}${same ? ` · ${shortId(r.id)}` : ''}${r.id === head ? ' · Newest' : ''}`)
  })
  return labels
}

/** The revision's label from a list that has been read, or the start of its id when it is not in one. */
export const revisionLabel = (id: string, list: PrivateRevision[] | undefined, head: string | undefined): string => (list ? revisionLabels(list, head).get(id) : undefined) ?? shortId(id)

/** Bundles whose name is the reported skill's come first; each group keeps the library's own order. */
export function groupBundles(bundles: PrivateBundle[], name?: string): { matching: PrivateBundle[]; others: PrivateBundle[] } {
  const matching = name ? bundles.filter(b => b.name === name) : []
  return { matching, others: bundles.filter(b => !matching.includes(b)) }
}

/** The bundle to choose for the person: the reported skill's, but only when the library holds exactly one of that name. */
export function preselectBundle(bundles: PrivateBundle[], name?: string): string {
  const { matching } = groupBundles(bundles, name)
  return matching.length === 1 ? matching[0].id : ''
}

/**
 * Newest first, so the first entry is the one to preselect and the rest are there for a rollback. The library lists
 * oldest first and two revisions can share a millisecond, so the bundle's head (the revision most recently appended)
 * leads, then creation time, then id for a stable order.
 */
export function newestFirst(revisions: PrivateRevision[], head?: string): PrivateRevision[] {
  const lead = (r: PrivateRevision) => (r.id === head ? 0 : 1)
  return [...revisions].sort((a, b) => lead(a) - lead(b) || b.createdAt - a.createdAt || (a.id < b.id ? 1 : a.id > b.id ? -1 : 0))
}

const pinWords = (p: ContentPin, text: (p: ContentPin) => string) => `revision ${text(p)} (${modeLabel(p.mode)})`

/** The server's 404 says only that something is unavailable; this says which, and that nothing was done. */
export const missingNote = (action: 'save' | 'remove'): string => (action === 'remove' ? "That pin isn't there any more, so nothing was removed." : "The library skill or revision you chose isn't there any more, so nothing was saved.")

/**
 * What to tell the person when a pin write was refused because the pin is not as they last saw it: before is the pin
 * they were looking at, after is the pin read again, and action is what they were doing. Empty when nothing differs, so
 * the server's own refusal is shown.
 */
export function pinChangeNote(before: ContentPin | undefined, after: ContentPin | undefined, name: string, action: 'save' | 'remove' = 'save', revisionText: (p: ContentPin) => string = p => shortId(p.revisionId)): string {
  const check = action === 'remove' ? 'Nothing was removed. Review it, then remove it again if you still want to.' : 'Nothing was saved. Review it, then save again if you still want your change.'
  if (!before && !after) return ''
  if (before && !after) return action === 'remove' ? `The pin for ${name} was already removed elsewhere, so there was nothing to remove.` : `The pin for ${name} was removed elsewhere. Nothing was saved.`
  if (!before && after) return `${name} was pinned elsewhere, at ${pinWords(after, revisionText)}. ${check}`
  if (!before || !after || before.version === after.version) return ''
  if (before.bundleId !== after.bundleId || before.revisionId !== after.revisionId || before.mode !== after.mode) return `The pin for ${name} changed elsewhere. It is now ${pinWords(after, revisionText)}; it was ${pinWords(before, revisionText)}. ${check}`
  return `The pin for ${name} was saved again elsewhere, with the same revision and mode. ${check}`
}
