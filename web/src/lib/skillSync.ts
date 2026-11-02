// What a person asked sync to do with one skill, read from the saved selections, as opposed to what a scan observed.
// A skill badge says Installed or Missing from inventory; this says "set to sync to laptop" from `sync_targets.desired`.
// The two are kept apart on purpose: being set to sync is never evidence that the files are there.
import type { SyncState } from './api'
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
}

export function skillSyncIntent(skillId: string, snapshot: SyncState | undefined): SkillSyncIntent {
  const pending = new Map<string, string>()
  const set = new Map<string, string>()
  for (const target of snapshot?.targets ?? []) {
    if (!(target.desired ?? []).some(skill => skill.id === skillId)) continue
    const host = target.target.host
    const key = foldHost(host)
    if ((target.missing ?? []).some(skill => skill.id === skillId)) { pending.set(key, host); set.delete(key) }
    else if (!pending.has(key)) set.set(key, host)
  }
  const byName = (a: string, b: string) => a.localeCompare(b, undefined, { sensitivity: 'base' })
  return { set: [...set.values()].sort(byName), pending: [...pending.values()].sort(byName) }
}

function fold(hosts: string[]): string {
  if (hosts.length <= 3) return hosts.join(', ')
  return `${hosts.slice(0, 2).join(', ')} and ${hosts.length - 2} more`
}

/** One short line for a card: "Not set to sync", "Set to sync to a, b" or "Pending on b · synced to a". */
export function skillSyncSummary(intent: SkillSyncIntent): string {
  if (!intent.set.length && !intent.pending.length) return 'Not set to sync'
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
  if (filter === 'not-set') return !intent.set.length && !intent.pending.length
  return intent.pending.length > 0
}
