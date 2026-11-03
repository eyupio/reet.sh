import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./adminNotifications.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { notificationDefaultCountLabel, notificationDefaultGroups, notificationDefaultPayload } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const item = (key, group, over = {}) => ({
  key, group, label: key, description: 'What happens.', audience: 'user',
  default: { enabled: true, mode: 'immediate', owner: true, admin: true },
  ...over,
})
const defaults = [
  item('security.signin', 'Security', { locked: 'Security notices are always sent.' }),
  item('machine.offline', 'Machines'),
  item('sync.failed', 'Sync'),
  item('ticket.reply', 'Support', { audience: 'staff' }),
  item('machine.updated', 'Machines'),
]

test('groups keep the order the server sent and the order within each group', () => {
  const groups = notificationDefaultGroups(defaults)
  assert.deepEqual(groups.map((group) => group.name), ['Security', 'Machines', 'Sync', 'Support'])
  assert.deepEqual(groups[1].items.map((entry) => entry.key), ['machine.offline', 'machine.updated'])
})

test('the save payload leaves a locked kind out and copies only the editable values', () => {
  const payload = notificationDefaultPayload(defaults)
  assert.deepEqual(payload.map((entry) => entry.kind), ['machine.offline', 'sync.failed', 'ticket.reply', 'machine.updated'])
  assert.deepEqual(payload[0], { kind: 'machine.offline', enabled: true, mode: 'immediate', owner: true, admin: true })
})

test('the count says N notification kinds, or none plainly', () => {
  assert.equal(notificationDefaultCountLabel(0), 'No notification kinds')
  assert.equal(notificationDefaultCountLabel(1), '1 notification kind')
  assert.equal(notificationDefaultCountLabel(4), '4 notification kinds')
  assert.equal(notificationDefaultCountLabel(0, true), 'Reading the defaults…')
})
