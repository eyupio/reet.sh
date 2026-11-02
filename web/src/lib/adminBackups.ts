import type { BackupItem, BackupRun } from './api'

export type BackupVerified = Record<string, { ok: boolean; problems: string[] }>
export type BackupCopyScope = 'all' | 'local' | 'remote' | 'damaged'
export type BackupRunScope = 'all' | 'succeeded' | 'failed' | 'running'

export const BACKUP_COPY_SCOPES: { key: BackupCopyScope; label: string }[] = [
  { key: 'all', label: 'All' },
  { key: 'local', label: 'This machine' },
  { key: 'remote', label: 'Off-site' },
  { key: 'damaged', label: 'Damaged' },
]

export const BACKUP_RUN_SCOPES: { key: BackupRunScope; label: string }[] = [
  { key: 'all', label: 'All' },
  { key: 'succeeded', label: 'Succeeded' },
  { key: 'failed', label: 'Failed' },
  { key: 'running', label: 'Running' },
]

export function backupCopyMatchesSearch(item: BackupItem, query: string, verified: BackupVerified = {}): boolean {
  const term = query.trim().toLowerCase()
  if (!term) return true
  const problems = verified[item.id]?.problems ?? []
  const haystack = `${item.id} ${item.target} ${item.version ?? ''} ${item.local ? 'this machine local' : 'off-site'} ${item.remotes.join(' ')} ${problems.join(' ')}`.toLowerCase()
  return term.split(/\s+/).every((word) => haystack.includes(word))
}

function inCopyScope(item: BackupItem, scope: BackupCopyScope, verified: BackupVerified): boolean {
  if (scope === 'all') return true
  if (scope === 'local') return item.local
  if (scope === 'remote') return item.remotes.length > 0
  return Boolean(verified[item.id] && !verified[item.id].ok)
}

export function filterBackupCopies(items: BackupItem[], scope: BackupCopyScope, query = '', verified: BackupVerified = {}): BackupItem[] {
  return items.filter((item) => inCopyScope(item, scope, verified) && backupCopyMatchesSearch(item, query, verified))
}

export function backupCopyScopeCounts(items: BackupItem[], verified: BackupVerified = {}): Record<BackupCopyScope, number> {
  return {
    all: items.length,
    local: items.filter((item) => item.local).length,
    remote: items.filter((item) => item.remotes.length > 0).length,
    damaged: items.filter((item) => verified[item.id] && !verified[item.id].ok).length,
  }
}

export function backupCopyCountLabel(shown: number, total: number): string {
  if (total === 0) return 'No copies'
  if (shown === total) return `${total} ${total === 1 ? 'copy' : 'copies'}`
  return `${shown} of ${total} copies`
}

export function backupRunMatchesSearch(run: BackupRun, query: string): boolean {
  const term = query.trim().toLowerCase()
  if (!term) return true
  const haystack = `${run.id} ${run.trigger} ${run.state} ${run.offsite} ${run.object} ${run.error}`.toLowerCase()
  return term.split(/\s+/).every((word) => haystack.includes(word))
}

export function filterBackupRuns(runs: BackupRun[], scope: BackupRunScope, query = ''): BackupRun[] {
  return runs.filter((run) => (scope === 'all' || run.state === scope) && backupRunMatchesSearch(run, query))
}

export function backupRunScopeCounts(runs: BackupRun[]): Record<BackupRunScope, number> {
  return {
    all: runs.length,
    succeeded: runs.filter((run) => run.state === 'succeeded').length,
    failed: runs.filter((run) => run.state === 'failed').length,
    running: runs.filter((run) => run.state === 'running').length,
  }
}

export function backupRunCountLabel(shown: number, total: number): string {
  if (total === 0) return 'No runs'
  if (shown === total) return `${total} ${total === 1 ? 'run' : 'runs'}`
  return `${shown} of ${total} runs`
}
