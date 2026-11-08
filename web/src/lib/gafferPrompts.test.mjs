import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = readFileSync(new URL('./gafferPrompts.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const p = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)

const fakeStorage = () => {
  const data = new Map()
  return { getItem: key => (data.has(key) ? data.get(key) : null), setItem: (key, value) => data.set(key, String(value)), removeItem: key => data.delete(key), data }
}
const text = n => `Goal ${n}`

test('the built-in prompts are distinct, non-empty and fit the goal box', () => {
  const ids = new Set(), titles = new Set()
  for (const prompt of p.BUILT_IN_GOAL_PROMPTS) {
    assert.ok(prompt.id && !ids.has(prompt.id), `duplicate or empty id: ${prompt.id}`)
    assert.ok(prompt.title && !titles.has(prompt.title), `duplicate or empty title: ${prompt.title}`)
    assert.ok(prompt.text.trim().length > 0)
    assert.ok(p.goalPromptBytes(prompt.text) <= p.MAX_GOAL_PROMPT_BYTES, `${prompt.id} is too big`)
    assert.match(prompt.text, /Do not merge\.$/, `${prompt.id} must keep merging in the person's hands`)
    ids.add(prompt.id)
    titles.add(prompt.title)
  }
  assert.ok(p.BUILT_IN_GOAL_PROMPTS.length >= 3)
})

test('a title is the first line with something to say, shortened', () => {
  assert.equal(p.goalPromptTitle('  \n\nShip the pull request\nand fix checks'), 'Ship the pull request')
  assert.equal(p.goalPromptTitle('  Ship   it  '), 'Ship it')
  assert.equal(p.goalPromptTitle('   '), 'Untitled prompt')
  assert.equal(p.goalPromptTitle('x'.repeat(200), 12), `${'x'.repeat(11)}…`)
})

test('stored prompts are read defensively', () => {
  assert.deepEqual(p.parseGoalPrompts(null), [])
  assert.deepEqual(p.parseGoalPrompts('not json'), [])
  assert.deepEqual(p.parseGoalPrompts('{"version":1,"prompts":"nope"}'), [])
  const good = { id: 'a', text: 'Keep going', createdAt: 5 }
  assert.deepEqual(p.parseGoalPrompts(JSON.stringify({ version: 1, prompts: [good] })), [good])
  assert.deepEqual(p.parseGoalPrompts(JSON.stringify([good])), [good])
  const rows = [
    good,
    { id: 'b', text: '  ' },
    { id: '', text: 'No id' },
    { id: 'c', text: 'Keep going' },
    { id: 'd', text: 'x'.repeat(p.MAX_GOAL_PROMPT_BYTES + 1) },
  ]
  assert.deepEqual(p.parseGoalPrompts(JSON.stringify(rows)).map(row => row.id), ['a'])
})

test('saving keeps the newest first, refuses duplicates and empty text, and drops the oldest past the limit', () => {
  const one = p.addGoalPrompt([], '  Ship it  ', 'a', 1)
  assert.deepEqual(one, [{ id: 'a', text: 'Ship it', createdAt: 1 }])
  assert.equal(p.addGoalPrompt(one, 'Ship it', 'b', 2), one)
  assert.equal(p.addGoalPrompt(one, '   ', 'b', 2), one)
  assert.equal(p.addGoalPrompt(one, 'x'.repeat(p.MAX_GOAL_PROMPT_BYTES + 1), 'b', 2), one)
  let list = []
  for (let i = 0; i < p.MAX_GOAL_PROMPTS + 3; i++) list = p.addGoalPrompt(list, text(i), `id-${i}`, i)
  assert.equal(list.length, p.MAX_GOAL_PROMPTS)
  assert.equal(list[0].id, `id-${p.MAX_GOAL_PROMPTS + 2}`)
  assert.equal(list.some(row => row.id === 'id-0'), false)
})

test('removing takes only the named prompt', () => {
  const list = [{ id: 'a', text: 'A', createdAt: 1 }, { id: 'b', text: 'B', createdAt: 2 }]
  assert.deepEqual(p.removeGoalPrompt(list, 'a').map(row => row.id), ['b'])
  assert.deepEqual(p.removeGoalPrompt(list, 'missing'), list)
})

test('each account has its own prompts, and a blocked store leaves the list working', () => {
  const area = fakeStorage()
  p.writeGoalPrompts(1, [{ id: 'a', text: 'Mine', createdAt: 1 }], area)
  assert.deepEqual(p.readGoalPrompts(1, area), [{ id: 'a', text: 'Mine', createdAt: 1 }])
  assert.deepEqual(p.readGoalPrompts(2, area), [])
  assert.notEqual(p.goalPromptsKey(1), p.goalPromptsKey(2))
  const blocked = { getItem: () => { throw new Error('blocked') }, setItem: () => { throw new Error('blocked') } }
  assert.deepEqual(p.readGoalPrompts(1, blocked), [])
  assert.doesNotThrow(() => p.writeGoalPrompts(1, [], blocked))
})
