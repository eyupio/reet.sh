import type { SyncJob, SyncState } from './api'
import { groupLabel } from './machines'
import { foldHost } from './syncOverview'

const ACTIVE = new Set(['queued', 'running'])
const FAILED = new Set(['failed', 'rolled_back'])
const RESOLVED = new Set(['succeeded', 'cancelled'])

export type SyncActivityView = Pick<SyncState, 'agents' | 'targets' | 'environments'>

export type SyncActivityEntry = {
  key: string
  host: string
  job: SyncJob
  jobs: SyncJob[]
  scopes: string[]
  active: boolean
  review: boolean
  failed: boolean
  resolved: boolean
  cancelled: boolean
}

export type SyncHostActivity = {
  host: string
  entries: SyncActivityEntry[]
  active: number
  review: number
  failed: number
  done: number
  cancelled: number
}

export type SyncActivityOverview = {
  hosts: SyncHostActivity[]
  active: number
  review: number
  failed: number
  done: number
  cancelled: number
  total: number
}

function jobHost(job: SyncJob, view: SyncActivityView): string {
  const agent = (view.agents ?? []).find(item => item.id === job.agentId)
  if (agent) return agent.host
  const target = (view.targets ?? []).find(item => item.target.id === job.targetId)
  if (target) return target.target.host
  const environment = (view.environments ?? []).find(item => item.id === job.targetId)
  if (environment) return environment.host
  return 'Machine'
}

function scopeLabel(job: SyncJob, view: SyncActivityView): string {
  const target = (view.targets ?? []).find(item => item.target.id === job.targetId)
  if (target) return groupLabel(target.target)
  const environment = (view.environments ?? []).find(item => item.id === job.targetId)
  if (environment) return groupLabel(environment)
  return job.targetId ? 'Agent scope' : ''
}

function entryRank(entry: SyncActivityEntry): number {
  if (entry.active) return 0
  if (entry.failed) return 1
  if (entry.review) return 2
  if (entry.cancelled) return 3
  return 4
}

function entryFor(jobs: SyncJob[], view: SyncActivityView): SyncActivityEntry {
  const ordered = [...jobs].sort((a, b) => b.updatedAt - a.updatedAt)
  const job = ordered[0]
  const host = jobHost(job, view)
  return {
    key: FAILED.has(job.status) ? `${host}|${job.kind}|${job.status}|${job.message}` : job.id,
    host,
    job,
    jobs: ordered,
    scopes: [...new Set(ordered.map(item => scopeLabel(item, view)).filter(Boolean))],
    active: ACTIVE.has(job.status),
    review: job.status === 'awaiting-approval',
    failed: FAILED.has(job.status),
    resolved: RESOLVED.has(job.status),
    cancelled: job.status === 'cancelled',
  }
}

function count(entries: SyncActivityEntry[], match: (job: SyncJob) => boolean): number {
  return entries.reduce((total, entry) => total + entry.jobs.filter(match).length, 0)
}

function newest(entries: SyncActivityEntry[]): number {
  return Math.max(0, ...entries.map(entry => entry.job.updatedAt))
}

export function syncActivityOverview(jobs: SyncJob[], view: SyncActivityView = { environments: [], targets: [] }): SyncActivityOverview {
  const seen = new Set<string>()
  const grouped = new Map<string, SyncJob[]>()

  for (const job of jobs) {
    if (!job || seen.has(job.id)) continue
    seen.add(job.id)
    const host = jobHost(job, view)
    const key = FAILED.has(job.status) ? `${foldHost(host)}|${job.kind}|${job.status}|${job.message}` : job.id
    const list = grouped.get(key) ?? []
    list.push(job)
    grouped.set(key, list)
  }

  const entries = [...grouped.values()].map(list => entryFor(list, view))
  const byHost = new Map<string, { host: string; entries: SyncActivityEntry[] }>()
  for (const entry of entries) {
    const key = foldHost(entry.host)
    const group = byHost.get(key) ?? { host: entry.host, entries: [] }
    group.entries.push(entry)
    byHost.set(key, group)
  }

  const hosts = [...byHost.values()].map(({ host, entries: list }) => {
    list.sort((a, b) => entryRank(a) - entryRank(b) || b.job.updatedAt - a.job.updatedAt)
    return {
      host,
      entries: list,
      active: count(list, job => ACTIVE.has(job.status)),
      review: count(list, job => job.status === 'awaiting-approval'),
      failed: count(list, job => FAILED.has(job.status)),
      done: count(list, job => job.status === 'succeeded'),
      cancelled: count(list, job => job.status === 'cancelled'),
    }
  }).sort((a, b) => entryRank(a.entries[0]) - entryRank(b.entries[0]) || newest(b.entries) - newest(a.entries))

  return {
    hosts,
    total: seen.size,
    active: hosts.reduce((total, host) => total + host.active, 0),
    review: hosts.reduce((total, host) => total + host.review, 0),
    failed: hosts.reduce((total, host) => total + host.failed, 0),
    done: hosts.reduce((total, host) => total + host.done, 0),
    cancelled: hosts.reduce((total, host) => total + host.cancelled, 0),
  }
}
