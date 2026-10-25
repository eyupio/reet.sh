import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
// Compile the pure label helper without a browser or an extra test dependency.
const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const asURL = source => `data:text/javascript;base64,${Buffer.from(source).toString('base64')}`
const launchURL = asURL(compile('./agentLaunch.ts'))
const { eventLabel } = await import(asURL(compile('./events.ts').replace("from './agentLaunch'", `from '${launchURL}'`)))
const event = (action, detail) => ({ id: 1, at: 0, source: 'ui', action, ...(detail ? { detail: JSON.stringify(detail) } : {}) })
test('ownership receipt events read as plain sentences from their metadata', () => {
  assert.equal(eventLabel(event('skill-content-adopt', { name: 'demo', files: 3, receiptId: 'a'.repeat(48) })), 'Recorded ownership of 3 files of demo')
  assert.equal(eventLabel(event('skill-content-release', { name: 'demo', receiptId: 'a'.repeat(48) })), 'Released ownership of demo')
})
test('one adopted file reads as "1 file", and a release without a name still reads', () => {
  assert.equal(eventLabel(event('skill-content-adopt', { name: 'demo', files: 1, receiptId: 'a'.repeat(48) })), 'Recorded ownership of 1 file of demo')
  assert.equal(eventLabel(event('skill-content-release', { receiptId: 'a'.repeat(48) })), 'Released ownership of a skill')
  assert.equal(eventLabel(event('skill-content-release')), 'Released ownership of a skill')
})
test('other activity labels are unchanged', () => {
  assert.equal(eventLabel(event('add', { name: 'demo' })), 'Added demo')
  assert.equal(eventLabel(event('merge-machines', { from: 'old', to: 'new' })), 'Consolidated machine old as new')
  assert.equal(eventLabel(event('frobnicate', { name: 'demo' })), 'frobnicate demo')
})
test('a terminal launch reads as which agent started where, never the prompt', () => {
  assert.equal(eventLabel(event('terminal-launch', { machine: 'agent-1', host: 'studio', tool: 'claude', model: '', dir: '/home/me/work/site' })), 'Started Claude Code in /home/me/work/site on studio')
  assert.equal(eventLabel(event('terminal-launch', { machine: 'agent-1', host: 'studio', tool: 'gemini', model: 'gemini-2.5-pro', dir: '' })), 'Started Gemini CLI with gemini-2.5-pro in the home folder on studio')
  assert.equal(eventLabel(event('terminal-launch')), 'Started an agent in the home folder on a machine')
})
