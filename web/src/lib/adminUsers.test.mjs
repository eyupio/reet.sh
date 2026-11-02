import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./adminUsers.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { ADMIN_USER_SCOPES, adminUserCountLabel, adminUserMatchesSearch, adminUserRoleCounts, filterAdminUsers } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const user = (over = {}) => ({ id: 1, name: 'Sam Taylor', email: 'sam@example.test', role: 'owner', createdAt: 1, tokens: 0, ...over })
const users = [
  user(),
  user({ id: 2, name: 'Alex Morgan', email: 'alex@example.test', role: 'admin' }),
  user({ id: 3, name: 'Jo Patel', email: 'jo@example.test', role: 'member' }),
  user({ id: 4, name: 'Riley Chen', email: 'riley@example.test', role: 'viewer' }),
]

test('a person is found by name, address or role in any word order', () => {
  assert.equal(adminUserMatchesSearch(users[1], ''), true)
  assert.equal(adminUserMatchesSearch(users[1], 'alex'), true)
  assert.equal(adminUserMatchesSearch(users[1], 'alex example'), true)
  assert.equal(adminUserMatchesSearch(users[1], 'admin'), true)
  assert.equal(adminUserMatchesSearch(users[1], 'jo'), false)
})

test('the role chips count the people the search leaves', () => {
  assert.deepEqual(adminUserRoleCounts(users), { all: 4, owner: 1, admin: 1, member: 1, viewer: 1 })
  const searched = users.filter((u) => adminUserMatchesSearch(u, 'alex'))
  assert.deepEqual(adminUserRoleCounts(searched), { all: 1, owner: 0, admin: 1, member: 0, viewer: 0 })
  assert.deepEqual(ADMIN_USER_SCOPES.map((s) => s.id), ['all', 'owner', 'admin', 'member', 'viewer'])
})

test('filterAdminUsers combines the pressed role with the search and keeps API order', () => {
  assert.deepEqual(filterAdminUsers(users, 'all').map((u) => u.id), [1, 2, 3, 4])
  assert.deepEqual(filterAdminUsers(users, 'admin').map((u) => u.id), [2])
  assert.deepEqual(filterAdminUsers(users, 'all', 'example').map((u) => u.id), [1, 2, 3, 4])
  assert.deepEqual(filterAdminUsers(users, 'viewer', 'alex').map((u) => u.id), [])
})

test('the count says N people or N of M people', () => {
  assert.equal(adminUserCountLabel(0, 0), 'No people')
  assert.equal(adminUserCountLabel(1, 1), '1 person')
  assert.equal(adminUserCountLabel(4, 4), '4 people')
  assert.equal(adminUserCountLabel(1, 4), '1 of 4 people')
})
