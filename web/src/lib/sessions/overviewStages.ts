import { safeInsightURL, type InsightPull } from '../sessionInsights'
import type { FixKind } from './fixMessages'
import { splitChecks, type Tone } from './pullVerdict'

export type StepKey = 'code' | 'pr' | 'checks' | 'review' | 'merge'
export type StepState = 'done' | 'current' | 'attention' | 'upcoming' | 'unknown'
export type Step = { key: StepKey; label: string; state: StepState; detail: string } // label is 'Planning' for code while planning
export type NextAction = { label: string; kind: 'session' | 'link'; href?: string }
export type Next = { text: string; tone: Tone; primary?: NextAction; fix?: FixKind }
export type StageInput = { live: boolean; planning: boolean; needsInput: string; active: boolean; pull?: InsightPull; pullReason?: string }
export type Stages = { steps: Step[]; next: Next; merged?: { at?: number; url?: string } }

const READY = 'Ready. GitHub decides whether it can merge.'
const NOTHING = 'Nothing needs you right now.'
const THREADS_UNREAD = 'Threads not read on this machine'

// Short enough for a step's detail line; insightReason's full sentences belong to the GitHub tab.
const reasons: Record<string, string> = {
  github_unavailable: 'GitHub is not available on this machine',
  denied: 'Access was refused',
  busy: 'The machine is busy',
  timed_out: 'The check took too long',
  git_failed: 'Git could not be read',
  not_repo: 'This folder is not a Git repository',
}

function plural(n: number, one: string, many: string) { return `${n} ${n === 1 ? one : many}` }

// Absent threads mean "not read", never "none": only a machine that says it read the detail can vouch for them.
function threadsOf(pull: InsightPull) { return pull.detailKnown === true ? pull.threads : undefined }

function waitingOn(names: string[]) {
  if (names.length <= 2) return names.join(' and ')
  return `${names.slice(0, 2).join(' and ')}, plus ${names.length - 2} more`
}

export function deriveStages(input: StageInput): Stages {
  const { live, planning, needsInput, active, pull, pullReason } = input
  const open = pull?.state === 'OPEN'
  const merged = pull?.state === 'MERGED'
  const closed = pull?.state === 'CLOSED'
  const checks = pull?.checks ?? []
  const split = splitChecks(checks)
  const threads = pull ? threadsOf(pull) : undefined
  const openThreads = threads?.open ?? 0
  const conflicting = pull?.mergeable === 'CONFLICTING'
  const unavailable = !pull && !!pullReason && pullReason !== 'no_pr'
  const notRead = `Not read: ${reasons[pullReason || ''] || 'This view could not be read'}`
  const url = pull ? safeInsightURL(pull.url) : undefined

  const code: Step = planning
    ? { key: 'code', label: 'Planning', state: 'current', detail: 'The agent is in a planning mode' }
    : pull ? { key: 'code', label: 'Code', state: 'done', detail: 'A pull request exists' }
    : live && active ? { key: 'code', label: 'Code', state: 'current', detail: 'The agent is working' }
    : active ? { key: 'code', label: 'Code', state: 'attention', detail: 'Finished with changes and no pull request' }
    : { key: 'code', label: 'Code', state: 'upcoming', detail: 'Nothing yet' }

  let prStep: Step
  let checksStep: Step
  let reviewStep: Step
  let mergeStep: Step
  if (!pull) {
    const [state, detail] = unavailable ? (['unknown', notRead] as const) : (['upcoming', 'No pull request yet'] as const)
    prStep = { key: 'pr', label: 'PR', state, detail }
    checksStep = { key: 'checks', label: 'Checks', state, detail: unavailable ? notRead : 'No pull request' }
    reviewStep = { key: 'review', label: 'Review', state, detail: unavailable ? notRead : 'No pull request' }
    mergeStep = { key: 'merge', label: 'Merge', state, detail: unavailable ? notRead : 'Not yet' }
  } else {
    prStep = { key: 'pr', label: 'PR', state: pull.draft && open ? 'current' : 'done', detail: `#${pull.number}${pull.draft ? ' (draft)' : merged ? ' (merged)' : closed ? ' (closed)' : ''}` }

    // No reported checks is not green: CI may simply not have started, or may not exist.
    if (!checks.length) checksStep = { key: 'checks', label: 'Checks', state: 'unknown', detail: 'None reported' }
    else if (split.failed.length) checksStep = { key: 'checks', label: 'Checks', state: 'attention', detail: `${split.failed.length} of ${checks.length} failing` }
    else if (split.running.length) checksStep = { key: 'checks', label: 'Checks', state: 'current', detail: `${split.running.length} of ${checks.length} running` }
    else checksStep = { key: 'checks', label: 'Checks', state: 'done', detail: `${plural(checks.length, 'check', 'checks')} passed or skipped` }

    if (pull.draft && open) reviewStep = { key: 'review', label: 'Review', state: 'upcoming', detail: 'Draft pull request' }
    else if (pull.review === 'CHANGES_REQUESTED') reviewStep = { key: 'review', label: 'Review', state: 'attention', detail: 'Changes requested' }
    else if (openThreads > 0) reviewStep = { key: 'review', label: 'Review', state: 'attention', detail: `${plural(openThreads, 'open thread', 'open threads')}${threads?.more ? ' or more' : ''}` }
    else if (pull.review === 'REVIEW_REQUIRED') reviewStep = { key: 'review', label: 'Review', state: 'current', detail: 'Review required' }
    else if (pull.review === 'APPROVED') {
      // Approved but unread threads may still be open, so this is never done until they have been read.
      reviewStep = threads
        ? { key: 'review', label: 'Review', state: 'done', detail: 'Approved, no open threads' }
        : { key: 'review', label: 'Review', state: 'current', detail: `Approved. ${THREADS_UNREAD}` }
    } else reviewStep = threads
      ? { key: 'review', label: 'Review', state: 'current', detail: 'No review decision yet' }
      : { key: 'review', label: 'Review', state: 'unknown', detail: THREADS_UNREAD }

    const ready = open && !pull.draft && !conflicting && checksStep.state === 'done' && reviewStep.state === 'done'
    if (merged) mergeStep = { key: 'merge', label: 'Merge', state: 'done', detail: 'Merged' }
    else if (closed) mergeStep = { key: 'merge', label: 'Merge', state: 'unknown', detail: 'Closed without merging' }
    else if (conflicting) mergeStep = { key: 'merge', label: 'Merge', state: 'attention', detail: `Conflicts with ${pull.base || 'the base branch'}` }
    else if (pull.autoMerge) mergeStep = { key: 'merge', label: 'Merge', state: 'current', detail: 'Auto-merge is on' }
    else if (ready) mergeStep = { key: 'merge', label: 'Merge', state: 'current', detail: 'Everything reported is done' }
    else mergeStep = { key: 'merge', label: 'Merge', state: 'upcoming', detail: 'Not yet' }
  }

  const steps = [code, prStep, checksStep, reviewStep, mergeStep]
  const result = (next: Next): Stages => ({ steps, next })
  const open_ = (label: string): NextAction | undefined => url ? { kind: 'link', label, href: url } : undefined
  const withPrimary = (next: Next, primary: NextAction | undefined): Next => primary ? { ...next, primary } : next

  if (needsInput) return result({ text: needsInput, tone: 'warn', primary: { kind: 'session', label: 'Review' } })
  if (pull && merged) return { steps, next: withPrimary({ text: 'Merged.', tone: 'good' }, open_('Open pull request')), merged: { at: pull.mergedAt, url } }
  if (pull && closed) return result({ text: 'Closed without merging.', tone: 'muted' })
  if (pull && conflicting) return result({ ...withPrimary({ text: `Conflicts with ${pull.base || 'the base branch'}.`, tone: 'bad' }, open_('Open pull request')), fix: 'conflicts' })
  if (pull && split.failed.length) {
    const href = split.failed.map(c => safeInsightURL(c.url)).find(Boolean)
    const primary: NextAction | undefined = href ? { kind: 'link', label: 'Open failing check', href } : undefined
    return result({ ...withPrimary({ text: `${plural(split.failed.length, 'check', 'checks')} failing.`, tone: 'bad' }, primary), fix: 'checks' })
  }
  if (pull && pull.review === 'CHANGES_REQUESTED') {
    const next = withPrimary({ text: 'A reviewer asked for changes.', tone: 'bad' }, open_('Open review'))
    return result(openThreads > 0 ? { ...next, fix: 'threads' } : next)
  }
  if (pull && openThreads > 0) return result({ ...withPrimary({ text: `${plural(openThreads, 'unresolved review thread', 'unresolved review threads')}.`, tone: 'warn' }, open_('Open review')), fix: 'threads' })
  if (pull && split.running.length) return result({ text: `Waiting on ${waitingOn(split.running.map(c => c.name))}.`, tone: 'warn' })
  if (pull && pull.draft) return result({ text: 'Draft pull request. Mark it ready when the work is complete.', tone: 'muted' })
  if (pull && pull.review === 'REVIEW_REQUIRED') return result({ text: 'A review is required before this can merge.', tone: 'warn' })
  if (pull && checksStep.state === 'done' && reviewStep.state === 'done') return result(withPrimary({ text: READY, tone: 'good' }, open_('Open pull request')))
  if (unavailable && pullReason === 'github_unavailable') return result({ text: 'GitHub status is not available on this machine.', tone: 'muted' })
  if (!pull && !live && active) return result({ text: 'No pull request yet.', tone: 'muted' })
  return result({ text: NOTHING, tone: 'muted' })
}
