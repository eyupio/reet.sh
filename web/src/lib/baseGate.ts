import { isolatedLaunch } from './agentLaunch'
import { baseSentence, needsConfirm } from './baseNotice'
import { checkBase, type FolderProject, type LaunchRequest } from './terminal/api'

// The one step every browser start path takes before it creates an isolated agent session: ask the machine where the
// session would start, and hand back the sentence to confirm, or nothing. Home's launch bar and the Start an agent
// dialog both use this and show the same BaseConfirm dialog, so the rule and the words live in one place.
//
// The check is a courtesy, never a gate: a machine too old to answer it, a busy or rate-limited one, a refusal and a
// reply that never comes all come back as nothing to ask, and the start goes on. Nothing here starts or changes
// anything; the only write on the machine is the fetch updating its remote-tracking branch.

/** The launch fields the question depends on: what will run, where, and the base it was given. */
export type BaseLaunch = Pick<LaunchRequest, 'tool' | 'dir' | 'resume' | 'resumeAcp' | 'worktree' | 'base'>
/** The saved project a launch names, as far as this question cares. */
export type BaseProject = { isRepo?: boolean; cloned?: boolean }
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
 * The question to put to the person before this launch starts, or '' when there is none: the launch will not be
 * isolated, the base and GitHub hold the same commit, GitHub has no such branch, the machine could not answer, or the
 * repository is one reet cloned itself. `host` is the machine's name, which the sentence uses.
 */
export async function baseQuestion(agentId: string, host: string, launch: BaseLaunch, project: BaseProject | null | undefined, signal?: AbortSignal): Promise<string> {
  if (!isolatedLaunch(launch, project)) return ''
  try {
    const state = await checkBase(agentId, launch.dir ?? '', launch.base || undefined, signal)
    const say = baseSentence(state, host)
    return say && needsConfirm(state, { cloned: project?.cloned }) ? say : ''
  } catch {
    return ''
  }
}
