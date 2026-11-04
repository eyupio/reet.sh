import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
const source=ts.transpileModule(readFileSync(new URL('./sessionInsights.ts',import.meta.url),'utf8'),{compilerOptions:{module:ts.ModuleKind.ESNext,target:ts.ScriptTarget.ES2022}}).outputText
const {checkTone,checkLabel,safeInsightURL,fileWord,chatInsight,elapsedWord}=await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`)
test('unknown, queued and absent checks never read as passing',()=>{
 for(const state of ['','QUEUED','IN_PROGRESS','PENDING','UNKNOWN'])assert.equal(checkTone(state),'warn')
 for(const state of ['FAILURE','ERROR','CANCELLED','TIMED_OUT','ACTION_REQUIRED'])assert.equal(checkTone(state),'bad')
 for(const state of ['SUCCESS','NEUTRAL','SKIPPED'])assert.equal(checkTone(state),'good')
})
test('check labels use words people can scan, not raw provider states',()=>{
 assert.equal(checkLabel('FAILURE'),'Failed');assert.equal(checkLabel('IN_PROGRESS'),'Running')
 assert.equal(checkLabel('SUCCESS'),'Passed');assert.equal(checkLabel('SKIPPED'),'Skipped')
 assert.equal(checkLabel('SOMETHING_NEW'),'something new')
})
test('provider links cannot execute code or carry URL credentials',()=>{
 for(const link of ['javascript:alert(1)','data:text/html,x','https://token@github.com/a','http://github.com/a','/local'])assert.equal(safeInsightURL(link),undefined)
 assert.equal(safeInsightURL('https://github.com/o/r/pull/1'),'https://github.com/o/r/pull/1')
})
test('conflicts cover both-added and both-deleted as well as unmerged states',()=>{
 for(const status of ['UU','AA','DD','AU','DU'])assert.equal(fileWord({status}),'Conflict')
 assert.equal(fileWord({status:'??'}),'Untracked');assert.equal(fileWord({status:' M'}),'Modified')
})
test('chat recap uses actual tool states and the most recent agent reply',()=>{
 const items=[{kind:'agent',text:'Earlier'},{kind:'tool',status:'completed'},{kind:'tool',status:'in_progress'},{kind:'tool',status:'failed'},{kind:'tool',status:'cancelled'},{kind:'agent',text:'Latest'}]
 const recap=chatInsight({items});assert.equal(recap.done.length,1);assert.equal(recap.failed.length,1);assert.equal(recap.active.length,1);assert.equal(recap.latest,'Latest');assert.equal(chatInsight().total,0)
 assert.equal(elapsedWord(undefined,1000),'not recorded');assert.equal(elapsedWord(2000,1000),'just now')
})
