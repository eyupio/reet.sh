import type { MailMessage } from './api'

export type AdminMailScope = 'all' | 'queued' | 'sending' | 'held' | 'delivered' | 'failed' | 'bounced'

export const ADMIN_MAIL_SCOPES: { key: AdminMailScope; label: string }[] = [
  { key: 'all', label: 'All' },
  { key: 'queued', label: 'Queued' },
  { key: 'sending', label: 'Sending' },
  { key: 'held', label: 'Held' },
  { key: 'delivered', label: 'Delivered' },
  { key: 'failed', label: 'Failed' },
  { key: 'bounced', label: 'Bounced' },
]

export function adminMailMatchesSearch(message: MailMessage, query: string): boolean {
  const term = query.trim().toLowerCase()
  if (!term) return true
  const haystack = `${message.subject} ${message.to} ${message.template} ${message.lastError ?? ''} ${message.id}`.toLowerCase()
  return term.split(/\s+/).every((word) => haystack.includes(word))
}

export function filterAdminMail(messages: MailMessage[], scope: AdminMailScope, query = ''): MailMessage[] {
  return messages.filter((message) => (scope === 'all' || message.state === scope) && adminMailMatchesSearch(message, query))
}

export function adminMailStateCounts(messages: MailMessage[]): Record<AdminMailScope, number> {
  const counts: Record<AdminMailScope, number> = { all: messages.length, queued: 0, sending: 0, held: 0, delivered: 0, failed: 0, bounced: 0 }
  for (const message of messages) {
    if (message.state in counts) counts[message.state as AdminMailScope]++
  }
  return counts
}

export function adminMailCountLabel(shown: number, total: number): string {
  if (total === 0) return 'No messages'
  if (shown === total) return `${total} ${total === 1 ? 'message' : 'messages'}`
  return `${shown} of ${total} messages`
}
