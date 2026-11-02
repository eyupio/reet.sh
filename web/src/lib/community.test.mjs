import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { communityCountLabel, filterCommunityLists, filterCommunitySkills, matchesCommunity } = await import(`data:text/javascript;base64,${Buffer.from(compile('./community.ts')).toString('base64')}`)

const list = (over = {}) => ({ id: 1, ownerId: 1, ownerName: 'Sam Taylor', name: 'Launch checklist', description: 'Release week.', visibility: 'public', skillCount: 3, memberCount: 1, createdAt: 1, updatedAt: 2, ...over })
const skill = (over = {}) => ({ id: 'demo/toolkit/pdf', source: 'demo/toolkit', slug: 'pdf', sourceType: 'github', url: '', name: 'PDF', description: 'Read and combine PDFs.', notes: '', installs: 0, installsWeek: 0, trend: [], liked: false, bookmarked: false, used: false, createdAt: 1, updatedAt: 2, useCount: 0, memberCount: 1, ...over })

test('community search reads every word across fields', () => {
  assert.equal(matchesCommunity('', 'Launch checklist'), true)
  assert.equal(matchesCommunity('LAUNCH', 'Launch checklist'), true)
  assert.equal(matchesCommunity('release sam', 'Launch checklist', 'Release week.', 'Sam Taylor'), true)
  assert.equal(matchesCommunity('release alex', 'Launch checklist', 'Release week.', 'Sam Taylor'), false)
})

test('lists and skills are filtered without changing the order the API sent', () => {
  const lists = [list({ id: 1 }), list({ id: 2, name: 'Data wrangling', description: 'CSV and SQL.', ownerName: 'Jo Patel' })]
  const skills = [skill({ id: 'a', name: 'PDF', source: 'demo/toolkit' }), skill({ id: 'b', name: 'Charts', source: 'demo/charts', description: 'Bar charts.' })]
  assert.deepEqual(filterCommunityLists(lists, '').map((l) => l.id), [1, 2])
  assert.deepEqual(filterCommunityLists(lists, 'data jo').map((l) => l.id), [2])
  assert.deepEqual(filterCommunitySkills(skills, '').map((s) => s.id), ['a', 'b'])
  assert.deepEqual(filterCommunitySkills(skills, 'charts').map((s) => s.id), ['b'])
  assert.deepEqual(filterCommunitySkills(skills, 'pdf demo').map((s) => s.id), ['a'])
})

test('the count says N or N of M, and names the noun', () => {
  assert.equal(communityCountLabel(0, 0, 'lists'), 'No lists')
  assert.equal(communityCountLabel(0, 0, 'skills'), 'No skills')
  assert.equal(communityCountLabel(1, 1, 'lists'), '1 list')
  assert.equal(communityCountLabel(3, 3, 'skills'), '3 skills')
  assert.equal(communityCountLabel(1, 3, 'lists'), '1 of 3 lists')
  assert.equal(communityCountLabel(1, 1, 'skills'), '1 skill')
})
