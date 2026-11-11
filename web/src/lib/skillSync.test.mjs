import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
// Compile the pure helper without a browser or an extra test dependency.
const { outputText } = ts.transpileModule(readFileSync(new URL('./skillSync.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { skillSyncIntent, skillSyncSummary, writtenSkillWhere, matchesSyncFilter, settledJobsKey, SYNC_FILTERS } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

const sk = id => ({ id, name: id, slug: id, source: 'demo/toolkit', sourceType: 'github' })
const target = (host, agent, desired, missing = []) => ({ target: { id: `${host}-${agent}`, host, agent, name: host }, sourceId: 'shelf', sourceName: 'Shelf', desired, missing, extra: [], revision: 'r1', updatedAt: 0 })
const snapshot = targets => ({ environments: [], targets })
const report = (host, installed, complete = true) => ({ host, installed: new Set(installed), complete, groupCount: 1 })

test('a skill in no saved selection is not set to sync anywhere', () => {
  const intent = skillSyncIntent('a', snapshot([target('laptop', 'agents', [sk('b')])]))
  assert.deepEqual(intent, { set: [], pending: [], held: [] })
  assert.equal(skillSyncSummary(intent), 'Not set to sync')
})

test('a desired skill that the machine already has is set; one still missing is pending', () => {
  const snap = snapshot([target('laptop', 'agents', [sk('a')]), target('box', 'claude', [sk('a')], [sk('a')])])
  const intent = skillSyncIntent('a', snap)
  assert.deepEqual(intent, { set: ['laptop'], pending: ['box'], held: [] })
  assert.equal(skillSyncSummary(intent), 'Pending on box · synced to laptop')
})

test('a host with several scopes counts once and is pending if any scope is', () => {
  const snap = snapshot([target('laptop', 'agents', [sk('a')]), target('laptop', 'claude', [sk('a')], [sk('a')])])
  assert.deepEqual(skillSyncIntent('a', snap), { set: [], pending: ['laptop'], held: [] })
})

test('hosts are listed alphabetically and aliases match case-insensitively', () => {
  const snap = snapshot([target('Zed', 'agents', [sk('a')]), target('alpha', 'agents', [sk('a')]), { ...target('old-name', 'agents', [sk('a')]), hostAliases: ['ALPHA'] }])
  assert.deepEqual(skillSyncIntent('a', snap).set, ['alpha', 'old-name', 'Zed'])
  assert.equal(skillSyncSummary({ set: ['alpha', 'Zed'], pending: [] }), 'Set to sync to alpha, Zed')
})

test('the summary folds long host lists', () => {
  assert.equal(skillSyncSummary({ set: ['a', 'b', 'c', 'd'], pending: [] }), 'Set to sync to a, b and 2 more')
  assert.equal(skillSyncSummary({ set: [], pending: ['a', 'b', 'c', 'd'] }), 'Pending on a, b and 2 more')
})

test('an empty or missing snapshot is simply not set', () => {
  assert.deepEqual(skillSyncIntent('a', undefined), { set: [], pending: [], held: [] })
  assert.deepEqual(skillSyncIntent('a', { environments: [], targets: undefined }), { set: [], pending: [], held: [] })
})

test('sync filters answer where-is-it questions from intent and observed reports', () => {
  const snap = snapshot([target('laptop', 'agents', [sk('a'), sk('c')]), target('box', 'agents', [sk('c')], [sk('c')])])
  const reports = [report('laptop', ['a', 'b']), report('box', []), report('new', [], false)]
  assert.deepEqual(SYNC_FILTERS.map(f => f.id), ['nowhere', 'missing', 'not-set', 'pending'])
  assert.deepEqual(SYNC_FILTERS.map(f => f.needsSnapshot), [false, false, true, true])
  // a: installed on laptop, set there, missing on box, not pending
  assert.equal(matchesSyncFilter('nowhere', 'a', snap, reports), false)
  assert.equal(matchesSyncFilter('missing', 'a', snap, reports), true)
  assert.equal(matchesSyncFilter('not-set', 'a', snap, reports), false)
  assert.equal(matchesSyncFilter('pending', 'a', snap, reports), false)
  // b: installed on laptop, in no selection
  assert.equal(matchesSyncFilter('not-set', 'b', snap, reports), true)
  assert.equal(matchesSyncFilter('nowhere', 'b', snap, reports), false)
  // c: on no machine, pending on box
  assert.equal(matchesSyncFilter('nowhere', 'c', snap, reports), true)
  assert.equal(matchesSyncFilter('pending', 'c', snap, reports), true)
  // d: unknown everywhere except confirmed absent on laptop and box
  assert.equal(matchesSyncFilter('nowhere', 'd', snap, reports), true)
  assert.equal(matchesSyncFilter('missing', 'd', snap, reports), true)
  assert.equal(matchesSyncFilter('', 'd', snap, reports), true)
})

test('a stale scan never counts as missing, but it does count as not installed', () => {
  const reports = [report('new', [], false)]
  assert.equal(matchesSyncFilter('missing', 'a', snapshot([]), reports), false)
  assert.equal(matchesSyncFilter('nowhere', 'a', snapshot([]), reports), true)
})

test('the settled-jobs key changes only when a job reaches a final state', () => {
  const job = (id, status) => ({ id, status })
  assert.equal(settledJobsKey(undefined), '')
  assert.equal(settledJobsKey({ environments: [], targets: [], jobs: [job('a', 'queued'), job('b', 'running'), job('c', 'awaiting-approval')] }), '')
  assert.equal(settledJobsKey({ environments: [], targets: [], jobs: [job('b', 'done'), job('a', 'failed')] }), 'a:failed|b:done')
})


test('a written skill reads its saved plan first, then falls back to complete scans', () => {
  const id = 'hosted/reet.sh/sam/review'
  assert.equal(writtenSkillWhere(id, undefined, []), 'Not on a machine yet')
  assert.equal(writtenSkillWhere(id, undefined, [report('laptop', [])]), 'Missing on 1 machine')
  assert.equal(writtenSkillWhere(id, undefined, [report('laptop', [id]), report('box', [])]), 'On 1 machine · 1 missing')
  assert.equal(writtenSkillWhere(id, snapshot([target('laptop', 'agents', [sk(id)], [sk(id)])]), [report('laptop', [])]), 'Pending on laptop')
  assert.equal(writtenSkillWhere(id, snapshot([target('laptop', 'agents', [sk(id)])]), []), 'Set to sync to laptop')
})

test('a skill selected in the library but held back is never called not set to sync', () => {
  const heldTarget = { ...target('laptop', 'claude', []), held: [{ itemId: 'i1', id: 'a', name: 'a', reason: 'content' }] }
  const intent = skillSyncIntent('a', snapshot([heldTarget]))
  assert.deepEqual(intent, { set: [], pending: [], held: ['laptop'], heldReason: 'content' })
  assert.equal(skillSyncSummary(intent), 'Selected for laptop · waiting for a saved copy in reet')
  assert.equal(matchesSyncFilter('not-set', 'a', snapshot([heldTarget]), []), false)
  // Installed from one folder of the same machine: that machine is not held.
  const both = snapshot([heldTarget, target('laptop', 'agents', [sk('a')])])
  assert.deepEqual(skillSyncIntent('a', both), { set: ['laptop'], pending: [], held: [] })
})
