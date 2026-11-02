import type { Ticket, TicketStatus } from './api'

export type AdminTicketScope = 'all' | TicketStatus

export function adminTicketMatchesSearch(ticket: Ticket, query: string): boolean {
  const term = query.trim().toLowerCase()
  if (!term) return true
  const haystack = `${ticket.subject} ${ticket.opener} ${ticket.groupName ?? 'personal'} ${ticket.assignee ?? ''} ${ticket.id}`.toLowerCase()
  return term.split(/\s+/).every((word) => haystack.includes(word))
}

export function filterAdminTickets(tickets: Ticket[], scope: AdminTicketScope, query = ''): Ticket[] {
  return tickets.filter((ticket) => (scope === 'all' || ticket.status === scope) && adminTicketMatchesSearch(ticket, query))
}

export function adminTicketStatusCounts(tickets: Ticket[]): Record<AdminTicketScope, number> {
  const counts: Record<AdminTicketScope, number> = { all: tickets.length, open: 0, waiting: 0, closed: 0 }
  for (const ticket of tickets) {
    if (ticket.status in counts) counts[ticket.status]++
  }
  return counts
}

export function adminTicketCountLabel(shown: number, total: number): string {
  if (total === 0) return 'No tickets'
  if (shown === total) return `${total} ${total === 1 ? 'ticket' : 'tickets'}`
  return `${shown} of ${total} tickets`
}
