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
  const when = at => `at ${at}`
  assert.equal(t.usedWords(2, 1795, when), 'Last used at 1795 (version 2)')
  assert.equal(t.usedWords(2, 0, when), 'Last used at version 2')
  assert.equal(t.usedWords(undefined, 0, when), 'Not used yet')
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

test('a changed template says which version was saved elsewhere and which this saves, other refusals in the server’s words', () => {
  assert.equal(t.conflictWords('template_changed', 'x', 4), 'Version 4 was saved elsewhere. This now saves as version 5.')
  assert.equal(t.conflictWords('template_changed', 'x', 4, true), 'Version 4 was saved elsewhere, so nothing was deleted. Delete again if you still want to.')
  assert.match(t.conflictWords('template_changed', 'x'), /another tab/i)
  assert.equal(t.conflictWords('template_limit', 'You already have 20 templates.'), 'You already have 20 templates.')
})

test('what a template does at the end, and for how long, is said plainly', () => {
  assert.equal(t.mergeWords(true), 'Merges by itself once checks pass.')
  assert.equal(t.mergeWords(false), 'Stops at Ready to merge.')
  assert.equal(t.limitsWords({ maxReviews: 8, hours: 24 }), 'Up to 8 reviews and 24 hours for each session.')
  assert.equal(t.limitsWords({ maxReviews: 1, hours: 1 }), 'Up to 1 review and 1 hour for each session.')
})

test('the access paragraph says which pipelines merge by themselves', () => {
  assert.match(t.mergeAccessWords([], 2, false), /^Gaffer cannot merge, run commands or answer approvals\. Every pipeline stops at Ready to merge/)
  assert.equal(t.mergeAccessWords(['Export API'], 2, false), 'Gaffer cannot run commands or answer approvals. “Export API” merges by itself once checks pass. The others stop at Ready to merge and wait for you.')
  assert.match(t.mergeAccessWords(['A', 'B'], 2, true), /^Gaffer cannot run commands or answer approvals\. Every pipeline merges by itself once checks pass\. On YOLO they still stop/)
})

test('an outcome is offered as a template only where the server would keep it', () => {
  const done = { state: 'done', stages: [{}] }
  assert.equal(t.canSaveAsTemplate({ state: 'settled', sessions: [done, { state: 'delivery_review', stages: [{}], deliveryAccepted: true }] }), true)
  assert.equal(t.canSaveAsTemplate({ state: 'settled', sessions: [done, { state: 'failed', stages: [{}], deliveryAccepted: true }] }), false)
  assert.equal(t.canSaveAsTemplate({ state: 'settled', sessions: [done, { state: 'following', stages: [{}] }] }), false)
  assert.equal(t.canSaveAsTemplate({ state: 'settled', sessions: [{ state: 'done' }] }), false)
  assert.equal(t.canSaveAsTemplate({ state: 'starting', sessions: [done] }), false)
  assert.equal(t.canSaveAsTemplate({ state: 'settled', sessions: [] }), false)
})

test('pipelines put in another order say so', () => {
  const lines = t.diffLines({ from: 1, to: 2, changed: true, nameChanged: false, descriptionChanged: false, objectiveChanged: false, limitsChanged: false, orderChanged: true, pipelines: [] })
  assert.deepEqual(lines.general, ['The pipelines run in another order.'])
})

test('changes are titled from the version last used, or the oldest kept when that one is gone', () => {
  assert.deepEqual(t.changesWords({ from: 2, to: 3, olderNotShown: false }, 2), { title: 'Changed since version 2, the one you last used', note: '' })
  assert.deepEqual(t.changesWords({ from: 2, to: 1, olderNotShown: false }, 2), { title: 'Differs from version 2, the one you last used', note: '' })
  assert.deepEqual(t.changesWords({ from: 4, to: 23, olderNotShown: true }, 1), { title: 'Changed since version 4, the oldest kept', note: 'Version 1, the one you last used, is no longer kept, so changes before version 4 are not shown.' })
})

test('a skills refusal gives its gaps when the server sent them, and nothing otherwise', () => {
  const gap = { pipeline: 0, stage: 1, id: 'library:a', name: 'A', revision: 'r1', status: 'unavailable', key: '0:1:library:a' }
  assert.deepEqual(t.refusedGaps({ code: 'template_skills_changed', gaps: [gap] }), [gap])
  assert.deepEqual(t.refusedGaps({ code: 'template_skills_changed', gaps: [] }), [])
  assert.equal(t.refusedGaps({ code: 'template_skills_changed' }), null)
  assert.equal(t.refusedGaps(undefined), null)
})

test('a tier of the launch crew is the stage entry without its stage fields', () => {
  assert.deepEqual(t.tierCrew({ kind: 'code', tier: 'heavy', tool: 'claude', model: 'm', effort: 'high', backup: { tool: 'codex' }, moreBackups: [{ tool: 'opencode' }] }),
    { tool: 'claude', model: 'm', effort: 'high', backup: { tool: 'codex' }, moreBackups: [{ tool: 'opencode' }] })
  assert.deepEqual(t.tierCrew({ kind: 'code', tool: 'cline', provider: 'own' }), { tool: 'cline', provider: 'own' })
})
