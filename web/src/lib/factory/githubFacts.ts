import { checkTone, type InsightCheck } from '../sessionInsights'

// The GitHub facts a Factory pipeline's detail shows: what CI is doing, how long it ran, and how often Gaffer had to
// put a stage right. Pure, so a node test holds the honesty rules: a fact the machine did not report is not invented.

export type CheckCounts = { failed: InsightCheck[]; running: InsightCheck[]; passed: InsightCheck[] }

export function ciCounts(checks: readonly InsightCheck[] | undefined): CheckCounts {
  const own = checks ?? []
  return {
    failed: own.filter(c => checkTone(c.state) === 'bad'),
    running: own.filter(c => checkTone(c.state) === 'warn'),
    passed: own.filter(c => checkTone(c.state) === 'good'),
  }
}

/** What the checks are doing, in one line, or that none were reported. */
export function ciWord(checks: readonly InsightCheck[] | undefined): string {
  const { failed, running, passed } = ciCounts(checks)
  const parts: string[] = []
  if (failed.length) parts.push(`${failed.length} failed`)
  if (running.length) parts.push(`${running.length} running`)
  if (passed.length) parts.push(`${passed.length} passed`)
  return parts.length ? parts.join(' · ') : 'No checks reported'
}

const seconds = (ms: number) => Math.max(0, Math.round(ms / 1000))
function span(ms: number): string {
  const s = seconds(ms)
  if (s < 60) return `${s}s`
  if (s < 3600) return `${Math.floor(s / 60)}m ${s % 60}s`
  return `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`
}

export type CiRun = { text: string; running: boolean; tone: 'good' | 'warn' | 'muted' }

/** How long the CI checks have been running, or ran. Only checks that reported a start time count; when any check is
 *  still running the clock runs to now. No start time means the run is not reported, never zero. */
export function ciRun(checks: readonly InsightCheck[] | undefined, now: number): CiRun {
  const own = checks ?? []
  const starts = own.flatMap(c => (c.startedAt && c.startedAt > 0 ? [c.startedAt] : []))
  if (!starts.length) return { text: 'Not reported', running: false, tone: 'muted' }
  const running = own.some(c => checkTone(c.state) === 'warn')
  const ends = own.flatMap(c => (c.completedAt && c.completedAt > 0 ? [c.completedAt] : []))
  if (!running && !ends.length) return { text: 'Not reported', running: false, tone: 'muted' }
  const start = Math.min(...starts)
  const end = running ? now : Math.max(...ends)
  return { text: `${running ? 'Running' : 'Ran'} ${span(Math.max(0, end - start))}`, running, tone: running ? 'warn' : 'good' }
}

/** A Gaffer choice that puts work right: another review, an instruction to the agent, or the stage run again. */
export const FIX_CHOICES = ['again', 'tell', 'restart'] as const

export type FixDecision = { key: string; stage: string; choice?: string; failed?: boolean }

/** How many times Gaffer put one pipeline's stages right, and which stages. A choice that failed is not a fix. */
export function gafferFixes(decisions: readonly FixDecision[] | undefined, key: string): { count: number; stages: string[] } {
  const own = (decisions ?? []).filter(d => d.key === key && !d.failed && !!d.choice && (FIX_CHOICES as readonly string[]).includes(d.choice))
  return { count: own.length, stages: [...new Set(own.map(d => d.stage))] }
}
