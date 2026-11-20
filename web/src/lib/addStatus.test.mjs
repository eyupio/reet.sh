import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

// The words of Community's Save and a list's Add all, which add skills Active (preselected) or Archived.
const source = ts.transpileModule(readFileSync(new URL('./addStatus.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const a = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)

test('Add all says how many skills and machines before it sends', () => {
  assert.equal(a.addAllTitle(12, 3, 'active'), 'Put 12 skills on all 3 machines?')
  assert.equal(a.addAllTitle(1, 3, 'active'), 'Put 1 skill on all 3 machines?')
  assert.equal(a.addAllTitle(12, 1, 'active'), 'Put 12 skills on your machine?')
  assert.equal(a.addAllTitle(12, 0, 'active'), 'Add 12 skills to your library?')
  assert.equal(a.addAllTitle(12, null, 'active'), 'Put 12 skills on every machine?')
  assert.equal(a.addAllTitle(12, 3, 'archived'), 'Add 12 skills to your library as Archived?')
})

test('Save and Add all say where the skills went', () => {
  assert.equal(a.savedNotice('pdf', 'active'), 'Saved pdf to your library as Active · going on every machine')
  assert.equal(a.savedNotice('pdf', 'archived'), 'Saved pdf to your library as Archived · kept off your machines')
  assert.equal(a.addedNotice(2, 'active'), 'Added 2 skills to your library as Active · going on every machine')
  assert.equal(a.addedNotice(1, 'archived'), 'Added 1 skill to your library as Archived · kept off your machines')
  assert.equal(a.addedNotice(0, 'active'), 'Every skill was already in your library. Nothing changed.')
})
