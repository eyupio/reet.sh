import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import assert from 'node:assert/strict'
import ts from 'typescript'

const compile = name => ts.transpileModule(readFileSync(new URL(name, import.meta.url), 'utf8'), {
  compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
}).outputText
const asURL = text => `data:text/javascript;base64,${Buffer.from(text).toString('base64')}`
const catalogue = asURL(compile('./catalogue.ts'))
const agentLaunch = asURL(compile('./agentLaunch.ts').replace("from './catalogue'", `from '${catalogue}'`))
const terminalApi = asURL(compile('./terminal/api.ts'))
const source = compile('./launchBar.ts')
  .replace("from './catalogue'", `from '${catalogue}'`)
  .replace("from './agentLaunch'", `from '${agentLaunch}'`)
  .replace("from './terminal/api'", `from '${terminalApi}'`)
const { initialChoice, afterLaunch, baseBranchFor, baseFor, sentence, startable, toCreate, defaultTitle, sendKey, projectSlug, lastChoice, withProject, withHome, summary, toSelectionRun, startedLine, withSelection, runRefusalText } = await import(asURL(source))
const { refusalText } = await import(agentLaunch)

const caps = (extra = {}) => ({ protocolVersion: 1, os: 'linux', osUser: 'u', terminalEnabled: true, terminalControl: true, healthEnabled: true, readiness: 'ready',
  launch: true, worktree: true, agentsV2: true, branches: true, tools: ['claude', 'codex', 'cline'], ...extra })
const machine = (agentId, host, extra = {}, ready = true) => ({ agentId, host, ready, capabilities: caps(extra) })
const machines = [machine('m-studio', 'studio'), machine('m-server', 'server'), machine('m-desk', 'desk')]
const project = (id, name, host, extra = {}) => ({ id, name, host, machine: { 'm-studio': 'studio', 'm-server': 'server', 'm-desk': 'desk' }[host] ?? '', dir: `/home/u/${name}`,
  isRepo: true, baseBranch: '', worktreeDefault: 'inherit', defaultTool: '', defaultModel: '', hasStarterPrompt: false, note: '', position: 0, createdAt: 1, updatedAt: 1, version: 1, ...extra })
const reet = project('p-reet', 'reet', 'm-studio')
const site = project('p-site', 'site', 'm-server')
const notes = project('p-notes', 'notes', 'm-desk', { isRepo: false })
const projects = [reet, site, notes]
const profile = { defaultBaseBranch: '', worktreesByDefault: true }
const choice = (extra = {}) => ({ agent: 'claude', model: 'sonnet', projectIds: ['p-reet'], branch: 'main', worktree: true, effort: '', prompt: '', approvals: 'ask', ...extra })
const words = parts => parts.map(p => p.text).join(' ')
const slots = parts => parts.filter(p => p.slot).map(p => p.slot)

test('base precedence project, profile, default, current', () => {
  const branches = { current: 'feature', default: 'main', branches: ['main', 'feature'], more: false }
  assert.equal(baseBranchFor({ ...reet, baseBranch: 'release' }, { ...profile, defaultBaseBranch: 'develop' }, branches), 'release')
  assert.equal(baseBranchFor(reet, { ...profile, defaultBaseBranch: 'develop' }, branches), 'develop')
  assert.equal(baseBranchFor(reet, profile, branches), 'main')
  assert.equal(baseBranchFor(reet, profile, { ...branches, default: '' }), 'feature')
  assert.equal(baseBranchFor(reet, profile, null), '')
  assert.equal(baseBranchFor(reet, {}, { ...branches, default: '' }), 'feature')
})

test('the sentence reads as the mock-up draws it', () => {
  const parts = sentence(choice(), projects, machines)
  assert.equal(words(parts), 'Run Claude Code with sonnet on reet at studio from main in a fresh worktree')
  assert.deepEqual(slots(parts), ['agent', 'model', 'project', 'machine', 'branch', 'worktree'])
  assert.equal(words(sentence(choice({ model: '' }), projects, machines)), 'Run Claude Code with default on reet at studio from main in a fresh worktree')
  assert.equal(words(sentence(choice({ branch: '' }), projects, machines)), 'Run Claude Code with sonnet on reet at studio from the current branch in a fresh worktree')
})

test('checkout hides branch choice and reads Works in the checkout on main', () => {
  const parts = sentence(choice({ worktree: false }), projects, machines, 'main')
  assert.equal(words(parts), 'Run Claude Code with sonnet on reet at studio in the checkout Works in the checkout on main.')
  assert.ok(!slots(parts).includes('branch'))
  assert.deepEqual(parts.at(-1), { slot: 'checkout', text: 'Works in the checkout on main.' })
  assert.equal(sentence(choice({ worktree: false }), projects, machines).at(-1).text, 'Works in the checkout.')
  // No base is sent for a checkout: reet never switches a checkout's branch.
  assert.equal(toCreate(choice({ worktree: false }), reet, 'r'.repeat(16), 'a'.repeat(16)).launch.base, undefined)
})

test('non-repository hides branch and worktree', () => {
  const parts = sentence(choice({ projectIds: ['p-notes'] }), projects, machines)
  assert.equal(words(parts), 'Run Claude Code with sonnet on notes at desk')
  const home = sentence(choice({ projectIds: [], homeMachine: 'm-server' }), projects, machines)
  assert.equal(words(home), 'Run Claude Code with sonnet on the home folder at server')
  const create = toCreate(choice({ projectIds: ['p-notes'] }), notes, 'r'.repeat(16), 'a'.repeat(16))
  assert.deepEqual(create.launch, { tool: 'claude', model: 'sonnet', dir: '/home/u/notes', projectId: 'p-notes' })
})

test('thinking shows only for an agent that takes effort', () => {
  assert.equal(words(sentence(choice({ agent: 'cline', model: '', effort: 'high' }), projects, machines)), 'Run Cline with default on reet at studio from main in a fresh worktree · thinking high')
  assert.equal(words(sentence(choice({ agent: 'cline', model: '' }), projects, machines)).endsWith('· thinking default'), true)
  assert.ok(!slots(sentence(choice({ effort: 'high' }), projects, machines)).includes('effort'))
})

test('sentence for several projects', () => {
  const parts = sentence(choice({ projectIds: ['p-reet', 'p-site', 'p-notes'] }), projects, machines)
  assert.equal(words(parts), "Run Claude Code with sonnet on 3 projects at each project's machine from each project's default in a fresh worktree")
  const other = project('p-x', 'site', 'm-studio')
  const two = sentence(choice({ projectIds: ['p-reet', 'p-x'] }), [...projects, other], machines)
  assert.equal(words(two), "Run Claude Code with sonnet on 2 projects at studio from each project's default in a fresh worktree")
})

test('approvals reset to ask', () => {
  const last = choice({ approvals: 'yolo', prompt: 'Fix the tests' })
  const next = initialChoice(last, projects, profile)
  assert.equal(next.approvals, 'ask')
  assert.equal(next.prompt, '')
  assert.equal(next.agent, 'claude')
  assert.equal(next.model, 'sonnet')
  assert.deepEqual(next.projectIds, ['p-reet'])
  assert.equal(next.branch, 'main')
  const after = afterLaunch(last)
  assert.equal(after.approvals, 'ask')
  assert.equal(after.prompt, '')
  assert.equal(after.agent, 'claude')
})

test('first choice comes from the first project and the profile', () => {
  assert.deepEqual(initialChoice(null, projects, profile), { agent: 'claude', model: '', projectIds: ['p-reet'], branch: '', worktree: true, effort: '', prompt: '', approvals: 'ask' })
  const own = { ...reet, defaultTool: 'codex', defaultModel: 'gpt-5-codex', baseBranch: 'release', worktreeDefault: 'no' }
  assert.deepEqual(initialChoice(null, [own], { defaultBaseBranch: 'develop', worktreesByDefault: true }), { agent: 'codex', model: 'gpt-5-codex', projectIds: ['p-reet'], branch: 'release', worktree: false, effort: '', prompt: '', approvals: 'ask' })
  assert.equal(initialChoice(null, [reet], { defaultBaseBranch: 'develop', worktreesByDefault: false }).worktree, false)
  assert.equal(initialChoice(null, [reet], { defaultBaseBranch: 'develop', worktreesByDefault: false }).branch, 'develop')
  assert.equal(initialChoice(null, [{ ...reet, worktreeDefault: 'yes' }], { worktreesByDefault: false }).worktree, true)
  assert.deepEqual(initialChoice(null, [], profile).projectIds, [])
  // A removed project falls back to the first one; effort is dropped for an agent without it.
  const gone = initialChoice(choice({ projectIds: ['p-gone'], effort: 'high' }), projects, profile)
  assert.deepEqual(gone.projectIds, ['p-reet'])
  assert.equal(gone.effort, '')
  assert.equal(initialChoice(choice({ agent: 'cline', effort: 'low' }), projects, profile).effort, 'low')
  const home = initialChoice(choice({ projectIds: [], homeMachine: 'm-desk' }), projects, profile)
  assert.deepEqual([home.projectIds, home.homeMachine], [[], 'm-desk'])
})

test('Enter never sends on touch; Ctrl+Enter and Cmd+Enter send', () => {
  const key = (extra = {}) => ({ key: 'Enter', ctrlKey: false, metaKey: false, shiftKey: false, altKey: false, isComposing: false, ...extra })
  // A hardware keyboard: only Ctrl+Enter or Cmd+Enter sends.
  assert.equal(sendKey(key(), false), false)
  assert.equal(sendKey(key({ shiftKey: true }), false), false)
  assert.equal(sendKey(key({ ctrlKey: true }), false), true)
  assert.equal(sendKey(key({ metaKey: true }), false), true)
  assert.equal(sendKey(key({ ctrlKey: true, isComposing: true }), false), false)
  assert.equal(sendKey(key({ ctrlKey: true, altKey: true }), false), false)
  assert.equal(sendKey(key({ key: 'a', ctrlKey: true }), false), false)
  // Touch: the Start button is the only way to send (R8), whatever the key.
  for (const extra of [{}, { ctrlKey: true }, { metaKey: true }, { shiftKey: true }, { key: 'a', ctrlKey: true }]) assert.equal(sendKey(key(extra), true), false)
})

test('title from the first line', () => {
  assert.equal(defaultTitle('Fix the failing tests\nThen push'), 'Fix the failing tests')
  assert.equal(defaultTitle('\n  \n  Docs pass  \nmore'), 'Docs pass')
  assert.equal(defaultTitle('x'.repeat(90)), 'x'.repeat(80))
  assert.equal(defaultTitle('é'.repeat(81)), 'é'.repeat(80))
  assert.equal(defaultTitle('tab\there\u200bnow'), 'tab herenow')
  assert.equal(defaultTitle(''), '')
  assert.equal(defaultTitle('', 'Claude Code · reet'), 'Claude Code · reet')
})

test('a prompt starting with - is refused with the launch sentence', () => {
  assert.equal(startable(choice({ prompt: '-rf everything' }), projects, machines), 'Start the prompt with a word, not a dash.')
  assert.equal(startable(choice({ prompt: '--help' }), projects, machines), 'Start the prompt with a word, not a dash.')
  // A folder named -x is a path, not an option: it launches with dir unchanged.
  const dash = project('p-dash', '-x', 'm-studio')
  assert.equal(startable(choice({ projectIds: ['p-dash'], prompt: 'Fix it now' }), [dash], machines), null)
  assert.equal(toCreate(choice({ projectIds: ['p-dash'] }), dash, 'r'.repeat(16), 'a'.repeat(16)).launch.dir, '/home/u/-x')
})

test('startable names the first reason', () => {
  assert.equal(startable(choice(), projects, machines), null)
  assert.equal(startable(choice({ agent: '' }), projects, machines), 'Choose an agent.')
  assert.equal(startable(choice({ projectIds: [] }), projects, machines), 'Choose a project or a home folder.')
  assert.equal(startable(choice({ agent: 'pi' }), projects, machines), "reet doesn't start this agent yet.")
  assert.equal(startable(choice({ agent: 'codex', prompt: 'one' }), projects, machines), 'Write the opening prompt as at least two words.')
  assert.equal(startable(choice({ effort: 'high' }), projects, machines), "That agent doesn't take that option.")
  assert.equal(startable(choice({ agent: 'gemini' }), projects, machines), 'Not installed on studio')
  const down = [machine('m-studio', 'studio', {}, false)]
  assert.equal(startable(choice(), projects, down), 'The machine is reconnecting. Existing shells will be kept.')
  assert.equal(startable(choice(), projects, []), 'Choose an enrolled machine.')
  const old = [machine('m-studio', 'studio', { agentsV2: false })]
  assert.equal(startable(choice(), projects, old), 'Update reet on that machine to use that option.')
  assert.equal(startable(choice({ branch: '' }), projects, old), null)
  assert.equal(startable(choice({ worktree: false }), projects, old), null)
  const noWorktree = [machine('m-studio', 'studio', { worktree: false })]
  assert.equal(startable(choice(), projects, noWorktree), 'Update reet on that machine to start agents with worktrees.')
  assert.equal(startable(choice({ approvals: 'yolo' }), projects, machines), "studio doesn't accept YOLO sessions.")
  assert.equal(startable(choice({ approvals: 'yolo' }), projects, [machine('m-studio', 'studio', { yolo: true })]), null)
  assert.equal(startable(choice({ projectIds: [], homeMachine: 'm-desk' }), projects, machines), null)
  assert.equal(startable(choice({ projectIds: ['p-reet', 'p-site'] }), projects, [machines[0], machine('m-server', 'server', {}, false)]), 'The machine is reconnecting. Existing shells will be kept.')
})

test('toCreate adds base, effort and yolo only when set', () => {
  const made = toCreate(choice({ prompt: 'Fix the failing tests\nThen push' }), reet, 'r'.repeat(16), 'a'.repeat(16))
  assert.deepEqual(made, { agentId: 'm-studio', requestId: 'r'.repeat(16), title: 'Fix the failing tests', launch: {
    tool: 'claude', model: 'sonnet', dir: '/home/u/reet', prompt: 'Fix the failing tests\nThen push', projectId: 'p-reet', worktree: 'a'.repeat(16), project: 'reet', base: 'main' } })
  const all = toCreate(choice({ agent: 'cline', model: ' ', effort: 'medium', approvals: 'yolo', branch: '' }), reet, 'r'.repeat(16), 'a'.repeat(16))
  assert.deepEqual(all.launch, { tool: 'cline', dir: '/home/u/reet', projectId: 'p-reet', worktree: 'a'.repeat(16), project: 'reet', effort: 'medium', yolo: true })
  assert.equal(all.title, 'Cline · reet')
  const home = toCreate(choice({ projectIds: [], homeMachine: 'm-desk' }), null, 'r'.repeat(16), 'a'.repeat(16))
  assert.deepEqual(home, { agentId: 'm-desk', requestId: 'r'.repeat(16), title: 'Claude Code', launch: { tool: 'claude', model: 'sonnet' } })
})

test('project slug is a valid worktree slug', () => {
  assert.equal(projectSlug('reet'), 'reet')
  assert.equal(projectSlug('My Site.v2'), 'my-site-v2')
  assert.equal(projectSlug('-x'), 'x')
  assert.equal(projectSlug('ß…'), 'project')
  assert.equal(projectSlug('a-'.repeat(30)).length <= 40, true)
  for (const name of ['a-'.repeat(30), 'Ω mega', '--', 'x'.repeat(50)]) assert.match(projectSlug(name), /^[a-z0-9]+(-[a-z0-9]+)*$/)
})

test('refusals for options and disabled agents', () => {
  assert.equal(refusalText(409, 'launch_option_unsupported', "That agent doesn't take that option."), "That agent doesn't take that option.")
  assert.equal(refusalText(409, 'launch_option_unsupported', 'Update reet on that machine to use that option.'), 'Update reet on that machine to use that option.')
  assert.equal(refusalText(422, 'tool_unavailable', "reet doesn't start this agent yet."), "reet doesn't start this agent yet.")
})

test('several projects: each sends its own base, never the single choice', () => {
  const r = 'r'.repeat(16), a = 'a'.repeat(16)
  const own = { ...reet, baseBranch: 'release' }
  const many = choice({ projectIds: ['p-reet', 'p-site'], branch: 'release' })
  assert.equal(toCreate(many, own, r, a).launch.base, 'release')
  assert.equal(toCreate(many, site, r, a).launch.base, undefined)
  assert.equal(toCreate(many, site, r, a, { defaultBaseBranch: 'develop' }).launch.base, 'develop')
  assert.equal(toCreate(many, { ...site, baseBranch: 'trunk' }, r, a, { defaultBaseBranch: 'develop' }).launch.base, 'trunk')
  // The probe: reet on a current machine, site on one without agents-v2 and no base of its own.
  const mixed = [machines[0], machine('m-server', 'server', { agentsV2: false })]
  assert.equal(startable(many, [own, site], mixed), null)
  assert.equal(startable(many, [own, { ...site, baseBranch: 'trunk' }], mixed), 'Update reet on that machine to use that option.')
  assert.equal(startable(many, [own, site], mixed, { defaultBaseBranch: 'develop' }), 'Update reet on that machine to use that option.')
  // startable and toCreate use one rule.
  for (const p of [own, site, { ...site, baseBranch: 'trunk' }]) for (const prof of [{}, { defaultBaseBranch: 'develop' }]) assert.equal(baseFor(many, p, prof), toCreate(many, p, r, a, prof).launch.base ?? '')
})

test('a branch only goes out when chosen or set by the project or profile', () => {
  const r = 'r'.repeat(16), a = 'a'.repeat(16)
  const old = [machine('m-studio', 'studio', { agentsV2: false })]
  // '' means the checkout's current commit: nothing sent, and an old machine still starts a worktree.
  const unset = choice({ branch: '' })
  assert.equal(toCreate(unset, reet, r, a).launch.base, undefined)
  assert.equal(startable(unset, projects, old), null)
  // The suggestion from the machine's branches is for the picker and the sentence only.
  const branches = { current: 'main', default: 'main', branches: ['main'], more: false }
  assert.equal(baseBranchFor(reet, profile, branches), 'main')
  assert.equal(initialChoice(null, [reet], profile).branch, '')
  assert.equal(words(sentence(unset, projects, machines, 'main')), 'Run Claude Code with sonnet on reet at studio from main in a fresh worktree')
  // A base the project sets is sent, and an old machine is told to update.
  const set = initialChoice(null, [{ ...reet, baseBranch: 'release' }], profile)
  assert.equal(set.branch, 'release')
  assert.equal(toCreate(set, { ...reet, baseBranch: 'release' }, r, a).launch.base, 'release')
  assert.equal(startable(set, [{ ...reet, baseBranch: 'release' }], old), 'Update reet on that machine to use that option.')
})

test('the last launch comes from the newest session that names an agent, never from storage', () => {
  const s = (id, createdAt, extra = {}) => ({ id, agentId: 'm-studio', createdAt, ...extra })
  assert.equal(lastChoice([s('a', 1), s('b', 2)], projects, profile), null)
  const sessions = [s('old', 1, { tool: 'claude', projectId: 'p-site' }), s('new', 5, { tool: 'codex', model: 'gpt-5-codex', projectId: 'p-reet', worktreePath: '/home/u/.reet/worktrees/reet/reet/x' }), s('plain', 9)]
  assert.deepEqual(lastChoice(sessions, projects, { defaultBaseBranch: 'develop' }), { agent: 'codex', model: 'gpt-5-codex', projectIds: ['p-reet'], branch: 'develop', worktree: true, effort: '', prompt: '', approvals: 'ask' })
  // A repository session without a worktree worked in the checkout; the project's own base wins over the account's.
  const checkout = lastChoice([s('c', 1, { tool: 'claude', projectId: 'p-reet' })], [{ ...reet, baseBranch: 'release' }], { defaultBaseBranch: 'develop' })
  assert.equal(checkout.worktree, false)
  assert.equal(checkout.branch, 'release')
  // Effort only for an agent that takes it.
  assert.equal(lastChoice([s('e', 1, { tool: 'cline', effort: 'high', projectId: 'p-reet' })], projects, profile).effort, 'high')
  assert.equal(lastChoice([s('e', 1, { tool: 'claude', effort: 'high', projectId: 'p-reet' })], projects, profile).effort, '')
  assert.equal(lastChoice([s('e', 1, { tool: 'cline', effort: 'max', projectId: 'p-reet' })], projects, profile).effort, '')
  // Outside every project: the home folder on that machine. A project since removed: no project (the first one is used).
  assert.deepEqual(lastChoice([s('h', 1, { tool: 'claude' })], projects, profile).homeMachine, 'm-studio')
  const gone = lastChoice([s('g', 1, { tool: 'claude', projectId: 'p-gone' })], projects, profile)
  assert.deepEqual([gone.projectIds, gone.homeMachine], [[], undefined])
  assert.deepEqual(initialChoice(gone, projects, profile).projectIds, ['p-reet'])
})

test('choosing a project re-derives its branch and worktree and keeps the prompt', () => {
  const start = choice({ prompt: 'Fix the tests', effort: '', homeMachine: 'm-desk', projectIds: [] })
  const next = withProject(start, { ...site, baseBranch: 'trunk', worktreeDefault: 'no' }, profile)
  assert.deepEqual(next, { agent: 'claude', model: 'sonnet', projectIds: ['p-site'], branch: 'trunk', worktree: false, effort: '', prompt: 'Fix the tests', approvals: 'ask' })
  assert.equal(withProject(start, site, { defaultBaseBranch: 'develop', worktreesByDefault: false }).branch, 'develop')
  assert.equal(withProject(start, site, { worktreesByDefault: false }).worktree, false)
  // The project's own default agent and model come with it; effort is dropped for an agent without it.
  const own = withProject(choice({ agent: 'cline', effort: 'high' }), { ...site, defaultTool: 'codex', defaultModel: 'gpt-5-codex' }, profile)
  assert.deepEqual([own.agent, own.model, own.effort], ['codex', 'gpt-5-codex', ''])
  const home = withHome(choice(), 'm-server')
  assert.deepEqual([home.projectIds, home.homeMachine, home.branch], [[], 'm-server', ''])
})

test('the phone summary is the sentence in one line', () => {
  assert.equal(summary(sentence(choice(), projects, machines)), 'Claude Code on reet at studio · main · fresh worktree')
  assert.equal(summary(sentence(choice({ worktree: false }), projects, machines, 'main')), 'Claude Code on reet at studio · the checkout')
  assert.equal(summary(sentence(choice({ projectIds: ['p-notes'] }), projects, machines)), 'Claude Code on notes at desk')
  assert.equal(summary(sentence(choice({ agent: 'cline', effort: 'low' }), projects, machines)), 'Cline on reet at studio · main · fresh worktree · thinking low')
})

test('several projects send one run request, never YOLO', () => {
  const r = 'r'.repeat(36)
  const many = choice({ agent: 'cline', model: ' sonnet ', effort: 'high', projectIds: ['p-reet', 'p-site', 'p-notes'], prompt: 'Fix the failing tests', approvals: 'yolo', branch: 'release' })
  assert.deepEqual(toSelectionRun(many, r), { requestId: r, projectIds: ['p-reet', 'p-site', 'p-notes'], tool: 'cline', model: 'sonnet', effort: 'high', prompt: 'Fix the failing tests', worktree: true })
  // Effort only for an agent that takes it; no model is ''; the checkout is worktree false.
  assert.deepEqual(toSelectionRun(choice({ agent: 'claude', model: '', effort: 'high', projectIds: ['p-reet', 'p-site'], worktree: false, prompt: '' }), r),
    { requestId: r, projectIds: ['p-reet', 'p-site'], tool: 'claude', model: '', effort: '', prompt: '', worktree: false })
})

test('the started line counts only what the controller says started', () => {
  const m = state => ({ state })
  assert.equal(startedLine({ members: [m('started'), m('failed'), m('pending')] }), '1 of 3 started')
  assert.equal(startedLine({ members: [m('started'), m('removed'), m('failed')] }), '2 of 3 started')
  assert.equal(startedLine({ members: [m('pending'), m('pending')] }), '0 of 2 started')
})

test('a selection puts its projects in the sentence and one project its own defaults', () => {
  const start = choice({ agent: 'codex', model: 'gpt-5-codex', projectIds: ['p-reet'], prompt: 'keep me' })
  const many = withSelection(start, ['p-site', 'p-notes'], projects, profile)
  assert.deepEqual(many.projectIds, ['p-site', 'p-notes'])
  assert.equal(many.agent, 'codex')
  assert.equal(many.prompt, 'keep me')
  assert.equal(words(sentence(many, projects, machines)), "Run Codex with gpt-5-codex on 2 projects at each project's machine from each project's default in a fresh worktree")
  const one = withSelection(many, ['p-site'], projects, profile)
  assert.deepEqual(one.projectIds, ['p-site'])
  // Nothing chosen: the sentence goes back to the first project it named.
  assert.deepEqual(withSelection(many, [], projects, profile).projectIds, ['p-site'])
  assert.deepEqual(withSelection(start, [], projects, profile).projectIds, ['p-reet'])
})

test('a start in several projects words its own refusals', () => {
  const cases = [
    [409, 'run_in_progress', 'One of those projects is still starting. Wait for it to finish, then start again.'],
    [422, 'invalid_selection', 'Choose 2 to 12 different projects.'],
    [409, 'yolo_refused', "reet doesn't start YOLO sessions yet."],
    [422, 'invalid_effort', "Choose low, medium or high thinking, or the agent's default."],
    [422, 'invalid_model', "That model name isn't one reet can pass on. Use letters, digits and . _ : / @ + -, up to 100 characters, starting with a letter or digit."],
    [422, 'request_id_required', 'Starting needs a request id: 16 to 128 letters, digits, hyphens or underscores.'],
    [422, 'invalid_prompt', 'The prompt must be valid text of at most 2,000 bytes, without control characters other than line breaks, and must not start with a dash.'],
    [404, 'project_not_found', "That project isn't on this account."],
    [429, 'project_rate_limited', 'Slow down a moment, then try again.'],
    [409, 'request_changed', 'That request changed. Start it again.'],
    [422, 'prompt_unsupported', 'Write the opening prompt as at least two words.'],
    [409, 'launch_option_unsupported', "That agent doesn't take that option."],
  ]
  for (const [status, code, want] of cases) assert.equal(runRefusalText(status, code, 'server words'), want, code)
  // A refusal it doesn't know keeps the server's own sentence rather than a vague one.
  assert.equal(runRefusalText(422, 'something_new', 'Choose fewer projects.'), 'Choose fewer projects.')
  assert.equal(runRefusalText(401, 'x', 'no'), "You've been signed out. Sign in again to continue.")
})
