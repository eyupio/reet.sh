import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const { stageItems, STAGE_KEYS, GOAL_LIMIT } = await import(asURL(compile('./stageActions.ts')))

test('every stage offers at least one follow-up item', () => {
  assert.deepEqual(STAGE_KEYS, ['code', 'pr', 'checks', 'review', 'merge'])
  for (const key of STAGE_KEYS) assert.ok(stageItems(key).length >= 1, key)
  assert.deepEqual(stageItems('nonsense'), [])
})

test('item ids are unique and every goal fits the follow-up editor', () => {
  const all = STAGE_KEYS.flatMap(key => stageItems(key))
  assert.equal(new Set(all.map(item => item.id)).size, all.length)
  for (const item of all) {
    assert.ok(item.label.trim() && item.text.trim(), item.id)
    assert.ok(item.text.length <= GOAL_LIMIT, item.id)
    assert.doesNotMatch(item.text, /https?:\/\//i, item.id)
  }
})

test('only the merge items let the pull request be merged, and they say when', () => {
  const merging = STAGE_KEYS.flatMap(key => stageItems(key)).filter(item => !/Do not merge/.test(item.text))
  assert.deepEqual(merging.map(item => item.id).sort(), ['merge-when-green', 'pr-babysit-merge'])
  for (const item of merging) {
    assert.match(item.text, /every check passes/, item.id)
    assert.match(item.text, /no review thread is open/, item.id)
    assert.match(item.text, /no (merge )?conflicts/, item.id)
    assert.match(item.text, /Bring decisions back to me/, item.id)
  }
})

test('the stages offer what the person asked for', () => {
  const ids = key => stageItems(key).map(item => item.id)
  assert.ok(ids('pr').includes('pr-babysit') && ids('pr').includes('pr-babysit-merge'))
  assert.ok(ids('review').includes('review-by-gaffer'))
  assert.ok(ids('merge').includes('merge-when-green'))
  assert.match(stageItems('review').find(item => item.id === 'review-by-gaffer').text, /feedback|instructions/)
})
