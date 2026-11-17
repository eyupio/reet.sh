import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = path => ts.transpileModule(readFileSync(new URL(path, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const dataUrl = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
// The list model imports the outcome model, so that import is pointed at the compiled copy.
const outcomeUrl = dataUrl(compile('../gafferOutcome.ts'))
const f = await import(dataUrl(compile('./factoryList.ts').replace(/(['"])\.\.\/gafferOutcome\1/, JSON.stringify(outcomeUrl))))

const KINDS = ['plan', 'design', 'code', 'review', 'test', 'pr', 'babysit', 'merge']
const stages = (n, at) => KINDS.slice(0, n).map((kind, i) => ({ kind, tool: 'claude', state: i < at ? 'done' : 'pending' }))
const piece = (key, state, extra = {}) => ({ key, title: `T ${key}`, state, conversationId: `c-${key}`, sessionId: `s-${key}`, sessionState: 'running', stage: 0, stages: stages(3, 0), updatedAt: 1000, note: '', ...extra })
const goal = (extra = {}, attention = false) => ({ attention, reason: 'Gaffer brought this back to you.', goal: { state: 'active', note: '', updatedAt: 500, ...extra } })

test('each pipeline is in exactly one section', () => {
  const record = { id: 'o1', objective: 'Ship it', projectId: 'p1', sessions: [
    piece('a', 'following'), piece('b', 'ready_to_merge'), piece('c', 'following'), piece('d', 'waiting'), piece('e', 'done'), piece('f', 'cancelled'), piece('g', 'failed', { note: 'No machine' }),
  ] }
  const goals = new Map([['c-a', goal({}, true)]])
  const items = f.factoryItems([record], goals)
  assert.deepEqual(items.map(i => i.section), ['waiting', 'waiting', 'working', 'working', 'finished', 'finished', 'waiting'])
  assert.deepEqual(items.map(i => i.kind), ['decision', 'merge', 'running', 'queued', 'done', 'halted', 'failed'])
  assert.deepEqual(items.map(i => i.chip), ['Decision', 'Ready to merge', 'Running', 'Waiting its turn', 'Finished', 'Stopped', 'Failed'])
  assert.equal(items[0].id, 'o1.a')
  assert.equal(items[0].index, 1)
  assert.equal(items[6].of, 7)
  assert.ok(items[0].need && items[1].need)
  assert.equal(items[6].need, undefined)
  assert.equal(items[6].why, 'No machine')
  assert.equal(items[0].since, 500)
  assert.equal(items[1].since, 1000)
})

test('a lost session is an agent stopped item', () => {
  const record = { id: 'o1', objective: 'x', projectId: 'p', sessions: [piece('a', 'following', { sessionState: 'ended' })] }
  const [item] = f.factoryItems([record], new Map())
  assert.equal(item.kind, 'stopped')
  assert.equal(item.chip, 'Agent stopped')
  assert.equal(item.section, 'waiting')
})

test('a goal waiting on the agent is an approval only when its session holds a permission', () => {
  const record = { id: 'o1', objective: 'x', projectId: 'p', sessions: [piece('a', 'following')] }
  const goals = new Map([['c-a', goal({ waitingOnWorker: true }, true)]])
  const [yes] = f.factoryItems([record], goals, new Set(['s-a']))
  assert.equal(yes.kind, 'approval')
  assert.equal(yes.chip, 'Approval')
  const [no] = f.factoryItems([record], goals)
  assert.equal(no.kind, 'decision')
  const [other] = f.factoryItems([record], goals, new Set(['s-other']))
  assert.equal(other.kind, 'decision')
})

test('waiting items come oldest first', () => {
  const sessions = [piece('a', 'ready_to_merge', { updatedAt: 300 }), piece('b', 'ready_to_merge', { updatedAt: 100 }), piece('c', 'ready_to_merge', { updatedAt: 200 }), piece('w', 'following', { conversationId: 'cw' })]
  const items = f.factoryItems([{ id: 'o', objective: 'x', projectId: 'p', sessions }], new Map())
  assert.deepEqual(f.inSection(items, 'waiting').map(i => i.key), ['b', 'c', 'a'])
  assert.deepEqual(f.inSection(items, 'working').map(i => i.key), ['w'])
})

test('the other sections keep record order', () => {
  const sessions = [piece('z', 'following', { updatedAt: 9 }), piece('y', 'following', { updatedAt: 1 })]
  const items = f.factoryItems([{ id: 'o', objective: 'x', projectId: 'p', sessions }], new Map())
  assert.deepEqual(f.inSection(items, 'working').map(i => i.key), ['z', 'y'])
})

test('a pipeline with no stages has at -1 and an empty caption', () => {
  const [item] = f.factoryItems([{ id: 'o', objective: 'x', projectId: 'p', sessions: [piece('a', 'following', { stages: undefined })] }], new Map())
  assert.equal(item.at, -1)
  assert.deepEqual(item.nodes, [])
  assert.equal(f.stageCaption(item.nodes, item.at, 'dots'), '')
  assert.equal(f.stageCaption(item.nodes, item.at, 'circles'), '')
})

test('at is clamped to the last stage', () => {
  const [item] = f.factoryItems([{ id: 'o', objective: 'x', projectId: 'p', sessions: [piece('a', 'following', { stage: 9 })] }], new Map())
  assert.equal(item.at, 2)
})

test('the caption names the stage in hand and the next', () => {
  const nodes = (n, at) => f.factoryItems([{ id: 'o', objective: 'x', projectId: 'p', sessions: [piece('a', 'following', { stages: stages(n, at), stage: at })] }], new Map())[0].nodes
  assert.equal(f.stageCaption(nodes(8, 2), 2, 'dots'), 'Code · 3 of 8 · next Review')
  assert.equal(f.stageCaption(nodes(8, 2), 2, 'circles'), 'Code in hand · Plan, Design done · next Review')
  assert.equal(f.stageCaption(nodes(8, 7), 7, 'dots'), 'Merge · 8 of 8 · the last')
  const six = ['code', 'review', 'test', 'pr', 'babysit', 'merge'].map(kind => ({ kind, tool: 'claude', state: 'pending' }))
  const first = f.factoryItems([{ id: 'o', objective: 'x', projectId: 'p', sessions: [piece('a', 'following', { stages: six })] }], new Map())[0]
  assert.equal(f.stageCaption(first.nodes, 0, 'circles'), 'Code in hand · the first stage · next Review')
})

test('a closed section still says what is in it', () => {
  const at = (n, extra) => Array.from({ length: n }, (_, i) => piece(String.fromCharCode(97 + i), 'following', { title: String.fromCharCode(65 + i), ...extra }))
  const make = sessions => f.factoryItems([{ id: 'o', objective: 'x', projectId: 'p', sessions }], new Map())
  assert.equal(f.sectionSummary(make(at(3)), 'working', 0), 'A, B, C')
  assert.equal(f.sectionSummary(make(at(5)), 'working', 0), 'A, B, C and 2 more')
  const now = 10 * 3600000
  assert.equal(f.sectionSummary(make([piece('a', 'ready_to_merge', { updatedAt: now - 3600000 }), piece('b', 'ready_to_merge', { updatedAt: now - 60000 })]), 'waiting', now), 'oldest 1 h')
  assert.equal(f.sectionSummary([], 'working', 0), '')
  assert.equal(f.sectionSummary(make(at(2)), 'finished', 0), '')
})
