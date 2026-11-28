import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = readFileSync(new URL('./gafferOutcome.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const o = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)

// The Go fixture both sides read, so the published example can never drift from either parser.
const example = readFileSync(new URL('../../../internal/gaffer/testdata/workorder_example.md', import.meta.url), 'utf8')

test('isWorkOrder agrees with the Go cases', () => {
  const crlf = example.replace(/\n/g, '\r\n')
  const fenced = `Here is the work order you asked for.\n\n\`\`\`markdown\n${example}\n\`\`\`\n\nHope that helps.\n`
  assert.equal(o.isWorkOrder(example), true)
  assert.equal(o.isWorkOrder(crlf), true)
  assert.equal(o.isWorkOrder(fenced), true)
  assert.equal(o.isWorkOrder('Add CSV export'), false)
  assert.equal(o.isWorkOrder('Please remember that reet: work-order/1 is the format to use.'), false)
})

// The example order, as internal/api/gaffer_workorder.go's check route would hand it back.
const order = {
  objective: 'Add CSV export to the reports page', project: 'reports-api',
  crew: { heavy: { tool: 'claude', model: 'opus', effort: 'high' }, standard: { tool: 'codex' } },
  sessions: [{
    title: 'Export endpoint', prompt: 'Add GET /reports/{id}/export.csv … (the brief: self-contained prose, any Markdown,\nup to the first ### heading)',
    stages: [
      { kind: 'plan' },
      { kind: 'code', part: 'Streaming writer', tier: 'heavy', brief: 'Write rows as they are read, never the whole file in memory.' },
      { kind: 'code', part: 'Endpoint and auth', tier: 'heavy', brief: "Wire the handler and reuse the report's read permission." },
      { kind: 'review', tier: 'standard' },
      { kind: 'test' },
      { kind: 'pr' },
    ],
    criteria: ['An export of 10,000 rows finishes in under 5 seconds', "A reader of another team's report is refused"],
  }],
}

test('workOrderSessions turns the example into one PlanSession the plan review already understands', () => {
  const [session] = o.workOrderSessions(order)
  assert.equal(session.title, 'Export endpoint')
  assert.deepEqual(session.stages, ['plan', 'code', 'review', 'test', 'pr'])
  assert.equal(session.criteria.length, 2)
  const plan = o.stagePlan(session.stages, { heavy: { tool: 'claude', model: '' }, standard: { tool: 'codex', model: '' }, light: { tool: 'claude', model: '' } }, true, session)
  assert.deepEqual(plan.map(stage => stage.kind), ['plan', 'code', 'code', 'review', 'test', 'pr'])
  assert.equal(plan.find(stage => stage.kind === 'review').tier, 'standard')
  assert.equal(plan.filter(stage => stage.kind === 'code').every(stage => stage.tier === 'heavy'), true)
})

const crew = {
  heavy: { tool: 'claude', model: 'sonnet', backup: { tool: 'codex', model: '' } },
  standard: { tool: 'codex', model: '', backup: { tool: 'claude', model: 'haiku' } },
  light: { tool: 'claude', model: '' },
}

test('workOrderCrew replaces only the suggested tiers and drops their backups, keeping the rest', () => {
  const { crew: next, from } = o.workOrderCrew(crew, order.crew, ['claude', 'codex'])
  assert.deepEqual(from, { heavy: true, standard: true, light: false })
  assert.deepEqual(next.heavy, { tool: 'claude', model: 'opus', effort: 'high' })
  assert.equal(next.heavy.backup, undefined)
  assert.deepEqual(next.standard, { tool: 'codex', model: '' })
  // Untouched: still codex's agent, still its backup.
  assert.deepEqual(next.light, crew.light)
})

test('a suggested agent not in tools is still applied; readiness, not this function, reports it', () => {
  const { crew: next, from } = o.workOrderCrew(crew, { heavy: { tool: 'opencode' } }, ['claude', 'codex'])
  assert.equal(next.heavy.tool, 'opencode')
  assert.equal(from.heavy, true)
})

test('outsideAgentPrompt starts with the hosted use sentence, then a Context block naming only what was given', () => {
  const text = o.outsideAgentPrompt('https://example.test', { project: 'billing', pathways: ['Standard delivery'], savedTiers: ['heavy'] })
  assert.match(text, /^Run `npx skills@1\.7\.4 use "https:\/\/example\.test\/eyupio\/reet-integration" --skill "reet-integration"` and follow the generated skill instructions now\./)
  // The Context block names exactly the project, the pathways and the saved tiers given, nothing else about the account.
  const context = text.slice(text.indexOf('Context:'))
  assert.equal(context, 'Context:\nProject: billing\nPathways: Standard delivery\nSaved crew: Heavy')
})
