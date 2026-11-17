import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const source = readFileSync(new URL('./gafferOutcome.ts', import.meta.url), 'utf8')
const compiled = ts.transpileModule(source, { compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 } }).outputText
const o = await import(`data:text/javascript;base64,${Buffer.from(compiled).toString('base64')}`)
const bytes = text => new TextEncoder().encode(text).length
const row = (text, state = 'active', attention = false) => ({ goal: { text, state }, attention })
const plan = (...titles) => titles.map((title, n) => ({ key: `k${n}`, title, prompt: `Do ${title}. Do not merge.` }))

test('a session goal names its outcome, fits the goal limit and keeps merging with the person', () => {
  const goal = o.outcomeGoal('Ship CSV export\n  for billing', 'Export API')
  assert.equal(goal.split('\n')[0], 'Outcome: Ship CSV export for billing')
  assert.match(goal, /This session’s part: Export API\n/)
  assert.match(goal, /Do not merge\.$/)
  const huge = o.outcomeGoal('é'.repeat(4000), 'T'.repeat(500))
  assert.ok(bytes(huge) <= 2000, `goal is ${bytes(huge)} bytes`)
  assert.match(huge, /Do not merge\.$/)
  assert.equal(o.outcomeOf(huge), o.outcomeLine('é'.repeat(4000)))
})

test('only a goal that starts with the outcome line belongs to an outcome', () => {
  assert.equal(o.outcomeOf('Outcome: Ship it\nThis session’s part: A'), 'Ship it')
  assert.equal(o.outcomeOf('Raise the PR. Outcome: no'), '')
  assert.equal(o.outcomeOf('  Outcome: indented'), '')
  assert.equal(o.outcomeOf(''), '')
  assert.equal(o.outcomePart(o.outcomeGoal('Ship it', 'Export API')), 'Export API')
  assert.equal(o.outcomePart('Outcome: Ship it'), '')
  assert.equal(o.outcomePart('Ordinary goal\nThis session’s part: no'), '')
})

test('followed sessions group by outcome with honest counts', () => {
  const a = o.outcomeGoal('Ship export', 'API'), b = o.outcomeGoal('Ship export', 'UI'), c = o.outcomeGoal('Fix login', 'Tokens')
  const groups = o.outcomeGroups([row(a), row('Ordinary goal. Do not merge.'), row(b, 'needs_user', true), row(c, 'completed'), row(o.outcomeGoal('Ship export', 'Docs'), 'paused')])
  assert.deepEqual(groups.map(g => [g.objective, g.rows.length, g.needsReview, g.following, g.finished]), [['Ship export', 3, 1, 1, 0], ['Fix login', 1, 0, 0, 1]])
  assert.equal(o.outcomeProgress(groups[0]), '3 sessions · 1 needs review · 1 following · 1 paused')
  assert.equal(o.outcomeProgress(groups[1]), '1 session · 1 finished')
})

test('the objective says what is missing before a plan is asked for', () => {
  const ready = { objective: 'Ship it', project: true, machineReady: true, agent: true, room: 4 }
  assert.equal(o.objectiveBlocker(ready), '')
  assert.match(o.objectiveBlocker({ ...ready, objective: ' ' }), /Write the outcome/)
  assert.match(o.objectiveBlocker({ ...ready, objective: 'é'.repeat(2001) }), /within 4,000 bytes/)
  assert.match(o.objectiveBlocker({ ...ready, project: false }), /Choose the project/)
  assert.match(o.objectiveBlocker({ ...ready, machineReady: false }), /not connected/)
  assert.match(o.objectiveBlocker({ ...ready, agent: false }), /no agent/)
  assert.match(o.objectiveBlocker({ ...ready, room: 0 }), /already following ten/)
})

test('a plan is approved only when every session can start and be followed', () => {
  assert.equal(o.planBlocker(plan('A', 'B'), 5), '')
  assert.match(o.planBlocker([], 5), /at least one session/)
  assert.match(o.planBlocker(plan('A', 'B', 'C', 'D', 'E', 'F'), 9), /at most 5 sessions/)
  assert.match(o.planBlocker(plan('A', 'B', 'C'), 2), /room to follow 2 more sessions\. Remove 1\./)
  assert.match(o.planBlocker(plan('A'), 0), /already following ten/)
  assert.match(o.planBlocker(plan('A', ' '), 5), /Give session 2 a title/)
  assert.match(o.planBlocker(plan('x'.repeat(81)), 5), /Shorten the title of session 1/)
  assert.match(o.planBlocker(plan('A', ' A '), 5), /both called “A”/)
  assert.match(o.planBlocker([{ key: 'k', title: 'A', prompt: '  ' }], 5), /Write the prompt for “A”/)
  assert.match(o.planBlocker([{ key: 'k', title: 'A', prompt: 'é'.repeat(4001) }], 5), /over 8,000 bytes/)
})

test('the result never claims a session that was not started or not followed', () => {
  const r = (state) => ({ key: state, title: state, state, note: '' })
  assert.equal(o.outcomeSummary([r('following'), r('following')]), 'Reet. Gaffer owns this outcome: 2 started and followed.')
  assert.equal(o.outcomeSummary([r('following'), r('unfollowed'), r('failed'), r('cancelled')]), '1 started and followed, 1 started but not followed yet, 2 not started.')
  assert.equal(o.outcomeSummary([r('failed')]), '1 not started.')
  assert.equal(o.outcomeSummary([]), 'Nothing was started.')
  assert.equal(o.startingProgress([r('following'), r('starting'), r('waiting')]), 'Starting session 2 of 3. The controller carries on if you close this.')
  assert.equal(o.startingProgress([r('following'), r('failed')]), 'Starting session 2 of 2. The controller carries on if you close this.')
})

test('a prompt the launch would refuse is caught before anything starts', () => {
  assert.equal(o.cleanPrompt('  a\r\nb\tc\rd  '), 'a\nb    c\nd')
  assert.equal(o.planBlocker([{ key: 'k', title: 'A', prompt: 'Line one\r\n\tindented' }], 5), '')
  assert.match(o.planBlocker([{ key: 'k', title: 'A', prompt: '- Add the endpoint' }], 5), /with a word, not a dash/)
  assert.match(o.planBlocker([{ key: 'k', title: 'A', prompt: 'Do it\u0000now' }], 5), /cannot be sent/)
  assert.match(o.planBlocker([{ key: 'k', title: 'A\u200Db', prompt: 'Do it' }], 5), /title of session 1/)
})

test('a limit must be a whole number in range', () => {
  assert.equal(o.wholeBetween(12, 1, 48), true)
  for (const bad of [1.5, 0, 49, NaN, '12', null, undefined]) assert.equal(o.wholeBetween(bad, 1, 48), false, String(bad))
})

test('the floor groups by the controller record first, then by goals that only name an outcome', () => {
  const g = (sessionId, text, state = 'active', attention = false) => ({ goal: { sessionId, text, state }, attention })
  const record = { id: 'o1', objective: 'Ship export', state: 'starting', sessions: [
    { key: 'a', title: 'API', state: 'following', note: '', sessionId: 's1' },
    { key: 'b', title: 'UI', state: 'unfollowed', note: '', sessionId: 's2' },
    { key: 'c', title: 'Docs', state: 'waiting', note: '' },
  ] }
  const rows = [g('s1', o.outcomeGoal('Ship export', 'API')), g('s9', o.outcomeGoal('Old outcome', 'Part'), 'needs_user', true), g('s8', 'Ordinary goal')]
  const groups = o.floorOutcomes([record], rows)
  assert.deepEqual(groups.map(x => [x.id, x.objective, x.rows.length]), [['o1', 'Ship export', 1], ['text:Old outcome', 'Old outcome', 1]])
  assert.equal(o.floorOutcomeLine(groups[0]), '3 sessions planned · starting · 1 following · 1 not followed')
  assert.equal(o.floorOutcomeLine(groups[1]), '1 session · 1 needs review')
  const settled = o.floorOutcomes([{ ...record, state: 'stopped', sessions: [record.sessions[0], { ...record.sessions[2], state: 'cancelled' }] }], rows)
  assert.equal(o.floorOutcomeLine(settled[0]), '2 sessions planned · 1 following · 1 not started')
})

test('stages keep pipeline order and take the agent and model of their tier', () => {
  const crew = { heavy: { tool: 'claude', model: ' opus ' }, standard: { tool: 'codex', model: '' }, light: { tool: 'claude', model: 'haiku' } }
  assert.deepEqual(o.orderStages(['merge', 'code', 'bogus', 'plan']), ['plan', 'code', 'merge'])
  assert.deepEqual(o.stagePlan(['review', 'code', 'pr'], crew), [
    { kind: 'code', tool: 'codex', model: '', tier: 'standard' }, { kind: 'review', tool: 'claude', model: 'opus', tier: 'heavy' }, { kind: 'pr', tool: 'claude', model: 'haiku', tier: 'light' },
  ])
  // Each kind has a default tier: thinking stages heavy, building ones standard, shipping ones light.
  assert.deepEqual(o.STAGES.map(stage => [stage.kind, o.defaultTier(stage.kind)]), [['plan', 'heavy'], ['design', 'heavy'], ['code', 'standard'], ['review', 'heavy'], ['test', 'standard'], ['pr', 'light'], ['babysit', 'light'], ['merge', 'light']])
  // Gaffer's assessment puts a stage on another tier: it is done by that tier's agent, and the reason goes with it.
  const ratings = o.planRatings([{ kind: 'code', tier: 'heavy', tierWhy: ' Touches the ledger. ' }, { kind: 'review', tier: 'standard' }, { kind: 'pr', tier: 'enormous', tierWhy: 'ignored' }, { kind: 'test' }])
  assert.deepEqual(ratings, { tiers: { code: 'heavy', review: 'standard' }, whys: { code: 'Touches the ledger.', review: '' }, parts: {} })
  assert.deepEqual(o.stagePlan(['code', 'review', 'pr'], crew, true, ratings), [
    { kind: 'code', tool: 'claude', model: 'opus', tier: 'heavy', tierWhy: 'Touches the ledger.' }, { kind: 'review', tool: 'codex', model: '', tier: 'standard' }, { kind: 'pr', tool: 'claude', model: 'haiku', tier: 'light' },
  ])
  // What the dialog says about one stage: the tier in force, its default, whether they differ, and Gaffer's reason.
  assert.deepEqual(o.stageTier('code', ratings), { tier: 'heavy', standard: 'standard', changed: true, by: 'gaffer', why: 'Touches the ledger.' })
  assert.deepEqual(o.stageTier('review', ratings), { tier: 'standard', standard: 'heavy', changed: true, by: 'gaffer', why: '' })
  assert.deepEqual(o.stageTier('pr', ratings), { tier: 'light', standard: 'light', changed: false, by: 'default', why: '' })
  assert.deepEqual(o.stageTier('merge', undefined), { tier: 'light', standard: 'light', changed: false, by: 'default', why: '' })
  // The person's own choice replaces Gaffer's and its reason; choosing the default puts the stage back.
  const mine = o.withTier(ratings, 'code', 'light')
  assert.deepEqual(mine, { tiers: { code: 'light', review: 'standard' }, whys: { review: '' }, parts: {} })
  assert.deepEqual(o.stageTier('code', mine), { tier: 'light', standard: 'standard', changed: true, by: 'you', why: '' })
  assert.deepEqual(o.withTier(mine, 'code', 'standard'), { tiers: { review: 'standard' }, whys: { review: '' }, parts: {} })
  assert.deepEqual(o.withTier(undefined, 'pr', 'bogus'), { tiers: {}, whys: {}, parts: {} })
  // What is sent keeps the person's tier and drops a reason that was for another.
  assert.equal(o.tierLabel('heavy'), 'Heavy')
  assert.deepEqual(o.CREW_TIERS.map(item => item.tier), ['heavy', 'standard', 'light'])
  // Every stage is a session of its own, whether or not the crew changes between them.
  assert.equal(o.sessionCount(o.stagePlan(o.DEFAULT_STAGES, crew)), o.DEFAULT_STAGES.length)
  const one = { tool: 'claude', model: '' }
  assert.equal(o.sessionCount(o.stagePlan(o.DEFAULT_STAGES, { heavy: one, standard: one, light: one })), o.DEFAULT_STAGES.length)
  assert.equal(o.sessionCount([]), 0)
  assert.equal(o.stageLabel('babysit'), 'Babysit')
})

test('a pipeline track shows what is done, in hand, waiting on the person or failed', () => {
  const stages = [{ kind: 'code', tool: 'codex', state: 'done', summary: 'Committed.' }, { kind: 'review', tool: 'claude', model: 'opus', state: 'running' }, { kind: 'merge', tool: 'claude', state: 'pending' }]
  assert.deepEqual(o.track({ state: 'following', stage: 1, stages }).map(n => n.state), ['done', 'running', 'pending'])
  assert.deepEqual(o.track({ state: 'ready_to_merge', stage: 2, stages: [stages[0], { ...stages[1], state: 'done' }, stages[2]] }).map(n => n.state), ['done', 'done', 'waiting'])
  assert.deepEqual(o.track({ state: 'unfollowed', stage: 1, stages }).map(n => n.state), ['done', 'failed', 'pending'])
  assert.deepEqual(o.track({ state: 'done', stage: 2, stages: stages.map(s => ({ ...s, state: 'done' })) }).map(n => n.state), ['done', 'done', 'done'])
  assert.equal(o.track({ state: 'following', stage: 1, stages })[1].model, 'opus')
  // The tier that chose a stage's agent is on the track, with Gaffer's reason where the tier was its assessment.
  const rated = o.track({ state: 'following', stage: 0, stages: [{ kind: 'code', tool: 'claude', state: 'running', tier: 'heavy', tierWhy: 'Touches the ledger.' }, { kind: 'pr', tool: 'claude', state: 'pending', tier: 'light' }, { kind: 'merge', tool: 'claude', state: 'pending' }] })
  assert.deepEqual(rated.map(n => [n.tier, n.tierWhy]), [['Heavy', 'Touches the ledger.'], ['Light', ''], ['', '']])
  // Whether the choice was applied is said only for a stage that was checked, and a difference is never called confirmed.
  assert.equal(o.stageChoice(undefined), null)
  assert.equal(o.stageChoice({ tool: 'claude', model: 'opus', effort: 'high' }), null)
  assert.deepEqual(o.stageChoice({ tool: 'claude', model: 'opus', effort: 'high', choiceCheck: 'applied', appliedModel: 'opus', appliedEffort: 'high' }), { tone: 'good', label: 'Confirmed', text: 'The session is on opus · high effort, as chosen.' })
  assert.equal(o.stageChoice({ tool: 'claude', choiceCheck: 'applied', appliedModel: 'sonnet', appliedEffort: 'medium' }).text, 'The session is on sonnet · medium effort. The model and effort were left to the agent.')
  assert.equal(o.stageChoice({ tool: 'claude', effort: 'high', choiceCheck: 'applied', appliedModel: 'sonnet', appliedEffort: 'high' }).text, 'The session is on sonnet · high effort. The model was left to the agent.')
  const off = o.stageChoice({ tool: 'claude', model: 'opus', effort: 'high', choiceCheck: 'mismatch', appliedModel: 'opus', appliedEffort: 'low', choiceNote: 'You chose the effort high; the session is on low.' })
  assert.deepEqual([off.tone, off.label, off.text], ['bad', 'Not as chosen', 'You chose the effort high; the session is on low.'])
  assert.equal(o.stageChoice({ tool: 'claude', choiceCheck: 'unchecked', choiceNote: 'reet could not read back what this session is running: offline' }).label, 'Not checked')
  assert.equal(o.track({ state: 'following', stage: 1, stages: stages.map((s, i) => i === 1 ? { ...s, effort: 'high', choiceCheck: 'mismatch', choiceNote: 'x' } : s) })[1].choice.tone, 'bad')
  assert.equal(o.track({ state: 'following', stage: 1, stages })[1].choice, null)
  // An effort the agent does not offer was dropped before the start: the stage is confirmed, with the reason, and never shown as not applied.
  const reason = "You chose the effort high; Claude Code does not offer it for this model here, so the session was started on the agent's default effort."
  const dropped = { tool: 'claude', model: 'opus', effortNotOffered: 'high', choiceCheck: 'applied', appliedModel: 'opus', appliedEffort: 'medium', choiceNote: reason }
  assert.deepEqual(o.stageChoice(dropped), { tone: 'good', label: 'Confirmed', text: `${reason} The session reports opus · medium effort.` })
  assert.equal(o.stageChoice({ ...dropped, appliedEffort: undefined, choiceNote: undefined }).text, "You chose the effort high; the agent does not offer it for this model here, so the session was started on the agent's default effort. The session reports opus.")
  // A model left to the agent is still said to be; a model the session did not report is not named as one.
  assert.equal(o.stageChoice({ ...dropped, model: undefined }).text, `${reason} The session reports opus · medium effort. The model was left to the agent.`)
  assert.equal(o.stageChoice({ ...dropped, model: undefined, appliedModel: undefined }).text, `${reason} The session reports medium effort. The model was left to the agent.`)
  assert.equal(o.stageChoice({ ...dropped, appliedModel: undefined, appliedEffort: undefined }).text, reason)
  const node = o.track({ state: 'following', stage: 1, stages: stages.map((s, i) => i === 1 ? { ...s, ...dropped } : s) })[1]
  assert.deepEqual([node.effort, node.effortNotOffered, node.choice.tone, node.choice.text.startsWith(reason)], ['', 'high', 'good', true])
  assert.equal(o.track({ state: 'following', stage: 1, stages })[1].effortNotOffered, '')
  assert.deepEqual(o.track({ state: 'following' }), [])
})

test('a pipeline badge and the page tally say who has to act', () => {
  assert.deepEqual(o.pipelineStatus({ state: 'ready_to_merge' }, false), { label: 'Ready to merge', tone: 'you' })
  assert.deepEqual(o.pipelineStatus({ state: 'following' }, true), { label: 'Needs you', tone: 'you' })
  assert.deepEqual(o.pipelineStatus({ state: 'following' }, false), { label: 'Running', tone: 'busy' })
  assert.deepEqual(o.pipelineStatus({ state: 'done' }, true), { label: 'Done', tone: 'good' })
  assert.equal(o.pipelineStatus({ state: 'failed' }, false).tone, 'bad')
  const records = [{ sessions: [{ state: 'following', conversationId: 'a' }, { state: 'following', conversationId: 'b' }, { state: 'ready_to_merge' }, { state: 'done' }, { state: 'failed' }, { state: 'waiting' }] }]
  assert.deepEqual(o.factoryTally(records, new Set(['b'])), { running: 2, needYou: 1, ready: 1, done: 1 })
})

test('usage counts what Gaffer used, stage by stage, and says when a count is missing', () => {
  const done = (reviews, i, o, reported = true) => ({ state: 'done', reviews, inputTokens: i, outputTokens: o, usageReported: reported })
  const goal = { usedReviews: 2, inputTokens: 500, outputTokens: 100, usageReported: true }
  assert.deepEqual(o.pipelineUsage({ stages: [done(3, 1000, 200), { state: 'running' }] }, goal), { tokens: 1800, reviews: 5, reported: true })
  assert.deepEqual(o.pipelineUsage({ stages: [done(1, 0, 0, false)] }, goal), { tokens: 600, reviews: 3, reported: false })
  assert.deepEqual(o.pipelineUsage({ stages: [] }), { tokens: 0, reviews: 0, reported: false })
  assert.deepEqual(o.pipelineUsage({}, { ...goal, usedReviews: 0 }), { tokens: 0, reviews: 0, reported: false })
  const a = o.pipelineUsage({ stages: [done(3, 1000, 200)] }), b = o.pipelineUsage({})
  assert.deepEqual(o.outcomeUsage([a, b]), { tokens: 1200, reviews: 3, reported: true })
  assert.deepEqual(o.outcomeUsage([a], { inputTokens: 300, outputTokens: 80, usageReported: true }), { tokens: 1580, reviews: 3, reported: true })
  assert.deepEqual(o.outcomeUsage([b]), { tokens: 0, reviews: 0, reported: false })
  assert.equal(o.tokensText(a), '1,200')
  assert.equal(o.tokensText(b), 'None yet')
  assert.equal(o.tokensText({ tokens: 600, reviews: 3, reported: false }), '600+ (not all reported)')
  assert.equal(o.tokensText({ tokens: 0, reviews: 2, reported: false }), 'Not reported')
})

test('time left reads in words', () => {
  const now = 1_000_000_000
  assert.equal(o.timeLeft(now - 1, now), 'Ended')
  assert.equal(o.timeLeft(now + 45 * 60000, now), '45 min')
  assert.equal(o.timeLeft(now + (5 * 60 + 12) * 60000, now), '5 h 12 min')
  assert.equal(o.timeLeft(now + 72 * 3600000, now), '3 days')
})

test('sessions of a pipeline are marked with the stage they are doing', () => {
  const records = [{ id: 'o1', objective: 'Ship export', sessions: [
    { title: 'API', sessionId: 's2', stage: 1, stages: [{ kind: 'plan', sessionId: 's1' }, { kind: 'code', sessionId: 's2' }, { kind: 'review' }] },
    { title: 'Docs', sessionId: 's9' },
    { title: 'One agent', sessionId: 's5', stage: 1, stages: [{ kind: 'code', sessionId: 's5' }, { kind: 'test', sessionId: 's5' }] },
  ] }]
  const members = o.factoryMembers(records)
  assert.equal(o.factoryBadge(members.get('s1')), 'Factory · Plan 1 of 3')
  assert.equal(members.get('s1').current, false)
  assert.equal(o.factoryBadge(members.get('s2')), 'Factory · Code 2 of 3')
  assert.equal(members.get('s2').current, true)
  assert.equal(o.factoryBadge(members.get('s9')), 'Factory')
  assert.equal(o.factoryBadge(members.get('s5')), 'Factory · Test 2 of 2')
  assert.equal(members.get('unknown'), undefined)
  assert.equal(members.get('s2').pipeline, 'API')
})

test('a tier\'s effort is sent with its stages', () => {
  const one = { tool: 'claude', model: 'opus' }
  const crew = { heavy: { ...one, effort: 'high' }, standard: { ...one, effort: '' }, light: { ...one, effort: 'low' } }
  assert.deepEqual(o.stagePlan(['design', 'code', 'merge'], crew), [
    { kind: 'design', tool: 'claude', model: 'opus', effort: 'high', tier: 'heavy' },
    { kind: 'code', tool: 'claude', model: 'opus', tier: 'standard' },
    { kind: 'merge', tool: 'claude', model: 'opus', effort: 'low', tier: 'light' },
  ])
  assert.equal(o.sessionCount(o.stagePlan(['design', 'code', 'merge'], crew)), 3)
  assert.equal(o.sessionCount(o.stagePlan(['plan', 'design', 'review'], crew)), 3)
})

test('a stage never carries an effort its machine does not report', () => {
  const efforts = tool => tool === 'claude' ? ['', 'low', 'high'] : ['']
  const plan = [
    { kind: 'code', tool: 'claude', model: '', effort: 'high' },
    { kind: 'review', tool: 'claude', model: '', effort: 'medium' },
    { kind: 'pr', tool: 'opencode', model: '', effort: 'low' },
    { kind: 'merge', tool: 'claude', model: '' },
  ]
  assert.deepEqual(o.withReportedEfforts(plan, efforts), [
    { kind: 'code', tool: 'claude', model: '', effort: 'high' },
    { kind: 'review', tool: 'claude', model: '' },
    { kind: 'pr', tool: 'opencode', model: '' },
    { kind: 'merge', tool: 'claude', model: '' },
  ])
  // A plan with nothing to drop is left as it is.
  const clean = [{ kind: 'code', tool: 'claude', model: '', effort: 'low' }]
  assert.equal(o.withReportedEfforts(clean, efforts)[0], clean[0])
})

test('every backup of a stage, not only the first, drops an effort its machine does not report', () => {
  const efforts = tool => tool === 'claude' ? ['', 'low', 'high'] : ['']
  const plan = [{
    kind: 'code', tool: 'claude', model: '', effort: 'high',
    backup: { tool: 'opencode', model: 'm1', effort: 'high' },
    moreBackups: [{ tool: 'claude', effort: 'medium' }, { tool: 'claude', effort: 'low' }, { tool: 'codex', model: 'm2', effort: 'high' }],
  }]
  assert.deepEqual(o.withReportedEfforts(plan, efforts), [{
    kind: 'code', tool: 'claude', model: '', effort: 'high',
    backup: { tool: 'opencode', model: 'm1' },
    moreBackups: [{ tool: 'claude' }, { tool: 'claude', effort: 'low' }, { tool: 'codex', model: 'm2' }],
  }])
  // Backups with nothing to drop leave the stage as it is.
  const clean = [{ kind: 'code', tool: 'claude', model: '', backup: { tool: 'claude', effort: 'low' }, moreBackups: [{ tool: 'opencode' }] }]
  assert.equal(o.withReportedEfforts(clean, efforts)[0], clean[0])
})

test('when what the machine offers is not known, every effort is sent as chosen, backups included', () => {
  const plan = [{
    kind: 'code', tool: 'claude', model: '', effort: 'high',
    backup: { tool: 'opencode', model: 'm1', effort: 'high' },
    moreBackups: [{ tool: 'codex', effort: 'medium' }],
  }]
  // The machine was not read, its read failed or it is gone: the controller drops an unoffered effort itself, so
  // nothing is stripped here on a guess.
  assert.equal(o.withReportedEfforts(plan, null), plan)
  assert.deepEqual(o.withReportedEfforts(plan, null), [{
    kind: 'code', tool: 'claude', model: '', effort: 'high',
    backup: { tool: 'opencode', model: 'm1', effort: 'high' },
    moreBackups: [{ tool: 'codex', effort: 'medium' }],
  }])
})

test('a Cline tier carries its billing source', () => {
  const crew = { heavy: { tool: 'cline', model: 'cline-pass/deepseek-v4', source: 'cline-pass' }, standard: { tool: 'cline', model: '' }, light: { tool: 'codex', model: '' } }
  assert.deepEqual(o.stagePlan(['review', 'code', 'pr'], crew), [
    { kind: 'code', tool: 'cline', model: '', tier: 'standard' }, { kind: 'review', tool: 'cline', model: 'cline-pass/deepseek-v4', provider: 'cline-pass', tier: 'heavy' }, { kind: 'pr', tool: 'codex', model: '', tier: 'light' },
  ])
  assert.equal(o.sessionCount(o.stagePlan(['plan', 'review'], crew)), 2)
  // A machine that did not negotiate a Cline source setting never has one sent for it.
  assert.deepEqual(o.stagePlan(['review'], crew, false), [{ kind: 'review', tool: 'cline', model: 'cline-pass/deepseek-v4', tier: 'heavy' }])
  const switched = { heavy: { tool: 'cline', model: 'cline-pass/deepseek-v4', source: 'cline-pass' }, standard: { tool: 'cline', model: 'cline-pass/deepseek-v4', source: 'cline' }, light: { tool: 'codex', model: '' } }
  assert.equal(o.sessionCount(o.stagePlan(['review', 'test'], switched)), 2)
})

test('agent usage is what agents reported, split by agent, and never a made-up zero', () => {
  const pieces = [
    { stages: [{ tool: 'claude', state: 'done', agentInputTokens: 1000, agentOutputTokens: 200, agentCachedTokens: 5000, agentUsageReported: true }, { tool: 'codex', state: 'running', agentInputTokens: 300, agentOutputTokens: 50, agentUsageReported: true, contextUsed: 170000, contextSize: 200000 }] },
    { stages: [{ tool: 'claude', state: 'running' }] },
  ]
  assert.deepEqual(o.pipelineAgentUsage(pieces[0]), { tokens: 1550, cached: 5000, reported: true })
  assert.equal(o.agentTokensText(o.pipelineAgentUsage(pieces[0])), '1,550 · 5,000 cached')
  assert.equal(o.agentTokensText(o.pipelineAgentUsage(pieces[1])), 'Not reported')
  const all = o.outcomeAgentUsage(pieces)
  assert.deepEqual(all.total, { tokens: 1550, cached: 5000, reported: true })
  assert.deepEqual(all.byAgent, [{ tool: 'claude', tokens: 1200 }, { tool: 'codex', tokens: 350 }])
  assert.equal(o.agentTokensText({ tokens: 2_400_000, cached: 0, reported: true }), '2.4M')
  assert.equal(o.agentTokensText({ tokens: 84_600, cached: 0, reported: true }), '85k')
  assert.deepEqual(o.contextLeft(pieces[0].stages[1]), { text: '15% · 30k of 200k', low: true })
  assert.deepEqual(o.contextLeft({ tool: 'claude', contextUsed: 20000, contextSize: 200000 }), { text: '90% · 180k of 200k', low: false })
  assert.deepEqual(o.contextLeft({ tool: 'claude', contextUsed: 500000, contextSize: 200000 }), { text: '0% · 0 of 200k', low: true })
  assert.deepEqual(o.contextLeft(pieces[1].stages[0]), { text: '', low: false })
  assert.deepEqual(o.contextLeft(undefined), { text: '', low: false })
})

test('a stage the person moved on is marked, and the agent line says how long it has been quiet', () => {
  const nodes = o.track({ state: 'following', stage: 2, stages: [
    { kind: 'plan', tool: 'opencode', state: 'done', closedBy: 'gaffer' },
    { kind: 'design', tool: 'opencode', state: 'done', closedBy: 'chat' },
    { kind: 'code', tool: 'opencode', state: 'running' },
  ] })
  assert.deepEqual(nodes.map(node => node.moved), [false, true, false])
  const now = 10 * 3600_000
  assert.equal(o.ago(now - 30_000, now), 'under a minute')
  assert.equal(o.ago(now - 7 * 60_000, now), '7 min')
  assert.equal(o.ago(now - 95 * 60_000, now), '1 h 35 min')
  assert.equal(o.ago(undefined, now), '')
  assert.deepEqual(o.agentActivity({ agentState: 'working', agentActiveAt: now - 2 * 60_000 }, now), { text: 'Working · last active 2 min ago', quiet: false })
  assert.deepEqual(o.agentActivity({ agentState: 'working', agentActiveAt: now - 20 * 60_000 }, now), { text: 'Working · last active 20 min ago', quiet: true })
  assert.deepEqual(o.agentActivity({ agentState: 'idle', agentActiveAt: now - 60_000 }, now), { text: 'Idle · last active 1 min ago', quiet: true })
  assert.deepEqual(o.agentActivity(undefined, now), { text: '', quiet: false })
})

test('a pipeline that needs the person says why and offers what they can do, the likeliest first', () => {
  const ids = need => need.choices.map(choice => choice.id)
  const waiting = o.needsYou({ state: 'needs_user', note: 'n', waitingOnWorker: true }, undefined, false)
  assert.deepEqual(ids(waiting), ['answer']); assert.match(waiting.why, /waiting for your answer/)
  const failed = o.needsYou({ state: 'needs_user', note: 'Gaffer’s review did not complete (failed): overloaded.', reviewFailed: true }, undefined, false)
  assert.deepEqual(ids(failed), ['retry', 'tell', 'move']); assert.match(failed.why, /overloaded/)
  assert.deepEqual(ids(o.needsYou({ state: 'limit', note: 'The review budget is used.' }, undefined, false)), ['budget', 'move'])
  const idle = o.needsYou({ state: 'needs_user', note: 'long note', noStep: true }, undefined, true)
  assert.deepEqual(ids(idle), ['again', 'tell', 'move', 'open']); assert.equal(idle.choices[2].label, 'Finish this stage'); assert.match(idle.why, /chose no next step/)
  const asked = o.needsYou({ state: 'needs_user', note: 'Gaffer needs your decision. Read its latest review.' }, 'Gaffer brought a question back to you.', false)
  assert.equal(asked.why, 'Gaffer brought a question back to you.'); assert.equal(asked.choices[0].primary, true); assert.equal(asked.choices[2].label, 'Move to next stage')
})
test('a reply to Gaffer is plain text within the byte limit', () => {
  assert.match(o.replyBlocker(''), /Write a reply/)
  assert.match(o.replyBlocker('   '), /Write a reply/)
  assert.equal(o.replyBlocker('Keep the export in one session.'), '')
  assert.equal(o.replyBlocker('Line one\n\tLine two'), '')
  assert.match(o.replyBlocker('é'.repeat(1001)), /within 2,000 bytes/)
  assert.match(o.replyBlocker('Do it\u0000now'), /cannot be sent/)
})

test('a reply carries only the summary, titles and stages of the proposal it answers', () => {
  const previous = o.previousPlan('  Two independent halves.  ', [
    { key: 'a', title: ' Export API ', prompt: 'Build it.', stages: ['code', 'pr'] },
    { key: 'b', title: ' ', prompt: 'Build the button.', stages: [] },
    { key: 'c', title: 'x'.repeat(81), prompt: 'Too long.', stages: ['test'] },
  ])
  assert.equal(previous.summary, 'Two independent halves.')
  assert.deepEqual(previous.sessions, [
    { title: 'Export API', stages: ['code', 'pr'] },
    { title: 'Session 2', stages: [] },
    { title: 'Session 3', stages: ['test'] },
  ])
})

test('a pipeline whose session was ended needs the person, with choices that do not need a live agent', () => {
  const ended = { key: 'a', title: 'Export API', state: 'following', sessionState: 'ended', sessionId: 's1', conversationId: 'c1', stage: 1, stages: [{ kind: 'plan', sessionId: 's0' }, { kind: 'code', sessionId: 's1' }] }
  assert.equal(o.sessionLost(ended), true)
  assert.equal(o.sessionLost({ ...ended, sessionState: 'running' }), false)
  assert.equal(o.sessionLost({ ...ended, sessionState: undefined }), false, 'a controller that does not say is not read as ended')
  assert.equal(o.sessionLost({ ...ended, state: 'done' }), false)
  assert.deepEqual(o.pipelineStatus(ended, false), { label: 'Needs you', tone: 'you' })
  assert.deepEqual(o.factoryTally([{ sessions: [ended] }], new Set()), { running: 0, needYou: 1, ready: 0, done: 0 })
  const choices = o.sessionEndedChoices(false, false)
  assert.deepEqual(choices.choices.map(item => item.id), ['restart', 'move', 'stop', 'open']); assert.equal(choices.choices[0].primary, true)
  assert.deepEqual(o.sessionEndedChoices(true, true).choices.map(item => item.id), ['restart', 'move', 'stop'], 'a removed session cannot be read')
  assert.equal(o.sessionEndedChoices(true, true).choices[1].label, 'Finish this stage')
})
test('ending a session warns only while its pipeline still works in it, and its badge leads to that pipeline', () => {
  const record = { id: 'o1', objective: 'Ship it.', sessions: [
    { key: 'a', title: 'Export API', state: 'following', sessionId: 's1', stage: 1, stages: [{ kind: 'plan', sessionId: 's0' }, { kind: 'code', sessionId: 's1' }] },
    { key: 'b', title: 'Docs', state: 'done', sessionId: 's2', stage: 0, stages: [{ kind: 'code', sessionId: 's2' }] },
  ] }
  const members = o.factoryMembers([record])
  assert.equal(members.get('s1').working, true); assert.equal(members.get('s1').key, 'a')
  assert.equal(members.get('s0').working, false, 'an earlier stage’s own session is finished with')
  assert.equal(members.get('s2').working, false, 'a finished pipeline loses nothing')
  assert.match(o.factoryEndWarning(members.get('s1')), /doing the code stage of the Factory pipeline “Export API”\. Ending it leaves that pipeline with nothing working on it/)
  assert.match(o.factoryEndWarning(members.get('s1')), /choose below what happens to the outcome/)
  assert.match(o.factoryEndWarning(members.get('s1'), true), /^This session.*Removing it leaves that pipeline/)
  assert.match(o.factoryEndWarning(members.get('s1'), true), /worktree/)
  assert.equal(o.factoryEndWarning(members.get('s0')), ''); assert.equal(o.factoryEndWarning(members.get('s2')), ''); assert.equal(o.factoryEndWarning(undefined), '')
})
test('stopping a pipeline’s session defaults to deleting its outcome, and a saved choice overrides it', () => {
  assert.equal(o.FACTORY_SESSION_END_DEFAULT, 'delete')
  assert.equal(o.factorySessionEndModeOf(undefined), 'delete')
  assert.equal(o.factorySessionEndModeOf('anything else'), 'delete')
  for (const mode of ['leave', 'archive', 'delete']) assert.equal(o.factorySessionEndModeOf(mode), mode)
  assert.equal(o.factorySessionEndRequest('leave'), '')
  assert.equal(o.factorySessionEndRequest('archive'), 'archive')
  assert.equal(o.factorySessionEndRequest('delete'), 'delete')
})
test('token counts round up into millions instead of reading 1000k', () => {
  assert.equal(o.agentTokensText({ tokens: 999_700, cached: 0, reported: true }), '1.0M')
  assert.equal(o.agentTokensText({ tokens: 998_000, cached: 0, reported: true }), '998k')
})

test('the first choice is the recommended one, and Gaffer says which and why when its review advised', () => {
  // With no advice the fixed default leads and is the recommendation.
  const plain = o.needsYou({ state: 'needs_user', note: 'Gaffer needs your decision.' }, undefined, false)
  assert.deepEqual(plain.choices.map(c => [c.id, !!c.recommended, !!c.primary]), [['again', true, true], ['tell', false, false], ['move', false, false], ['open', false, false]])
  assert.equal(plain.recommend.by, 'default')
  // Gaffer's advice moves its choice to the front, with its reason and the instruction it would give.
  const advised = o.needsYou({ state: 'needs_user', note: 'n', advice: { choice: 'tell', why: 'The endpoint is untested.', text: 'Add tests for the export endpoint.' } }, undefined, false)
  assert.deepEqual(advised.choices.map(c => c.id), ['tell', 'again', 'move', 'open'])
  assert.deepEqual(advised.choices.map(c => !!c.recommended), [true, false, false, false])
  assert.deepEqual(advised.choices.map(c => !!c.primary), [true, false, false, false])
  assert.deepEqual(advised.recommend, { by: 'gaffer', label: 'Tell the agent…', why: 'The endpoint is untested.', text: 'Add tests for the export endpoint.' })
  // Advice for a choice the person does not have here is ignored.
  assert.equal(o.needsYou({ state: 'limit', note: 'n', advice: { choice: 'tell', why: 'x', text: 'y' } }, undefined, false).recommend.by, 'default')
  // One choice is not a recommendation.
  assert.equal(o.needsYou({ state: 'needs_user', note: 'n', waitingOnWorker: true }, undefined, false).recommend, undefined)
  assert.equal(o.sessionEndedChoices(false, false).choices[0].recommended, true)
})

test('an outcome lists every decision waiting for the person, the merge included', () => {
  const stages = [{ kind: 'code', tool: 'claude', state: 'running' }, { kind: 'merge', tool: 'claude', state: 'pending' }]
  const record = { id: 'o1', sessions: [
    { key: 'a', title: 'API', state: 'following', conversationId: 'c1', sessionId: 's1', stage: 0, stages },
    { key: 'b', title: 'Button', state: 'following', conversationId: 'c2', sessionId: 's2', stage: 0, stages },
    { key: 'c', title: 'Docs', state: 'ready_to_merge', conversationId: 'c3', sessionId: 's3', stage: 1, stages },
    { key: 'd', title: 'Notes', state: 'following', conversationId: 'c4', sessionId: 's4', sessionState: 'ended', stage: 0, stages },
  ] }
  const goals = new Map([['c1', { attention: true, goal: { state: 'needs_user', note: 'Gaffer needs your decision.' } }], ['c2', { attention: false, goal: { state: 'active', note: '' } }]])
  const waiting = o.waitingDecisions(record, goals)
  assert.deepEqual(waiting.map(w => [w.piece.key, w.stage, w.need.choices[0].id]), [['a', 'Code', 'again'], ['c', 'Merge', 'merge'], ['d', 'Code', 'restart']])
  assert.match(waiting[1].need.why, /Nothing merges until you approve/)
})

test('what Gaffer decided reads as a sentence, and a stage says when it has used its decisions', () => {
  assert.deepEqual(o.decisionText({ about: 'question', choice: 'move', why: 'w' }), { about: 'Gaffer’s review brought the stage back', did: 'Moved to the next stage' })
  assert.deepEqual(o.decisionText({ about: 'session_ended', choice: 'restart', why: 'w' }), { about: 'The stage’s session had ended', did: 'Ran the stage again in a new session' })
  assert.deepEqual(o.decisionText({ about: 'limit', choice: '', failed: true, why: 'w' }), { about: 'The stage had used its budget', did: 'Left to you' })
  assert.deepEqual(o.decisionText({ about: 'agent_question', choice: 'answer', why: 'w' }), { about: 'The agent asked a multiple-choice question', did: 'Answered it' })
  assert.equal(o.MAX_STAGE_DECISIONS, 3)
  // A stage Gaffer closed itself is told apart from one it handed over after a review and one the person moved on.
  const closed = o.track({ state: 'following', stage: 2, stages: [{ kind: 'code', tool: 'claude', state: 'done', closedBy: 'auto' }, { kind: 'review', tool: 'claude', state: 'done', closedBy: 'person' }, { kind: 'pr', tool: 'claude', state: 'running' }] })
  assert.deepEqual(closed.map(n => [n.auto, n.moved]), [[true, false], [false, true], [false, false]])
})

test('a tier’s backup goes with each of its stages, and one that is the first choice again is left out', () => {
  const one = { tool: 'claude', model: 'opus', effort: 'high' }
  const crew = { heavy: { ...one, backup: { tool: 'codex', model: ' gpt-strong ', effort: '' } }, standard: { tool: 'claude', model: '', backup: { tool: 'claude', model: '' } }, light: { tool: 'claude', model: '', backup: { tool: '', model: '' } } }
  const plan = o.stagePlan(['review', 'code', 'pr'], crew)
  assert.deepEqual(plan.map(stage => stage.backup), [undefined, { tool: 'codex', model: 'gpt-strong' }, undefined])
  assert.equal(o.BALANCE_LOW_PERCENT, 20)
  // The share is the person's to set, within bounds; anything that is not a number is the default.
  assert.deepEqual([35, '50', 2, 400, '', null, 'x', 19.6].map(o.balancePercent), [35, 50, 5, 90, 20, 20, 20, 20])
  assert.equal(o.track({ state: 'following', stage: 0, stages: [{ kind: 'code', tool: 'codex', state: 'running', balanced: 'Claude Code’s plan had 12% left.' }] })[0].balanced, 'Claude Code’s plan had 12% left.')
})

test('a split stage is sent as one entry for each part, on that part’s own tier', () => {
  const crew = { heavy: { tool: 'claude', model: 'opus' }, standard: { tool: 'claude', model: '' }, light: { tool: 'codex', model: 'small' } }
  const ratings = o.planRatings([
    { kind: 'code', part: 'Ledger query', brief: 'Write the query.', tier: 'heavy', tierWhy: 'Touches the ledger.' },
    { kind: 'code', part: 'Download button', brief: 'Add the button.', tier: 'light' },
    { kind: 'review' }, { kind: 'pr', tier: 'standard', tierWhy: 'A long description.' },
  ])
  assert.deepEqual(Object.keys(ratings.parts), ['code'])
  assert.deepEqual(ratings.tiers, { pr: 'standard' })
  const plan = o.stagePlan(['code', 'review', 'pr'], crew, true, ratings)
  assert.deepEqual(plan.map(s => [s.kind, s.part ?? '', s.tier, s.tool, s.tierWhy ?? '']), [
    ['code', 'Ledger query', 'heavy', 'claude', 'Touches the ledger.'], ['code', 'Download button', 'light', 'codex', ''], ['review', '', 'heavy', 'claude', ''], ['pr', '', 'standard', 'claude', 'A long description.'],
  ])
  assert.equal(plan[0].brief, 'Write the query.')
  assert.equal(o.sessionCount(plan), 4)
  // The person can change a part, which drops Gaffer's reason for its tier, and removing down to one part ends the split.
  const edited = o.withPart(ratings, 'code', 0, { tier: 'standard' })
  assert.deepEqual([edited.code[0].tier, edited.code[0].why], ['standard', undefined])
  assert.deepEqual(o.withPart(ratings, 'code', 1, null), {})
  assert.deepEqual(o.withoutParts(ratings, 'code'), {})
  // On the card each part is its own step, named, with a key of its own.
  const nodes = o.track({ state: 'following', stage: 1, stages: [{ kind: 'code', part: 'Ledger query', tool: 'claude', state: 'done' }, { kind: 'code', part: 'Download button', tool: 'codex', state: 'running' }, { kind: 'review', tool: 'claude', state: 'pending' }] })
  assert.deepEqual(nodes.map(n => [n.key, n.label, n.state]), [['code:0', 'Code: Ledger query', 'done'], ['code:1', 'Code: Download button', 'running'], ['review:2', 'Review', 'pending']])
})

test('a stage can be split by hand, and its parts need names and briefs before they are sent', () => {
  const two = o.withNewPart({ tiers: { code: 'heavy' } }, 'code')
  assert.deepEqual(two.code, [{ title: 'Part 1', brief: '', tier: 'heavy' }, { title: 'Part 2', brief: '', tier: 'heavy' }])
  const three = o.withNewPart({ parts: two }, 'code')
  assert.equal(three.code.length, 3)
  // At most four, and never the pull request, babysitting or merge.
  assert.equal(o.withNewPart({ parts: { code: [1, 2, 3, 4].map(n => ({ title: `P${n}`, brief: 'b', tier: 'standard' })) } }, 'code').code.length, 4)
  assert.deepEqual(o.withNewPart(undefined, 'merge'), {})
  assert.equal(o.stageTakesParts('review'), true)
  const item = { title: 'Export', stages: ['code', 'pr'], parts: two }
  assert.match(o.partsBlocker([item]), /Give every part of the code stage of “Export” a name and a brief/)
  const filled = { ...item, parts: { code: two.code.map((part, n) => ({ ...part, brief: `Do ${n}.` })) } }
  assert.equal(o.partsBlocker([filled]), '')
  assert.match(o.partsBlocker([{ ...filled, parts: { code: filled.parts.code.map(part => ({ ...part, title: 'Same' })) } }]), /have the same name/)
})

test('a tier’s backups are sent in order, without repeats, and a Cline backup carries its source', () => {
  const crew = { heavy: { tool: 'claude', model: 'opus', backup: { tool: 'codex', model: 'big' }, more: [{ tool: 'cline', model: '', source: 'openrouter' }, { tool: 'codex', model: 'big' }, { tool: 'claude', model: 'opus' }] }, standard: { tool: 'claude', model: '' }, light: { tool: 'claude', model: '', backup: { tool: '', model: '' }, more: [{ tool: 'codex', model: '' }] } }
  const plan = o.stagePlan(['review', 'pr'], crew)
  assert.deepEqual(plan[0].backup, { tool: 'codex', model: 'big' })
  assert.deepEqual(plan[0].moreBackups, [{ tool: 'cline', provider: 'openrouter' }])
  // A first backup left empty does not hide the one after it.
  assert.deepEqual([plan[1].backup, plan[1].moreBackups], [{ tool: 'codex' }, undefined])
  assert.equal(o.MAX_BACKUPS, 3)
})

test('an objective of paragraphs is named in one short line', () => {
  assert.equal(o.short('Ship CSV export.'), 'Ship CSV export.')
  const long = o.short(`First paragraph ${'word '.repeat(30)}\n\nSecond paragraph.`)
  assert.equal(long.length, 80)
  assert.ok(long.endsWith('…'))
  assert.ok(!long.includes('\n'))
})
