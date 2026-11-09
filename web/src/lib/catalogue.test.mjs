import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
const source = readFileSync(new URL('./catalogue.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { CATALOGUE, agentChoices, agentDisplayName } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
const machine = (tools, v2 = true) => ({ agentId: 'a', host: 'studio', ready: true, capabilities: { launch: true, agentsV2: v2, tools } })
test('installed first, then not installed, then disabled', () => {
  const choices = agentChoices(machine(['codex']))
  assert.deepEqual(choices.filter(c => c.available).map(c => c.agent.id), ['codex'])
  assert.equal(choices.find(c => c.agent.id === 'claude').reason, 'Not installed on studio')
  assert.equal(new Set(CATALOGUE.map(a => a.id)).size, CATALOGUE.length)
  assert.deepEqual(CATALOGUE.filter(a => a.enabled).map(a => a.id), ['claude', 'codex', 'opencode', 'cline', 'antigravity'])
})
test('old machine offers only three', () => {
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

test('Google Antigravity requires installation and agents-v2', () => {
  const choice = agentChoices(machine(['antigravity'])).find(c => c.agent.id === 'antigravity')
  assert.equal(choice?.available, true)
  assert.equal(choice.agent.name, 'Google Antigravity (AGY)')
  assert.equal(choice.agent.effort, true)
  assert.equal(choice.agent.yolo, true)
  assert.equal(agentChoices(machine([])).find(c => c.agent.id === 'antigravity').reason, 'Not installed on studio')
  assert.equal(agentChoices(machine(['antigravity'], false)).find(c => c.agent.id === 'antigravity').reason, 'Update reet on that machine')
})
test('Claude Code offers thinking effort, resume and YOLO because its captured help supports them', () => {
  const choice = agentChoices(machine(['claude'])).find(c => c.agent.id === 'claude')
  assert.equal(choice.available, true)
  assert.equal(choice.agent.effort, true)
  assert.equal(choice.agent.resume, true)
  assert.equal(choice.agent.yolo, true)
})
test('a retired agent is never offered, even when a machine still reports it', () => {
  assert.ok(!CATALOGUE.some(a => a.id === 'gemini'))
  const choices = agentChoices(machine(['claude', 'gemini']))
  assert.ok(!choices.some(c => c.agent.id === 'gemini'))
  assert.deepEqual(choices.filter(c => c.available).map(c => c.agent.id), ['claude'])
})
test('an older machine without agents-v2 offers the three original agents', () => {
  const choices = agentChoices(machine(['claude', 'codex', 'opencode', 'gemini'], false))
  assert.deepEqual(choices.filter(c => c.available).map(c => c.agent.id), ['claude', 'codex', 'opencode'])
})
test('past sessions keep a retired agent\'s name', () => {
  assert.equal(agentDisplayName('gemini'), 'Gemini CLI')
  assert.equal(agentDisplayName('claude'), 'Claude Code')
  assert.equal(agentDisplayName('some-other-agent'), 'some-other-agent')
})
