import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = path => ts.transpileModule(readFileSync(new URL(path, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const dataUrl = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
// The list model imports the outcome model, and the merged list imports the list model: both are pointed at compiled copies.
const outcomeUrl = dataUrl(compile('../gafferOutcome.ts'))
const listUrl = dataUrl(compile('../factory/factoryList.ts').replace(/(['"])\.\.\/gafferOutcome\1/, JSON.stringify(outcomeUrl)))
const w = await import(dataUrl(compile('./homeWaiting.ts').replace(/(['"])\.\.\/factory\/factoryList\1/, JSON.stringify(listUrl))))

const HOUR = 3_600_000
const item = (id, kind, section, since, extra = {}) => ({ id, sessionId: '', outcomeId: 'o', key: id, title: `T ${id}`, objective: '', projectId: 'p', index: 1, of: 1, section, kind, chip: kind, why: '', since, nodes: [], at: -1, archived: false, ...extra })
const card = (sessionId, kind, at, extra = {}) => ({ kind, reasons: [], label: '', tone: 'warn', sessionId, agentId: 'a', title: sessionId, where: '', href: '', at, age: '', markSeen: false, worktreeTarget: null, ...extra })

test('a finished session is not waiting, a failed one and one to review are', () => {
  const out = w.homeWaiting([], [card('a', 'finished', 1), card('b', 'failed', 2), card('c', 'review', 3)])
  assert.deepEqual(out.map(e => e.id), ['session:b', 'session:c'])
  assert.equal(out[0].source, 'session')
  assert.equal(out[0].since, 2)
})

test('a pipeline’s own session appears once, as the Factory item, when that item is waiting and shows what the card asks', () => {
  const ask = { permissionId: 'p', title: 't', command: 'ls', path: '', truncated: false, queued: 0 }
  const items = [item('o.a', 'decision', 'waiting', 100, { sessionId: 's1' }), item('o.p', 'approval', 'waiting', 100, { sessionId: 's5' })]
  const out = w.homeWaiting(items, [card('s1', 'waiting', 90), card('s5', 'waiting', 90, { approval: ask, reasons: ['approval'] }), card('s4', 'review', 90)])
  assert.deepEqual(out.map(e => e.id), ['session:s4', 'o.a', 'o.p'])
})

test('a session’s card stays when its pipeline is not waiting, or waits for something else than the card’s approval', () => {
  const ask = { permissionId: 'p', title: 't', command: 'ls', path: '', truncated: false, queued: 0 }
  // A working pipeline whose session holds an approval: the approval is only on the card, so the card stays.
  const working = w.homeWaiting([item('o.b', 'running', 'working', 50, { sessionId: 's2' })], [card('s2', 'waiting', 90, { approval: ask, reasons: ['approval'] })])
  assert.deepEqual(working.map(e => e.id), ['session:s2'])
  // A decision item does not show the approval its session holds: both are listed.
  const decision = w.homeWaiting([item('o.a', 'decision', 'waiting', 100, { sessionId: 's1' })], [card('s1', 'waiting', 90, { approval: ask, reasons: ['approval'] })])
  assert.deepEqual(decision.map(e => e.id), ['session:s1', 'o.a'])
  // A finished or working pipeline's session that failed still asks the person.
  const done = w.homeWaiting([item('o.c', 'done', 'finished', 50, { sessionId: 's3' }), item('o.d', 'running', 'working', 50, { sessionId: 's6' })], [card('s3', 'review', 90), card('s6', 'failed', 80)])
  assert.deepEqual(done.map(e => e.id), ['session:s6', 'session:s3'])
})

test('a Gaffer goal that needs review on a session with no card and no pipeline is an entry of its own', () => {
  const goal = (sessionId, since) => ({ id: `goal-${sessionId}`, sessionId, title: sessionId, where: '', href: `/sessions?session=${sessionId}`, reason: '', since })
  const out = w.homeWaiting([item('o.a', 'running', 'working', 1, { sessionId: 'piped' })], [card('carded', 'failed', 5)], [goal('alone', 3), goal('carded', 4), goal('piped', 2)])
  assert.deepEqual(out.map(e => e.id), ['gaffer:goal-alone', 'session:carded'])
  assert.equal(out[0].source, 'gaffer'); assert.equal(out[0].since, 3)
  assert.equal(w.waitingSummary(out, HOUR + 3), '1 failed, 1 Gaffer goal · oldest 1 h')
})

test('a goal that needs review on a finished session still shows: the finished card is not in the list to say so', () => {
  const goal = (sessionId, since) => ({ id: `goal-${sessionId}`, sessionId, title: sessionId, where: '', href: `/sessions?session=${sessionId}`, reason: 'Review what it did.', since })
  const out = w.homeWaiting([], [card('done', 'finished', 5), card('broke', 'failed', 6)], [goal('done', 7), goal('broke', 8)])
  assert.deepEqual(out.map(e => e.id), ['session:broke', 'gaffer:goal-done'])
})

test('entries come oldest first across both sources', () => {
  const out = w.homeWaiting([item('o.a', 'decision', 'waiting', 100)], [card('x', 'failed', 300), card('y', 'failed', 200)])
  assert.deepEqual(out.map(e => e.since), [100, 200, 300])
  assert.deepEqual(out.map(e => e.source), ['factory', 'session', 'session'])
  const tied = w.homeWaiting([], [card('b', 'failed', 5), card('a', 'failed', 5)])
  assert.deepEqual(tied.map(e => e.id), ['session:a', 'session:b'])
})

test('an archived pipeline is not waiting', () => {
  assert.deepEqual(w.homeWaiting([item('o.a', 'decision', 'waiting', 1, { archived: true })], []), [])
})

test('the summary counts each kind and names the oldest', () => {
  const items = [
    item('o.a', 'decision', 'waiting', 0), item('o.b', 'approval', 'waiting', 10), item('o.c', 'merge', 'waiting', 20),
  ]
  const cards = [
    card('s1', 'waiting', 30, { reasons: ['approval'], approval: { permissionId: 'p', title: 't', command: '', path: '', truncated: false, queued: 0 } }),
    card('s2', 'waiting', 40, { reasons: ['question'], question: { questionId: 'q', message: 'm', truncated: false, queued: 0 } }),
    card('s3', 'failed', 50), card('s4', 'review', 60),
  ]
  // The second approval is a Factory approval, so two in all.
  const now = HOUR
  assert.equal(w.waitingSummary(w.homeWaiting(items, cards), now), '1 decision, 2 approvals, 1 merge, 1 question, 1 failed, 1 to review · oldest 1 h')
  assert.equal(w.waitingSummary([], now), '')
  const stopped = w.homeWaiting([item('o.s', 'stopped', 'waiting', 0), item('o.f', 'failed', 'waiting', 0)], [])
  assert.equal(w.waitingSummary(stopped, 5 * 60000), '1 stopped, 1 failed · oldest 5 min')
})

test('the working summary names the pipelines', () => {
  const items = ['a', 'b', 'c', 'd', 'e'].map(k => item(`o.${k}`, 'running', 'working', 1))
  assert.equal(w.workingSummary(items), 'T o.a, T o.b, T o.c and 2 more')
  assert.equal(w.workingSummary([item('o.z', 'decision', 'waiting', 1)]), '')
})

test('a question the pipeline’s item does not offer to answer keeps the session’s card', () => {
  const question = { questionId: 'q', message: 'Which branch?', truncated: false, queued: 0 }
  const asks = sessionId => card(sessionId, 'waiting', 90, { question, reasons: ['question'] })
  const answer = { why: 'w', choices: [{ id: 'answer', label: 'Open the session to answer', primary: true }] }
  const review = { why: 'w', choices: [{ id: 'again', label: 'Have Gaffer look again', primary: true }, { id: 'open', label: 'Open the session' }] }
  // Ready to merge: the item offers the merge, not the question.
  const merge = w.homeWaiting([item('o.m', 'merge', 'waiting', 100, { sessionId: 's1' })], [asks('s1')])
  assert.deepEqual(merge.map(e => e.id), ['session:s1', 'o.m'])
  // A decision not waiting on the worker does not answer it either.
  const decision = w.homeWaiting([item('o.d', 'decision', 'waiting', 100, { sessionId: 's2', need: review })], [asks('s2')])
  assert.deepEqual(decision.map(e => e.id), ['session:s2', 'o.d'])
  // A decision whose choices include answering it speaks for the card.
  const answered = w.homeWaiting([item('o.a', 'decision', 'waiting', 100, { sessionId: 's3', need: answer })], [asks('s3')])
  assert.deepEqual(answered.map(e => e.id), ['o.a'])
})

test('a plain waiting session is counted as waiting, as its chip says', () => {
  const out = w.homeWaiting([], [card('s1', 'waiting', 0), card('s2', 'waiting', 10, { reasons: ['question'], question: { questionId: 'q', message: 'm', truncated: false, queued: 0 } }), card('s3', 'failed', 20)])
  assert.equal(w.waitingSummary(out, HOUR), '1 question, 1 waiting, 1 failed · oldest 1 h')
})
