import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const compiled = ts.transpileModule(readFileSync(new URL('./templates.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
  .replace("from '../gafferOutcome'", `from '${asURL('export const stageLabel = kind => kind[0].toUpperCase() + kind.slice(1)')}'`)
const t = await import(asURL(compiled))

test('changes are shown since the version last used, and never for a first use', () => {
  assert.equal(t.changesSince(2, 3), 2)
  assert.equal(t.changesSince(3, 3), 0)
  assert.equal(t.changesSince(undefined, 3), 0)
  assert.equal(t.changesSince(0, 3), 0)
})

test('newer than last used means the latest is past the last used version', () => {
  assert.equal(t.newerThanLastUsed({ latest: 3, lastUsedVersion: 2 }), true)
  assert.equal(t.newerThanLastUsed({ latest: 3, lastUsedVersion: 3 }), false)
  assert.equal(t.newerThanLastUsed({ latest: 3 }), false)
  assert.equal(t.usedWords(2), 'Last used at version 2')
  assert.equal(t.usedWords(undefined), 'Not used yet')
})

test('skipped skills are exactly the keys of the gaps shown', () => {
  assert.deepEqual(t.skippedKeys([{ key: '0:0:library:a' }, { key: '1:2:builtin:b' }]), ['0:0:library:a', '1:2:builtin:b'])
  assert.deepEqual(t.skippedKeys([]), [])
  assert.match(t.gapWords({ name: 'Go tests', status: 'changed' }), /newer approved revision/)
  assert.match(t.gapWords({ name: 'Go tests', status: 'unavailable' }), /not available now/)
})

test('a difference reads as plain lines, and a pipeline that is the same says nothing', () => {
  const lines = t.diffLines({
    from: 2, to: 3, changed: true, nameChanged: true, descriptionChanged: false, objectiveChanged: false, limitsChanged: false,
    pipelines: [
      { title: 'Export API', change: 'changed', promptChanged: true, autoMergeChanged: false, stagesChanged: true, stagesBefore: ['code', 'review'], stagesAfter: ['code', 'review', 'test'],
        criteria: { added: ['It is fast.'], removed: ['It is slow.'], reworded: [{ id: 'c1', from: 'Exports CSV.', to: 'Downloads as CSV.' }] },
        skills: [{ stage: 'code', id: 'a', name: 'Go tests', from: 'abcdef012345', to: '123456abcdef' }, { stage: 'test', id: 'b', name: 'Lint', from: '', to: 'ffff0000aaaa' }, { stage: 'review', id: 'c', name: 'Docs', from: 'eeee1111bbbb', to: '' }] },
      { title: 'Importer', change: 'added', promptChanged: false, autoMergeChanged: false, stagesChanged: false, stagesBefore: [], stagesAfter: [], criteria: { added: [], removed: [], reworded: [] }, skills: [] },
      { title: 'Old', change: 'removed', promptChanged: false, autoMergeChanged: false, stagesChanged: false, stagesBefore: [], stagesAfter: [], criteria: { added: [], removed: [], reworded: [] }, skills: [] },
      { title: 'Same', change: 'same', promptChanged: false, autoMergeChanged: false, stagesChanged: false, stagesBefore: [], stagesAfter: [], criteria: { added: [], removed: [], reworded: [] }, skills: [] },
    ],
  })
  assert.deepEqual(lines.general, ['The name changed.'])
  assert.deepEqual(lines.pipelines.map(p => p.title), ['Export API', 'Importer', 'Old'])
  assert.deepEqual(lines.pipelines[0].lines, [
    'The prompt changed.', 'Stages were Code, Review; now Code, Review, Test.',
    'Criterion added: “It is fast.”', 'Criterion removed: “It is slow.”', 'Criterion reworded: “Exports CSV.” to “Downloads as CSV.”',
    '“Go tests” in Code moved from revision abcdef01 to 123456ab.', '“Lint” added to Test at revision ffff0000.', '“Docs” removed from Review.',
  ])
  assert.deepEqual(lines.pipelines[1].lines, ['Added.'])
})

test('a changed template is said in the person’s words, other refusals in the server’s', () => {
  assert.match(t.conflictWords('template_changed', 'x'), /another tab/)
  assert.equal(t.conflictWords('template_limit', 'You already have 20 templates.'), 'You already have 20 templates.')
})
