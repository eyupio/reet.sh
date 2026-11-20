import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
const compile = ts.transpileModule(readFileSync(new URL('./legacyRoutes.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext } }).outputText
const {syncRedirect} = await import(`data:text/javascript;base64,${Buffer.from(compile).toString('base64')}`)
test('old plan, removal and job links reach the current status page with obsolete intent discarded', () => {
  for (const args of [['/sync/advanced'],['/sync/advanced/','?remove=skill','#sync-jobs'],['/account/sync'],['/machines','?view=compare&machine=laptop'],['/sync','?list=7'],['/sync','','#sync-selection']]) assert.equal(syncRedirect(...args), '/sync')
})
test('ordinary machine and sync addresses are left in place', () => {
  assert.equal(syncRedirect('/machines','?machine=laptop'),null)
  assert.equal(syncRedirect('/sync'),null)
})
