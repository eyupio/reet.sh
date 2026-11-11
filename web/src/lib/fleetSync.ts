import type { SyncAgent, SyncState } from './api'
import { agentOnline } from './machines'
import { agoLabel, foldHost, machineSyncOverview } from './syncOverview'

/**
 * The whole fleet in one sentence. A person who presses Sync wants to know one
 * thing, "am I in sync?", and what to press if not. Everything else on the page is
 * detail for someone who goes looking.
 *
 * An offline machine is not behind and not an error: it catches up by itself when
 * it reconnects, and there is nothing a person can do about it. A paused machine
 * is left alone on purpose. Only a machine that is missing skills, or has
 * automatic sync turned off, is something pressing the button fixes.
 */
export type FleetSyncState = 'none' | 'syncing' | 'behind' | 'attention' | 'unchosen' | 'waiting' | 'paused' | 'in-sync'

export type FleetSyncStatus = {
  state: FleetSyncState
  /** The one sentence that answers "am I in sync?". */
  headline: string
  /** What else is worth a short line. Never an instruction. */
  detail: string
  machines: number
  inSync: number
  behind: number
  syncing: number
  attention: number
  offline: number
  paused: number
  /** Pressing Sync is a sensible move right now. */
  canSync: boolean
}

/**
 * A machine with several folders finishes one and starts the next when it next checks in, a few seconds later.
 * That gap is part of syncing, so a machine that has just finished something is not yet one the button is for.
 */
const BETWEEN_FOLDERS_MS = 20_000

type Kind = 'paused' | 'offline' | 'behind' | 'syncing' | 'attention' | 'unchosen' | 'in-sync'
type Classified = { host: string; kind: Kind; text: string }

/** How long after a machine is first seen a missing plan still counts as being prepared. */
const SETTLING_MS = 60_000

function settling(machine: { groups?: { firstSeen?: number }[] }, now: number): boolean {
  const seen = (machine.groups ?? []).map(group => group.firstSeen ?? 0).filter(Boolean)
  return seen.length > 0 && now - Math.min(...seen) < SETTLING_MS
}

function classify(agent: SyncAgent, snapshot: SyncState, now: number): Classified {
  const host = agent.host
  if (agent.paused) return { host, kind: 'paused', text: '' }
  // Checked before offline: the button turns automatic sync on, wherever the machine is.
  if (!agent.keepSynced) return { host, kind: 'behind', text: '' }
  if (!agentOnline(agent, now)) return { host, kind: 'offline', text: '' }
  const machine = (snapshot.machines ?? []).find(item => foldHost(item.host) === foldHost(host))
  // Connected but not scanned yet: the library is prepared on its own after the first scan.
  if (!machine) return { host, kind: 'syncing', text: '' }
  const overview = machineSyncOverview(machine, snapshot, now)
  switch (overview.tone) {
    case 'synced': return { host, kind: 'in-sync', text: '' }
    case 'action': return { host, kind: overview.lastSync > 0 && now - overview.lastSync < BETWEEN_FOLDERS_MS ? 'syncing' : 'behind', text: '' }
    case 'working': return { host, kind: 'syncing', text: '' }
    // Connected, automatic sync on, and nothing planned yet. A machine that has only just been scanned may still be
    // having its plan prepared; after that, nothing planned means nobody has chosen its skills, and saying
    // "Syncing…" for ever would be untrue.
    case 'setup': return { host, kind: settling(machine, now) ? 'syncing' : 'unchosen', text: '' }
    default: return { host, kind: 'attention', text: overview.detail }
  }
}

/** "a", "a and b", "a, b and c". */
function names(hosts: string[]): string {
  if (hosts.length <= 1) return hosts[0] ?? ''
  return `${hosts.slice(0, -1).join(', ')} and ${hosts[hosts.length - 1]}`
}

function plural(count: number, one: string, many: string): string {
  return `${count} ${count === 1 ? one : many}`
}

/**
 * What the library adds to the sentence: skills being copied in and skills that cannot sync yet. They are part of
 * the one answer, so the page does not print a second notice beside it.
 */
export type FleetLibraryNotes = {
  /** "Copying seo privately from your machines. …", or empty. */
  copying?: string
  /** "3 skills are left out for now.", or empty. */
  leftOut?: string
}

export function fleetSyncStatus(snapshot: Pick<SyncState, 'agents' | 'machines' | 'environments' | 'targets' | 'jobs' | 'runs'>, now = Date.now(), library: FleetLibraryNotes = {}): FleetSyncStatus {
  const agents = snapshot.agents ?? []
  const all = agents.map(agent => classify(agent, snapshot as SyncState, now))
  const of = (kind: Kind) => all.filter(item => item.kind === kind)
  const [paused, offline, behind, syncing, attention, unchosen, inSync] = (['paused', 'offline', 'behind', 'syncing', 'attention', 'unchosen', 'in-sync'] as const).map(of)
  const counts = { machines: all.length, inSync: inSync.length, behind: behind.length, syncing: syncing.length, attention: attention.length, offline: offline.length, paused: paused.length }
  const hosts = (list: Classified[]) => list.map(item => item.host)

  let state: FleetSyncState
  let headline: string
  if (!all.length) {
    return { ...counts, state: 'none', headline: 'Connect your first machine.', detail: '', canSync: false }
  } else if (paused.length === all.length) {
    state = 'paused'
    headline = 'Sync is paused on all your machines.'
  } else if (syncing.length) {
    state = 'syncing'
    headline = 'Syncing your machines…'
  } else if (behind.length) {
    state = 'behind'
    headline = `${plural(behind.length, 'machine needs', 'machines need')} syncing.`
  } else if (attention.length) {
    state = 'attention'
    headline = `${plural(attention.length, 'machine needs', 'machines need')} a look.`
  } else if (unchosen.length) {
    state = 'unchosen'
    headline = unchosen.length === 1 ? `No skills chosen for ${unchosen[0].host} yet.` : `${unchosen.length} machines have no skills chosen yet.`
  } else if (!inSync.length) {
    state = 'waiting'
    headline = "Your machines will sync when they're back online."
  } else {
    state = 'in-sync'
    headline = 'Everything is in sync.'
  }

  const parts: string[] = []
  if (state === 'waiting') {
    parts.push(offline.length <= 3 ? `Offline right now: ${names(hosts(offline))}.` : `${offline.length} machines are offline right now.`)
  } else if (state === 'attention') {
    // The machine's own words, so the person can act on exactly what it says.
    parts.push(attention.length === 1 ? `${attention[0].host}: ${attention[0].text}` : `${names(hosts(attention))} need a look.`)
  } else if (state === 'unchosen') {
    parts.push(unchosen.length === 1 ? 'Open Skills on this machine to choose them.' : `Open Skills on this machine for ${names(hosts(unchosen))} to choose them.`)
    if (offline.length) parts.push(offline.length === 1 ? `${offline[0].host} is offline.` : `${offline.length} machines are offline.`)
  } else if (state !== 'paused') {
    if (attention.length) parts.push(attention.length === 1 ? `${attention[0].host} needs a look.` : `${names(hosts(attention))} need a look.`)
    if (offline.length) {
      parts.push(offline.length <= 2
        ? `${names(hosts(offline))} will catch up when ${offline.length === 1 ? "it's" : "they're"} back online.`
        : `${offline.length} machines will catch up when they're back online.`)
    }
    if (paused.length) parts.push(paused.length === 1 ? `${paused[0].host} is paused.` : `${paused.length} machines are paused.`)
  }

  if (library.copying) parts.push(library.copying)
  if (library.leftOut) parts.push(library.leftOut)

  return { ...counts, state, headline, detail: parts.join(' '), canSync: state !== 'syncing' && state !== 'paused' }
}

/**
 * One machine's line on the Sync page. It is sorted the same way as the sentence above it (`classify`), so a row
 * never says something the sentence contradicts, and whatever needs a person on this machine is said here and
 * nowhere else on the page.
 */
export type MachineSyncRow = {
  host: string
  online: boolean
  /** The short word in the row's chip. */
  label: string
  detail: string
  tone: 'synced' | 'attention' | 'review' | 'working' | 'waiting'
  /** Set when Auto fix can settle what stopped this machine. */
  fix?: { targetId: string; agentId: string }
  /** Something that happened or is waiting on this machine: a review to give, or an update that finished. */
  note?: { text: string; action?: string; href?: string }
}

export function machineSyncRow(agent: SyncAgent, snapshot: Pick<SyncState, 'agents' | 'machines' | 'environments' | 'targets' | 'jobs' | 'runs'>, now = Date.now()): MachineSyncRow {
  const online = agentOnline(agent, now)
  const row = (label: string, detail: string, tone: MachineSyncRow['tone']): MachineSyncRow => {
    const note = machineNote(agent, snapshot, now)
    return { host: agent.host, online, label, detail, tone, ...(note ? { note } : {}) }
  }
  if (agent.paused) return row('Paused', 'Paused · resume in machine settings', 'attention')
  if (!agent.keepSynced) return row('Sync off', 'Automatic sync is off · Sync all machines to enable', 'attention')
  if (!online) return row('Waiting for machine', 'Offline · library sync resumes when this machine reconnects', 'waiting')
  const machine = (snapshot.machines ?? []).find(item => foldHost(item.host) === foldHost(agent.host))
  if (!machine) return row('Setting up', 'Connected · waiting for the first scan', 'working')
  const overview = machineSyncOverview(machine, snapshot as SyncState, now)
  // Nothing planned yet on a connected machine is prepared without anyone doing anything, which is what the
  // sentence calls syncing. The row says the same and does not ask for a choice.
  if (overview.tone === 'setup') {
    return settling(machine, now)
      ? row('Setting up', 'Reet is getting this machine’s skills ready.', 'working')
      : row('No skills chosen', 'Choose what this machine should have in Skills on this machine.', 'attention')
  }
  const tone = overview.tone === 'action' ? 'working' : overview.tone
  return { ...row(overview.label, overview.detail, tone), ...(overview.fix ? { fix: overview.fix } : {}) }
}

function machineNote(agent: SyncAgent, snapshot: Pick<SyncState, 'jobs'>, now: number): MachineSyncRow['note'] {
  const jobs = (snapshot.jobs ?? []).filter(job => job.agentId === agent.id).sort((a, b) => b.updatedAt - a.updatedAt)
  const waiting = jobs.filter(job => job.status === 'awaiting-approval').length
  if (waiting) {
    return { text: `${plural(waiting, 'plan is', 'plans are')} waiting for your review.`, action: 'Review changes', href: '#sync-jobs' }
  }
  // An update that finished is news about this machine, said once here while it is the latest thing that happened.
  const latest = jobs[0]
  if (latest?.kind === 'update' && latest.status === 'succeeded') return { text: `Update complete · ${agoLabel(latest.updatedAt, now)}` }
  return undefined
}

/** The status as one line for a toast or a note: what is true now, then what will follow. */
export function fleetSyncSentence(status: FleetSyncStatus): string {
  return [status.headline, status.detail].filter(Boolean).join(' ')
}
