import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./tabs.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { jumpMatches, pageGroup } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const item = (kind, label) => ({ kind, label, detail: kind, href: `/${kind}/${label}` })

test('jump searches sessions, Projects, skills and machines in that order', () => {
  const items = [item('machine', 'store-box'), item('skill', 'store-review'), item('project', 'Store rewrite'), item('session', 'Store split')]
  assert.deepEqual(jumpMatches(items, 'store').map((i) => i.kind), ['session', 'project', 'skill', 'machine'])
})

test('a skill is found by its name, case-insensitively, and other rows are left out', () => {
  const items = [item('session', 'Unrelated'), item('skill', 'Code review'), item('machine', 'build-box')]
  assert.deepEqual(jumpMatches(items, 'CODE').map((i) => i.label), ['Code review'])
  assert.deepEqual(jumpMatches(items, 'code review').map((i) => i.label), ['Code review'])
})

test('an empty query shows nothing, and the list is capped after sorting', () => {
  assert.deepEqual(jumpMatches([item('skill', 'a')], '   '), [])
  const many = Array.from({ length: 12 }, (_, i) => item('skill', `skill-${String(i).padStart(2, '0')}`))
  assert.equal(jumpMatches(many, 'skill').length, 8)
  assert.deepEqual(jumpMatches(many, 'skill').map((i) => i.label), ['skill-00', 'skill-01', 'skill-02', 'skill-03', 'skill-04', 'skill-05', 'skill-06', 'skill-07'])
})

const SKILLS = 'Skills & distribution'
const HELP = 'Help & community'
const WORKSPACE = 'Workspace'
const tab = (label, group, current = false) => ({ href: `/${label}`, label, current, group })
const moreList = (current, { writer = true, support = true, admin = true } = {}) => [
  tab('Setups', SKILLS, current === 'Setups'),
  tab('Lists', SKILLS, current === 'Lists'),
  ...(writer ? [tab('Write a skill', SKILLS, current === 'Write a skill')] : []),
  tab('Community', HELP, current === 'Community'),
  ...(support ? [tab('Support', HELP, current === 'Support')] : []),
  tab('Teams', WORKSPACE, current === 'Teams'),
  ...(admin ? [tab('Instance administration', WORKSPACE, current === 'Instance administration')] : []),
]

test('the current page gives its group and every sibling, in order, with only itself current', () => {
  const result = pageGroup(moreList('Lists'))
  assert.equal(result.group, SKILLS)
  assert.deepEqual(result.items.map((i) => i.label), ['Setups', 'Lists', 'Write a skill'])
  assert.deepEqual(result.items.map((i) => i.current), [false, true, false])
  assert.deepEqual(Object.keys(result.items[0]).sort(), ['current', 'href', 'label'])
})

test('a group shrinks with what the viewer can reach', () => {
  assert.equal(pageGroup(moreList('Lists', { writer: false })).items.length, 2)
  const help = pageGroup(moreList('Community', { support: false }))
  assert.equal(help.group, HELP)
  assert.deepEqual(help.items.map((i) => i.label), ['Community'])
  const workspace = pageGroup(moreList('Teams', { admin: false }))
  assert.equal(workspace.group, WORKSPACE)
  assert.equal(workspace.items.length, 1)
})

test('a detail page that marks its list entry current marks that sibling', () => {
  const result = pageGroup(moreList('Support'))
  assert.equal(result.group, HELP)
  assert.deepEqual(result.items.map((i) => [i.label, i.current]), [['Community', false], ['Support', true]])
})

test('no current entry, or a current entry without a group, is not a grouped page', () => {
  assert.equal(pageGroup(moreList(null)), null)
  assert.equal(pageGroup([...moreList(null), { href: '/x', label: 'Ungrouped', current: true }]), null)
  assert.equal(pageGroup([]), null)
})
