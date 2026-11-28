import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = file => ts.transpileModule(readFileSync(new URL(file, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const url = code => `data:text/javascript;base64,${Buffer.from(code).toString('base64')}`
const o = await import(url(compile('./gafferOutcome.ts')))
const { installPrompt } = await import(url(compile('./install.ts')))

// The canonical example, internal/gaffer/testdata/workorder_example.md, and the order the server reads from it.
const example = readFileSync(new URL('../../../internal/gaffer/testdata/workorder_example.md', import.meta.url), 'utf8')
const brief = 'Add GET /reports/{id}/export.csv … (the brief: self-contained prose, any Markdown,\nup to the first ### heading)'
const order = {
  objective: 'Add CSV export to the reports page', project: 'reports-api',
  crew: { heavy: { tool: 'claude', model: 'opus', effort: 'high' }, standard: { tool: 'codex' } },
  sessions: [{ title: 'Export endpoint', prompt: brief, criteria: ['An export of 10,000 rows finishes in under 5 seconds', 'A reader of another team’s report is refused'], stages: [
    { kind: 'plan' },
    { kind: 'code', tier: 'heavy', part: 'Streaming writer', brief: 'Write rows as they are read, never the whole file in memory.' },
    { kind: 'code', tier: 'heavy', part: 'Endpoint and auth', brief: 'Wire the handler and reuse the report’s read permission.' },
    { kind: 'review', tier: 'standard' }, { kind: 'test' }, { kind: 'pr' },
  ] }],
}

test('a work order is recognised as gaffer.IsWorkOrder recognises one', () => {
  // The five inputs of TestWorkOrderTolerates in internal/gaffer/workorder_test.go.
  const fenced = 'Here is the work order.\n\n```markdown\n' + example + '```\n\nTell me what you think.\n'
  assert.equal(o.isWorkOrder(example), true)
  assert.equal(o.isWorkOrder(example.replaceAll('\n', '\r\n')), true)
  assert.equal(o.isWorkOrder(fenced), true)
  assert.equal(o.isWorkOrder('Add CSV export'), false)
  assert.equal(o.isWorkOrder('Use reet: work-order/1 for this.'), false)
})

test('a work order’s pipelines open as the plan review’s rows', () => {
  const sessions = o.workOrderSessions(order)
  assert.equal(sessions.length, 1)
  const [one] = sessions
  assert.equal(one.title, 'Export endpoint')
  assert.equal(one.prompt, brief)
  assert.deepEqual(one.stages, ['plan', 'code', 'review', 'test', 'pr'])
  assert.deepEqual(one.criteria, order.sessions[0].criteria)
  assert.equal(one.from, 'order')
  const crew = { heavy: { tool: 'claude', model: 'opus' }, standard: { tool: 'codex', model: '' }, light: { tool: 'opencode', model: '' } }
  const sent = o.stagePlan(one.stages, crew, true, one)
  assert.deepEqual(sent.map(s => [s.kind, s.part ?? '', s.tier, s.tool]), [
    ['plan', '', 'heavy', 'claude'],
    ['code', 'Streaming writer', 'heavy', 'claude'],
    ['code', 'Endpoint and auth', 'heavy', 'claude'],
    ['review', '', 'standard', 'codex'],
    ['test', '', 'standard', 'codex'],
    ['pr', '', 'light', 'opencode'],
  ])
  assert.equal(sent[1].brief, 'Write rows as they are read, never the whole file in memory.')
  // A tier the work order set is not Gaffer's reason, and nothing is sent as one.
  assert.ok(sent.every(s => !('tierWhy' in s)))
  // A pipeline that names no stages runs the default ones, as the server does.
  assert.deepEqual(o.workOrderSessions({ objective: 'x', sessions: [{ title: 'A', prompt: 'p' }] })[0].stages, o.DEFAULT_STAGES)
  assert.equal(o.workOrderSessions({ objective: 'x', sessions: [{ title: 'A', prompt: 'p' }] })[0].criteria, undefined)
})

test('a work order’s crew replaces only the tiers it suggests, for this outcome', () => {
  const spare = { tool: 'opencode', model: 'paid/backup' }
  const crew = {
    heavy: { tool: 'opencode', model: 'big', effort: 'max', backup: spare, more: [{ tool: 'codex', model: '' }] },
    standard: { tool: 'opencode', model: '', backup: spare },
    light: { tool: 'opencode', model: 'small', backup: spare, more: [{ tool: 'claude', model: '' }] },
  }
  const out = o.workOrderCrew(crew, order.crew)
  assert.deepEqual(out.from, { heavy: true, standard: true, light: false })
  // Suggested tiers take the agent, model and effort given, and none of the saved backups.
  assert.deepEqual(out.crew.heavy, { tool: 'claude', model: 'opus', effort: 'high' })
  // codex is not on this machine: it is still applied, and Launch readiness says it cannot start.
  assert.deepEqual(out.crew.standard, { tool: 'codex', model: '', effort: '' })
  assert.deepEqual(out.crew.light, crew.light)
  // The crew handed in is not changed.
  assert.equal(crew.heavy.tool, 'opencode')
  assert.deepEqual(o.workOrderCrew(crew, undefined).from, { heavy: false, standard: false, light: false })
})

test('the prompt for an outside agent is the skill’s use line and what the dialog knows', () => {
  const origin = 'https://reet.example.test'
  const use = installPrompt({ sourceType: 'hosted', url: `${origin}/eyupio/factory-contract`, name: 'factory-contract', slug: 'factory-contract' })
  assert.equal(use, 'Run `npx skills@1.7.4 use "https://reet.example.test/eyupio/factory-contract" --skill "factory-contract"` and follow the generated skill instructions now. Read its complete output, redirecting it to a temporary file first if necessary. Resolve relative paths from the supporting-files directory it provides.')
  const prompt = o.outsideAgentPrompt(origin, { project: 'billing', pathways: ['Careful review', 'Quick fix'], savedTiers: ['heavy', 'light'] })
  assert.equal(prompt, `${use}\n\nContext:\n- Project: billing\n- Pathways switched on: Careful review, Quick fix\n- Tiers with a saved crew: heavy, light`)
  assert.equal(o.outsideAgentPrompt(origin, { pathways: [], savedTiers: [] }), `${use}\n\nContext:\n- Project: not chosen yet\n- Pathways switched on: none\n- Tiers with a saved crew: none`)
  // A name is one line: it cannot add lines of its own to the context.
  assert.ok(o.outsideAgentPrompt(origin, { project: 'a\nb', pathways: ['c\n- d'], savedTiers: [] }).endsWith('- Project: a b\n- Pathways switched on: c - d\n- Tiers with a saved crew: none'))
})
