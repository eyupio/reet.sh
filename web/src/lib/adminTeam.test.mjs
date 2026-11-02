import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./adminTeam.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { teamUsageCountLabel, teamUsageMatchesSearch } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const skill = (over = {}) => ({ id: 'a', name: 'code-review', source: 'demo/toolkit', lists: [{ id: 1, name: 'Team essentials' }], tags: ['review'], usedBy: 3, onShelf: 4, installedBy: 3, environmentLinks: 2, ...over })

test('a skill is found by name, source, list or tag in any word order', () => {
  assert.equal(teamUsageMatchesSearch(skill(), ''), true)
  assert.equal(teamUsageMatchesSearch(skill(), 'code'), true)
  assert.equal(teamUsageMatchesSearch(skill(), 'demo toolkit'), true)
  assert.equal(teamUsageMatchesSearch(skill(), 'team essentials'), true)
  assert.equal(teamUsageMatchesSearch(skill(), 'review team'), true)
  assert.equal(teamUsageMatchesSearch(skill(), 'unrelated'), false)
})

test('the count says N skills or N of M skills', () => {
  assert.equal(teamUsageCountLabel(0, 0), 'No skills')
  assert.equal(teamUsageCountLabel(1, 1), '1 skill')
  assert.equal(teamUsageCountLabel(4, 4), '4 skills')
  assert.equal(teamUsageCountLabel(1, 4), '1 of 4 skills')
})
