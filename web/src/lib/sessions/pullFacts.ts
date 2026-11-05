import type { InsightPull } from '../sessionInsights'

// The words the pull request card uses for facts the machine may or may not have read. Pure, so a node test holds the
// honesty rules: a count the machine did not read is "not read", never zero.

export type Fact = { text: string; tone: 'bad' | 'warn' | 'good' | 'muted' }

const DECISIONS: Record<string, string> = {
  CHANGES_REQUESTED: 'Changes requested',
  APPROVED: 'Approved',
  REVIEW_REQUIRED: 'Review required',
}

/** Who has said what. Only a machine that read the detail can vouch for the counts; otherwise the decision word is all there is. */
export function reviewsLine(pull: InsightPull): string {
  const r = pull.detailKnown === true ? pull.reviews : undefined
  if (!r) return DECISIONS[pull.review] || 'No review decision yet'
  const parts: string[] = []
  if (r.approved > 0) parts.push(`${r.approved} approved`)
  if (r.changesRequested > 0) parts.push(`changes requested by ${r.changesRequested}`)
  if (r.commented > 0) parts.push(`${r.commented} commented`)
  if (r.requested > 0) parts.push(`${r.requested} requested`)
  if (parts.length) return parts.join(' · ')
  // The decision can say more than an empty tally (a required review nobody has answered yet).
  return DECISIONS[pull.review] || 'No reviews yet'
}

/** Open review threads. A pull request that is not open has none worth reading, so the slot stays empty. */
export function threadsFact(pull: InsightPull): Fact {
  if (pull.state !== 'OPEN') return { text: '', tone: 'muted' }
  const t = pull.detailKnown === true ? pull.threads : undefined
  if (!t) return { text: 'Review threads not read on this machine', tone: 'muted' }
  if (t.more) return { text: '100+ threads, not all read', tone: 'warn' }
  if (t.open > 0) return { text: `${t.open} open ${t.open === 1 ? 'thread' : 'threads'}`, tone: 'warn' }
  return { text: 'No open threads', tone: 'good' }
}

export function conflictFact(pull: InsightPull): Fact {
  if (pull.state !== 'OPEN') return { text: '', tone: 'muted' }
  if (pull.mergeable === 'CONFLICTING') return { text: `Conflicts with ${pull.base || 'the base branch'}`, tone: 'bad' }
  if (pull.mergeable === 'MERGEABLE') return { text: 'No conflicts reported', tone: 'muted' }
  return { text: 'Conflicts not reported yet', tone: 'muted' }
}

export type PullRead = { supported: boolean; loading: boolean; reason?: string }

/** The line that stands where a pull request card would be. Only `no_pr` says there is none; any other absence says it was not read. */
export function placeholderLine({ supported, loading, reason }: PullRead): string {
  if (!supported) return 'Update reet on this machine for GitHub status.'
  if (loading) return 'Checking GitHub…'
  if (reason === 'no_pr') return 'No pull request yet.'
  if (reason === 'github_unavailable') return 'GitHub status is not available on this machine.'
  // No answer and nothing running (Live updates off, machine was offline): say so rather than claim a read in progress.
  return reason ? 'GitHub status could not be read.' : 'Not read yet.'
}

export type ReadDue = { auto: boolean; canRead: boolean; hidden: boolean; pending: boolean; busy: boolean; hasStored: boolean; ageMs: number; cadenceMs: number }

/** Whether an automatic GitHub read should start now. The mount, the timer and the visibility handler all ask this, with the shared store's state (not a local copy that may not be seeded yet), so a visible session costs one read per cadence. */
export function githubReadDue({ auto, canRead, hidden, pending, busy, hasStored, ageMs, cadenceMs }: ReadDue): boolean {
  if (!auto || !canRead || hidden || pending || busy) return false
  return !hasStored || ageMs >= cadenceMs
}
