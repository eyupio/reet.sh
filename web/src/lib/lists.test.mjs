import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const {
  LIST_SCOPES, filterLists, listAccessLabel, listCountLabel, listScopeCounts, matchesListScope, matchesListSearch,
} = await import(`data:text/javascript;base64,${Buffer.from(compile('./lists.ts')).toString('base64')}`)

const list = (over = {}) => ({
  id: 1, ownerId: 1, ownerName: 'Sam Taylor', name: 'Team essentials',
  description: 'The baseline set every engineer gets.', visibility: 'team',
  skillCount: 12, memberCount: 3, createdAt: 1, updatedAt: 2, ...over,
})

const lists = [
  list({ id: 1, name: 'Team essentials', description: 'The baseline set.' }),
  list({ id: 2, name: 'Writing and research', description: 'Drafting helpers.', visibility: 'personal' }),
  list({ id: 3, name: 'Launch checklist', description: 'Release week.', visibility: 'public', ownerId: 2, ownerName: 'Alex Morgan', role: 'viewer', sharedWithMe: true }),
  list({ id: 4, name: 'Design reviews', description: 'Critique passes.', visibility: 'members', ownerId: 2, ownerName: 'Alex Morgan', role: 'editor', sharedWithMe: true }),
  list({ id: 5, name: 'Client kit', description: 'Link-only collection.', visibility: 'link', ownerId: 2, ownerName: 'Alex Morgan', role: 'editor', sharedWithMe: true }),
]

test('search reads names, descriptions and owners', () => {
  assert.equal(matchesListSearch(lists[0], ''), true)
  assert.equal(matchesListSearch(lists[0], '   '), true)
  assert.equal(matchesListSearch(lists[0], 'ESSENTIAL'), true)
  assert.equal(matchesListSearch(lists[0], 'baseline'), true)
  assert.equal(matchesListSearch(lists[2], 'alex'), true)
  assert.equal(matchesListSearch(lists[2], 'launch alex'), true, 'words may sit in different fields')
  assert.equal(matchesListSearch(lists[2], 'launch sam'), false)
})

test('one scope is pressed at a time and the legacy scope is its own', () => {
  assert.equal(matchesListScope(lists[0], 'all', 1), true)
  assert.equal(matchesListScope(lists[0], 'mine', 1), true)
  assert.equal(matchesListScope(lists[2], 'mine', 1), false)
  assert.equal(matchesListScope(lists[0], 'team', 1), true)
  assert.equal(matchesListScope(lists[2], 'team', 1), false)
  assert.equal(matchesListScope(lists[3], 'legacy', 1), true)
  assert.equal(matchesListScope(lists[0], 'legacy', 1), false)
  assert.equal(matchesListScope(lists[2], 'people', 1), true)
  assert.equal(matchesListScope(lists[3], 'people', 1), true)
  assert.equal(matchesListScope(lists[0], 'people', 1), false, 'the owner does not see their own list under Shared with me')
})

test('filterLists combines the pressed scope with the search and keeps API order', () => {
  assert.deepEqual(filterLists(lists, 'all', 1).map((l) => l.id), [1, 2, 3, 4, 5])
  assert.deepEqual(filterLists(lists, 'mine', 1).map((l) => l.id), [1, 2])
  assert.deepEqual(filterLists(lists, 'all', 1, 'alex').map((l) => l.id), [3, 4, 5])
  assert.deepEqual(filterLists(lists, 'team', 1, 'launch').map((l) => l.id), [])
})

test('chip counts are taken from the records the current search leaves', () => {
  const searched = lists.filter((l) => matchesListSearch(l, 'alex'))
  assert.deepEqual(listScopeCounts(searched, 1), { all: 3, mine: 0, people: 3, team: 0, legacy: 1 })
  assert.deepEqual(listScopeCounts(lists, 1), { all: 5, mine: 2, people: 3, team: 1, legacy: 1 })
  const none = lists.filter((l) => matchesListSearch(l, 'nowt'))
  assert.deepEqual(listScopeCounts(none, 1), { all: 0, mine: 0, people: 0, team: 0, legacy: 0 })
})

test('a legacy scope with no lists can be left out of the chips', () => {
  assert.deepEqual(LIST_SCOPES.filter((s) => s.id !== 'legacy' || listScopeCounts(lists.filter((l) => l.visibility === 'members'), 1).legacy > 0).map((s) => s.id), ['all', 'mine', 'people', 'team', 'legacy'])
  assert.deepEqual(LIST_SCOPES.filter((s) => s.id !== 'legacy' || listScopeCounts(lists.slice(0, 2), 1).legacy > 0).map((s) => s.id), ['all', 'mine', 'people', 'team'])
})

test('ownership is said once, with the access that differs from it', () => {
  assert.equal(listAccessLabel(lists[0], 1), 'Owned by you')
  assert.equal(listAccessLabel({ ...lists[0], role: 'owner' }, 1), 'Owned by you')
  assert.equal(listAccessLabel(lists[2], 1), 'By Alex Morgan · Can view')
  assert.equal(listAccessLabel(lists[3], 1), 'By Alex Morgan · Can edit')
  assert.equal(listAccessLabel({ ...lists[2], role: 'admin' }, 1), 'By Alex Morgan · Admin access')
  assert.equal(listAccessLabel({ ...lists[2], ownerName: '', role: 'viewer' }, 1), 'Shared with you · Can view')
  assert.equal(listAccessLabel({ ...lists[2], ownerName: '', role: undefined }, 1), 'Shared with you')
})

test('the count says N or N of M, and says none plainly', () => {
  assert.equal(listCountLabel(0, 0), 'No lists')
  assert.equal(listCountLabel(1, 1), '1 list')
  assert.equal(listCountLabel(5, 5), '5 lists')
  assert.equal(listCountLabel(2, 5), '2 of 5 lists')
})
