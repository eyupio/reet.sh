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
const models = asURL(compile('./models.ts').replace("from './catalogue'", `from '${catalogue}'`))
const source = compile('./launchBar.ts')
  .replace("from './catalogue'", `from '${catalogue}'`)
  .replace("from './agentLaunch'", `from '${agentLaunch}'`)
  .replaceAll("from './terminal/api'", `from '${terminalApi}'`)
  .replace("from './models'", `from '${models}'`)
const { initialChoice, afterLaunch, baseBranchFor, baseFor, sentence, startable, toCreate, defaultTitle, sendKey, projectSlug, lastChoice, withProject, withAgent, withHome, summary, toSelectionRun, startedLine, withSelection, runRefusalText, yoloGates, yoloCanStart } = await import(asURL(source))
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
  const next = initialChoice(projects, profile)
  assert.equal(next.approvals, 'ask')
  assert.equal(next.prompt, '')
  assert.equal(next.agent, 'claude')
  assert.equal(next.model, '')
  assert.deepEqual(next.projectIds, ['p-reet'])
  assert.equal(next.branch, '')
  const after = afterLaunch(last)
  assert.equal(after.approvals, 'ask')
  assert.equal(after.prompt, '')
  assert.equal(after.agent, 'claude')
})

test('first choice comes from the project, account, favourite and agent defaults', () => {
  assert.deepEqual(initialChoice(projects, profile), { agent: 'claude', model: '', projectIds: ['p-reet'], branch: '', worktree: true, effort: '', prompt: '', approvals: 'ask' })
  const own = { ...reet, defaultTool: 'codex', defaultModel: 'gpt-5-codex', defaultEffort: 'high', baseBranch: 'release', worktreeDefault: 'no' }
  assert.deepEqual(initialChoice([own], { defaultBaseBranch: 'develop', worktreesByDefault: true }), { agent: 'codex', model: 'gpt-5-codex', projectIds: ['p-reet'], branch: 'release', worktree: false, effort: '', prompt: '', approvals: 'ask' })
  assert.equal(initialChoice([reet], { defaultBaseBranch: 'develop', worktreesByDefault: false }).worktree, false)
  assert.equal(initialChoice([reet], { defaultBaseBranch: 'develop', worktreesByDefault: false }).branch, 'develop')
  assert.equal(initialChoice([{ ...reet, worktreeDefault: 'yes' }], { worktreesByDefault: false }).worktree, true)
  assert.deepEqual(initialChoice([], profile).projectIds, [])
  // The account applies only when the project names no agent; its model only with its agent.
  assert.deepEqual([initialChoice([reet], { defaultTool: 'codex', defaultModel: 'gpt-5-codex' }).agent, initialChoice([reet], { defaultTool: 'codex', defaultModel: 'gpt-5-codex' }).model], ['codex', 'gpt-5-codex'])
  assert.equal(initialChoice([reet], { defaultTool: 'codex', defaultModel: 'gpt-5-codex' }).model, 'gpt-5-codex')
  // A favourite marked for the agent fills an otherwise empty model.
  assert.equal(initialChoice([reet], profile, { favourites: [{ tool: 'claude', model: 'opus', agentDefault: true }] }).model, 'opus')
  assert.equal(initialChoice([reet], { defaultTool: 'codex', defaultModel: 'gpt-5-codex' }, { favourites: [{ tool: 'claude', model: 'opus', agentDefault: true }] }).model, 'gpt-5-codex')
  // First project's machine decides the first available agent.
  assert.equal(initialChoice([reet], profile, { machines: [machine('m-studio', 'studio', { tools: ['codex'] })] }).agent, 'codex')
  // Effort only for an agent that takes it; a project's effort needs its agent too.
  assert.equal(initialChoice([{ ...reet, defaultTool: 'cline', defaultEffort: 'high' }], profile).effort, 'high')
  assert.equal(initialChoice([{ ...reet, defaultTool: 'claude', defaultEffort: 'high' }], profile).effort, '')
  assert.equal(initialChoice([{ ...reet, defaultModel: 'sonnet', defaultEffort: 'high' }], profile).effort, '')
})

test('opening uses the resolved defaults, never the last launch', () => {
  const sessions = [{ id: 's', agentId: 'm-studio', createdAt: 5, tool: 'codex', model: 'gpt-5-codex', projectId: 'p-reet' }]
  assert.deepEqual([lastChoice(sessions, projects, profile).agent, lastChoice(sessions, projects, profile).model], ['codex', 'gpt-5-codex'])
  const opened = initialChoice(projects, profile)
  assert.deepEqual([opened.agent, opened.model], ['claude', ''])
})

test('Enter sends on a hardware keyboard; Shift+Enter adds a line; touch never sends', () => {
  const key = (extra = {}) => ({ key: 'Enter', ctrlKey: false, metaKey: false, shiftKey: false, altKey: false, isComposing: false, ...extra })
  // A hardware keyboard: Enter sends, as do Ctrl+Enter and Cmd+Enter.
  assert.equal(sendKey(key(), false), true)
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
  assert.equal(startable(choice({ agent: 'claude', approvals: 'yolo' }), projects, [machine('m-studio', 'studio', { yolo: true })]), "That agent doesn't take that option.")
  assert.equal(startable(choice({ agent: 'cline', approvals: 'yolo' }), projects, machines), "studio doesn't accept YOLO sessions.")
  assert.equal(startable(choice({ agent: 'cline', approvals: 'yolo' }), projects, [machine('m-studio', 'studio', { yolo: true })]), null)
  const chat = choice({ agent: 'opencode', chat: true, trust: true, approvals: 'yolo', model: '' })
  assert.equal(startable(chat, projects, [machine('m-studio', 'studio', { acp: true, yolo: true, tools: ['opencode'] })]), null)
  assert.equal(startable(chat, projects, [machine('m-studio', 'studio', { acp: true, tools: ['opencode'] })]), "studio doesn't accept YOLO sessions.")
  assert.match(startable({ ...chat, model: 'bad model' }, projects, [machine('m-studio', 'studio', { acp: true, yolo: true, tools: ['opencode'] })]), /model name/)
  assert.equal(startable(choice({ projectIds: [], homeMachine: 'm-desk' }), projects, machines), null)
  assert.equal(startable(choice({ projectIds: ['p-reet', 'p-site'] }), projects, [machines[0], machine('m-server', 'server', {}, false)]), 'The machine is reconnecting. Existing shells will be kept.')
})

test('yolo gates read the account, each machine, the agent and the worktree', () => {
  const setting = { allowed: true, changedBy: 'Sam' }
  const gates = yoloGates(choice({ agent: 'cline', approvals: 'yolo' }), setting, projects, [machine('m-studio', 'studio', { yolo: true })])
  assert.equal(yoloCanStart(gates), true)
  assert.deepEqual(gates.map(g => [g.label, g.ok]), [
    ['Your account allows YOLO', true],
    ['studio accepts YOLO sessions', true],
    ['This agent has an option reet can use', true],
    ['New worktree and branch for this session', true],
  ])
  assert.equal(yoloCanStart(yoloGates(choice({ agent: 'cline' }), { allowed: false }, projects, machines)), false)
  assert.equal(yoloCanStart(yoloGates(choice({ agent: 'cline' }), setting, projects, machines)), false)
  assert.equal(yoloCanStart(yoloGates(choice({ agent: 'claude' }), setting, projects, [machine('m-studio', 'studio', { yolo: true })])), false)
  const several = yoloGates(choice({ agent: 'cline', projectIds: ['p-reet', 'p-site'] }), setting, projects, [machine('m-studio', 'studio', { yolo: true }), machine('m-server', 'server', { yolo: false })])
  assert.equal(yoloCanStart(several), false)
  assert.equal(several.filter(g => g.label.endsWith('accepts YOLO sessions')).length, 2)
  const checkout = yoloGates(choice({ agent: 'cline', worktree: false }), setting, projects, [machine('m-studio', 'studio', { yolo: true })])
  assert.equal(yoloCanStart(checkout), true)
  assert.equal(checkout.at(-1).warning, true)
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
  assert.equal(initialChoice([reet], profile).branch, '')
  assert.equal(words(sentence(unset, projects, machines, 'main')), 'Run Claude Code with sonnet on reet at studio from main in a fresh worktree')
  // A base the project sets is sent, and an old machine is told to update.
  const set = initialChoice([{ ...reet, baseBranch: 'release' }], profile)
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
})

test('choosing a project re-derives the agent, model, effort, branch and worktree', () => {
  const start = choice({ prompt: 'Fix the tests', effort: '', homeMachine: 'm-desk', projectIds: [] })
  const next = withProject(start, { ...site, baseBranch: 'trunk', worktreeDefault: 'no' }, profile)
  assert.deepEqual(next, { agent: 'claude', model: '', projectIds: ['p-site'], branch: 'trunk', worktree: false, effort: '', prompt: 'Fix the tests', approvals: 'ask' })
  assert.equal(withProject(start, site, { defaultBaseBranch: 'develop', worktreesByDefault: false }).branch, 'develop')
  assert.equal(withProject(start, site, { worktreesByDefault: false }).worktree, false)
  // The project's own default agent and model come with it; effort is dropped for an agent without it.
  const own = withProject(choice({ agent: 'cline', effort: 'high' }), { ...site, defaultTool: 'codex', defaultModel: 'gpt-5-codex' }, profile)
  assert.deepEqual([own.agent, own.model, own.effort], ['codex', 'gpt-5-codex', ''])
  // An account model only applies with its own agent; a project model without its tool never applies.
  assert.equal(withProject(choice({ agent: 'codex' }), site, { defaultTool: 'codex', defaultModel: 'gpt-5-codex' }).model, 'gpt-5-codex')
  assert.equal(withProject(choice({ agent: 'claude' }), site, { defaultTool: 'codex', defaultModel: 'gpt-5-codex' }).model, '')
  assert.equal(withProject(choice({ agent: 'codex' }), { ...site, defaultModel: 'gpt-5-codex' }, profile).model, '')
  // A favourite marked for the agent fills the gap.
  assert.equal(withProject(choice({ agent: 'claude' }), site, profile, { favourites: [{ tool: 'claude', model: 'opus', agentDefault: true }] }).model, 'opus')
  // Explicit model wins over the project and account defaults for the same agent; a new project agent re-resolves it.
  assert.equal(withProject(choice({ model: 'sonnet' }), site, profile, { explicitModel: true }).model, 'sonnet')
  assert.equal(withProject(choice({ model: 'sonnet' }), { ...site, defaultTool: 'claude', defaultModel: 'opus' }, { defaultTool: 'claude', defaultModel: 'haiku' }, { explicitModel: true }).model, 'sonnet')
  assert.equal(withProject(choice({ model: 'sonnet' }), { ...site, defaultTool: 'codex', defaultModel: 'gpt-5-codex' }, profile, { explicitModel: true }).model, 'gpt-5-codex')
  // Explicit effort survives a project change for the same agent; otherwise the chain decides.
  assert.equal(withProject(choice({ agent: 'cline', effort: 'high' }), { ...site, defaultTool: 'cline' }, { defaultTool: 'cline', defaultEffort: 'low' }, { explicitEffort: true }).effort, 'high')
  assert.equal(withProject(choice({ agent: 'cline', effort: 'high' }), { ...site, defaultTool: 'cline' }, { defaultTool: 'cline', defaultEffort: 'low' }).effort, 'low')
  const home = withHome(choice({ model: 'sonnet' }), 'm-server', { defaultTool: 'claude', defaultModel: 'haiku' })
  assert.deepEqual([home.projectIds, home.homeMachine, home.branch, home.model], [[], 'm-server', '', 'haiku'])
})

test('an agent switch resolves that agent\'s model and effort, never the old model', () => {
  const switched = withAgent(choice({ agent: 'claude', model: 'sonnet', effort: 'high' }), 'cline', reet, profile, { favourites: [{ tool: 'cline', model: 'cline-model', agentDefault: true }] })
  assert.deepEqual([switched.agent, switched.model, switched.effort], ['cline', 'cline-model', ''])
  const account = withAgent(choice({ agent: 'claude', model: 'sonnet' }), 'codex', reet, { defaultTool: 'codex', defaultModel: 'gpt-5-codex' })
  assert.deepEqual([account.agent, account.model], ['codex', 'gpt-5-codex'])
  const mismatch = withAgent(choice(), 'gemini', { ...reet, defaultTool: 'codex', defaultModel: 'gpt-5-codex' }, profile)
  assert.deepEqual([mismatch.agent, mismatch.model], ['gemini', ''])
  const effort = withAgent(choice(), 'cline', reet, { defaultTool: 'cline', defaultEffort: 'medium' })
  assert.deepEqual([effort.model, effort.effort], ['', 'medium'])
})

test('a model-list failure never changes startable', () => {
  assert.equal(startable(choice({ model: 'not-in-any-list' }), projects, machines), null)
  const withModels = [{ ...machines[0], capabilities: { ...caps(), models: true } }]
  assert.equal(startable(choice({ model: 'not-in-any-list' }), projects, withModels), null)
  assert.equal(toCreate(choice({ model: 'not-in-any-list' }), reet, 'r'.repeat(16), 'a'.repeat(16)).launch.model, 'not-in-any-list')
})

test('the phone summary is the sentence in one line', () => {
  assert.equal(summary(sentence(choice(), projects, machines)), 'Claude Code on reet at studio · main · fresh worktree')
  assert.equal(summary(sentence(choice({ worktree: false }), projects, machines, 'main')), 'Claude Code on reet at studio · the checkout')
  assert.equal(summary(sentence(choice({ projectIds: ['p-notes'] }), projects, machines)), 'Claude Code on notes at desk')
  assert.equal(summary(sentence(choice({ agent: 'cline', effort: 'low' }), projects, machines)), 'Cline on reet at studio · main · fresh worktree · thinking low')
})

test('several projects send one run request, with YOLO when chosen', () => {
  const r = 'r'.repeat(36)
  const many = choice({ agent: 'cline', model: ' sonnet ', effort: 'high', projectIds: ['p-reet', 'p-site', 'p-notes'], prompt: 'Fix the failing tests', approvals: 'yolo', branch: 'release' })
  assert.deepEqual(toSelectionRun(many, r), { requestId: r, projectIds: ['p-reet', 'p-site', 'p-notes'], tool: 'cline', model: 'sonnet', effort: 'high', prompt: 'Fix the failing tests', worktree: true, yolo: true })
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
  assert.equal(one.model, '')
  assert.equal(withSelection(many, ['p-site'], projects, profile, { explicitModel: true }).model, 'gpt-5-codex')
  // Nothing chosen: the sentence goes back to the first project it named.
  assert.deepEqual(withSelection(many, [], projects, profile).projectIds, ['p-site'])
  assert.deepEqual(withSelection(start, [], projects, profile).projectIds, ['p-reet'])
})

test('a start in several projects words its own refusals', () => {
  const cases = [
    [409, 'run_in_progress', 'One of those projects is still starting. Wait for it to finish, then start again.'],
    [422, 'invalid_selection', 'Choose 2 to 12 different projects.'],
    [409, 'yolo_not_allowed', 'An owner or admin can allow YOLO for this account in Settings.'],
    [409, 'yolo_refused', "That machine doesn't accept YOLO sessions."],
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

test('Claude gets a native chat only from a machine that reports the reviewed adapter as available', () => {
  const claudeAgent = (extra = {}) => ({ tool: 'claude', adapter: 'claude-agent-acp', state: 'available', minimumVersion: '0.99.1', model: true, approval: true, ...extra })
  const optedIn = machine('m-studio', 'studio', { tools: ['claude'], acp: true, acpAgents: [claudeAgent()] })
  const claudeProject = { ...reet, defaultTool: 'claude' }
  const start = initialChoice([claudeProject], profile, { machines: [optedIn] })
  assert.equal(start.agent, 'claude')
  assert.equal(start.chat, true)
  const create = toCreate(start, claudeProject, 'r'.repeat(16), 'a'.repeat(16))
  assert.equal(create.launch.structured, true)
  // OpenCode's plugin warning does not apply: no trust is asked for and none is sent.
  assert.equal(startable(start, [claudeProject], [optedIn], profile), null)
  assert.equal(create.launch.trust, undefined)
  // Even a stale trust flag on the choice is never sent for Claude.
  assert.equal(toCreate({ ...start, trust: true }, claudeProject, 'r'.repeat(16), 'a'.repeat(16)).launch.trust, undefined)
  // A dark machine (Claude only unverified), the wrong adapter, or a peer with no descriptors never gets a chat.
  for (const agents of [[claudeAgent({ state: 'unverified' })], [claudeAgent({ adapter: 'opencode' })], [claudeAgent({ state: 'missing' })], []]) {
    const dark = machine('m-studio', 'studio', { tools: ['claude'], acp: true, acpAgents: agents })
    assert.notEqual(initialChoice([claudeProject], profile, { machines: [dark] }).chat, true)
  }
  const legacy = machine('m-studio', 'studio', { tools: ['claude'], acp: true, acpHistory: true })
  assert.notEqual(initialChoice([claudeProject], profile, { machines: [legacy] }).chat, true)
  assert.match(startable({ ...start, chat: true }, [claudeProject], [legacy], profile), /does not offer Gaffer/)
  // A tool that is not a reviewed chat tool is never sent as one, whatever the choice says.
  assert.equal(toCreate({ ...start, agent: 'codex', chat: true }, claudeProject, 'r'.repeat(16), 'a'.repeat(16)).launch.structured, undefined)
})

test('OpenCode on an acp machine launches a native chat by default', () => {
  const acp = machine('m-studio', 'studio', { tools: ['opencode'], acp: true, acpHistory: true })
  const openProject = { ...reet, defaultTool: 'opencode' }
  const start = initialChoice([openProject], profile, { machines: [acp] })
  assert.equal(start.agent, 'opencode')
  assert.equal(start.chat, true)
  assert.equal(words(sentence(start, [openProject], [acp])), 'Run OpenCode in Gaffer with default on reet at studio')
  assert.deepEqual(slots(sentence(start, [openProject], [acp])), ['agent', 'chat', 'model', 'project', 'machine'])
  const create = toCreate(start, openProject, 'r'.repeat(16), 'a'.repeat(16))
  assert.equal(create.launch.structured, true)
  assert.equal(create.launch.trust, undefined)
  assert.equal(create.launch.model, undefined)
  assert.equal(create.launch.worktree, undefined)
  assert.equal(create.launch.effort, undefined)
  const chosenModel = { ...start, model: 'anthropic/claude-sonnet-4-5' }
  assert.equal(toCreate(chosenModel, openProject, 'r'.repeat(16), 'a'.repeat(16)).launch.model, 'anthropic/claude-sonnet-4-5')
  assert.equal(startable({ ...chosenModel, trust: true }, [openProject], [acp], profile), null)
  assert.match(startable(start, [openProject], [acp], profile), /trust this folder/)
  const trusted = { ...start, trust: true }
  assert.equal(toCreate(trusted, openProject, 'r'.repeat(16), 'a'.repeat(16)).launch.trust, true)
  assert.equal(startable(trusted, [openProject], [acp], profile), null)
  assert.match(startable({ ...trusted, projectIds: ['p-reet', 'p-site'] }, projects, [acp, machines[1]], profile), /one folder at a time/)

  const terminal = { ...start, chat: false }
  assert.match(words(sentence(terminal, [openProject], [acp])), /in OpenCode TUI with/)
  assert.equal(toCreate(terminal, openProject, 'r'.repeat(16), 'a'.repeat(16)).launch.structured, undefined)
})

test('without acp OpenCode stays a terminal and history is absent', () => {
  const openProject = { ...reet, defaultTool: 'opencode' }
  const start = initialChoice([openProject], profile, { machines: [] })
  assert.equal(start.chat, undefined)
  assert.equal(words(sentence(start, [openProject], [])).includes('Gaffer'), false)
  const moved = withAgent(choice(), 'opencode', openProject, profile, { machines: [] })
  assert.equal('chat' in moved, false)
  const acp = machine('m-studio', 'studio', { tools: ['opencode'], acp: true, acpHistory: true })
  assert.equal(withProject(choice({ agent: 'claude' }), openProject, profile, { machines: [acp] }).chat, true)
  const explicitTerminal = withProject({ ...start, chat: false }, openProject, profile, { machines: [acp] })
  assert.equal(explicitTerminal.chat, false)
  assert.equal(withHome(choice({ agent: 'opencode' }), 'm-studio', profile, { machines: [acp] }).chat, true)
})

test('explicit per-agent chat descriptors govern defaults and crafted launch choices', () => {
 const chatProject = project('chat-project', 'chat', 'chat-machine')
 const target = machine('chat-machine', 'chat-host', { tools: ['opencode', 'codex'], acp: true, acpAgents: [] })
 const defaults = { defaultTool: 'opencode', worktreesByDefault: false }
 const opts = { machines: [target] }
 assert.equal(initialChoice([chatProject], defaults, opts).chat, undefined)
 const selected = choice({ agent: 'opencode', model: '', projectIds: [chatProject.id], worktree: false, chat: true, trust: true })
 assert.match(startable(selected, [chatProject], [target]), /does not offer/)
 target.capabilities.acpAgents = [{ tool: 'opencode', adapter: 'opencode', state: 'available' }]
 assert.equal(initialChoice([chatProject], defaults, opts).chat, true)
 assert.equal(startable(selected, [chatProject], [target]), null)
 assert.match(startable({ ...selected, agent: 'codex' }, [chatProject], [target]), /does not offer/)
 target.capabilities.acpAgents[0].state = 'incompatible'
 assert.match(startable(selected, [chatProject], [target]), /does not offer/)
})
