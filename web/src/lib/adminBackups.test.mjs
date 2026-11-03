import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./adminBackups.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const {
  backupCopyCountLabel, backupCopyMatchesSearch, backupCopyScopeCounts, backupRunCountLabel,
  backupRunMatchesSearch, backupRunScopeCounts, filterBackupCopies, filterBackupRuns,
} = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const copy = (over = {}) => ({ id: 'b1', kind: 'instance', target: 'reet-b1.tar.age', takenAt: 1, bytes: 1024, local: true, remotes: [], ...over })
const copies = [
  copy(),
  copy({ id: 'b2', target: 'reet-b2.tar.age', local: false, remotes: ['s3-eu'] }),
  copy({ id: 'b3', target: 'reet-b3.tar.age', local: true, remotes: ['s3-eu', 'r2'] }),
]
const verified = { b3: { ok: false, problems: ['The checksum does not match.'] } }
const run = (over = {}) => ({ id: 1, scope: 'instance', trigger: 'schedule', state: 'succeeded', startedAt: 1, finishedAt: 2, bytes: 1024, object: 'reet-b1.tar.age', offsite: 's3-eu', error: '', ...over })
const runs = [
  run(),
  run({ id: 2, trigger: 'manual', state: 'failed', offsite: '', error: 'The destination refused the copy.' }),
  run({ id: 3, trigger: 'schedule', state: 'failed', error: 'Timed out.' }),
]

test('a copy is found by id, target, version, location or a verification problem', () => {
  assert.equal(backupCopyMatchesSearch(copies[2], ''), true)
  assert.equal(backupCopyMatchesSearch(copies[2], 'b3'), true)
  assert.equal(backupCopyMatchesSearch(copies[2], 's3-eu'), true)
  assert.equal(backupCopyMatchesSearch(copies[2], 'this machine'), true)
  assert.equal(backupCopyMatchesSearch(copies[1], 'this machine'), false)
  assert.equal(backupCopyMatchesSearch(copies[2], 'checksum', verified), true)
  assert.equal(backupCopyMatchesSearch(copies[2], 'checksum'), false)
})

test('the copy chips count the copies the search leaves, and two chips may overlap', () => {
  assert.deepEqual(backupCopyScopeCounts(copies), { all: 3, local: 2, remote: 2, damaged: 0 })
  assert.deepEqual(backupCopyScopeCounts(copies, verified), { all: 3, local: 2, remote: 2, damaged: 1 })
  const searched = copies.filter((item) => backupCopyMatchesSearch(item, 's3-eu', verified))
  assert.deepEqual(backupCopyScopeCounts(searched, verified), { all: 2, local: 1, remote: 2, damaged: 1 })
})

test('filterBackupCopies combines the chip with the search and keeps order', () => {
  assert.deepEqual(filterBackupCopies(copies, 'all').map((item) => item.id), ['b1', 'b2', 'b3'])
  assert.deepEqual(filterBackupCopies(copies, 'local').map((item) => item.id), ['b1', 'b3'])
  assert.deepEqual(filterBackupCopies(copies, 'remote').map((item) => item.id), ['b2', 'b3'])
  assert.deepEqual(filterBackupCopies(copies, 'damaged', '', verified).map((item) => item.id), ['b3'])
  assert.deepEqual(filterBackupCopies(copies, 'remote', 'b1').map((item) => item.id), [])
})

test('a run is found by trigger, outcome, destination, object or error', () => {
  assert.equal(backupRunMatchesSearch(runs[1], 'manual'), true)
  assert.equal(backupRunMatchesSearch(runs[1], 'failed'), true)
  assert.equal(backupRunMatchesSearch(runs[1], 'refused'), true)
  assert.equal(backupRunMatchesSearch(runs[0], 's3-eu'), true)
  assert.equal(backupRunMatchesSearch(runs[0], 'timed out'), false)
})

test('the run chips count the runs the search leaves', () => {
  assert.deepEqual(backupRunScopeCounts(runs), { all: 3, succeeded: 1, failed: 2, running: 0 })
  const searched = runs.filter((item) => backupRunMatchesSearch(item, 'schedule'))
  assert.deepEqual(backupRunScopeCounts(searched), { all: 2, succeeded: 1, failed: 1, running: 0 })
})

test('filterBackupRuns combines the outcome with the search and keeps order', () => {
  assert.deepEqual(filterBackupRuns(runs, 'failed').map((item) => item.id), [2, 3])
  assert.deepEqual(filterBackupRuns(runs, 'all', 'schedule').map((item) => item.id), [1, 3])
  assert.deepEqual(filterBackupRuns(runs, 'succeeded', 'schedule').map((item) => item.id), [1])
})

test('the counts say N copies or N of M copies, and N runs or N of M runs', () => {
  assert.equal(backupCopyCountLabel(0, 0), 'No copies')
  assert.equal(backupCopyCountLabel(1, 1), '1 copy')
  assert.equal(backupCopyCountLabel(2, 3), '2 of 3 copies')
  assert.equal(backupRunCountLabel(0, 0), 'No runs')
  assert.equal(backupRunCountLabel(1, 1), '1 run')
  assert.equal(backupRunCountLabel(2, 3), '2 of 3 runs')
})
