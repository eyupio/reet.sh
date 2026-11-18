// The Usage page's words and numbers: which range a period covers in the person's time zone, the totals in words, what
// each agent used, and, for an agent with no plan figures, the one sentence that says why. Pure on purpose: the unit test
// loads this file with the two self-contained modules it borrows from.
import { compactTokens } from './gafferOutcome'
import { limitLines, planEntries, readAgo, type AgentAccount, type MachineAccounts, type PlanEntry } from './agentAccounts'

export type UsagePeriod = 'today' | 'week' | 'month'
export const PERIODS: { id: UsagePeriod; label: string; days: number }[] = [
  { id: 'today', label: 'Today', days: 1 }, { id: 'week', label: '7 days', days: 7 }, { id: 'month', label: '30 days', days: 30 },
]

/** What reet can count for an agent: "chat" reports tokens in a chat; "terminal_only" has no chat here; "retired" is an
 *  agent reet no longer starts whose earlier figures still count. The controller decides, from its own catalogue. */
export type UsageAgent = { tool: string; name: string; reports: 'chat' | 'terminal_only' | 'retired'; input: number; output: number; cached: number; sessions: number; factoryStages: number; firstAt?: number }
export type UsageGaffer = { provider: string; model: string; input: number; output: number; replies: number }
export type UsageSession = { id: string; title: string; tool: string; model: string; agentId: string; host: string; projectId: string | null; structured: boolean; state: string; archived: boolean; factory: boolean; input: number; output: number; cached: number; lastAt: number; contextUsed: number; contextSize: number }
export type UsageAnswer = { from: number; to: number; since: number; agents: UsageAgent[]; gaffer: UsageGaffer[]; sessions: UsageSession[]; more: boolean }

/** One machine as the page knows it: whether it reads sign-ins, whether terminal access is off, and its last answer. */
export type MachinePlanRead = { host: string; reads: boolean; terminalOff: boolean; answer: MachineAccounts | null; error?: boolean }

export const tokens = compactTokens
const plural = (n: number, one: string, many = `${one}s`) => `${n.toLocaleString('en-GB')} ${n === 1 ? one : many}`

// The offset of a time zone from UTC at an instant, in milliseconds. An unknown zone is the browser's own.
function zoneParts(at: number, timeZone: string): { y: number; m: number; d: number; h: number; min: number; s: number } {
  const options: Intl.DateTimeFormatOptions = { year: 'numeric', month: 'numeric', day: 'numeric', hour: 'numeric', minute: 'numeric', second: 'numeric', hourCycle: 'h23' }
  let format: Intl.DateTimeFormat
  try { format = new Intl.DateTimeFormat('en-US', { ...options, timeZone: timeZone || undefined }) } catch { format = new Intl.DateTimeFormat('en-US', options) }
  const part = (type: string) => Number(format.formatToParts(new Date(at)).find(p => p.type === type)?.value ?? 0)
  return { y: part('year'), m: part('month'), d: part('day'), h: part('hour') % 24, min: part('minute'), s: part('second') }
}
function zoneOffset(at: number, timeZone: string): number {
  const p = zoneParts(at, timeZone)
  return Date.UTC(p.y, p.m - 1, p.d, p.h, p.min, p.s) - Math.floor(at / 1000) * 1000
}
/** The instant a local date's midnight falls on in the zone. A midnight the clocks skip is the first moment after it. */
function localMidnight(y: number, m: number, d: number, timeZone: string): number {
  const wall = Date.UTC(y, m - 1, d)
  let at = wall - zoneOffset(wall, timeZone)
  // Once more with the offset at the guess, which differs from the one at the wall time across a change of clocks.
  at = wall - zoneOffset(at, timeZone)
  const back = zoneParts(at, timeZone)
  if (back.d !== new Date(wall).getUTCDate()) at += 60 * 60_000
  return at
}

/** The range a period covers: from local midnight `days - 1` days before today to the next local midnight. */
export function periodRange(period: string, timeZone: string, now: number): { from: number; to: number } {
  const days = PERIODS.find(p => p.id === period)?.days ?? 1
  const today = zoneParts(now, timeZone)
  const start = new Date(Date.UTC(today.y, today.m - 1, today.d - (days - 1)))
  const next = new Date(Date.UTC(today.y, today.m - 1, today.d + 1))
  return {
    from: localMidnight(start.getUTCFullYear(), start.getUTCMonth() + 1, start.getUTCDate(), timeZone),
    to: localMidnight(next.getUTCFullYear(), next.getUTCMonth() + 1, next.getUTCDate(), timeZone),
  }
}

/** A day in the person's own order and zone, short: "18 Nov". */
export function dayLabel(at: number, timeZone: string, dateFormat = 'locale'): string {
  const locale = dateFormat === 'mdy' ? 'en-US' : dateFormat === 'ymd' ? 'sv-SE' : 'en-GB'
  const options: Intl.DateTimeFormatOptions = { day: 'numeric', month: 'short' }
  try { return new Date(at).toLocaleDateString(locale, { ...options, timeZone: timeZone || undefined }) } catch { return new Date(at).toLocaleDateString(locale, options) }
}

const spent = (a: { input: number; output: number }) => (a.input || 0) + (a.output || 0)

/** The line under the page's title: the period's total, and since when reet has counted when the period starts before. */
export function totalsLine(answer: UsageAnswer, timeZone: string, dateFormat = 'locale'): string {
  if (!answer.since) return 'Nothing has been counted yet. reet counts what your agents report from now on.'
  const total = answer.agents.reduce((sum, a) => sum + spent(a), 0), sessions = answer.agents.reduce((sum, a) => sum + (a.sessions || 0), 0)
  const since = answer.since > answer.from ? ` · counted since ${dayLabel(answer.since, timeZone, dateFormat)}` : ''
  if (!total && !sessions) return `No tokens reported in this period${since}`
  return `${tokens(total)} ${total === 1 ? 'token' : 'tokens'} across ${plural(sessions, 'session')}${since}`
}

/** What one agent used in the period: the figure itself (the most prominent thing for an agent with no plan to read),
 *  the sessions behind it, and a quieter detail; or, with no figure, the sentence that says why. */
export function agentTokens(agent: UsageAgent): { figure: string; line: string; detail: string } {
  if (agent.reports === 'terminal_only') return { figure: '', line: `${agent.name} runs only in a terminal here, which reports no token counts.`, detail: '' }
  const total = spent(agent)
  if (!total && !agent.cached && !agent.sessions) return { figure: '', line: 'No tokens reported in this period.', detail: '' }
  const factory = agent.factoryStages ? ` · ${plural(agent.factoryStages, 'Factory stage')}` : ''
  const detail = [`Input ${tokens(agent.input || 0)}`, `Output ${tokens(agent.output || 0)}`, ...(agent.cached ? [`Cached ${tokens(agent.cached)}`] : [])].join(' · ')
  return { figure: `${tokens(total)} ${total === 1 ? 'token' : 'tokens'}`, line: `${plural(agent.sessions, 'session')}${factory}`, detail }
}

/** The plans signed in for an agent across the machines, one per account (the freshest read wins, as the chips do). */
export function agentPlans(tool: string, machines: MachinePlanRead[], now: number): PlanEntry[] {
  return planEntries(machines.filter(m => m.reads).map(m => ({ host: m.host, answer: m.answer })), now).filter(entry => entry.tool === tool)
}

/** Each window of a plan in words: "Session 81% left · resets in 2 h 10 min". */
export function planLines(entry: PlanEntry, now: number): { key: string; name: string; left: number; low: boolean; resets: string; words: string }[] {
  return limitLines(entry.account, now).map(line => ({ ...line, words: `${line.text}${line.resets ? ` · ${line.resets}` : ''}` }))
}

const orList = (hosts: string[]) => hosts.length <= 1 ? hosts.join('') : `${hosts.slice(0, -1).join(', ')} or ${hosts[hosts.length - 1]}`

/** Why the latest read of a signed-in plan has no new figures, in the page's words; '' when it has. Figures kept from
 *  before are still shown, dimmed, under this line. */
function readProblem(account: AgentAccount, name: string, host: string): string {
  switch (account.limitsState) {
    case 'renew': return `Open ${name} on ${host} to sign in again.`
    case 'refused': return `${name}'s provider refused the sign-in on ${host} when asked for the plan.`
    case 'rate_limited': return `${name}'s provider asked reet to slow down.`
    case 'failed': return `${name}'s provider could not be reached from ${host} just now.`
  }
  return ''
}

/** The line under a plan whose latest read failed, with how old the figures shown are; '' when they are current. */
export function planNote(entry: PlanEntry, name: string, now: number): string {
  const problem = readProblem(entry.account, name, entry.host)
  if (!problem) return ''
  const age = readAgo(entry.account.limitsAt, now)
  return age ? `${problem} Showing the figures read ${age}.` : problem
}

/** Why an agent has no plan figure, in one sentence chosen from what the machines answered for it. Nothing here
 *  depends on which agent it is: any agent a machine reads appears in its answer, and one no machine answers for is one
 *  reet cannot read there (the machine's reet is too old for it, or reet has no reader for it). */
export function planSentence(tool: string, name: string, machines: MachinePlanRead[], now: number): string {
  void now
  if (!machines.length) return 'No connected machine reports sign-ins.'
  const reading = machines.filter(m => m.reads)
  if (!reading.length) {
    const off = machines.filter(m => m.terminalOff)
    if (off.length) return `Terminal access is off on ${orList(off.map(m => m.host))}, so reet doesn't read its agents' sign-ins.`
    return `Update reet on ${orList(machines.map(m => m.host))} to see which agents are signed in.`
  }
  const found = reading.map(m => ({ machine: m, account: m.answer?.accounts?.find(a => a.tool === tool) as AgentAccount | undefined }))
  const signedIn = found.filter(f => f.account?.state === 'signed_in')
  // A sign-in to renew first, then a read that failed: they are what the person may be able to fix.
  const renew = signedIn.find(f => f.account!.limitsState === 'renew')
  if (renew) return readProblem(renew.account!, name, renew.machine.host)
  const problem = signedIn.find(f => readProblem(f.account!, name, f.machine.host))
  if (problem) return readProblem(problem.account!, name, problem.machine.host)
  const noPlan = signedIn.find(f => f.account!.method === 'api_key' || f.account!.limitsState === 'unsupported')
  if (noPlan) return noPlan.account!.method === 'api_key'
    ? `Signed in with an API key on ${noPlan.machine.host}: there is no plan allowance to read.`
    : `Signed in on ${noPlan.machine.host}, but the account has no plan allowance to read.`
  const unreadable = found.filter(f => f.account?.state === 'unreadable')
  if (unreadable.length) return `reet can't read ${name}'s sign-in on ${orList(unreadable.map(f => f.machine.host))}.`
  const waiting = signedIn[0]
  if (waiting) return `${waiting.machine.host} has not read the plan yet. It reads it the next time it is asked.`
  const none = found.filter(f => f.account?.state === 'none')
  if (none.length) return `No sign-in found on ${orList(none.map(f => f.machine.host))}.`
  const failed = found.find(f => f.machine.error)
  if (failed) return `reet couldn't ask ${failed.machine.host} for its sign-ins just now.`
  const said = found.find(f => f.machine.answer && f.machine.answer.state !== 'read' && f.machine.answer.sentence && !f.machine.answer.accounts?.length)
  if (said) return `${said.machine.host}: ${said.machine.answer!.sentence}`
  // Every machine answered, and none said anything of this agent.
  return "reet can't read this agent's plan."
}

/** How much of a session's context window was left at its last report. '' when the agent never said. */
export function contextLeftText(session: { contextUsed: number; contextSize: number }): string {
  if (!session.contextSize || session.contextSize <= 0) return ''
  const left = Math.max(0, session.contextSize - Math.min(session.contextSize, session.contextUsed || 0))
  return `${Math.round(left / session.contextSize * 100)}% left`
}

/** Where a session opens; '' for one that has been removed from the history, which cannot be opened. */
export function sessionHref(session: { id: string; archived: boolean }): string {
  return session.archived ? '' : `/sessions?session=${encodeURIComponent(session.id)}`
}
export function sessionTitle(session: { title: string; tool: string }, agent: string): string {
  return session.title.trim() || `${agent} session`
}

export const periodKey = (userId: number) => `reet.usage.period.${userId}`
/** The period this browser last showed for the account. Storage can be missing or refuse; then it is today. */
export function readPeriod(storage: Pick<Storage, 'getItem'> | undefined, userId: number): UsagePeriod {
  try {
    const kept = storage?.getItem(periodKey(userId))
    return PERIODS.some(p => p.id === kept) ? kept as UsagePeriod : 'today'
  } catch { return 'today' }
}
export function keepPeriod(storage: Pick<Storage, 'setItem'> | undefined, userId: number, period: UsagePeriod): void {
  try { storage?.setItem(periodKey(userId), period) } catch { /* a remembered period is a convenience */ }
}
