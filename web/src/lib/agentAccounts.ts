// What a machine says of its agents' sign-ins, in words. The machine reads each agent's own sign-in files and, when the
// account has plan limits on, asks the agent's provider from the machine with that sign-in; no credential reaches reet.
// These helpers only word the answer. Self-contained on purpose: the unit test loads this file alone.
export type AccountLimit = { kind: 'session' | 'day' | 'week' | 'month'; label?: string; usedPercent: number; resetsAt?: number }
export type AgentAccount = {
  tool: string; state: 'signed_in' | 'none' | 'unreadable'; method?: 'subscription' | 'api_key'; account?: string; plan?: string
  limitsState: 'read' | 'not_asked' | 'unsupported' | 'renew' | 'refused' | 'rate_limited' | 'failed'; limits?: AccountLimit[]; limitsAt?: number
}
/** A machine's answer as the controller serves it. `state` is "read", or why there is nothing newer; `planLimits` is the account's setting. */
export type MachineAccounts = { state: 'read' | 'offline' | 'update' | 'off' | 'busy' | 'failed'; sentence: string; planLimits: boolean; checkedAt: number | null; accounts: AgentAccount[] }
export type LimitLine = { key: string; name: string; left: number; low: boolean; text: string; resets: string }

/** The account setting's label and what turning it on does, said once for Settings and the machine page. */
export const PLAN_LIMITS_LABEL = "Show how much of each agent's plan is left"
export const PLAN_LIMITS_HELP = "Off until you turn it on. With it on, reet on each of your machines with terminal access on uses an agent's own sign-in to ask that agent's provider how much of your plan is left. The sign-in stays on the machine and is never renewed by reet; the controller is sent the account's name, the plan and percentages. Claude Code and Codex are read today. Which agents are signed in shows either way, read from files on the machine."
export const PLAN_LIMITS_OFF_HINT = 'Plan limits are off. Turn them on in Settings > Agents to see how much of each plan is left.'

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
  if (left < HOUR) return `resets in ${Math.round(left / MINUTE)} min`
  if (left < DAY) { const h = Math.floor(left / HOUR), m = Math.round((left - h * HOUR) / MINUTE); return `resets in ${h} h${m ? ` ${m} min` : ''}` }
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
