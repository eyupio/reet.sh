import type { SkillActivity, SkillWork, SyncMachine } from '../api'
import { pausedSentence, syncSentence, type Tone } from '../skills/skillsPage'

const MINUTE = 60_000
const HOUR = 60 * MINUTE
const DAY = 24 * HOUR

const count = (n: number, one: string, many = `${one}s`) => `${n} ${n === 1 ? one : many}`

/** How long ago, as the activity's time column says it: Now, 2 min, 1 h, Yesterday, 3 days, then the date. */
export function ago(ts: number, now = Date.now()): string {
  const ms = Math.max(0, now - ts)
  if (ms < MINUTE) return 'Now'
  if (ms < HOUR) return `${Math.floor(ms / MINUTE)} min`
  if (ms < DAY) return `${Math.floor(ms / HOUR)} h`
  if (ms < 2 * DAY) return 'Yesterday'
  if (ms < 7 * DAY) return `${Math.floor(ms / DAY)} days`
  return new Date(ts).toLocaleDateString('en-GB', { day: 'numeric', month: 'short' })
}

/** The same, inside a sentence: just now, 2 min ago, yesterday, on 12 Nov. */
function agoInText(ts: number, now: number): string {
  const short = ago(ts, now)
  if (short === 'Now') return 'just now'
  if (short === 'Yesterday') return 'yesterday'
  return /^\d+ (min|h|days)$/.test(short) ? `${short} ago` : `on ${short}`
}

/** "a", "a and b", "a, b and c". */
export function andList(items: string[]): string {
  if (items.length <= 1) return items[0] ?? ''
  return `${items.slice(0, -1).join(', ')} and ${items[items.length - 1]}`
}

/** The page's heading: one sentence for every machine, as the Skills page's header says it. */
export function pageSentence(work: SkillWork, machines: number): string {
  if (machines === 0) return 'No machines yet.'
  const text = syncSentence(work)
  return /[.…]$/.test(text) ? text : `${text}.`
}

/** The page's lede: the one rule, with how many skills it puts on each machine, and a paused machine said once. */
export function ledeText(active: number, work: SkillWork): string {
  const rule = `Your ${active} Active ${active === 1 ? 'skill goes' : 'skills go'} on every machine. Archived ones come off. That's the only rule.`
  const paused = pausedSentence(work)
  return paused ? `${rule} ${paused.slice(0, -1)}, so reet leaves ${(work.paused ?? 0) > 1 ? 'their' : 'its'} skills as they are.` : rule
}

/** One machine's line: what it is doing with your skills, in one tone. */
export function machineLine(m: SyncMachine, active: number, now = Date.now()): { text: string; tone: Tone } {
  const waiting = m.waiting.length
  switch (m.state) {
    case 'ok':
      return { text: `In sync · ${count(active, 'skill')} · checked ${agoInText(m.lastSeen, now)}`, tone: 'ok' }
    case 'installing':
      return { text: `Installing ${count(waiting, 'skill')}…`, tone: 'work' }
    case 'removing':
      return { text: `Taking ${count(waiting, 'skill')} off…`, tone: 'work' }
    case 'updating':
      return { text: `Updating ${count(waiting, 'skill')}…`, tone: 'work' }
    case 'failed':
      return { text: `${count(m.failed.length, 'skill')} failed: ${andList(m.failed.map(f => f.name))}`, tone: 'bad' }
    case 'blocked':
      return { text: "Can't write skills here", tone: 'bad' }
    case 'offline':
      // It catches up only on what waits for it.
      return { text: `Offline · last seen ${m.lastSeen ? agoInText(m.lastSeen, now) : 'never'}${waiting ? " · catches up when it's back" : ''}`, tone: 'off' }
    case 'paused':
      return { text: 'Paused by you · left alone', tone: 'off' }
    case 'needs-update':
      return { text: `Update reet on ${m.host} to keep its skills in sync`, tone: 'bad' }
    case 'needs-newer-controller':
      return { text: `${m.host} runs a newer reet than this controller; update the controller to keep its skills in sync`, tone: 'bad' }
  }
  return { text: 'Checking', tone: 'work' }
}

export type ActivityGroup = { at: number; name: string; action: string; detail: string; hosts: string[] }

/**
 * The activity, newest first, with the lines that say the same of one skill on several machines at about the same time
 * made one. A failure joins another only when it failed for the same reason.
 */
export function groupActivity(activity: SkillActivity[]): ActivityGroup[] {
  const out: ActivityGroup[] = []
  for (const a of activity) {
    const last = out[out.length - 1]
    const same = last && last.name === a.name && last.action === a.action && Math.abs(last.at - a.at) < MINUTE && (a.action !== 'failed' || last.detail === a.detail)
    if (same) {
      if (a.host && !last.hosts.includes(a.host)) last.hosts.push(a.host)
      continue
    }
    out.push({ at: a.at, name: a.name, action: a.action, detail: a.detail, hosts: a.host ? [a.host] : [] })
  }
  return out
}

/** One activity line, with the skill's name apart so the page can set it in bold. */
export function activityText(g: ActivityGroup): { lead: string; name: string; rest: string } {
  const where = g.hosts.length > 2 ? `${g.hosts.length} machines` : andList(g.hosts) || 'a machine'
  const line = (lead: string, rest: string) => ({ lead, name: g.name, rest })
  switch (g.action) {
    case 'installed':
      return line('Installed ', ` on ${where}.`)
    case 'updated':
      return line('Updated ', ` on ${where}.`)
    case 'removed':
      return line('Removed ', ` from ${where}.`)
    case 'removed again':
      return line('Removed ', ` from ${where} again: it came back.`)
    case 'edited':
      return line('', ` was edited on ${where}; reet's copy put back.`)
    case 'found':
      return line('Found ', ` on ${where}. Waiting for you on Skills.`)
    case 'retrying':
      return line(`Asked ${where} to try `, ' again.')
    case 'failed': {
      const why = g.detail.trim()
      return line('', why ? ` failed on ${where}: ${why}${/[.!?]$/.test(why) ? '' : '.'}` : ` failed on ${where}.`)
    }
  }
  return line(`${g.action.charAt(0).toUpperCase()}${g.action.slice(1)} `, ` on ${where}.`)
}
