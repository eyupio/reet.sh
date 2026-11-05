import type { ChatState } from './chat'

export type InsightFile = { path: string; status?: string; added: number; removed: number; binary?: boolean; directory?: boolean }
export type InsightCheck = { name: string; state: string; url?: string }
export type InsightPull = { number: number; title: string; url: string; state: string; draft: boolean; review: string; mergeable: string; added: number; removed: number; checks?: InsightCheck[]; more?: boolean
  // Added by the extended read. Absent means the machine did not read it (never zero); times are ms.
  base?: string; head?: string; createdAt?: number; mergedAt?: number; closedAt?: number; lastPushAt?: number
  autoMerge?: boolean
  reviews?: { approved: number; changesRequested: number; commented: number; requested: number }
  threads?: { open: number; resolved: number; more?: boolean }
  detailKnown?: boolean }
export type SessionInsight = { reason?: string; checkedAt?: number; repo?: boolean; branch?: string; ahead: number; behind: number; upstream?: boolean; changed: number; changedMore?: boolean; countsKnown?: boolean; conflicts: number; files?: InsightFile[]; more?: boolean; text?: string; binary?: boolean; pull?: InsightPull; remote?: string }
export type InsightView = 'status' | 'files' | 'file' | 'diff' | 'github'

export function checkTone(state: string): 'good' | 'bad' | 'warn' {
  if (['SUCCESS', 'NEUTRAL', 'SKIPPED'].includes(state)) return 'good'
  if (['FAILURE', 'ERROR', 'TIMED_OUT', 'CANCELLED', 'ACTION_REQUIRED', 'STARTUP_FAILURE'].includes(state)) return 'bad'
  return 'warn'
}
export function checkWord(state: string): string { return state.toLowerCase().replaceAll('_', ' ') || 'unknown' }
export function checkLabel(state: string): string {
  const labels: Record<string, string> = {
    SUCCESS: 'Passed', FAILURE: 'Failed', ERROR: 'Error', TIMED_OUT: 'Timed out', CANCELLED: 'Cancelled',
    ACTION_REQUIRED: 'Action required', STARTUP_FAILURE: 'Startup failure', NEUTRAL: 'Neutral', SKIPPED: 'Skipped',
    QUEUED: 'Queued', IN_PROGRESS: 'Running', PENDING: 'Pending', WAITING: 'Waiting', REQUESTED: 'Requested',
  }
  return labels[state] || checkWord(state)
}
export function safeInsightURL(link?: string): string | undefined {
  try { const u = new URL(link || ''); return u.protocol === 'https:' && !u.username && !u.password ? u.href : undefined } catch { return undefined }
}
export function insightReason(reason?: string): string {
  const reasons: Record<string, string> = {
    unavailable: 'This session has no accessible working folder. Start a session in a project to inspect its workspace.',
    denied: 'File browsing is switched off on this machine, or access was refused.',
    busy: 'The machine is checking another workspace. Try again in a moment.',
    timed_out: 'The workspace check took too long. Try again.',
    not_repo: 'This folder is not a Git repository root. You can still browse its files.',
    git_failed: 'Git status could not be read safely. Check Git on the machine, then refresh.',
    no_pr: 'No pull request was found for this branch.',
    github_unavailable: 'GitHub status is unavailable. This needs a github.com origin and the GitHub CLI signed in on this machine.',
    unreadable: 'This path could not be read. Links, special files and Git metadata are excluded.',
  }
  return reasons[reason || ''] || 'This view could not be read. Try again.'
}
export function fileWord(file: InsightFile): string {
  if (file.directory) return 'Folder'
  if (!file.status) return 'File'
  if (file.status.includes('U') || ['AA', 'DD'].includes(file.status)) return 'Conflict'
  if (file.status === '??') return 'Untracked'
  if (file.status.includes('D')) return 'Deleted'
  if (file.status.includes('R')) return 'Renamed'
  if (file.status.includes('A')) return 'Added'
  return 'Modified'
}
export function chatInsight(chat?: ChatState) {
  const tools = chat?.items.filter(item => item.kind === 'tool') ?? []
  const active = tools.filter(item => item.status === 'in_progress' || item.status === 'pending')
  const failed = tools.filter(item => item.status === 'failed')
  const done = tools.filter(item => item.status === 'completed')
  const latest = [...(chat?.items ?? [])].reverse().find(item => item.kind === 'agent' && item.text.trim())
  return { active, failed, done, total: tools.length, latest: latest?.kind === 'agent' ? latest.text : '' }
}
export function elapsedWord(at: number | undefined, now: number): string {
  if (!at) return 'not recorded'
  const seconds = Math.max(0, Math.floor((now - at) / 1000))
  if (seconds < 60) return 'just now'
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m ago`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ${Math.floor(seconds % 3600 / 60)}m ago`
  return `${Math.floor(seconds / 86400)}d ago`
}
