import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
const source = readFileSync(new URL('./catalogue.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { CATALOGUE, agentChoices } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
const machine = (tools, v2 = true) => ({ agentId: 'a', host: 'studio', ready: true, capabilities: { launch: true, agentsV2: v2, tools } })
test('installed first, then not installed, then disabled', () => {
  const choices = agentChoices(machine(['codex']))
  assert.deepEqual(choices.filter(c => c.available).map(c => c.agent.id), ['codex'])
  assert.equal(choices.find(c => c.agent.id === 'claude').reason, 'Not installed on studio')
  assert.equal(new Set(CATALOGUE.map(a => a.id)).size, CATALOGUE.length)
  assert.deepEqual(CATALOGUE.filter(a => a.enabled).map(a => a.id), ['claude', 'codex', 'opencode', 'gemini', 'cline'])
})
test('old machine offers only four', () => {
  const choices = agentChoices(machine(['codex', 'pi'], false))
  assert.deepEqual(choices.filter(c => c.available).map(c => c.agent.id), ['codex'])
  assert.equal(choices.find(c => c.agent.id === 'pi').reason, "reet doesn't start this agent yet.")
})
test('no capability or launch never claims an available agent', () => {
  assert.ok(agentChoices({ agentId:'a', host:'studio', ready:false }).every(c => !c.available))
  assert.ok(agentChoices({ ...machine(['codex']), capabilities: { agentsV2: true, tools:['codex'] } }).every(c => !c.available))
})

test('a future enabled preset still requires agents-v2 on the machine', () => {
  const pi = CATALOGUE.find(a => a.id === 'pi')
  const original = pi.enabled
  try {
    pi.enabled = true
    assert.equal(agentChoices(machine(['pi'], false)).find(c => c.agent.id === 'pi').reason, 'Update reet on that machine')
    assert.equal(agentChoices(machine(['pi'], true)).find(c => c.agent.id === 'pi').available, true)
  } finally { pi.enabled = original }
})

test('Cline effort is offered only after confirmed CLI help supports it', () => {
  const choice = agentChoices(machine(['cline'])).find(c => c.agent.id === 'cline')
  assert.equal(choice.available, true)
  assert.equal(choice.agent.effort, true)
  assert.equal(agentChoices(machine(['cline'], false)).find(c => c.agent.id === 'cline').reason, 'Update reet on that machine')
})
