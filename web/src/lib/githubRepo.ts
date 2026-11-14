import type { FolderProject } from './terminal/api'

// A project from GitHub (docs/projects.md): the person names a repository and a machine; the machine clones it into
// ~/reet/repos/<owner>/<repo> with its own GitHub sign-in and the controller saves it as a project. The clone runs on the
// controller as a job that the page reads every two seconds until it ends.

// The same names GitHub takes and the controller checks (terminalproto.ValidGitHubOwner, ValidGitHubRepo): ASCII only.
const OWNER = /^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$/
const REPO = /^[A-Za-z0-9._-]{1,100}$/
// What Go's strings.TrimSpace takes off each end (the Unicode White_Space set). JavaScript's trim differs: it also takes
// the byte-order mark and leaves the next-line control, so the controller's set is spelled out here.
const SPACE = '[\\t\\n\\v\\f\\r \\u0085\\u00a0\\u1680\\u2000-\\u200a\\u2028\\u2029\\u202f\\u205f\\u3000]'
const EDGES = new RegExp(`^${SPACE}+|${SPACE}+$`, 'g')
const CONTROL = /\p{Cc}/u
const PREFIX = 'https://github.com/'

function validRepo(s: string): boolean {
  return REPO.test(s) && s.replace(/^\.+|\.+$/g, '') !== '' && !s.endsWith('.') && !s.endsWith('.git')
}

/**
 * The repository a person named, read exactly as the controller reads it (terminalproto.ParseGitHubRepo):
 * `owner/repo` or `https://github.com/owner/repo`, each with an optional `.git` and, on the link only, one trailing
 * slash; the host in any letter case. Anything else (another host or scheme, a port, a sign-in, a query, more path) is
 * null.
 */
export function parseGitHubRepo(input: string): { owner: string; repo: string } | null {
  let s = input.replace(EDGES, '')
  if (CONTROL.test(s)) return null
  if (s.slice(0, PREFIX.length).replace(/[A-Z]/g, c => c.toLowerCase()) === PREFIX) {
    s = s.slice(PREFIX.length)
    if (s.endsWith('/')) s = s.slice(0, -1)
  }
  const slash = s.indexOf('/')
  if (slash < 0) return null
  const owner = s.slice(0, slash)
  let repo = s.slice(slash + 1)
  if (repo.endsWith('.git')) repo = repo.slice(0, -4)
  return OWNER.test(owner) && validRepo(repo) ? { owner, repo } : null
}

/** What is said when the controller no longer knows a clone this page started (it restarted mid-clone, or the job
 *  expired): the page cannot tell how it ended. `repo` is owner/name. */
export const lostCloneLine = (repo: string): string => `reet lost track of cloning ${repo}. Add it again; if the clone finished, reet picks it up.`

/** Where the machine puts the clone, as the person reads it. */
export const cloneFolder = (r: { owner: string; repo: string }): string => `~/reet/repos/${r.owner}/${r.repo}`

/** A clone this page started: the controller's job id, the machine (enrollment id and name) and the repository. */
export type OutstandingClone = { id: string; host: string; machine: string; repo: string }
export type CloneStatus = { state: 'cloning' } | { state: 'done'; project: FolderProject } | { state: 'failed'; code: string; sentence: string }
/** How a watched clone ended: as the controller said, `gone` when it no longer knows the job, `signedout` on a 401. */
export type CloneEnd = Exclude<CloneStatus, { state: 'cloning' }> | { state: 'gone' } | { state: 'signedout'; error: unknown }

export const CLONE_POLL_MS = 2000

// The page's one outstanding clone, kept in memory for the page's life only: never in browser storage.
let outstanding: OutstandingClone | null = null
export const outstandingClone = (): OutstandingClone | null => outstanding
export function rememberClone(clone: OutstandingClone | null): void { outstanding = clone }
function forget(id: string) { if (outstanding?.id === id) outstanding = null }

/**
 * Reads a clone's state every `interval` until it ends, then forgets it and calls `onend` once. A read that does not
 * arrive is tried again at the same pace; aborting `signal` stops the reads and keeps the clone outstanding, so a page
 * that opens later can watch it again. A signed-out answer keeps it too.
 */
export function watchClone(clone: OutstandingClone, { status, onend, interval = CLONE_POLL_MS, signal }: {
  status: (id: string, signal?: AbortSignal) => Promise<CloneStatus>
  onend: (end: CloneEnd) => void
  interval?: number
  signal?: AbortSignal
}): void {
  let timer: ReturnType<typeof setTimeout> | undefined
  const next = () => { timer = setTimeout(read, interval) }
  signal?.addEventListener('abort', () => clearTimeout(timer), { once: true })
  async function read() {
    if (signal?.aborted) return
    let answer: CloneStatus
    try {
      answer = await status(clone.id, signal)
    } catch (e) {
      if (signal?.aborted) return
      const code = (e as { status?: number })?.status
      if (code === 404) { forget(clone.id); onend({ state: 'gone' }); return }
      if (code === 401) { onend({ state: 'signedout', error: e }); return }
      next()
      return
    }
    if (signal?.aborted) return
    if (answer?.state === 'done' && answer.project) { forget(clone.id); onend(answer); return }
    if (answer?.state === 'failed') { forget(clone.id); onend(answer); return }
    next()
  }
  if (!signal?.aborted) next()
}
