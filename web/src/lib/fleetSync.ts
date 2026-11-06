import type { SyncAgent, SyncState } from './api'
import { agentOnline } from './machines'
import { foldHost, machineSyncOverview } from './syncOverview'

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
export type FleetSyncState = 'none' | 'syncing' | 'behind' | 'attention' | 'waiting' | 'paused' | 'in-sync'

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

type Kind = 'paused' | 'offline' | 'behind' | 'syncing' | 'attention' | 'in-sync'
type Classified = { host: string; kind: Kind; text: string }

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
    // Connected, automatic sync on, and nothing planned yet: that is set up on its own.
    case 'setup': return { host, kind: 'syncing', text: '' }
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

export function fleetSyncStatus(snapshot: Pick<SyncState, 'agents' | 'machines' | 'environments' | 'targets' | 'jobs' | 'runs'>, now = Date.now()): FleetSyncStatus {
  const agents = snapshot.agents ?? []
  const all = agents.map(agent => classify(agent, snapshot as SyncState, now))
  const of = (kind: Kind) => all.filter(item => item.kind === kind)
  const [paused, offline, behind, syncing, attention, inSync] = (['paused', 'offline', 'behind', 'syncing', 'attention', 'in-sync'] as const).map(of)
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
  } else if (state !== 'paused') {
    if (attention.length) parts.push(attention.length === 1 ? `${attention[0].host} needs a look.` : `${names(hosts(attention))} need a look.`)
    if (offline.length) {
      parts.push(offline.length <= 2
        ? `${names(hosts(offline))} will catch up when ${offline.length === 1 ? "it's" : "they're"} back online.`
        : `${offline.length} machines will catch up when they're back online.`)
    }
    if (paused.length) parts.push(paused.length === 1 ? `${paused[0].host} is paused.` : `${paused.length} machines are paused.`)
  }

  return { ...counts, state, headline, detail: parts.join(' '), canSync: state !== 'syncing' && state !== 'paused' }
}

/** The status as one line for a toast or a note: what is true now, then what will follow. */
export function fleetSyncSentence(status: FleetSyncStatus): string {
  return [status.headline, status.detail].filter(Boolean).join(' ')
}
