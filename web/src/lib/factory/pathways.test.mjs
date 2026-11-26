import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const compile = file => ts.transpileModule(readFileSync(new URL(file, import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const p = await import(asURL(compile('./pathways.ts').replace("from '../gafferOutcome'", `from '${asURL(compile('../gafferOutcome.ts'))}'`)))

const freeze = v => { if (v && typeof v === 'object') { Object.values(v).forEach(freeze); Object.freeze(v) } return v }
const kinds = stages => stages.map(s => s.kind)
const of = (...list) => list.map(k => p.newStage(k, []))
const skill = (id, bytes = 100) => ({ id, name: id, description: '', bytes })
const pathway = (...list) => ({ ...p.blankPathway(), name: 'Mine', stages: of(...list) })

test('inserting refuses what breaks an order rule and allows what does not', () => {
  const std = p.standardPathway().stages
  assert.match(p.canInsert(std, p.newStage('code', []), 1), /already/)
  assert.equal(p.canInsert(std, p.newStage('review', []), 0) === '', false) // a second review
  assert.equal(p.canInsert(of('code', 'pr', 'merge'), p.newStage('review', []), 0), '')
  assert.equal(p.canInsert(of('code', 'pr', 'merge'), p.newStage('custom', of('code')), 1), '')
  assert.equal(p.canInsert(of('review', 'pr', 'merge'), p.newStage('code', []), 0), '')
  assert.equal(p.canInsert(of('code', 'pr', 'merge'), p.newStage('custom', []), 2), '')
  assert.equal(p.canInsert(of('code', 'pr', 'merge'), p.newStage('test', []), 2), '')
  assert.equal(p.canInsert(of('code', 'pr', 'merge'), p.newStage('test', []), 3), 'Merge is always the last stage.')
  assert.equal(p.canInsert(of('code', 'pr'), p.newStage('merge', []), 1), 'Merge is always the last stage.')
  assert.match(p.canInsert(of('code'), p.newStage('merge', []), 1), /PR stage before/)
  assert.match(p.canInsert(of('code', 'pr'), p.newStage('babysit', []), 0), /PR stage before/)
  assert.equal(p.canInsert(of('code', 'pr'), p.newStage('babysit', []), 2), '')
  const twelve = Array.from({ length: 12 }, (_, i) => p.newStage('custom', Array.from({ length: i }, (_, j) => ({ kind: 'custom', name: `Stage ${j + 1}` }))))
  assert.equal(p.canInsert(twelve, p.newStage('code', []), 0), 'A pathway can have up to 12 stages.')
})

test('moving refuses a pr after babysit and anything past merge', () => {
  const s = of('code', 'pr', 'babysit', 'merge')
  assert.match(p.canMove(s, 1, 2), /PR stage before/)
  assert.equal(p.canMove(s, 0, 3), 'Merge is always the last stage.')
  assert.equal(p.canMove(s, 0, 1), '')
  assert.equal(p.canMove(s, 3, 0), 'Merge is always the last stage.')
  assert.equal(p.canMove(s, 1, 1), '')
})

test('a custom stage is named Stage 1, then Stage 2, taking the first free number', () => {
  const one = p.newStage('custom', [])
  assert.deepEqual(one, { kind: 'custom', name: 'Stage 1', prompt: '' })
  assert.equal(p.newStage('custom', [one]).name, 'Stage 2')
  assert.equal(p.newStage('custom', [{ kind: 'custom', name: 'stage 1' }, { kind: 'custom', name: 'Stage 3' }]).name, 'Stage 2')
  assert.deepEqual(p.newStage('review', []), { kind: 'review', tier: 'heavy' })
})

test('edits return a new pathway and never change their input', () => {
  const base = freeze({ ...pathway('code', 'pr', 'merge'), id: 'p1', updatedAt: 5 })
  const sk = skill('a')
  assert.deepEqual(kinds(p.insertStage(base, p.newStage('review', []), 0).stages), ['review', 'code', 'pr', 'merge'])
  assert.deepEqual(kinds(p.moveStage(base, 0, 1).stages), ['pr', 'code', 'merge'])
  assert.deepEqual(kinds(p.removeStage(base, 1).stages), ['code', 'merge'])
  assert.equal(p.patchStage(base, 0, { tier: 'light' }).stages[0].tier, 'light')
  assert.deepEqual(p.addSkill(base, 0, sk.id).stages[0].skills, ['a'])
  assert.equal(p.removeSkill(p.addSkill(base, 0, 'a'), 0, 'a').stages[0].skills, undefined)
  assert.equal(base.stages.length, 3)
})

test('setting a tier clears the agent and setting an agent clears the tier', () => {
  const base = freeze(pathway('code'))
  const withAgent = p.patchStage(base, 0, { agent: { tool: 'claude', model: 'opus', effort: 'high' } })
  assert.equal(withAgent.stages[0].tier, undefined)
  assert.equal(withAgent.stages[0].agent.model, 'opus')
  const back = p.patchStage(withAgent, 0, { tier: 'heavy' })
  assert.equal(back.stages[0].agent, undefined)
  assert.equal(back.stages[0].tier, 'heavy')
  assert.equal(p.stageWho(back.stages[0]), 'heavy · crew')
  assert.equal(p.stageWho(withAgent.stages[0]), 'opus · high')
  assert.equal(p.stageWho({ kind: 'code', agent: { tool: 'claude' } }), 'claude')
  assert.equal(p.stageTitle({ kind: 'custom', name: 'Audit' }), 'Audit')
  assert.equal(p.stageTitle({ kind: 'pr' }), 'PR')
})

test('a skill is refused when it is a third, a repeat, or takes the pair over 6,000 bytes', () => {
  const shelf = [skill('a', 900), skill('b', 900), skill('c', 100), skill('big', 5200)]
  const stage = { kind: 'code', skills: ['a'] }
  assert.equal(p.canAddSkill({ kind: 'code' }, shelf[0], shelf), '')
  assert.equal(p.canAddSkill(stage, shelf[1], shelf), '')
  assert.equal(p.canAddSkill({ kind: 'code', skills: ['a', 'b'] }, shelf[2], shelf), 'A stage can have up to 2 skills.')
  assert.equal(p.canAddSkill(stage, shelf[0], shelf), 'That skill is already on this stage.')
  assert.match(p.canAddSkill(stage, shelf[3], shelf), /6,000 bytes/)
})

test('every rule the server applies is reported with where it is', () => {
  const shelf = [skill('a', 900), skill('b', 900), skill('big', 5200)]
  const stagesOf = (...s) => ({ ...pathway(), stages: s })
  const at = (path, field) => p.pathwayProblems(path, shelf).stages.filter(s => s.field === field)
  assert.deepEqual(p.pathwayProblems(p.standardPathway(), shelf), { pathway: [], stages: [] })
  assert.deepEqual(p.pathwayProblems(p.standardPathway(), null), { pathway: [], stages: [] })
  assert.deepEqual(p.pathwayProblems(p.blankPathway(), shelf).pathway, ['Give the pathway a name.', 'Add at least one stage.'])
  assert.deepEqual(p.pathwayProblems({ ...p.standardPathway(), name: 'x'.repeat(61) }, shelf).pathway, ['Keep the name to 60 characters.'])
  assert.deepEqual(p.pathwayProblems({ ...p.standardPathway(), description: 'é'.repeat(151) }, shelf).pathway, ['Keep the description to 300 bytes.'])
  assert.equal(p.pathwayProblems({ ...p.standardPathway(), description: 'é'.repeat(150) }, shelf).pathway.length, 0)

  assert.match(at(stagesOf({ kind: 'custom', name: 'Audit', prompt: '  ' }), 'prompt')[0].text, /needs a prompt/)
  assert.match(at(stagesOf({ kind: 'custom', name: 'Merge', prompt: 'x' }), 'name')[0].text, /built-in/)
  assert.match(at(stagesOf({ kind: 'custom', name: ' merge ', prompt: 'x' }), 'name')[0].text, /built-in/)
  assert.match(at(stagesOf({ kind: 'custom', name: 'Custom', prompt: 'x' }), 'name')[0].text, /built-in/)
  assert.match(at(stagesOf({ kind: 'custom', name: '', prompt: 'x' }), 'name')[0].text, /name/)
  assert.match(at(stagesOf({ kind: 'custom', name: 'x'.repeat(41), prompt: 'x' }), 'name')[0].text, /40/)
  const dup = at(stagesOf({ kind: 'custom', name: 'Audit', prompt: 'x' }, { kind: 'custom', name: 'audit', prompt: 'x' }), 'name')
  assert.deepEqual(dup.map(d => d.at), [1])
  // 2,000 é is 4,000 bytes (fine), one more is 4,002 (too long), though only 2,001 characters.
  assert.equal(at(stagesOf({ kind: 'custom', name: 'A', prompt: 'é'.repeat(2000) }), 'prompt').length, 0)
  assert.match(at(stagesOf({ kind: 'custom', name: 'A', prompt: 'é'.repeat(2001) }), 'prompt')[0].text, /4,000 bytes/)
  assert.match(at(stagesOf({ kind: 'code', prompt: 'x'.repeat(4001) }), 'prompt')[0].text, /4,000 bytes/)

  const order = p.pathwayProblems({ ...pathway(), stages: of('merge', 'code') }, shelf).stages.filter(s => s.field === 'order')
  assert.deepEqual(order.map(o => [o.at, o.text]), [[0, 'Merge is always the last stage.']])
  assert.match(at(stagesOf(...of('code', 'code')), 'order')[0].text, /only once|already/)
  assert.match(p.pathwayProblems({ ...pathway(), stages: of('pr', 'merge', 'custom') }, shelf).stages.find(s => s.field === 'order').text, /last/)
  assert.match(p.pathwayProblems({ ...pathway(), stages: of('code', 'merge') }, shelf).stages.find(s => s.field === 'order').text, /PR stage before/)

  assert.match(at(stagesOf({ kind: 'code', skills: ['gone'] }), 'skills')[0].text, /no longer/)
  assert.equal(p.pathwayProblems(stagesOf({ kind: 'code', skills: ['gone'] }), null).stages.length, 0)
  assert.match(at(stagesOf({ kind: 'code', skills: ['a', 'b', 'big'] }), 'skills')[0].text, /2 skills/)
  assert.match(at(stagesOf({ kind: 'code', skills: ['a', 'big'] }), 'skills')[0].text, /6,000 bytes/)
  assert.match(at(stagesOf({ kind: 'code', skills: ['a', 'a'] }), 'skills')[0].text, /once/)
  assert.match(at(stagesOf({ kind: 'code', agent: { tool: '' } }), 'who')[0].text, /agent/)

  const many = { ...pathway(), stages: Array.from({ length: 13 }, (_, i) => ({ kind: 'custom', name: `S${i}`, prompt: 'x' })) }
  assert.deepEqual(p.pathwayProblems(many, shelf).pathway, ['A pathway can have up to 12 stages.'])
})

test('byteLength counts UTF-8 bytes and sameEdit ignores updatedAt and problems', () => {
  assert.equal(p.byteLength('é'), 2)
  const a = { ...p.standardPathway(), id: 'x', updatedAt: 1, problems: ['!'] }
  assert.equal(p.sameEdit(a, { ...a, updatedAt: 2, problems: [] }), true)
  assert.equal(p.sameEdit(a, { ...a, name: 'Other' }), false)
  assert.equal(p.sameEdit(a, p.patchStage(a, 0, { tier: 'light' })), false)
})
