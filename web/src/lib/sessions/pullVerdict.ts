import { checkTone, type InsightCheck, type InsightPull } from '../sessionInsights'

export type Tone = 'bad' | 'warn' | 'good' | 'muted'
export type Verdict = { tone: Tone; icon: 'conflict' | 'clock' | 'check' | 'github'; title: string; detail: string }

// One place decides what a pull request's state means, so the GitHub tab and the Overview can never disagree about it.
export function splitChecks(checks: InsightCheck[]): { failed: InsightCheck[]; running: InsightCheck[]; passed: InsightCheck[] } {
  return {
    failed: checks.filter(c => checkTone(c.state) === 'bad'),
    running: checks.filter(c => checkTone(c.state) === 'warn'),
    passed: checks.filter(c => checkTone(c.state) === 'good'),
  }
}

function count(n: number, one: string, many = `${one}s`) { return `${n} ${n === 1 ? one : many}` }
function nameList(list: InsightCheck[], limit = 2) {
  const names = list.slice(0, limit).map(c => c.name)
  if (!names.length) return 'the reported checks'
  const more = list.length - names.length
  if (names.length === 1) return names[0]
  return `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}${more > 0 ? `, plus ${count(more, 'more')}` : ''}`
}

export function pullVerdict(pr: InsightPull): Verdict {
  const checks = pr.checks ?? []
  const { failed, running, passed } = splitChecks(checks)
  const conflict = pr.mergeable === 'CONFLICTING'
  const changesRequested = pr.review === 'CHANGES_REQUESTED'
  const approved = pr.review === 'APPROVED'
  const reviewRequired = pr.review === 'REVIEW_REQUIRED'
  const total = checks.length
  if (pr.state === 'MERGED') return { tone: 'good', icon: 'check', title: 'Pull request merged', detail: 'This branch’s pull request is merged.' }
  if (pr.state === 'CLOSED') return { tone: 'muted', icon: 'github', title: 'Pull request closed', detail: 'This pull request was closed without merging.' }
  if (conflict) {
    const also = failed.length ? ` ${count(failed.length, 'check')} ${failed.length === 1 ? 'is' : 'are'} also failing.` : changesRequested ? ' A reviewer also requested changes.' : ''
    return { tone: 'bad', icon: 'conflict', title: 'Resolve merge conflicts', detail: `GitHub reports conflicts with the base branch.${also}` }
  }
  if (failed.length) {
    const also = changesRequested ? ' A reviewer also requested changes.' : ''
    return { tone: 'bad', icon: 'conflict', title: `${count(failed.length, 'check')} failed`, detail: `Failing: ${nameList(failed)}.${also}` }
  }
  if (changesRequested) return { tone: 'bad', icon: 'conflict', title: 'Changes requested', detail: 'A reviewer asked for changes before this can merge.' }
  if (running.length) return { tone: 'warn', icon: 'clock', title: `${count(running.length, 'check')} running`, detail: `Waiting on ${nameList(running)}.` }
  if (pr.draft) return { tone: 'muted', icon: 'github', title: 'Draft pull request', detail: 'Mark it ready for review when the work is complete.' }
  if (reviewRequired) return { tone: 'warn', icon: 'clock', title: 'Review required', detail: 'A review is required before this can merge.' }
  if (!total) return { tone: 'muted', icon: 'github', title: 'No checks reported', detail: 'This does not confirm that CI passed.' }
  if (approved) return { tone: 'good', icon: 'check', title: 'Checks passed', detail: `Review approved. ${count(passed.length, 'check')} passed.` }
  return { tone: 'good', icon: 'check', title: 'Checks passed', detail: `${count(passed.length, 'check')} passed. No review decision yet.` }
}
