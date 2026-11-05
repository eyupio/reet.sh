// The run clock: when a session started and how long it ran. Pure, with no timer of its own;
// callers pass the `now` that SessionsPage already ticks, so a panel never adds a second clock.
import type { TerminalSession } from '../terminal/api'

const MINUTE = 60_000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

// Live means a process may still be running. Anything else is finished, and a finished session
// with no completedAt (an older row, or one that was lost) shows no length rather than a guess.
const LIVE = new Set(['creating', 'running', 'end_requested'])

export function formatLength(ms: number): string {
  // A clock that ran backwards, or a missing value, must never print a negative length.
  if (!Number.isFinite(ms) || ms < MINUTE) return 'under a minute'
  if (ms < HOUR) return `${Math.floor(ms / MINUTE)}m`
  if (ms < DAY) return `${Math.floor(ms / HOUR)}h ${Math.floor((ms % HOUR) / MINUTE)}m`
  return `${Math.floor(ms / DAY)}d ${Math.floor((ms % DAY) / HOUR)}h`
}

export function startLabel(createdAt: number, now: number, locale?: string): string {
  const start = new Date(createdAt)
  const time = new Intl.DateTimeFormat(locale, { hour: '2-digit', minute: '2-digit', hourCycle: 'h23' }).format(start)
  if (start.toDateString() === new Date(now).toDateString()) return time
  const day = new Intl.DateTimeFormat(locale, { day: 'numeric', month: 'short' }).format(start)
  return `${day}, ${time}`
}

export function runLine(s: Pick<TerminalSession, 'state' | 'createdAt' | 'completedAt'>, now: number, locale?: string): string {
  // A missing or zero start would read as 1 January 1970 and a run of twenty thousand days; say nothing instead.
  if (!(typeof s.createdAt === 'number' && Number.isFinite(s.createdAt) && s.createdAt > 0)) return ''
  const started = `Started ${startLabel(s.createdAt, now, locale)}`
  if (LIVE.has(s.state)) return `${started} · running ${formatLength(now - s.createdAt)}`
  if (s.completedAt == null) return started
  return `Ran ${formatLength(s.completedAt - s.createdAt)}`
}
