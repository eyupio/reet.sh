import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./adminTickets.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { adminTicketCountLabel, adminTicketMatchesSearch, adminTicketStatusCounts, filterAdminTickets } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const ticket = (over = {}) => ({ id: 1, opener: 'Sam Taylor', subject: 'Sync stalls on build-box', status: 'open', messages: 3, updatedAt: 1, ...over })
const tickets = [
  ticket(),
  ticket({ id: 2, opener: 'Jo Patel', groupName: 'Platform engineering', subject: 'Invite link expired', status: 'waiting', assignee: 'Alex Morgan' }),
  ticket({ id: 3, opener: 'Riley Chen', subject: 'Paste strips indentation', status: 'closed' }),
]

test('a ticket is found by subject, opener, team, assignee or id', () => {
  assert.equal(adminTicketMatchesSearch(tickets[1], ''), true)
  assert.equal(adminTicketMatchesSearch(tickets[1], 'invite'), true)
  assert.equal(adminTicketMatchesSearch(tickets[1], 'jo platform'), true)
  assert.equal(adminTicketMatchesSearch(tickets[1], 'alex'), true)
  assert.equal(adminTicketMatchesSearch(tickets[1], '2'), true)
  assert.equal(adminTicketMatchesSearch(tickets[1], 'sam'), false)
})

test('the status chips count the tickets the search leaves', () => {
  assert.deepEqual(adminTicketStatusCounts(tickets), { all: 3, open: 1, waiting: 1, closed: 1 })
  const searched = tickets.filter((t) => adminTicketMatchesSearch(t, 'jo'))
  assert.deepEqual(adminTicketStatusCounts(searched), { all: 1, open: 0, waiting: 1, closed: 0 })
})

test('filterAdminTickets combines the status with the search and keeps order', () => {
  assert.deepEqual(filterAdminTickets(tickets, 'all').map((t) => t.id), [1, 2, 3])
  assert.deepEqual(filterAdminTickets(tickets, 'waiting').map((t) => t.id), [2])
  assert.deepEqual(filterAdminTickets(tickets, 'all', 'jo').map((t) => t.id), [2])
  assert.deepEqual(filterAdminTickets(tickets, 'closed', 'jo').map((t) => t.id), [])
})

test('the count says N tickets or N of M tickets', () => {
  assert.equal(adminTicketCountLabel(0, 0), 'No tickets')
  assert.equal(adminTicketCountLabel(1, 1), '1 ticket')
  assert.equal(adminTicketCountLabel(3, 3), '3 tickets')
  assert.equal(adminTicketCountLabel(1, 3), '1 of 3 tickets')
})
