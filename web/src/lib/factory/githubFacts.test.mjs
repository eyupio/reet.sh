import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import ts from 'typescript'

const insight = ts.transpileModule(readFileSync(new URL('../sessionInsights.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const source = ts.transpileModule(readFileSync(new URL('./githubFacts.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const data = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const facts = await import(data(source.replace(/(['"])\.\.\/sessionInsights\1/, JSON.stringify(data(insight)))))

const check = (name, state, extra = {}) => ({ name, state, ...extra })
const at = s => Date.parse(s)

test('CI counts group the checks by what their state means', () => {
  const { failed, running, passed } = facts.ciCounts([
    check('unit', 'SUCCESS'), check('e2e', 'FAILURE'), check('lint', 'IN_PROGRESS'), check('docs', 'SKIPPED'),
  ])
  assert.deepEqual(failed.map(c => c.name), ['e2e'])
  assert.deepEqual(running.map(c => c.name), ['lint'])
  assert.deepEqual(passed.map(c => c.name), ['unit', 'docs'])
})

test('the CI line names only what is there, and admits no checks', () => {
  assert.equal(facts.ciWord([]), 'No checks reported')
  assert.equal(facts.ciWord([check('a', 'SUCCESS'), check('b', 'SUCCESS')]), '2 passed')
  assert.equal(facts.ciWord([check('a', 'FAILURE'), check('b', 'IN_PROGRESS'), check('c', 'SUCCESS')]), '1 failed · 1 running · 1 passed')
})

test('the CI run is the span from the first start to the last finish', () => {
  const checks = [
    check('unit', 'SUCCESS', { startedAt: at('2026-11-03T10:00:00Z'), completedAt: at('2026-11-03T10:02:00Z') }),
    check('e2e', 'SUCCESS', { startedAt: at('2026-11-03T10:01:00Z'), completedAt: at('2026-11-03T10:04:30Z') }),
  ]
  assert.deepEqual(facts.ciRun(checks, at('2026-11-03T11:00:00Z')), { text: 'Ran 4m 30s', running: false, tone: 'good' })
})

test('a run still going counts to now, and one with no times is not reported', () => {
  const checks = [
    check('unit', 'SUCCESS', { startedAt: at('2026-11-03T10:00:00Z'), completedAt: at('2026-11-03T10:02:00Z') }),
    check('e2e', 'IN_PROGRESS', { startedAt: at('2026-11-03T10:03:00Z') }),
  ]
  assert.deepEqual(facts.ciRun(checks, at('2026-11-03T10:05:00Z')), { text: 'Running 5m 0s', running: true, tone: 'warn' })
  assert.deepEqual(facts.ciRun([check('unit', 'SUCCESS')], 0), { text: 'Not reported', running: false, tone: 'muted' })
  assert.deepEqual(facts.ciRun([check('unit', 'SUCCESS', { completedAt: at('2026-11-03T10:02:00Z') })], 0), { text: 'Not reported', running: false, tone: 'muted' })
})

test('Gaffer fixes count only its own pipeline and only choices that put a stage right', () => {
  const decisions = [
    { key: 'a', stage: 'pr', choice: 'tell' },
    { key: 'a', stage: 'test', choice: 'restart' },
    { key: 'a', stage: 'code', choice: 'move' },
    { key: 'a', stage: 'code', choice: 'again' },
    { key: 'b', stage: 'pr', choice: 'tell' },
    { key: 'a', stage: 'review', choice: 'tell', failed: true },
  ]
  assert.deepEqual(facts.gafferFixes(decisions, 'a'), { count: 3, stages: ['pr', 'test', 'code'] })
  assert.deepEqual(facts.gafferFixes(decisions, 'b'), { count: 1, stages: ['pr'] })
  assert.deepEqual(facts.gafferFixes(undefined, 'a'), { count: 0, stages: [] })
})
