import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compiled = ts.transpileModule(readFileSync(new URL('./crewChoices.ts', import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const c = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)

const row = (host, state, ids, over = {}) => ({
  agentId: host, host, state, models: ids.map(id => ({ id, provider: id.includes('/') ? id.slice(0, id.indexOf('/')) : '' })),
  more: false, source: 'list', version: '1', fetchedAt: 10, checkedAt: 20, reason: '', sentence: '', ...over,
})

test('the models of every machine merge into one list, each model once with its provider', () => {
  const merged = c.mergeMachineModels('opencode', [
    row('studio', 'listed', ['acme/one', 'acme/two', 'local']),
    row('laptop', 'failed', ['acme/two', 'other/three'], { more: true, fetchedAt: 5 }),
  ])
  assert.equal(merged.tool, 'opencode')
  assert.equal(merged.state, 'listed')
  assert.deepEqual(merged.models, [
    { id: 'acme/one', provider: 'acme' }, { id: 'acme/two', provider: 'acme' }, { id: 'local', provider: '' }, { id: 'other/three', provider: 'other' },
  ])
  assert.equal(merged.more, true)
  // A machine whose last read failed still lends the list it last read; with none listed, the first machine's state stands.
  assert.equal(c.mergeMachineModels('codex', [row('laptop', 'failed', ['x'])]).state, 'failed')
  // A provider the row leaves out is read from the name, as a single machine's list is.
  assert.deepEqual(c.mergeMachineModels('opencode', [{ ...row('a', 'listed', []), models: [{ id: 'acme/four' }] }]).models, [{ id: 'acme/four', provider: 'acme' }])
  // No machine has read this agent's models: nothing to merge.
  assert.equal(c.mergeMachineModels('claude', []), null)
  assert.equal(c.mergeMachineModels('claude', [{ ...row('a', 'off', []), models: null }]).models.length, 0)
})

test('the efforts offered are every machine’s levels for the agent, low to high first, then the rest as reported', () => {
  assert.deepEqual(c.effortUnion([['', 'high', 'low'], ['max', 'medium', 'low'], ['', 'minimal', 'max']]), ['low', 'medium', 'high', 'max', 'minimal'])
  assert.deepEqual(c.effortUnion([['default', ''], []]), [])
  assert.deepEqual(c.effortUnion([]), [])
})

test('an effort reads as a word with a capital, and an agent no machine runs says so', () => {
  assert.equal(c.effortLabel('high'), 'High')
  assert.equal(c.effortLabel('xhigh'), 'Xhigh')
  assert.equal(c.effortLabel(''), '')
  assert.equal(c.agentOptionLabel('OpenCode', true), 'OpenCode')
  assert.equal(c.agentOptionLabel('OpenCode', false), 'OpenCode (not on your machines)')
  // Machines not read: nothing is claimed either way.
  assert.equal(c.agentOptionLabel('OpenCode', null), 'OpenCode')
})

test('a tier’s effort choices come from the machines, keep a saved level they do not offer, and say when there is none', () => {
  assert.deepEqual(c.crewEffortChoices({ offered: ['low', 'high'], saved: '' }), {
    options: [{ value: '', label: 'Default effort' }, { value: 'low', label: 'Low' }, { value: 'high', label: 'High' }], disabled: false, unoffered: false,
  })
  assert.deepEqual(c.crewEffortChoices({ offered: ['low', 'high'], saved: 'high' }).unoffered, false)
  assert.deepEqual(c.crewEffortChoices({ offered: ['low', 'high'], saved: 'medium' }), {
    options: [{ value: '', label: 'Default effort' }, { value: 'low', label: 'Low' }, { value: 'high', label: 'High' }, { value: 'medium', label: 'Medium (not offered by your machines)' }],
    disabled: false, unoffered: true,
  })
  // No machine reports a level for this agent: nothing to choose and nothing saved.
  assert.deepEqual(c.crewEffortChoices({ offered: [], saved: '' }), { options: [{ value: '', label: 'No effort setting' }], disabled: true, unoffered: false })
  // …unless a level was saved before, which stays until it is changed.
  assert.deepEqual(c.crewEffortChoices({ offered: [], saved: 'high' }), {
    options: [{ value: '', label: 'Default effort' }, { value: 'high', label: 'High (not offered by your machines)' }], disabled: false, unoffered: true,
  })
  // The machines could not be read: the saved level and the default, with no claim about either.
  assert.deepEqual(c.crewEffortChoices({ offered: null, saved: 'high' }), {
    options: [{ value: '', label: 'Default effort' }, { value: 'high', label: 'High' }], disabled: false, unoffered: false,
  })
  assert.deepEqual(c.crewEffortChoices({ offered: null, saved: '' }), { options: [{ value: '', label: 'Default effort' }], disabled: false, unoffered: false })
})
