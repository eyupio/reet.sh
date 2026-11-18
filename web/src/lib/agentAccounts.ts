// What a machine says of its agents' sign-ins, in words. The machine reads each agent's own sign-in files and asks the
// agent's provider, from the machine with that sign-in, how much of the plan is left; no credential reaches reet.
// These helpers only word the answer. Self-contained on purpose: the unit test loads this file alone.
export type AccountLimit = { kind: 'session' | 'day' | 'week' | 'month'; label?: string; usedPercent: number; resetsAt?: number }
export type AgentAccount = {
  tool: string; state: 'signed_in' | 'none' | 'unreadable'; method?: 'subscription' | 'api_key'; account?: string; plan?: string
  limitsState: 'read' | 'not_asked' | 'unsupported' | 'renew' | 'refused' | 'rate_limited' | 'failed'; limits?: AccountLimit[]; limitsAt?: number
}
/** A machine's answer as the controller serves it. `state` is "read", or why there is nothing newer. `planLimits` is
 *  always true now that plan limits are read for every account; it is kept for the shape of older answers. */
export type MachineAccounts = { state: 'read' | 'offline' | 'update' | 'off' | 'busy' | 'failed'; sentence: string; planLimits: boolean; checkedAt: number | null; accounts: AgentAccount[] }
export type LimitLine = { key: string; name: string; left: number; low: boolean; text: string; resets: string }

/** What reet reads of each agent's plan, said once for Settings > Agents and the Usage page. */
export const PLAN_LIMITS_HELP = "Each of your machines with terminal access on uses an agent's own sign-in to ask that agent's provider how much of your plan is left. The sign-in stays on the machine and is never renewed by reet; the controller is sent the account's name, the plan and percentages. This is done for the agents reet can read a plan for."

/** At or under this share of a window left, the figure is marked low. */
export const LOW_LIMIT_PERCENT = 15

const MINUTE = 60_000, HOUR = 60 * MINUTE, DAY = 24 * HOUR
const WINDOWS: Record<AccountLimit['kind'], [string, string]> = { session: ['Session', 'this session'], day: ['Day', 'today'], week: ['Week', 'this week'], month: ['Month', 'this month'] }
const capital = (word: string) => word ? word[0].toUpperCase() + word.slice(1) : ''

/** The machine's entry for one agent; undefined for an agent it said nothing about, which reet cannot tell about. */
export function accountFor(answer: Pick<MachineAccounts, 'accounts'> | null | undefined, tool: string): AgentAccount | undefined {
  return answer?.accounts?.find(account => account.tool === tool)
}

/** Who is signed in, or that no sign-in was found, or that it could not be read. '' for an agent with no entry. */
export function signInLine(account: AgentAccount | undefined): string {
  if (!account) return ''
  if (account.state === 'none') return 'No sign-in found on this machine'
  if (account.state === 'unreadable') return "reet can't read this agent's sign-in on this machine"
  if (account.method === 'api_key') return 'Signed in with an API key'
  return `Signed in${account.account ? ` as ${account.account}` : ''}${account.plan ? ` · ${capital(account.plan)} plan` : ''}`
}

/** How long until a window starts again, from `now`. '' when the provider did not say. */
export function resetsIn(at: number | undefined, now: number): string {
  if (!at || at <= 0) return ''
  const left = at - now
  if (left <= 0) return 'reset due'
  if (left < MINUTE) return 'resets in under a minute'
  if (Math.round(left / MINUTE) < 60) return `resets in ${Math.round(left / MINUTE)} min`
  // Rounded to the minute first, so 2 h 59.6 min reads 3 h, never 2 h 60 min.
  if (left < DAY) { const minutes = Math.round(left / MINUTE), h = Math.floor(minutes / 60), m = minutes % 60; return `resets in ${h} h${m ? ` ${m} min` : ''}` }
  const d = Math.floor(left / DAY), h = Math.round((left - d * DAY) / HOUR)
  return `resets in ${d} ${d === 1 ? 'day' : 'days'}${d === 1 && h ? ` ${h} h` : ''}`
}

/** Each allowance window as what is left of it, in the order the machine gave them (plan-wide windows first). */
export function limitLines(account: AgentAccount | undefined, now: number): LimitLine[] {
  return (account?.limits ?? []).map(limit => {
    const [name, phrase] = WINDOWS[limit.kind] ?? ['Limit', '']
    const left = 100 - Math.max(0, Math.min(100, Math.round(limit.usedPercent)))
    const title = limit.label ? `${limit.label} ${phrase}`.trim() : name
    return { key: limit.label ? `${limit.kind}:${limit.label}` : limit.kind, name: title, left, low: left <= LOW_LIMIT_PERCENT, text: `${title} ${left}% left`, resets: resetsIn(limit.resetsAt, now) }
  })
}

/** A plan's allowance in one short line, for places with little room: beside a chat's status and on a Factory card. */
export type PlanLeft = { label: string; left: number; summary: string; detail: string; low: boolean }
/** The window with the least left is the one that will stop the agent first, so it is the one named. `summary` lists
 *  every window and `detail` adds when each starts again and, when the latest read failed, how old the figures are.
 *  Null when there are no figures: the agent is not signed in to a plan, the plan has not been read yet, or it is not one reet reads. */
export function planLeft(account: AgentAccount | undefined, now: number): PlanLeft | null {
  const lines = limitLines(account, now)
  if (!lines.length) return null
  const tight = lines.reduce((least, line) => line.left < least.left ? line : least)
  const stale = account!.limitsState !== 'read' && account!.limitsAt ? ` Read ${readAgo(account!.limitsAt, now)}.` : ''
  return {
    label: `Plan ${tight.left}% left${lines.length > 1 ? ` (${tight.name.toLowerCase()})` : ''}`, left: tight.left,
    summary: lines.map(line => line.text).join(' · '),
    detail: lines.map(line => `${line.text}${line.resets ? `, ${line.resets}` : ''}.`).join(' ') + stale,
    low: tight.low,
  }
}

/** One plan to draw: an agent's account on a machine that has figures for it. */
export type PlanEntry = { key: string; tool: string; host: string; account: AgentAccount; plan: PlanLeft; stale: boolean }
/** The plans across a set of machines, one per account. The same account signed in on two machines is one allowance,
 *  so it is listed once, from whichever machine read it last. An account with no figures (an API key, no sign-in, a plan
 *  not read yet) is left out. `stale` marks figures kept from a read that has since failed. */
export function planEntries(machines: { host: string; answer: Pick<MachineAccounts, 'accounts'> | null | undefined }[], now: number): PlanEntry[] {
  const out = new Map<string, PlanEntry>()
  for (const machine of machines) for (const account of machine.answer?.accounts ?? []) {
    const plan = planLeft(account, now)
    if (!plan) continue
    const key = account.account ? `${account.tool}:${account.account}` : `${account.tool}@${machine.host}`
    const have = out.get(key)
    if (have && (have.account.limitsAt ?? 0) >= (account.limitsAt ?? 0)) continue
    out.set(key, { key, tool: account.tool, host: machine.host, account, plan, stale: account.limitsState !== 'read' })
  }
  return [...out.values()]
}

/** How long ago something was read. '' when it never was. */
export function readAgo(at: number | null | undefined, now: number): string {
  if (!at || at <= 0) return ''
  const age = Math.max(0, now - at)
  if (age < MINUTE) return 'just now'
  if (age < HOUR) return `${Math.floor(age / MINUTE)} min ago`
  if (age < DAY) return `${Math.floor(age / HOUR)} h ago`
  const days = Math.floor(age / DAY)
  return `${days} ${days === 1 ? 'day' : 'days'} ago`
}

/** Why there are no new figures, with the age of the last ones still shown. '' when the figures are current or were never asked for. */
export function limitsNote(account: AgentAccount | undefined, agent: string, now: number): string {
  if (!account) return ''
  const why: Record<string, string> = {
    renew: `${agent}'s sign-in needs renewing before its limits can be read. Start ${agent} on this machine once; reet never renews a sign-in itself.`,
    refused: `${agent}'s provider refused the sign-in when asked for its limits.`,
    rate_limited: `${agent}'s provider asked reet to slow down.`,
    failed: `This machine couldn't read ${agent}'s limits just now.`,
    unsupported: 'An API key has no plan limits to read.',
  }
  const reason = why[account.limitsState]
  if (!reason) return ''
  return account.limits?.length ? `${reason} Showing the figures read ${readAgo(account.limitsAt, now)}.` : reason
}

/** What the Start dialog says before a start that will likely be refused. '' unless the machine looked and found no sign-in. */
export function startHint(account: AgentAccount | undefined, agent: string, host: string): string {
  return account?.state === 'none' ? `No ${agent} sign-in was found on ${host}. Sign in to ${agent} there first, or its first message may be refused.` : ''
}
