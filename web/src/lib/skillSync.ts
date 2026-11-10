// What a person asked sync to do with one skill, read from the saved selections, as opposed to what a scan observed.
// A skill badge says Installed or Missing from inventory; this says "set to sync to laptop" from `sync_targets.desired`.
// The two are kept apart on purpose: being set to sync is never evidence that the files are there.
import type { SyncHeld, SyncState } from './api'
import type { InstallationReport } from './machines'

// Same folding as syncOverview.foldHost and the same verdict as machines.installationStatus, repeated here so this
// module stays a single file the unit test can compile on its own.
const foldHost = (value: string) => (value ?? '').trim().toLowerCase()
const observed = (report: InstallationReport, skillId: string) => report.installed.has(skillId) ? 'installed' : report.complete ? 'missing' : 'unknown'

export type SkillSyncIntent = {
  /** Hosts whose saved selection includes the skill and whose last report had it. */
  set: string[]
  /** Hosts whose saved selection includes the skill but whose last report did not: the next run installs it. */
  pending: string[]
  /** Hosts where the skill is selected in the library but sync leaves it out, with the first reason found. */
  held: string[]
  heldReason?: SyncHeld['reason']
}

export function skillSyncIntent(skillId: string, snapshot: SyncState | undefined): SkillSyncIntent {
  const pending = new Map<string, string>()
  const set = new Map<string, string>()
  const held = new Map<string, string>()
  let heldReason: SyncHeld['reason'] | undefined
  for (const target of snapshot?.targets ?? []) {
    const entry = (target.held ?? []).find(item => item.id === skillId)
    if (entry) { held.set(foldHost(target.target.host), target.target.host); heldReason ??= entry.reason }
    if (!(target.desired ?? []).some(skill => skill.id === skillId)) continue
    const host = target.target.host
    const key = foldHost(host)
    if ((target.missing ?? []).some(skill => skill.id === skillId)) { pending.set(key, host); set.delete(key) }
    else if (!pending.has(key)) set.set(key, host)
  }
  const byName = (a: string, b: string) => a.localeCompare(b, undefined, { sensitivity: 'base' })
  // A host that installs the skill in one folder is not "held" because another folder leaves it out.
  for (const key of [...set.keys(), ...pending.keys()]) held.delete(key)
  return { set: [...set.values()].sort(byName), pending: [...pending.values()].sort(byName), held: [...held.values()].sort(byName), ...(held.size && heldReason ? { heldReason } : {}) }
}

function fold(hosts: string[]): string {
  if (hosts.length <= 3) return hosts.join(', ')
  return `${hosts.slice(0, 2).join(', ')} and ${hosts.length - 2} more`
}

/** One short line for a card: "Not set to sync", "Set to sync to a, b" or "Pending on b · synced to a". */
const HELD_WHY: Record<SyncHeld['reason'], string> = {
  content: 'waiting for a saved copy in reet',
  status: 'not installed until it is In use',
  duplicate: 'another selected skill has the same name',
}

export function skillSyncSummary(intent: SkillSyncIntent): string {
  const held = intent.held?.length ? `Selected for ${fold(intent.held)} · ${HELD_WHY[intent.heldReason ?? 'content']}` : ''
  if (!intent.set.length && !intent.pending.length) return held || 'Not set to sync'
  if (held) return `${held} · ${intent.pending.length ? `pending on ${fold(intent.pending)}` : `synced to ${fold(intent.set)}`}`
  if (!intent.pending.length) return `Set to sync to ${fold(intent.set)}`
  return `Pending on ${fold(intent.pending)}${intent.set.length ? ` · synced to ${fold(intent.set)}` : ''}`
}

export type SyncFilter = '' | 'nowhere' | 'missing' | 'not-set' | 'pending'

/** `needsSnapshot` filters read the saved selections, so they are disabled until a sync state has loaded: an empty answer must not pass as "not set". */
export const SYNC_FILTERS: { id: Exclude<SyncFilter, ''>; label: string; title: string; needsSnapshot: boolean }[] = [
  { id: 'nowhere', label: 'Not on any machine', title: 'No complete recent scan reports this skill.', needsSnapshot: false },
  { id: 'missing', label: 'Missing somewhere', title: 'At least one machine with a complete recent scan does not have it.', needsSnapshot: false },
  { id: 'not-set', label: 'Not set to sync', title: 'No saved selection on any machine includes it.', needsSnapshot: true },
  { id: 'pending', label: 'Sync pending', title: 'A saved selection includes it but the machine does not have it yet.', needsSnapshot: true },
]
export const SYNC_UNAVAILABLE = 'Sync state could not be read, so this filter is off until it loads.'

/** Which jobs have reached a final state: when this changes, inventory and the saved selections are worth re-reading. */
export function settledJobsKey(snapshot: SyncState | undefined): string {
  return (snapshot?.jobs ?? []).filter(job => !['queued', 'running', 'awaiting-approval'].includes(job.status)).map(job => `${job.id}:${job.status}`).sort().join('|')
}

export function matchesSyncFilter(filter: SyncFilter, skillId: string, snapshot: SyncState | undefined, reports: InstallationReport[]): boolean {
  if (!filter) return true
  const statuses = reports.map(report => observed(report, skillId))
  if (filter === 'nowhere') return !statuses.includes('installed')
  if (filter === 'missing') return statuses.includes('missing')
  const intent = skillSyncIntent(skillId, snapshot)
  if (filter === 'not-set') return !intent.set.length && !intent.pending.length && !intent.held.length
  return intent.pending.length > 0
}

/** One line for a written skill: the saved plan first, then what complete scans observed. */
export function writtenSkillWhere(skillId: string, snapshot: SyncState | undefined, reports: InstallationReport[]): string {
  const state = snapshot ? skillSyncSummary(skillSyncIntent(skillId, snapshot)) : ''
  if (state && state !== 'Not set to sync') return state
  const installed = reports.filter(report => report.installed.has(skillId)).length
  const missing = reports.filter(report => report.complete && !report.installed.has(skillId)).length
  if (installed) return `On ${installed} ${installed === 1 ? 'machine' : 'machines'}${missing ? ` · ${missing} missing` : ''}`
  if (missing) return `Missing on ${missing} ${missing === 1 ? 'machine' : 'machines'}`
  return 'Not on a machine yet'
}
