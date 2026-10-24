import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
const { outputText } = ts.transpileModule(readFileSync(new URL('./syncSelection.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { dedupeSkills, sourceConflicts, localDuplicates } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const skill = (slug, source, sourceType = source ? 'github' : 'local') => ({ id: `${source || 'local'}/${slug}`, name: slug, slug, source, sourceType })

test('dedupe keeps the source-linked copy when a local copy shares its name', () => {
  const out = dedupeSkills([skill('improve'), skill('improve', 'owner/skills')])
  assert.equal(out.length, 1)
  assert.equal(out[0].source, 'owner/skills')
})

test('dedupe keeps local copies when no linked copy exists', () => {
  const out = dedupeSkills([skill('local-only')])
  assert.equal(out.length, 1)
  assert.equal(out[0].sourceType, 'local')
})

test('unknown local copies are auto-replaced while different known sources conflict', () => {
  const want = skill('improve', 'owner/skills')
  assert.equal(localDuplicates([want], [skill('improve')]).length, 1)
  assert.equal(sourceConflicts([want], [skill('improve')]).length, 0)
  assert.equal(sourceConflicts([want], [skill('improve', 'other/skills')]).length, 1)
  assert.equal(sourceConflicts([want], [skill('improve', 'https://github.com/owner/skills.git')]).length, 0)
})
