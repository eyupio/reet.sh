import { isolatedLaunch } from './agentLaunch'
import { baseSentence, needsConfirm } from './baseNotice'
import { checkBase, type FolderProject, type LaunchRequest } from './terminal/api'

// The one step every browser start path takes before it starts an isolated agent session: ask the machine where the
// session would start, and hand back the sentence to confirm, or nothing. Home's launch bar, the Start an agent dialog
// and Start on a backlog plan all use this and show the same BaseConfirm dialog, so the rule and the words live in one
// place; `baseKey` is how each of them ties a yes to the start it was given for.
//
// The check is a courtesy, never a gate: a machine too old to answer it, a busy or rate-limited one, a refusal and a
// reply that never comes all come back as nothing to ask, and the start goes on. Nothing here starts or changes
// anything; the only write on the machine is the fetch updating its remote-tracking branch.

/** The launch fields the question depends on: what will run, where, and the base it was given. */
export type BaseLaunch = Pick<LaunchRequest, 'tool' | 'dir' | 'resume' | 'resumeAcp' | 'worktree' | 'base'>
/** The saved project a launch names, as far as this question cares: whether it is a repository reet would isolate, its
 *  own saved base branch, and whether reet cloned it itself. */
export type BaseProject = { isRepo?: boolean; cloned?: boolean; baseBranch?: string }
type ProjectRow = BaseProject & Pick<FolderProject, 'host' | 'dir'>

/**
 * The saved repository a launch in this folder would be isolated into, as the controller finds it
 * (`RepoProjectForDir`): the project on this machine whose folder is exactly this one. null when there is none, so a
 * folder nobody saved, another machine's project and a project that is not a repository all read as "not isolated".
 */
export function repoProjectAt<T extends ProjectRow>(projects: T[] | undefined, agentId: string, dir: string): T | null {
  if (!dir) return null
  return projects?.find(p => p.host === agentId && p.dir === dir && p.isRepo === true) ?? null
}

/**
 * What a yes was given for: the machine, folder, base and agent the question was asked about. A start whose own key
 * differs — another project, machine, folder or base — is asked again, so a confirmation can never be spent on a start
 * the person did not see.
 */
export function baseKey(agentId: string, launch: BaseLaunch): string {
  return [agentId, launch.dir ?? '', launch.base ?? '', launch.tool ?? ''].join('\u0000')
}

/**
 * The question to put to the person before this launch starts, or '' when there is none: the launch will not be
 * isolated, the base and GitHub hold the same commit, GitHub has no such branch, the machine could not answer, or the
 * repository is one reet cloned itself. `host` is the machine's name, which the sentence uses. `timeoutMs` shortens the
 * read's own deadline, which only a test does.
 *
 * It asks about exactly the branch this start will use: the base the create carries (the person's choice, the
 * project's base, or the account's default, whichever the page resolved), else — when it carries none — the project's
 * own saved base, which the controller fills in. With neither, no base is named and the machine answers for the branch
 * its remote calls default, which is what it will start from. The account's default base is never added here: the
 * controller does not fill it in either, and one branch name does not fit every repository.
 */
export async function baseQuestion(agentId: string, host: string, launch: BaseLaunch, project: BaseProject | null | undefined, signal?: AbortSignal, timeoutMs?: number): Promise<string> {
  if (!isolatedLaunch(launch, project)) return ''
  try {
    const base = launch.base || project?.baseBranch || undefined
    const state = await checkBase(agentId, launch.dir ?? '', base, signal, timeoutMs)
    const say = baseSentence(state, host)
    return say && needsConfirm(state, { cloned: project?.cloned }) ? say : ''
  } catch {
    return ''
  }
}
