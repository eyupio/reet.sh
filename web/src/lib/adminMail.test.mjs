import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./adminMail.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { adminMailCountLabel, adminMailMatchesSearch, adminMailStateCounts, filterAdminMail } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const message = (over = {}) => ({ id: 'm1', template: 'invite', to: 'sam@example.test', subject: 'You have been invited', state: 'failed', attempts: 3, createdAt: 1, secret: true, resendable: true, ...over })
const messages = [
  message(),
  message({ id: 'm2', template: 'reset', to: 'jo@example.test', subject: 'Reset your password', state: 'queued', lastError: '' }),
  message({ id: 'm3', template: 'welcome', to: 'riley@example.test', subject: 'Welcome to reet.sh', state: 'held', secret: false, resendable: false }),
  message({ id: 'm4', template: 'signin', to: 'alex@example.test', subject: 'Your sign-in link', state: 'delivered' }),
  message({ id: 'm5', template: 'ticket', to: 'team@example.test', subject: 'A ticket has a reply', state: 'bounced' }),
]

test('a message is found by subject, recipient, template, last error or id', () => {
  assert.equal(adminMailMatchesSearch(messages[0], ''), true)
  assert.equal(adminMailMatchesSearch(messages[0], 'invited'), true)
  assert.equal(adminMailMatchesSearch(messages[0], 'sam'), true)
  assert.equal(adminMailMatchesSearch(messages[0], 'invite'), true)
  assert.equal(adminMailMatchesSearch(messages[0], 'relaying denied'), false)
  assert.equal(adminMailMatchesSearch({ ...messages[0], lastError: '550 relaying denied' }, 'relaying'), true)
  assert.equal(adminMailMatchesSearch(messages[0], 'm1'), true)
  assert.equal(adminMailMatchesSearch(messages[0], 'ticket'), false)
})

test('the state chips count the messages the search leaves', () => {
  assert.deepEqual(adminMailStateCounts(messages), { all: 5, queued: 1, sending: 0, held: 1, delivered: 1, failed: 1, bounced: 1 })
  const searched = messages.filter((m) => adminMailMatchesSearch(m, 'example.test'))
  assert.deepEqual(adminMailStateCounts(searched), { all: 5, queued: 1, sending: 0, held: 1, delivered: 1, failed: 1, bounced: 1 })
  const one = messages.filter((m) => adminMailMatchesSearch(m, 'reset'))
  assert.deepEqual(adminMailStateCounts(one), { all: 1, queued: 1, sending: 0, held: 0, delivered: 0, failed: 0, bounced: 0 })
})

test('filterAdminMail combines the state with the search and keeps order', () => {
  assert.deepEqual(filterAdminMail(messages, 'all').map((m) => m.id), ['m1', 'm2', 'm3', 'm4', 'm5'])
  assert.deepEqual(filterAdminMail(messages, 'queued').map((m) => m.id), ['m2'])
  assert.deepEqual(filterAdminMail(messages, 'all', 'reset').map((m) => m.id), ['m2'])
  assert.deepEqual(filterAdminMail(messages, 'failed', 'reset').map((m) => m.id), [])
})

test('the count says N messages or N of M messages', () => {
  assert.equal(adminMailCountLabel(0, 0), 'No messages')
  assert.equal(adminMailCountLabel(1, 1), '1 message')
  assert.equal(adminMailCountLabel(5, 5), '5 messages')
  assert.equal(adminMailCountLabel(1, 5), '1 of 5 messages')
})
