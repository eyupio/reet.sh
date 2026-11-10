import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./librarySync.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { libraryNotice, groupLeftOut, leftOutHeadline, copyingLine, duplicatesNote, copiesSignature } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const skill = (name, problem) => ({ id: `local/${name}`, name, slug: name, source: problem ? '' : 'org/repo', sourceType: problem ? 'local' : 'github', ...(problem ? { problem } : {}) })
const copy = (name, state, changes = {}) => ({ id: `${name}-${state}-${changes.createdAt ?? 1}`, requestKey: 'k', agentId: 'a', environmentId: 'e', host: 'laptop', agent: 'agents', project: '', name, syncSkillId: `local/${name}`, destinationId: 'dest', state, bundleId: '', expectedBase: '', createdAt: 1, updatedAt: 1, expiresAt: 9, excluded: [], ...changes })

test('a skill whose copy failed twice is listed once, from the library, in plain words', () => {
  // The screenshot showed "seo: capture_failed" twice, because both the original
  // attempt and its retry were listed. Only the library's own reason is shown now.
  const reason = 'This skill folder could not be copied from its machine; it may contain unsupported files, or be larger than 256 files or 8 MiB.'
  const notice = libraryNotice(
    [skill('seo', reason), skill('review')],
    [copy('seo', 'failed', { createdAt: 1, error: 'capture_failed' }), copy('seo', 'failed', { createdAt: 2, error: 'capture_failed' })],
  )
  assert.equal(notice.leftOutCount, 1)
  assert.equal(notice.leftOut.length, 1)
  assert.deepEqual(notice.leftOut[0].names, ['seo'])
  assert.match(notice.leftOut[0].reason, /could not be copied from its machine/)
  assert.deepEqual(notice.copying, [])
  assert.deepEqual(notice.review, [])
  assert.ok(!JSON.stringify(notice).includes('capture_failed'), 'a raw error code reached the page')
})

test('a failed copy of a skill the library no longer needs shows nothing', () => {
  // reet-product-design had a failed machine copy even though the written skill of
  // that name syncs fine. Nothing about it is left to tell the person.
  const notice = libraryNotice([skill('reet-product-design')], [copy('reet-product-design', 'failed', { error: 'transfer_failed' })])
  assert.deepEqual(notice, { copying: [], review: [], leftOut: [], leftOutCount: 0 })
})

test('a copy in flight is shown as copying, and a retry hides the failure before it', () => {
  const notice = libraryNotice([skill('seo')], [copy('seo', 'failed', { createdAt: 1, error: 'capture_failed' }), copy('seo', 'previewing', { createdAt: 2 })])
  assert.equal(notice.copying.length, 1)
  assert.equal(notice.copying[0].state, 'previewing')
})

test('a request that a newer one replaced is not shown, even if it was never settled', () => {
  // An old "needs review" that was abandoned, then retried and installed, must not
  // keep asking the person to review files.
  const stale = copy('notes', 'reviewing', { createdAt: 1 })
  const replaced = copy('notes', 'completed', { createdAt: 2, syncRunId: 'run-9' })
  assert.deepEqual(libraryNotice([], [stale, replaced], [{ id: 'run-9', status: 'completed' }]), { copying: [], review: [], leftOut: [], leftOutCount: 0 })
  // And an older request still in flight does not outlive a newer one that failed.
  assert.deepEqual(libraryNotice([], [copy('seo', 'queued', { createdAt: 1 }), copy('seo', 'failed', { createdAt: 2 })]), { copying: [], review: [], leftOut: [], leftOutCount: 0 })
})

test('a copy that needs the person to look at some files is the one thing that asks for action', () => {
  const notice = libraryNotice([], [copy('notes', 'reviewing')])
  assert.equal(notice.review.length, 1)
  assert.equal(notice.copying.length, 0)
})

test('a copy whose install already finished is history, and one still installing is not repeated', () => {
  const finished = copy('done', 'completed', { syncRunId: 'run-1' })
  const installing = copy('busy', 'completed', { syncRunId: 'run-2' })
  const saved = copy('saved', 'completed')
  const notice = libraryNotice([], [finished, installing, saved], [{ id: 'run-1', status: 'completed' }, { id: 'run-2', status: 'running' }])
  assert.deepEqual(notice.copying.map(item => item.name), ['saved'])
})

test('requests that are not about a library skill and a destination are ignored', () => {
  const notice = libraryNotice([], [copy('manual', 'queued', { syncSkillId: undefined }), copy('nowhere', 'queued', { destinationId: undefined })])
  assert.deepEqual(notice, { copying: [], review: [], leftOut: [], leftOutCount: 0 })
})

test('skills left out for the same reason are one line, with the sentence said once', () => {
  // Eight written skills all hit the same storage message in the report. That is one
  // sentence naming eight skills, not eight copies of a long paragraph.
  const full = name => `${name} cannot be synced privately: private skill storage is full of skills still in use.`
  const skills = ['reet-api-design', 'reet-architect', 'reet-babysit'].map(name => ({ id: `w/${name}`, name, problem: full(name) }))
  const groups = groupLeftOut(skills)
  assert.equal(groups.length, 1)
  assert.deepEqual(groups[0].names, ['reet-api-design', 'reet-architect', 'reet-babysit'])
  assert.deepEqual(groups[0].ids, ['w/reet-api-design', 'w/reet-architect', 'w/reet-babysit'])
  assert.equal(groups[0].reason, ' cannot be synced privately: private skill storage is full of skills still in use.')
  // Read together: "<names> cannot be synced privately: …" with no skill named twice.
  const line = `${groups[0].names.join(', ')}${groups[0].reason}`
  assert.equal(line, 'reet-api-design, reet-architect, reet-babysit cannot be synced privately: private skill storage is full of skills still in use.')
  assert.ok(!/(reet-api-design)[:\s]+.*\1/.test(line), 'a skill name repeated in its own line')
})

test('different reasons stay separate, in the order they first appeared, and a reason without the name is attached with a colon', () => {
  const groups = groupLeftOut([
    { id: 'a', name: 'seo', problem: 'This skill folder could not be copied from its machine.' },
    { id: 'b', name: 'reet-x', problem: 'reet-x cannot be synced privately: storage is full.' },
    { id: 'c', name: 'big', problem: 'This skill folder could not be copied from its machine.' },
    { id: 'd', name: 'fine' },
    { id: 'e', name: 'blank', problem: '   ' },
  ])
  assert.deepEqual(groups.map(group => group.names), [['seo', 'big'], ['reet-x']])
  assert.equal(groups[0].reason, ': This skill folder could not be copied from its machine.')
  assert.equal(groups[1].reason, ' cannot be synced privately: storage is full.')
})

test('a longer name that merely begins with this one is not mistaken for it', () => {
  const [group] = groupLeftOut([{ id: 'x', name: 'reet', problem: 'reet-api-design cannot be synced' }])
  assert.equal(group.reason, ': reet-api-design cannot be synced')
})

test('the count is skills, however they are grouped', () => {
  const notice = libraryNotice([
    { id: 'a', name: 'a', problem: 'a cannot be synced privately: x' },
    { id: 'b', name: 'b', problem: 'b cannot be synced privately: x' },
    { id: 'c', name: 'c', problem: 'c cannot be synced privately: y' },
  ], [])
  assert.equal(notice.leftOutCount, 3)
  assert.equal(notice.leftOut.length, 2)
})

test('the headline says everything else is syncing, and counts correctly', () => {
  assert.equal(leftOutHeadline(1), '1 skill is left out for now. Everything else is syncing.')
  assert.equal(leftOutHeadline(9), '9 skills are left out for now. Everything else is syncing.')
})

test('what is being copied names a few skills, then counts the rest', () => {
  assert.match(copyingLine([{ name: 'seo' }]), /^Copying seo privately from your machines\./)
  assert.match(copyingLine(['a', 'b', 'c', 'd', 'e', 'f'].map(name => ({ name }))), /^Copying a, b, c, d and 2 more privately/)
  assert.match(copyingLine([{ name: 'seo' }, { name: 'seo' }]), /^Copying seo privately/)
})

test('a shared name is explained as information, not as a problem', () => {
  assert.equal(duplicatesNote([]), '')
  const note = duplicatesNote([{ slug: 'improve-animations', keep: {}, remove: [{}] }])
  assert.match(note, /^One name is in your library more than once/)
  assert.match(note, /improve-animations/)
  assert.match(note, /untouched/)
  const many = duplicatesNote(['a', 'b', 'c', 'd', 'e', 'f'].map(slug => ({ slug, keep: {}, remove: [{}] })))
  assert.match(many, /^6 names are/)
  assert.match(many, /a, b, c, d and 2 more/)
  assert.ok(!/error|problem|fail/i.test(`${note} ${many}`))
})

test('the copies fingerprint changes when a copy starts, finishes or fails, and only then', () => {
  // The Sync card reads the library again when this changes, so a copy that fails
  // while the page is open is reported at once instead of after a reload.
  const quiet = [copy('seo', 'previewing', { id: 'a' }), copy('notes', 'queued', { id: 'b' })]
  const same = copiesSignature(quiet)
  assert.equal(copiesSignature([...quiet].reverse()), same, 'the order the server lists them in is not a change')
  assert.equal(copiesSignature(quiet.map(item => ({ ...item, updatedAt: 99, expiresAt: 5 }))), same, 'a timestamp is not a change')
  assert.notEqual(copiesSignature([copy('seo', 'failed', { id: 'a' }), quiet[1]]), same, 'a copy failing is a change')
  assert.notEqual(copiesSignature([copy('seo', 'completed', { id: 'a' }), quiet[1]]), same, 'a copy finishing is a change')
  assert.notEqual(copiesSignature([...quiet, copy('new', 'queued', { id: 'c' })]), same, 'a new copy is a change')
  assert.notEqual(copiesSignature([quiet[0]]), same, 'a copy going away is a change')
})

test('requests that are not about a library skill never trigger a re-read', () => {
  assert.equal(copiesSignature([copy('manual', 'queued', { syncSkillId: undefined })]), '')
  assert.equal(copiesSignature([]), '')
})
