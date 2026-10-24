import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
const { outputText } = ts.transpileModule(readFileSync(new URL('./fleet.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } })
const { fleetPlan, queueFleet, destinationScopes } = await import(`data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`)
const now = Date.now()
function fixture() {
 return { agents: [{id:'a',host:'laptop',lastSeen:now,paused:false}], jobs:[], environments:['one','two'].map(id=>({id,complete:true,scannedAt:now})), targets:['one','two'].map(id=>({target:{id,host:'LAPTOP',agent:'claude'}})) }
}
test('fleet previews cover distinct scopes while scans, applies and duplicate scopes stay exclusive',()=>{
 const state=fixture()
 assert.equal(fleetPlan(state,'preview',now).operations.length,2)
 state.jobs=[{agentId:'a',targetId:'one',kind:'preview',status:'awaiting-approval'}]
 assert.deepEqual(fleetPlan(state,'preview',now).operations.map(op=>op.targetId),['two'])
 assert.equal(fleetPlan(state,'scan',now).operations.length,0)
 for(const kind of ['scan','apply']) { state.jobs[0].kind=kind; assert.equal(fleetPlan(state,'preview',now).operations.length,0) }
})
test('offline, paused, unenrolled, stale and incomplete targets produce explicit skips',()=>{
 for(const change of [s=>s.agents[0].paused=true,s=>s.agents[0].lastSeen=now-90000,s=>s.agents=[],s=>s.environments.forEach(e=>e.complete=false),s=>s.environments.forEach(e=>e.scannedAt=now-14*86400000)]) {
  const state=fixture();change(state)
  const plan=fleetPlan(state,'preview',now)
  assert.equal(plan.operations.length,0);assert.equal(plan.skipped.length,2)
 }
})
test('bulk queue continues after failure and never reports a failed request as queued or retries it',async()=>{
 const calls=[]
 const result=await queueFleet(fleetPlan(fixture(),'preview',now),async op=>{calls.push(op.targetId);if(op.targetId==='one')throw new Error('conflict')})
 assert.deepEqual(calls,['one','two']);assert.equal(result.queued.length,1);assert.match(result.failed[0],/conflict/)
})
test('all-machine selection preserves distinct scopes and excludes server heartbeats',()=>{
 const machines=[{host:'a',groups:[{id:'a1',agent:'agents'},{id:'a2',agent:'reet-server'}]},{host:'b',groups:[{id:'b1',agent:'claude'}]}]
 assert.deepEqual(destinationScopes(machines,'*').map(g=>g.id),['a1','b1'])
 assert.deepEqual(destinationScopes(machines,'b').map(g=>g.id),['b1'])
 assert.deepEqual(destinationScopes(machines,''),[])
})
