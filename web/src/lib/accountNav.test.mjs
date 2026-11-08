import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./accountNav.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { accountHref, accountNav } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const entry = (key, label = key) => ({ key, label })
const all = [
  entry('profile', 'Profile'), entry('agents', 'Agents'), entry('gaffer', 'Gaffer AI'), entry('security', 'Password'), entry('sso', 'Single sign-on'),
  entry('sessions', 'Sessions'), entry('tokens', 'API tokens'), entry('mcp', 'MCP access'), entry('backups', 'Backups'), entry('maintenance', 'Maintenance'),
  entry('activity', 'Activity'), entry('machines', 'Machines'), entry('sync', 'Compare & sync'),
]

test('a writer with single sign-on sees seven items: five links and two dropdowns, in the original order', () => {
  const items = accountNav(all, 'profile')
  assert.deepEqual(items.map((i) => i.label), ['Profile', 'Agents', 'Gaffer AI', 'Security', 'Data & activity', 'Machines', 'Compare & sync'])
  assert.deepEqual(items[3].children.map((c) => c.label), ['Password', 'Single sign-on', 'Sessions', 'API tokens', 'MCP access'])
  assert.deepEqual(items[4].children.map((c) => c.label), ['Backups', 'Maintenance', 'Activity'])
})

test('every section keeps its address, and Machines and Compare & sync still lead to the Machines page', () => {
  const flat = accountNav(all, 'profile').flatMap((i) => i.children ?? [i])
  assert.equal(flat.length, all.length)
  assert.deepEqual(flat.map((i) => i.href), all.map((e) => accountHref(e.key)))
  assert.equal(accountHref('machines'), '/machines')
  assert.equal(accountHref('sync'), '/machines?view=compare')
  assert.equal(accountHref('sessions'), '/account/sessions')
})

test('a group is current when one of its sections is, and only that child is marked', () => {
  const items = accountNav(all, 'sessions')
  const security = items.find((i) => i.label === 'Security')
  assert.equal(security.current, true)
  assert.deepEqual(security.children.filter((c) => c.current).map((c) => c.label), ['Sessions'])
  assert.equal(items.find((i) => i.label === 'Data & activity').current, false)
  assert.equal(items.find((i) => i.label === 'Profile').current, false)
})

test('a viewer has no Gaffer AI, API tokens or Maintenance, and the groups shrink to what is left', () => {
  const viewer = all.filter((e) => !['gaffer', 'sso', 'tokens', 'maintenance'].includes(e.key))
  const items = accountNav(viewer, 'activity')
  assert.deepEqual(items.map((i) => i.label), ['Profile', 'Agents', 'Security', 'Data & activity', 'Machines', 'Compare & sync'])
  assert.deepEqual(items[2].children.map((c) => c.label), ['Password', 'Sessions', 'MCP access'])
  assert.deepEqual(items[3].children.map((c) => c.label), ['Backups', 'Activity'])
  assert.equal(items[3].current, true)
})

test('a group with a single present section is a plain link, not a dropdown of one', () => {
  const items = accountNav([entry('profile', 'Profile'), entry('backups', 'Backups'), entry('machines', 'Machines')], 'backups')
  assert.deepEqual(items.map((i) => i.label), ['Profile', 'Backups', 'Machines'])
  assert.equal(items[1].children, undefined)
  assert.equal(items[1].current, true)
  assert.equal(items[1].href, '/account/backups')
})
