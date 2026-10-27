import type { Profile } from './api'
import type { BranchesResult, FolderProject, LaunchRequest, TerminalMachine, TerminalSession } from './terminal/api'
import { readiness } from './terminal/api'
import { CATALOGUE, agentChoices } from './catalogue'
import { cleanModel, codeSentence, refusalText, titleFor, toolName, validateModel, validatePrompt } from './agentLaunch'

// The launch bar's logic: what the sentence says, what it starts from, whether Start may be pressed, and the one create
// request per session. Pure: no DOM, no fetch, no storage. The controller and the machine check everything again.

export type Effort = '' | 'low' | 'medium' | 'high'
export type LaunchChoice = {
  agent: string
  /** '' is the agent's own default and sends nothing. */
  model: string
  projectIds: string[]
  /** The machine's enrollment id for *Home folder on…*, used only when no project is chosen. */
  homeMachine?: string
  /** The base branch for one project's fresh worktree, only when the person chose it or the project or profile sets one.
   * '' means "use the default": nothing is sent and the worktree starts from the checkout's current commit. A branch
   * suggested from the machine's own list (`baseBranchFor`) is shown, never put here unless the person picks it. */
  branch: string
  worktree: boolean
  effort: Effort
  prompt: string
  /** Never stored: every launch and every fresh choice is back on 'ask'. */
  approvals: 'ask' | 'yolo'
}
/** The person's launch defaults from the profile (`defaultBaseBranch`, `worktreesByDefault`, which is true when absent). */
export type LaunchDefaults = Partial<Pick<Profile, 'defaultBaseBranch' | 'worktreesByDefault'>>
export type LaunchProject = Pick<FolderProject, 'id' | 'name' | 'host' | 'machine' | 'dir' | 'isRepo' | 'baseBranch' | 'worktreeDefault' | 'defaultTool' | 'defaultModel'>
export type LaunchMachine = Pick<TerminalMachine, 'agentId' | 'host' | 'ready' | 'capabilities'>
/** One piece of the sentence: a named slot is a button; slot '' is a plain word between them. */
export type SentencePart = { slot: '' | 'agent' | 'model' | 'project' | 'machine' | 'branch' | 'worktree' | 'checkout' | 'effort'; text: string }
/** What `createTerminal(agentId, requestId, title, cols, rows, signal, launch)` takes for one session. */
export type LaunchCreate = { agentId: string; requestId: string; title: string; launch: LaunchRequest }

const UPDATE_FOR_OPTION = 'Update reet on that machine to use that option.'
const NO_OPTION = "That agent doesn't take that option."
const MAX_TITLE = 80
const MAX_SLUG = 40

const agentOf = (id: string) => CATALOGUE.find(agent => agent.id === id)
const takesEffort = (id: string): boolean => agentOf(id)?.effort === true
const chosen = (choice: LaunchChoice, projects: LaunchProject[]): LaunchProject[] =>
  choice.projectIds.map(id => projects.find(p => p.id === id)).filter((p): p is LaunchProject => !!p)

function worktreeFor(project: LaunchProject | undefined, profile: LaunchDefaults): boolean {
  if (project?.worktreeDefault === 'yes') return true
  if (project?.worktreeDefault === 'no') return false
  return profile.worktreesByDefault !== false
}

/** The branch the picker suggests for a project's fresh worktree: the project's own, else the account's, else the repository's
 * default, else its current branch. For display only: the default and current branches are never sent as `base` (see `baseFor`). */
export function baseBranchFor(project: Pick<FolderProject, 'baseBranch'> | null | undefined, profile: LaunchDefaults, branches: BranchesResult | null | undefined): string {
  return project?.baseBranch || profile.defaultBaseBranch || branches?.default || branches?.current || ''
}

/**
 * The base a project's create sends ('' for none). One project: the branch in the choice. Several: each project's own
 * `baseBranch`, else the account's default, never the single choice's branch. Only a repository with a fresh worktree has one.
 */
export function baseFor(choice: LaunchChoice, project: Pick<LaunchProject, 'isRepo' | 'baseBranch'> | null | undefined, profile: LaunchDefaults = {}): string {
  if (!project?.isRepo || !choice.worktree) return ''
  return choice.projectIds.length > 1 ? project.baseBranch || profile.defaultBaseBranch || '' : choice.branch
}

/**
 * The choice the bar opens with: the last launch's when there was one (its projects that still exist, else the first
 * project), else the first project's defaults and the account's. The prompt starts empty and Approvals on *Ask me*.
 */
export function initialChoice(last: LaunchChoice | null, projects: LaunchProject[], profile: LaunchDefaults): LaunchChoice {
  const first = projects[0]
  if (last) {
    const kept = last.projectIds.filter(id => projects.some(p => p.id === id))
    const home = kept.length === 0 && last.projectIds.length === 0 && last.homeMachine ? last.homeMachine : undefined
    const ids = kept.length > 0 || home ? kept : first ? [first.id] : []
    const same = ids.length === last.projectIds.length && ids.every((id, i) => id === last.projectIds[i])
    const project = projects.find(p => p.id === ids[0])
    const out: LaunchChoice = {
      agent: last.agent, model: last.model, projectIds: ids,
      branch: same ? last.branch : (project?.baseBranch || profile.defaultBaseBranch || ''),
      worktree: same ? last.worktree : worktreeFor(project, profile),
      effort: takesEffort(last.agent) ? last.effort : '', prompt: '', approvals: 'ask',
    }
    if (home) out.homeMachine = home
    return out
  }
  return {
    agent: first?.defaultTool || CATALOGUE[0].id, model: first?.defaultModel ?? '', projectIds: first ? [first.id] : [],
    branch: first?.baseBranch || profile.defaultBaseBranch || '', worktree: worktreeFor(first, profile), effort: '', prompt: '', approvals: 'ask',
  }
}

const EFFORTS: Effort[] = ['low', 'medium', 'high']

/**
 * The last launch, read from the sessions list (the newest session that names an agent): its agent, model, effort and
 * project, with a worktree when it ran in one, or the home folder on its machine when it belonged to no project. Never from
 * browser storage. null when no session names an agent. A project since removed gives no project (`initialChoice` then
 * takes the first one).
 */
export function lastChoice(sessions: Pick<TerminalSession, 'agentId' | 'createdAt' | 'tool' | 'model' | 'effort' | 'projectId' | 'worktreePath'>[], projects: LaunchProject[], profile: LaunchDefaults): LaunchChoice | null {
  const newest = sessions.filter(s => s.tool).reduce<typeof sessions[number] | null>((a, s) => (!a || s.createdAt > a.createdAt ? s : a), null)
  if (!newest?.tool) return null
  const project = newest.projectId ? projects.find(p => p.id === newest.projectId) : undefined
  const effort = takesEffort(newest.tool) && EFFORTS.includes(newest.effort as Effort) ? newest.effort as Effort : ''
  const out: LaunchChoice = {
    agent: newest.tool, model: newest.model ?? '', projectIds: project ? [project.id] : [],
    branch: project?.baseBranch || profile.defaultBaseBranch || '', worktree: project?.isRepo ? !!newest.worktreePath : worktreeFor(project, profile),
    effort, prompt: '', approvals: 'ask',
  }
  if (!newest.projectId) out.homeMachine = newest.agentId
  return out
}

/** The choice with one project in it (the rail's card or the project slot): its base and worktree default, and its own default
 * agent and model when it names one. The prompt and Approvals stay as they are. */
export function withProject(choice: LaunchChoice, project: LaunchProject, profile: LaunchDefaults): LaunchChoice {
  const agent = project.defaultTool || choice.agent
  const { homeMachine: _, ...rest } = choice
  return {
    ...rest, agent, model: project.defaultTool ? project.defaultModel : choice.model, projectIds: [project.id],
    branch: project.baseBranch || profile.defaultBaseBranch || '', worktree: worktreeFor(project, profile), effort: takesEffort(agent) ? choice.effort : '',
  }
}

/**
 * The choice for the rail's selection (in rail order): two or more projects keep the agent, model, worktree and prompt and
 * name them all; one project is that project as its card would choose it; none goes back to the first project the
 * sentence named.
 */
export function withSelection(choice: LaunchChoice, ids: string[], projects: LaunchProject[], profile: LaunchDefaults): LaunchChoice {
  const known = ids.filter(id => projects.some(p => p.id === id))
  if (known.length > 1) {
    const { homeMachine: _, ...rest } = choice
    return { ...rest, projectIds: known }
  }
  const one = projects.find(p => p.id === (known[0] ?? choice.projectIds[0]))
  if (!one) return choice
  if (known.length === 0 && choice.projectIds.length <= 1) return choice
  return withProject(choice, one, profile)
}

/** The choice for *Home folder on* a machine: no project, so no branch. */
export function withHome(choice: LaunchChoice, agentId: string): LaunchChoice {
  return { ...choice, projectIds: [], homeMachine: agentId, branch: '' }
}

/** The phone's one summary line: "Claude Code on reet at studio · main · fresh worktree" (the model is in the Change sheet). */
export function summary(parts: SentencePart[]): string {
  const text = (slot: SentencePart['slot']) => parts.find(p => p.slot === slot)?.text ?? ''
  const out = [`${text('agent')} on ${text('project')} at ${text('machine')}`]
  const worktree = text('worktree')
  if (worktree === 'a fresh worktree') out.push(text('branch'), 'fresh worktree')
  else if (worktree) out.push(worktree)
  if (parts.some(p => p.slot === 'effort')) out.push(`thinking ${text('effort')}`)
  return out.join(' · ')
}

/** After a launch: the same slots for a second session, the prompt cleared and Approvals back on *Ask me*. */
export function afterLaunch(choice: LaunchChoice): LaunchChoice {
  return { ...choice, prompt: '', approvals: 'ask' }
}

/**
 * The launch sentence: "Run [agent] with [model] on [project] at [machine] from [branch] in [a fresh worktree] · thinking
 * [effort]". `current` is the checkout's branch from `listBranches`, when known: an unset branch reads as it (the worktree
 * starts there), and a checkout has no branch slot and ends "Works in the checkout on main."; a folder that isn't a repository (and the home folder) has neither branch nor worktree.
 */
export function sentence(choice: LaunchChoice, projects: LaunchProject[], machines: LaunchMachine[], current = ''): SentencePart[] {
  const picked = chosen(choice, projects)
  const word = (text: string): SentencePart => ({ slot: '', text })
  const hostOf = (agentId: string | undefined, fallback = ''): string => machines.find(m => m.agentId === agentId)?.host || fallback || 'a machine'
  let project: string, machine: string
  if (picked.length > 1) {
    project = `${picked.length} projects`
    const hosts = new Set(picked.map(p => p.host))
    machine = hosts.size === 1 ? hostOf(picked[0].host, picked[0].machine) : "each project's machine"
  } else if (picked.length === 1) {
    project = picked[0].name
    machine = hostOf(picked[0].host, picked[0].machine)
  } else if (choice.homeMachine) {
    project = 'the home folder'
    machine = hostOf(choice.homeMachine)
  } else {
    project = 'a project'
    machine = 'a machine'
  }
  const parts: SentencePart[] = [
    word('Run'), { slot: 'agent', text: toolName(choice.agent || undefined) },
    word('with'), { slot: 'model', text: cleanModel(choice.model) || 'default' },
    word('on'), { slot: 'project', text: project },
    word('at'), { slot: 'machine', text: machine },
  ]
  if (picked.some(p => p.isRepo)) {
    if (choice.worktree) {
      const branch = picked.length > 1 ? "each project's default" : choice.branch || current || 'the current branch'
      parts.push(word('from'), { slot: 'branch', text: branch }, word('in'), { slot: 'worktree', text: 'a fresh worktree' })
    } else {
      parts.push(word('in'), { slot: 'worktree', text: 'the checkout' })
      if (picked.length === 1) parts.push({ slot: 'checkout', text: current ? `Works in the checkout on ${current}.` : 'Works in the checkout.' })
    }
  }
  if (takesEffort(choice.agent)) parts.push(word('·'), word('thinking'), { slot: 'effort', text: choice.effort || 'default' })
  return parts
}

/** Why Start can't be pressed yet, as the one sentence to show; null when it can. The first reason wins. */
export function startable(choice: LaunchChoice, projects: LaunchProject[], machines: LaunchMachine[], profile: LaunchDefaults = {}): string | null {
  if (!choice.agent) return 'Choose an agent.'
  const picked = chosen(choice, projects)
  if (picked.length === 0 && !choice.homeMachine) return 'Choose a project or a home folder.'
  const agent = agentOf(choice.agent)
  if (!agent?.enabled) return agent?.disabledReason ?? "reet doesn't start this agent yet."
  const prompt = validatePrompt(choice.agent, choice.prompt)
  if (prompt) return prompt
  const model = validateModel(choice.model)
  if (model) return model
  if (choice.effort && !agent.effort) return NO_OPTION
  const targets: { agentId: string; repo: boolean; base: boolean }[] = picked.length > 0
    ? picked.map(p => ({ agentId: p.host, repo: p.isRepo, base: !!baseFor(choice, p, profile) }))
    : [{ agentId: choice.homeMachine ?? '', repo: false, base: false }]
  for (const target of targets) {
    const m = machines.find(x => x.agentId === target.agentId)
    const ready = readiness(m as TerminalMachine | undefined)
    if (ready) return ready
    const caps = m!.capabilities!
    const offer = agentChoices(m as TerminalMachine).find(c => c.agent.id === choice.agent)
    if (offer && !offer.available) return offer.reason ?? "reet doesn't start this agent yet."
    if (target.repo && choice.worktree && !caps.worktree) return codeSentence('worktree_unsupported')
    if ((choice.effort || target.base || choice.approvals === 'yolo') && !caps.agentsV2) return UPDATE_FOR_OPTION
    if (choice.approvals === 'yolo' && !caps.yolo) return `${m!.host} doesn't accept YOLO sessions.`
  }
  return null
}

/** A folder- and branch-safe name for the worktree (the controller's Slug rule, without its hash suffix): a-z, 0-9 and single hyphens, at most 40 bytes. */
export function projectSlug(name: string): string {
  const s = String(name ?? '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')
  const cut = s.slice(0, MAX_SLUG).replace(/-+$/, '')
  return cut || 'project'
}

// Control and format characters and the two Unicode line separators; a title may hold none of them.
const INVISIBLE = new RegExp('[\\p{Cc}\\p{Cf}\\u2028\\u2029]', 'gu')

/** The session title from the prompt: its first line with text, without invisible characters, at most 80 characters; `fallback` when there is none. */
export function defaultTitle(prompt: string, fallback = ''): string {
  const line = String(prompt ?? '').split(/\r?\n/).map(l => l.replace(/\t/g, ' ').replace(INVISIBLE, '').trim()).find(l => l !== '') ?? ''
  if (line === '') return fallback
  return Array.from(line).slice(0, MAX_TITLE).join('').trim()
}

/**
 * One session's create request. `base` only as `baseFor` gives it (pass the profile for several projects), `effort` only for an agent that
 * takes it, `yolo` only when chosen; nothing else is added. The project folder is sent as it is, whatever its name.
 */
export function toCreate(choice: LaunchChoice, project: LaunchProject | null, requestId: string, runToken: string, profile: LaunchDefaults = {}): LaunchCreate {
  const launch: LaunchRequest = { tool: choice.agent }
  const model = cleanModel(choice.model)
  if (model) launch.model = model
  if (project) launch.dir = project.dir
  if (choice.prompt) launch.prompt = choice.prompt
  if (project) launch.projectId = project.id
  if (project?.isRepo && choice.worktree) {
    launch.worktree = runToken
    launch.project = projectSlug(project.name)
    const base = baseFor(choice, project, profile)
    if (base) launch.base = base
  }
  if (choice.effort && takesEffort(choice.agent)) launch.effort = choice.effort
  if (choice.approvals === 'yolo') launch.yolo = true
  return {
    agentId: project ? project.host : choice.homeMachine ?? '',
    requestId,
    title: defaultTitle(choice.prompt, titleFor(choice.agent, project?.dir ?? '')),
    launch,
  }
}

/** What a start in several projects sends: one request for a run (POST /api/v2/folder-projects/runs). Each project's base is
 * the controller's to settle (`baseFor`'s rule); YOLO is never sent until reet starts YOLO sessions. */
export type SelectionRunRequest = { requestId: string; projectIds: string[]; tool: string; model: string; effort: Effort; prompt: string; worktree: boolean }
export function toSelectionRun(choice: LaunchChoice, requestId: string): SelectionRunRequest {
  return {
    requestId, projectIds: [...choice.projectIds], tool: choice.agent, model: cleanModel(choice.model),
    effort: takesEffort(choice.agent) ? choice.effort : '', prompt: choice.prompt, worktree: choice.worktree,
  }
}

// The controller's own sentences for refusing a start in several projects (internal/api/project_runs.go), so the bar never
// falls back to a Project's words or a vague line.
const RUN_REFUSALS: Record<string, string> = {
  run_in_progress: 'One of those projects is still starting. Wait for it to finish, then start again.',
  invalid_selection: 'Choose 2 to 12 different projects.',
  yolo_refused: "reet doesn't start YOLO sessions yet.",
  invalid_effort: "Choose low, medium or high thinking, or the agent's default.",
  invalid_model: "That model name isn't one reet can pass on. Use letters, digits and . _ : / @ + -, up to 100 characters, starting with a letter or digit.",
  request_id_required: 'Starting needs a request id: 16 to 128 letters, digits, hyphens or underscores.',
  invalid_prompt: 'The prompt must be valid text of at most 2,000 bytes, without control characters other than line breaks, and must not start with a dash.',
  project_not_found: "That project isn't on this account.",
  project_rate_limited: 'Slow down a moment, then try again.',
}
const SOMETHING_CHANGED = 'Something changed. Check again.'
const PLAIN = /^[^\p{Cc}\p{Cf}\u2028\u2029]{1,300}$/u

/** The sentence for a refused start in several projects: its own words, then a launch's, then the server's sentence as sent. */
export function runRefusalText(status: number, code?: string, message?: string): string {
  if (status !== 401 && code && Object.hasOwn(RUN_REFUSALS, code)) return RUN_REFUSALS[code]
  const known = refusalText(status, code, message)
  const msg = String(message ?? '').trim()
  return known === SOMETHING_CHANGED && PLAIN.test(msg) ? msg : known
}

/** The status sheet's line: how many members the controller says started (a removed worktree's member did start), of all. */
export function startedLine(run: { members: { state: string }[] }): string {
  const started = run.members.filter(m => m.state === 'started' || m.state === 'removed').length
  return `${started} of ${run.members.length} started`
}

/** R8: on touch the Start button is the only way to send, whatever the key. On a hardware keyboard Enter adds a line and
 * Ctrl+Enter or Cmd+Enter sends. */
export function sendKey(e: Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'shiftKey' | 'altKey' | 'isComposing'>, touch: boolean): boolean {
  if (touch) return false
  return e.key === 'Enter' && !e.isComposing && !e.shiftKey && !e.altKey && (e.ctrlKey || e.metaKey)
}
