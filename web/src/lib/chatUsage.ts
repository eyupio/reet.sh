// What a chat's agent has reported using, for the chat's status line. The agent says how many tokens a finished turn
// took and how full its context window is; reet adds the turns up and attaches no price. An agent that has reported
// nothing is shown as nothing, never as zero. Self-contained on purpose: the unit test loads this file alone.
export type ChatUsage = { reported: boolean; inputTokens: number; outputTokens: number; cachedTokens: number; contextUsed: number; contextSize: number; updatedAt: number }
export type ChatUsageView = { label: string; detail: string; low: boolean; percentLeft: number | null }

/** At or under this share of the context window left, the figure is marked low. */
export const LOW_CONTEXT_PERCENT = 15

const compact = (n: number) => n >= 999_500 ? `${(n / 1_000_000).toFixed(n >= 10_000_000 ? 0 : 1)}M` : n >= 10_000 ? `${Math.round(n / 1000)}k` : n.toLocaleString('en-GB')
const count = (n: unknown) => typeof n === 'number' && Number.isFinite(n) && n > 0 ? n : 0

/** The label and detail for a chat's usage, or null when the agent has said nothing. `agent` is its display name. */
export function chatUsageView(usage: ChatUsage | null | undefined, agent: string): ChatUsageView | null {
  if (!usage?.reported) return null
  const tokens = count(usage.inputTokens) + count(usage.outputTokens), cached = count(usage.cachedTokens), size = count(usage.contextSize)
  if (!tokens && !cached && !size) return null
  const spent = tokens || cached ? `${compact(tokens)} ${tokens === 1 ? 'token' : 'tokens'}${cached ? `, and ${compact(cached)} cached` : ''}` : ''
  if (!size) return { label: `${compact(tokens)} ${tokens === 1 ? 'token' : 'tokens'}`, low: false, percentLeft: null, detail: `${agent} reports this chat has used ${spent}. It has not said how full its context window is.` }
  const left = Math.max(0, size - Math.min(size, count(usage.contextUsed))), percentLeft = Math.round(left / size * 100)
  return {
    label: `Context ${percentLeft}% left`, low: percentLeft <= LOW_CONTEXT_PERCENT, percentLeft,
    detail: `${agent} reports ${compact(left)} of its ${compact(size)}-token context window free.${spent ? ` This chat has used ${spent}.` : ''}`,
  }
}
