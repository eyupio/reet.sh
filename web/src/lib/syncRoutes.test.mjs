import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const { outputText } = ts.transpileModule(readFileSync(new URL('./syncRoutes.ts', import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
})
const { syncRedirect, advancedSyncHref, ADVANCED_SELECTION } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)

test('the Sync page stays where it is without a plan parameter', () => {
  assert.equal(syncRedirect('/sync'), null)
  assert.equal(syncRedirect('/sync', '', '#sync-jobs'), null)
  // `skill` is read by no Sync page, so it is no reason to leave.
  assert.equal(syncRedirect('/sync', '?skill=a%2Fb'), null)
  assert.equal(syncRedirect('/account/sync'), null)
  assert.equal(syncRedirect('/machines', '?view=compare'), null)
})

test('a plan parameter or the plan anchor goes to the advanced page, with everything it carried', () => {
  assert.equal(syncRedirect('/sync', '?machine=dev-laptop'), '/sync/advanced?machine=dev-laptop')
  assert.equal(syncRedirect('/sync', '?machine=*'), '/sync/advanced?machine=*')
  assert.equal(syncRedirect('/sync', '?machine=dev-laptop&remove=owner%2Frepo%2Fskill'), '/sync/advanced?machine=dev-laptop&remove=owner%2Frepo%2Fskill')
  assert.equal(syncRedirect('/sync', '?list=7'), '/sync/advanced?list=7')
  assert.equal(syncRedirect('/sync', '?machine=a', '#sync-jobs'), '/sync/advanced?machine=a#sync-jobs')
  assert.equal(syncRedirect('/sync', '', '#sync-selection'), '/sync/advanced#sync-selection')
  assert.equal(syncRedirect('/account/sync', '?machine=a'), '/sync/advanced?machine=a')
  assert.equal(syncRedirect('/machines', '?view=compare&machine=a'), '/sync/advanced?machine=a')
})

test('other pages are left alone, and the advanced page is not sent anywhere', () => {
  assert.equal(syncRedirect('/machines', '?machine=a'), null)
  assert.equal(syncRedirect('/skills', '?list=7'), null)
  assert.equal(syncRedirect('/sync/advanced', '?machine=a'), null)
  assert.equal(syncRedirect('/sync/advanced/', '?machine=a'), '/sync/advanced?machine=a')
})

test('links into the plan controls are built in one place', () => {
  assert.equal(advancedSyncHref(), '/sync/advanced')
  assert.equal(advancedSyncHref('dev laptop'), '/sync/advanced?machine=dev+laptop')
  assert.equal(advancedSyncHref('a', 'owner/repo/skill'), '/sync/advanced?machine=a&remove=owner%2Frepo%2Fskill')
  assert.equal(ADVANCED_SELECTION, '/sync/advanced#sync-selection')
  // What the links build, the redirect leaves alone.
  assert.equal(syncRedirect('/sync/advanced', '?machine=a&remove=b'), null)
})
