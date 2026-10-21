import type { Stage } from './shelf'

const MINUTE = 60_000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR
const WEEK = 7 * DAY
const MONTH = 30 * DAY
const YEAR = 365 * DAY

function unit(ms: number): { n: number; unit: string } {
  if (ms < HOUR) return { n: Math.floor(ms / MINUTE), unit: 'minute' }
  if (ms < DAY) return { n: Math.floor(ms / HOUR), unit: 'hour' }
  if (ms < 2 * WEEK) return { n: Math.floor(ms / DAY), unit: 'day' }
  if (ms < 2 * MONTH) return { n: Math.floor(ms / WEEK), unit: 'week' }
  if (ms < YEAR) return { n: Math.floor(ms / MONTH), unit: 'month' }
  return { n: Math.floor(ms / YEAR), unit: 'year' }
}

export function elapsed(ts: number): string {
  const ms = Math.max(0, Date.now() - ts)
  if (ms < MINUTE) return 'just now'
  const { n, unit: name } = unit(ms)
  return `${n} ${name}${n === 1 ? '' : 's'}`
}

export function sinceLabel(stage: Stage, ts: number): string {
  if (stage === 'use') {
    return `In use since ${new Date(ts).toLocaleDateString(undefined, { day: 'numeric', month: 'short' })}`
  }
  if (stage === 'try') {
    const age = elapsed(ts)
    return age === 'just now' ? 'To test just now' : `To test for ${age}`
  }
  return `On the shelf ${elapsed(ts)}`
}

export function isStale(ts: number): boolean {
  return Date.now() - ts >= 30 * DAY
}
