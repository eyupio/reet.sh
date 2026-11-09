import type { ContentInventory, LibraryItem, SyncJob, SyncSkill, SyncTarget } from './api'

/**
 * What one library item is doing on one machine, in plain words, from what the
 * controller actually holds: the saved selection, the item's status, the
 * machine's last report and its latest jobs. It never invents progress and
 * never says "Up to date": the controller knows an item was seen in the
 * machine's last report, not that its bytes were compared with the current
 * revision, so "Installed" says only that.
 */
export type ItemMachineStateKey = 'unselected' | 'inactive' | 'waiting_content' | 'waiting' | 'waiting_offline' | 'syncing' | 'attention' | 'verified'
export type ItemMachineState = { key: ItemMachineStateKey; label: string; detail?: string; tone: 'muted' | 'wait' | 'work' | 'bad' | 'ok' }

const RUNNING = new Set(['queued', 'running', 'awaiting-approval'])
const FINISHED = new Set(['succeeded', 'failed'])

type Input = { item: LibraryItem; selected: boolean; target?: SyncTarget; jobs: SyncJob[]; online: boolean }

const plain = (status: string) => status.replaceAll('_', ' ')
const same = (skill: SyncSkill, item: LibraryItem) => (skill.itemId ? skill.itemId === item.id : skill.slug === item.name)

export function itemMachineState({ item, selected, target, jobs, online }: Input): ItemMachineState {
  if (!selected) return { key: 'unselected', label: 'Not on this machine', tone: 'muted' }
  if (item.status !== 'in_use') {
    return { key: 'inactive', label: 'Kept in the library only', detail: `It is ${plain(item.status)}, so it is not installed here and any copy reet.sh installed is removed. Your selection is kept.`, tone: 'muted' }
  }
  if (item.problem || item.contentState !== 'complete') {
    return { key: 'waiting_content', label: 'Waiting for complete content', detail: 'It will sync once its content has been captured.', tone: 'wait' }
  }
  const waiting: ItemMachineState = online
    ? { key: 'waiting', label: 'Waiting to sync', tone: 'wait' }
    : { key: 'waiting_offline', label: 'Offline — update waiting', detail: 'It will sync when this machine reconnects.', tone: 'wait' }
  if (!target) return waiting
  const desired = target.desired.find(skill => same(skill, item))
  const slug = desired?.slug ?? item.name
  const mine = jobs.filter(job => job.targetId === target.target.id)
  if (mine.some(job => RUNNING.has(job.status))) return { key: 'syncing', label: 'Syncing', tone: 'work' }
  if (!desired) return waiting
  const latest = mine.filter(job => FINISHED.has(job.status)).sort((a, b) => b.updatedAt - a.updatedAt)[0]
  const problem = latest?.plan?.problems?.[slug]
  if (problem) return { key: 'attention', label: 'Needs attention', detail: problem, tone: 'bad' }
  if (latest?.status === 'failed' && latest.plan?.actions?.some(action => same(action.skill, item))) {
    return { key: 'attention', label: 'Needs attention', detail: latest.message || 'The last sync of this item failed.', tone: 'bad' }
  }
  if (target.missing.some(skill => same(skill, item))) return waiting
  return { key: 'verified', label: 'Installed', detail: 'Seen in this machine’s last report.', tone: 'ok' }
}

/**
 * Skills a machine reports that the library does not have, for the import step of connecting a machine. A skill counts as
 * already in the library when an item has the same name, ignoring case. Only complete skills can be imported, so incomplete
 * ones are counted separately instead of offered. Nothing is offered until the machine's content has been observed: a scan
 * that has not been read is not evidence that a skill is missing.
 */
export function foundNotInLibrary(content: ContentInventory | undefined, items: Pick<LibraryItem, 'name' | 'contentState'>[]): { importable: string[]; incomplete: number } {
  if (content?.state !== 'observed') return { importable: [], incomplete: 0 }
  // A library entry that has no content yet (a skill reet.sh only learned of from this machine) is not a saved copy: the machine's files are what it is waiting for.
  const known = new Set(items.filter(item => item.contentState === 'complete').map(item => item.name.trim().toLowerCase()))
  const importable = new Set<string>()
  const incomplete = new Set<string>()
  for (const skill of content.skills) {
    const key = skill.name.trim().toLowerCase()
    if (known.has(key)) continue
    if (skill.complete) importable.add(skill.name)
    else incomplete.add(key)
  }
  return { importable: [...importable].sort((a, b) => a.localeCompare(b)), incomplete: incomplete.size }
}
