// The words and rules of a pipeline's delivery review, kept apart from the card that draws them so they can be tested.
// Nothing here reads the network or the page. Types come from the API client; the server's contract is
// docs/handoffs/factory-delivery-review-design.md.
import type { DeliveryEvidence, DeliveryReview, DeliveryStatus } from '../api'

export const STATUS_WORD: Record<DeliveryStatus, string> = { passed: 'Passed', needs_work: 'Needs work', unverified: 'Unverified' }
/** A shape for each status, so it never rests on colour alone: a tick, a cross, a question mark. */
export const STATUS_MARK: Record<DeliveryStatus, string> = { passed: '✓', needs_work: '✕', unverified: '?' }
export const KIND_WORD: Record<DeliveryEvidence['kind'], string> = { note: 'Reviewer note', artefact: 'Artefact', check: 'GitHub check' }

/** The commit in a few characters; the full one belongs in a title and the clipboard. */
export const shortCommit = (sha: string | undefined): string => (sha ?? '').slice(0, 7)

/** A fresh request ID: 32 lowercase hex characters. It makes a decision or a piece of evidence idempotent. */
export const newRequestId = (): string => Array.from(crypto.getRandomValues(new Uint8Array(16)), byte => byte.toString(16).padStart(2, '0')).join('')

/** The statuses exactly as the reviewer was shown them, by criterion ID. */
export const statusMap = (review: Pick<DeliveryReview, 'criteria'>): Record<string, DeliveryStatus> => Object.fromEntries(review.criteria.map(item => [item.id, item.status]))

export type StatusCount = { passed: number; needsWork: number; unverified: number; notPassed: number; total: number }
export function statusCount(review: Pick<DeliveryReview, 'criteria'>): StatusCount {
  const count = (status: DeliveryStatus) => review.criteria.filter(item => item.status === status).length
  const passed = count('passed'), needsWork = count('needs_work'), unverified = count('unverified')
  return { passed, needsWork, unverified, notPassed: needsWork + unverified, total: review.criteria.length }
}
/** "2 of 3 criteria passed · 1 needs work", the line over the list. */
export function summaryLine(count: StatusCount): string {
  if (!count.total) return 'No criteria are agreed for this pipeline.'
  const parts = [`${count.passed} of ${count.total} ${count.total === 1 ? 'criterion' : 'criteria'} passed`]
  if (count.needsWork) parts.push(`${count.needsWork} needs work`)
  if (count.unverified) parts.push(`${count.unverified} unverified`)
  return parts.join(' · ')
}

/** Whether a decision can be recorded: the commit under review has to be known, or known to be absent. */
export function commitBlocker(review: Pick<DeliveryReview, 'commitState' | 'commitNote'>): string {
  if (review.commitState === 'verified' || review.commitState === 'none') return ''
  return `The reviewed commit could not be verified${review.commitNote ? `: ${review.commitNote}` : '.'} A decision is not recorded until it can.`
}
/** What the commit state says, in a few words. */
export function commitStateWord(state: DeliveryReview['commitState']): string {
  return state === 'verified' ? 'Verified on the machine' : state === 'none' ? 'Not a Git repository' : 'Could not be verified'
}

/** What accepting leads to, in a sentence. It always says that accepting is not the merge approval. */
export function nextWords(next: DeliveryReview['next']): string {
  if (next === 'ready_to_merge') return 'The pipeline then waits at Ready to merge. Accepting is not approving the merge: Approve the merge stays your separate decision.'
  if (next === 'merge') return 'The pipeline then goes on to its merge stage and merges by itself once its checks pass, because you set it to. Accepting is not the merge approval.'
  if (next === 'done') return 'The pipeline then finishes: it has no merge stage. Accepting is not approving any merge.'
  return 'Accepting is not approving the merge: that stays a separate decision of yours.'
}

/** The confirmation for Accept: it names how many criteria are not Passed when any are. */
export function acceptConfirm(review: Pick<DeliveryReview, 'criteria' | 'commit' | 'title' | 'next'>): { title: string; body: string; confirmLabel: string } {
  const count = statusCount(review)
  const commit = review.commit ? `commit ${shortCommit(review.commit)} of “${review.title}”` : `the work of “${review.title}”, which has no Git commit`
  const parts: string[] = []
  if (count.needsWork) parts.push(`${count.needsWork} ${count.needsWork === 1 ? 'needs' : 'need'} work`)
  if (count.unverified) parts.push(`${count.unverified} unverified`)
  const gap = count.notPassed ? ` ${count.notPassed} of ${count.total} ${count.total === 1 ? 'criterion is' : 'criteria are'} not Passed (${parts.join(', ')}).` : ''
  return {
    title: count.notPassed ? `Accept with ${count.notPassed} not Passed?` : 'Accept this delivery?',
    body: `You are accepting ${commit}.${gap} ${nextWords(review.next)}`,
    confirmLabel: count.notPassed ? 'Accept anyway' : 'Accept delivery',
  }
}

/** Why a send-back cannot be sent yet, or ''. Notes are required when no criterion needs work. */
export function sendBackBlocker(review: Pick<DeliveryReview, 'criteria'>, notes: string, stage: string): string {
  if (!stage) return 'Choose the stage to send it back to.'
  if (!notes.trim() && !review.criteria.some(item => item.status === 'needs_work')) return 'Say what is wrong. No criterion is marked Needs work, so the agent has nothing else to go on.'
  return ''
}

/** Which kind of earlier evidence this is: one from an earlier commit, from earlier criteria, or neither is known. */
export function earlierLabel(evidence: Pick<DeliveryEvidence, 'historical' | 'commit'>, review: Pick<DeliveryReview, 'commit'>): string {
  if (!evidence.historical) return ''
  if (evidence.commit && review.commit && evidence.commit !== review.commit) return 'Earlier commit'
  if (evidence.commit && review.commit && evidence.commit === review.commit) return 'Earlier criteria'
  return 'Earlier evidence'
}

/** What a 409's code says changed. An unknown code gives ''. */
const CONFLICTS: Record<string, string> = {
  delivery_closed: 'Someone else already decided this delivery.',
  delivery_changed: 'The review changed while you were looking at it: evidence or a decision was added.',
  delivery_criteria_changed: 'The criteria were changed while you were looking at them.',
  delivery_head_changed: 'A new commit arrived since you looked.',
  delivery_head_unverified: 'The commit could not be verified just now, so nothing was recorded.',
  delivery_request_reused: 'That request was already used for a different decision, so nothing was recorded.',
}
export const conflictWords = (code: string): string => CONFLICTS[code] ?? ''
/** What the card says after a conflict, once the review has been read again. */
export const conflictAdvice = 'The review below is as it is now. What you typed is kept. Look again, then decide on what is shown.'

/** Whether a failed request may have reached the server, so retrying it must carry the same request ID: a network
 *  failure or a server error. A refusal that names a problem (a 4xx) was answered, so the next attempt is a new one. */
export const mayHaveArrived = (error: unknown): boolean => {
  const status = typeof error === 'object' && error !== null && 'status' in error ? Number((error as { status: unknown }).status) : 0
  return !status || status >= 500 || status === 408
}

/** One decision as the history reads: "Accepted on 9f2c1ab by Paul" with the notes beside it. */
export function decisionLine(d: { decision: string; commit?: string; byName?: string; stage?: string }): string {
  const who = d.byName ? ` by ${d.byName}` : ''
  const commit = d.commit ? ` on ${shortCommit(d.commit)}` : ' with no Git commit'
  return d.decision === 'accept' ? `Accepted${commit}${who}` : `Sent back${d.stage ? ` to ${d.stage}` : ''}${commit}${who}`
}

/** The request ID of one attempt, kept while the same attempt is tried again. A request that may have arrived (a lost
 *  answer) is sent again with the ID it had, so the server records it once; a changed request gets a new one. */
export function attemptIds(): { idFor: (signature: string) => string; clear: () => void } {
  let held: { signature: string; id: string } | null = null
  return {
    idFor(signature) { if (held?.signature !== signature) held = { signature, id: newRequestId() }; return held.id },
    clear() { held = null },
  }
}
