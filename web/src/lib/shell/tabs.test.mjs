import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./tabs.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { jumpMatches } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

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
