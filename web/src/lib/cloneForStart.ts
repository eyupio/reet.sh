import { clonedProject, lostCloneLine, parseGitHubRepo, rememberClone, startUnheardLine, timedOut, watchClone, type CloneEnd } from './githubRepo'
import { addFromGitHub, cloneStatus, refusalSentence, TerminalError, type FolderProject } from './terminal/api'

/** What making a repository ready to start in came to: its project, a sentence saying why not, or a lost sign-in. */
export type CloneForStart = { project: FolderProject } | { sentence: string; field?: 'repo' | 'host' } | { signedout: unknown }

/**
 * Makes a GitHub repository ready to start an agent or an outcome in, without the person adding a project first. A
 * project already cloned from that repository on that machine is used as it is; otherwise the machine clones it into
 * ~/reet/repos/owner/name and the controller saves it as a temporary project, as Add a project From GitHub does but
 * removed with its clone once nothing uses it and nothing would be lost, and this waits for that to finish. Aborting
 * `signal` stops the waiting, never the clone: the project still appears when it is done.
 */
export async function cloneForStart(host: string, machine: string, typed: string, projects: readonly FolderProject[], signal: AbortSignal,
  { onwaiting }: { onwaiting?: (line: string) => void } = {}): Promise<CloneForStart> {
  const parsed = parseGitHubRepo(typed)
  if (!parsed) return { sentence: 'Write the repository as owner/name, or paste its github.com link.', field: 'repo' }
  const repo = `${parsed.owner}/${parsed.repo}`
  const known = clonedProject(projects, { host, repo })
  if (known) return { project: known }
  let started: { cloneId: string }
  try {
    // Cloned for this start: reet removes it, with its clone, once nothing uses it and nothing would be lost.
    started = await addFromGitHub(host, repo, '', signal, { temporary: true })
  } catch (e) {
    if (e instanceof TerminalError && e.status === 401) return { signedout: e }
    const field = e instanceof TerminalError && (e.field === 'repo' || e.field === 'host') ? e.field : undefined
    return { sentence: timedOut(e) ? startUnheardLine : refusalSentence(e), field }
  }
  const clone = { id: started.cloneId, host, machine, repo }
  rememberClone(clone)
  onwaiting?.(`Cloning ${repo} on ${machine}… This can take a few minutes; the agent starts when it's done.`)
  const end = await new Promise<CloneEnd>(resolve => {
    signal.addEventListener('abort', () => resolve({ state: 'gone' }), { once: true })
    watchClone(clone, { status: cloneStatus, onend: resolve, signal })
  })
  if (signal.aborted) return { sentence: '' }
  if (end.state === 'done') return { project: end.project }
  if (end.state === 'failed') return { sentence: end.sentence }
  if (end.state === 'signedout') return { signedout: end.error }
  return { sentence: lostCloneLine(repo) }
}
