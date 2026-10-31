import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'
const source = readFileSync(new URL('./sessionBoard.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const { boardItems, sessionStage, filterBoard, sortBoard, emptyFilters } = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
const draft = (id, over = {}) => ({ id, title: 'Improve checkout', prompt: 'Polish the empty state', agentId: 'm1', projectId: 'p1', dir: '/work/shop', tool: 'codex', model: 'gpt-test', effort: '', imageCount: 0, version: 1, createdAt: 1, updatedAt: 2, ...over })
const session = (id, state = 'running', over = {}) => ({ id, title: 'Build search', state, agentId: 'm2', projectId: 'p2', dir: '/work/search', tool: 'claude', model: '', createdAt: 3, updatedAt: 4, ...over })
const names = { machines: new Map([['m1', 'Studio'], ['m2', 'Laptop']]), projects: new Map([['p1', 'Shop'], ['p2', 'Search']]) }

test('stages follow session lifecycle, including quiet and completed chat turns', () => {
  for (const state of ['creating', 'running', 'end_requested']) assert.equal(sessionStage(session('a', state)), 'active')
  for (const state of ['ended', 'exited', 'lost']) assert.equal(sessionStage(session('a', state)), 'finished')
  assert.equal(sessionStage(session('a', 'running', { attention: 'finished', structured: true })), 'active')
})
test('a launched backlog item is represented only by its real session', () => {
  const items = boardItems([draft('a'), draft('b', { sessionId: 'live' })], [session('live')])
  assert.deepEqual(items.map(i => i.key), ['draft:a', 'session:live'])
  assert.equal(items[1].stage, 'active')
})
test('filters combine OR within a group and AND across groups', () => {
  const items = boardItems([draft('a'), draft('b', { agentId: 'm2' }), draft('c', { tool: 'claude' })], [session('live')])
  assert.deepEqual(filterBoard(items, { ...emptyFilters(), machines: ['m1', 'm2'], agents: ['codex'], projects: ['p1'] }, names).map(i => i.key), ['draft:a', 'draft:b'])
})
test('search includes all query terms across prompt, folder, machine, project and model', () => {
  const items = boardItems([draft('a')], [session('live')])
  assert.equal(filterBoard(items, { ...emptyFilters(), query: 'studio empty GPT-test' }, names).length, 1)
  assert.equal(filterBoard(items, { ...emptyFilters(), query: 'studio missing' }, names).length, 0)
})
test('attention filter includes failed exits and lost connections without inventing activity', () => {
  const items = boardItems([], [session('wait', 'running', { attention: 'waiting' }), session('quiet', 'running', { attention: 'quiet' }), session('lost', 'lost'), session('bad', 'exited', { exitCode: 1 }), session('ok', 'exited', { exitCode: 0 })])
  assert.deepEqual(filterBoard(items, { ...emptyFilters(), attention: true }, names).map(i => i.session.id), ['wait', 'lost', 'bad'])
})
test('images, no project and default model filters work together', () => {
  const items = boardItems([draft('a', { imageCount: 2, projectId: '', model: '' }), draft('b')], [])
  assert.deepEqual(filterBoard(items, { ...emptyFilters(), images: true, projects: ['none'], models: ['default'] }, names).map(i => i.key), ['draft:a'])
})
test('sorting never mutates the source and uses stable tie breaks', () => {
  const items = boardItems([draft('z', { createdAt: 1 }), draft('a', { createdAt: 5 })], [session('live')])
  assert.deepEqual(sortBoard(items, 'oldest').map(i => i.key), ['draft:z', 'session:live', 'draft:a'])
  assert.deepEqual(sortBoard(items, 'newest').map(i => i.key), ['session:live', 'draft:a', 'draft:z'])
  assert.equal(items[0].key, 'draft:z')
})
