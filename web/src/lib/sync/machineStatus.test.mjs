import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
const compile = ts.transpileModule(readFileSync(new URL('./machineStatus.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext } }).outputText
const { machineSkillStatus } = await import(`data:text/javascript;base64,${Buffer.from(compile).toString('base64')}`)
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
test('reconciling files remains work in progress, not a confirmed install', () => {
  for (const state of ['installing','updating','removing']) assert.equal(machineSkillStatus(machine, health(state)).tone, 'working')
})
