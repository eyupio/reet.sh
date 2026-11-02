import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const {
  GROUP_SCOPES, filterGroups, groupCountLabel, groupKindLabel, groupRoleLabel, groupScopeCounts, matchesGroupScope, matchesGroupSearch,
} = await import(`data:text/javascript;base64,${Buffer.from(compile('./spaces.ts')).toString('base64')}`)

const group = (over = {}) => ({
  id: 1, name: 'Platform engineering', kind: 'team', parentId: 9, ownerId: 1, role: 'owner',
  members: [
    { id: 1, name: 'Sam Taylor', email: 'sam@example.test', role: 'owner' },
    { id: 3, name: 'Jo Patel', email: 'jo@example.test', role: 'member' },
  ],
  ...over,
})
const groups = [
  group({ id: 9, name: 'Eyup Ltd', kind: 'organisation', parentId: undefined, members: [
    { id: 1, name: 'Sam Taylor', email: 'sam@example.test', role: 'owner' },
    { id: 2, name: 'Alex Morgan', email: 'alex@example.test', role: 'manager' },
  ] }),
  group({ id: 1 }),
  group({ id: 2, name: 'Design', parentId: undefined, role: 'member', ownerId: 2, members: [
    { id: 2, name: 'Alex Morgan', email: 'alex@example.test', role: 'owner' },
  ] }),
]

test('a group is found by name, kind or a member', () => {
  assert.equal(matchesGroupSearch(groups[1], ''), true)
  assert.equal(matchesGroupSearch(groups[1], 'platform'), true)
  assert.equal(matchesGroupSearch(groups[1], 'jo patel'), true)
  assert.equal(matchesGroupSearch(groups[1], 'jo@example'), true)
  assert.equal(matchesGroupSearch(groups[1], 'organisation'), false)
  assert.equal(matchesGroupSearch(groups[0], 'organisation'), true)
  assert.equal(matchesGroupSearch(groups[1], 'design'), false)
})

test('the scopes are all, organisations, teams and the groups you manage', () => {
  assert.equal(matchesGroupScope(groups[0], 'organisations'), true)
  assert.equal(matchesGroupScope(groups[1], 'organisations'), false)
  assert.equal(matchesGroupScope(groups[1], 'teams'), true)
  assert.equal(matchesGroupScope(groups[1], 'managed'), true)
  assert.equal(matchesGroupScope(groups[2], 'managed'), false)
  assert.deepEqual(filterGroups(groups, 'teams').map((g) => g.id), [1, 2])
  assert.deepEqual(filterGroups(groups, 'managed').map((g) => g.id), [9, 1])
  assert.deepEqual(filterGroups(groups, 'all', 'alex').map((g) => g.id), [9, 2], 'a search across member names keeps API order')
})

test('chip counts follow the searched groups', () => {
  assert.deepEqual(groupScopeCounts(groups), { all: 3, organisations: 1, teams: 2, managed: 2 })
  const searched = groups.filter((g) => matchesGroupSearch(g, 'alex'))
  assert.deepEqual(groupScopeCounts(searched), { all: 2, organisations: 1, teams: 1, managed: 1 })
  assert.deepEqual(GROUP_SCOPES.map((s) => s.id), ['all', 'organisations', 'teams', 'managed'])
})

test('the count says N or N of M', () => {
  assert.equal(groupCountLabel(0, 0), 'No groups')
  assert.equal(groupCountLabel(1, 1), '1 group')
  assert.equal(groupCountLabel(3, 3), '3 groups')
  assert.equal(groupCountLabel(1, 3), '1 of 3 groups')
})

test('kind and role words say what the group is and what the person may do', () => {
  assert.equal(groupKindLabel(groups[0], groups), 'Organisation')
  assert.equal(groupKindLabel(groups[1], groups), 'Team · Eyup Ltd')
  assert.equal(groupKindLabel(groups[2], groups), 'Team')
  assert.equal(groupRoleLabel('owner'), 'You manage this')
  assert.equal(groupRoleLabel('manager'), 'Manager')
  assert.equal(groupRoleLabel('member'), 'Member')
  assert.equal(groupRoleLabel(''), 'Member')
})
