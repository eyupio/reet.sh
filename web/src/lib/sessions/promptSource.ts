import type { TerminalSession } from '../terminal/api'
import type { ChatState } from '../chat'

export type PromptView =
  | { kind: 'stored'; excerpt: string; more: boolean }
  | { kind: 'chat'; text: string }
  | { kind: 'none'; why: 'old' | 'unloaded' }

// A truncated chat has dropped its oldest items, so its first user message is
// not the prompt; saying so ('unloaded') is better than showing the wrong one.
export function promptView(s: Pick<TerminalSession, 'promptExcerpt' | 'promptMore' | 'structured'>, chat?: ChatState): PromptView {
  if (s.promptExcerpt) return { kind: 'stored', excerpt: s.promptExcerpt, more: !!s.promptMore }
  if (chat && !chat.truncated) {
    for (const item of chat.items) {
      if (item.kind === 'user') return { kind: 'chat', text: item.text }
    }
  }
  return { kind: 'none', why: chat?.truncated ? 'unloaded' : 'old' }
}
