
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



/** One absolute timestamp for a title: the person's date order and time zone, without seconds. */
export function formatWhen(ts: number, timeZone = '', dateFormat = 'locale'): string {
  if (!ts) return ''
  const options: Intl.DateTimeFormatOptions = { dateStyle: 'medium', timeStyle: 'short' }
  if (timeZone) options.timeZone = timeZone
  const locale = dateFormat === 'dmy' ? 'en-GB' : dateFormat === 'mdy' ? 'en-US' : dateFormat === 'ymd' ? 'sv-SE' : undefined
  try {
    return new Date(ts).toLocaleString(locale, options)
  } catch {
    return new Date(ts).toLocaleString()
  }
}

export function isStale(ts: number): boolean {
  return Date.now() - ts >= 30 * DAY
}

export function isOldEnvironment(ts: number): boolean {
  return Date.now() - ts >= 90 * DAY
}

export function agoLabel(at: number, now = Date.now()): string {
  if (!at) return 'never'
  const ms = Math.max(0, now - at)
  if (ms < 60_000) return 'just now'
  if (ms < 3_600_000) return `${Math.floor(ms / 60_000)} min ago`
  if (ms < 86_400_000) return `${Math.floor(ms / 3_600_000)} hr ago`
  const days = Math.floor(ms / 86_400_000)
  return `${days} ${days === 1 ? 'day' : 'days'} ago`
}
