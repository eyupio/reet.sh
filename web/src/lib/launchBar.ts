import type { Profile } from './api'
import type { BranchesResult, FolderProject, LaunchRequest, TerminalMachine } from './terminal/api'
import { readiness } from './terminal/api'
import { CATALOGUE, agentChoices } from './catalogue'
import { cleanModel, codeSentence, titleFor, toolName, validateModel, validatePrompt } from './agentLaunch'

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
  /** The base branch for a fresh worktree; '' sends nothing (the worktree starts from the checkout's current commit). */
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

/** The base branch for a project's fresh worktree: the project's own, else the account's, else the repository's default, else its current branch. */
export function baseBranchFor(project: Pick<FolderProject, 'baseBranch'> | null | undefined, profile: LaunchDefaults, branches: BranchesResult | null | undefined): string {
  return project?.baseBranch || profile.defaultBaseBranch || branches?.default || branches?.current || ''
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

/** After a launch: the same slots for a second session, the prompt cleared and Approvals back on *Ask me*. */
export function afterLaunch(choice: LaunchChoice): LaunchChoice {
  return { ...choice, prompt: '', approvals: 'ask' }
}

/**
 * The launch sentence: "Run [agent] with [model] on [project] at [machine] from [branch] in [a fresh worktree] · thinking
 * [effort]". A checkout has no branch slot and ends "Works in the checkout on main." (`current` is the checkout's branch
 * from `listBranches`, when known); a folder that isn't a repository (and the home folder) has neither branch nor worktree.
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
      const branch = picked.length > 1 ? "each project's default" : choice.branch || 'the current branch'
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
export function startable(choice: LaunchChoice, projects: LaunchProject[], machines: LaunchMachine[]): string | null {
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
    ? picked.map(p => ({ agentId: p.host, repo: p.isRepo, base: p.isRepo && choice.worktree && (picked.length > 1 ? !!p.baseBranch : !!choice.branch) }))
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
 * One session's create request. `base` only with a fresh worktree and a chosen branch, `effort` only for an agent that
 * takes it, `yolo` only when chosen; nothing else is added. The project folder is sent as it is, whatever its name.
 */
export function toCreate(choice: LaunchChoice, project: LaunchProject | null, requestId: string, runToken: string): LaunchCreate {
  const launch: LaunchRequest = { tool: choice.agent }
  const model = cleanModel(choice.model)
  if (model) launch.model = model
  if (project) launch.dir = project.dir
  if (choice.prompt) launch.prompt = choice.prompt
  if (project) launch.projectId = project.id
  if (project?.isRepo && choice.worktree) {
    launch.worktree = runToken
    launch.project = projectSlug(project.name)
    if (choice.branch) launch.base = choice.branch
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

/** R8: Enter adds a line everywhere (and never sends on touch); Ctrl+Enter or Cmd+Enter on a hardware keyboard sends. */
export function sendKey(e: Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'shiftKey' | 'altKey' | 'isComposing'>, touch: boolean): boolean {
  void touch
  return e.key === 'Enter' && !e.isComposing && !e.shiftKey && !e.altKey && (e.ctrlKey || e.metaKey)
}
