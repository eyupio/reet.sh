
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

/** How long ago, in words: 5 min ago, 2 hr ago, 3 days ago. `short` is the home page's form: 2 h ago, 3 d ago. */
export function agoLabel(at: number, now = Date.now(), short = false): string {
  if (!at) return 'never'
  const ms = Math.max(0, now - at)
  if (ms < MINUTE) return 'just now'
  if (ms < HOUR) return `${Math.floor(ms / MINUTE)} min ago`
  if (ms < DAY) return `${Math.floor(ms / HOUR)} ${short ? 'h' : 'hr'} ago`
  const days = Math.floor(ms / DAY)
  return short ? `${days} d ago` : `${days} ${days === 1 ? 'day' : 'days'} ago`
}
