import type { Machine, SyncSkills } from '../api'
import { sameHost } from '../contentReview'

/** Inventory is not sync evidence. Only the reconcile report can say a machine is in sync. */
/** `lockedDetail` is what to say when the machine's owner locked it: a blocked report on a locked machine is that, not allow-apply being off. */
export function machineSkillStatus(machine: Pick<Machine, 'host'>, skills: SyncSkills | null, lockedDetail = '') {
  const report = skills?.machines.find(m => sameHost(m.host, machine.host))
  if (!report) return { tone: 'attention', label: 'Skill sync not confirmed', detail: 'Open Sync to check the connection and skill reports.' }
  const detail = `${skills!.active} Active ${skills!.active === 1 ? 'skill follows' : 'skills follow'} your library.`
  switch (report.state) {
    case 'ok': return { tone: 'synced', label: 'Skills in sync', detail }
    case 'installing': case 'removing': case 'updating':
      return { tone: 'working', label: report.state === 'removing' ? 'Taking skills off…' : report.state === 'updating' ? 'Updating skills…' : 'Installing skills…', detail }
    case 'paused': return { tone: 'attention', label: 'Sync paused', detail: 'This machine is left alone until you resume sync.' }
    case 'offline': return { tone: 'attention', label: 'Machine offline', detail: report.waiting.length ? "Pending skills catch up when it's back." : 'Its last report does not confirm its current files.' }
    case 'blocked': return { tone: 'attention', label: "Can't write skills here", detail: lockedDetail || 'Allow local skill writes on this machine. Open Sync for the command.' }
    case 'needs-update': return { tone: 'attention', label: 'Update reet on this machine', detail: 'This connector needs the current skills protocol.' }
    case 'needs-newer-controller': return { tone: 'attention', label: 'Update this controller', detail: 'This machine uses a newer skills protocol.' }
    case 'failed': return { tone: 'attention', label: 'Skill sync failed', detail: report.failed.map(f => `${f.name}: ${f.detail}`).join('; ') || 'Open Sync to read the failure and retry.' }
    default: return { tone: 'attention', label: 'Skill sync not confirmed', detail: 'Open Sync to check the latest report.' }
  }
}
