import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
const compile = file => ts.transpileModule(readFileSync(new URL(file, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext } }).outputText
const data = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const shared = data(compile('../contentReview.ts'))
const { machineSkillStatus } = await import(data(compile('./machineStatus.ts').replace("from '../contentReview'", `from '${shared}'`)))
const machine = {host:'Laptop'}
const health = (state, extra = {}) => ({active:3, machines:[{host:'laptop', state, waiting:[], failed:[], ...extra}]})
test('inventory alone and failed health reads never claim that files are in sync', () => {
  assert.equal(machineSkillStatus(machine, null).tone, 'attention')
  assert.equal(machineSkillStatus(machine, {active:3, machines:[]}).tone, 'attention')
  assert.equal(machineSkillStatus(machine, health('ok')).tone, 'synced')
})
test('offline, paused and protocol failures stay distinct; pending work is conditional', () => {
  assert.doesNotMatch(machineSkillStatus(machine, health('offline')).detail, /catch up/)
  assert.match(machineSkillStatus(machine, health('offline', {waiting:['skill']})).detail, /catch up/)
  for (const state of ['paused','blocked','needs-update','needs-newer-controller','failed']) assert.equal(machineSkillStatus(machine, health(state)).tone, 'attention')
  assert.match(machineSkillStatus(machine, health('failed', {failed:[{name:'audit',detail:'No write access'}]})).detail, /audit: No write access/)
})
test('a blocked report on a locked machine says it is locked, not that allow-apply is off', () => {
  assert.match(machineSkillStatus(machine, health('blocked')).detail, /Allow local skill writes/)
  assert.equal(machineSkillStatus(machine, health('blocked'), 'Locked on the machine. Run `reet agent unlock` there.').detail, 'Locked on the machine. Run `reet agent unlock` there.')
  assert.equal(machineSkillStatus(machine, health('ok'), 'Locked on the machine. Run `reet agent unlock` there.').tone, 'synced')
})
test('reconciling files remains work in progress, not a confirmed install', () => {
  for (const state of ['installing','updating','removing']) assert.equal(machineSkillStatus(machine, health(state)).tone, 'working')
})

test('a different Unicode hostname cannot supply proof of sync for this machine', () => {
 const reports = {active:1,machines:[{host:'Ë',state:'ok',waiting:[],failed:[]}]}
 assert.equal(machineSkillStatus({host:'ë'},reports).tone,'attention')
 assert.equal(machineSkillStatus({host:'Ë'},reports).tone,'synced')
})
