// Where an isolated session will start, in words. A session in a saved repository works in a worktree of its own, cut
// from the project's base branch after the machine has fetched that branch from GitHub (docs/terminal-worktrees.md). The
// machine says how the two sides stood; this says it in one sentence, and whether the start is worth asking about first.
//
// Facts only: nothing here reads, starts or changes anything, and a relation it does not know is left unsaid rather than
// guessed at. The commit is shown as its first seven characters, as git does.

/** What a machine said about a base branch: the route's answer (`GET /api/v2/machines/{agentId}/base-check`) and the
 *  same three facts a session keeps (`baseName`, `baseRelation`, `baseStart`). Every field can be absent. */
export type BaseState = { base?: string; relation?: string; local?: string; remote?: string; start?: string }

/** The relations worth telling someone about: the session did not start from the base as GitHub has it, or GitHub was
 *  never reached. `same` and `no_remote` need no notice; neither does `no_local`, where the repository has no such
 *  branch of its own, so there is nothing of the person's to be behind or ahead (the controller says the same, in
 *  `baseWorthANotice`). An unknown relation is not put into words. */
const NOTEWORTHY = new Set(['behind', 'ahead', 'diverged', 'fetch_failed'])

/** The commit's short id, as git shows it: the first seven characters of a SHA-1 or SHA-256 id, and nothing at all
 *  for anything that is not one. */
const short = (start: string | undefined): string => {
  const s = String(start ?? '')
  return /^[0-9a-f]{7,64}$/i.test(s) ? ` (${s.slice(0, 7)})` : ''
}

/**
 * One sentence saying where this session starts and why, or null when there is nothing to say (the base and GitHub
 * agree, GitHub has no such branch, or the machine reported a relation this build does not know).
 *
 * `host` is the machine's name. With no branch name (a machine that resolved one it did not report) the sentence says
 * "the base branch" instead.
 */
export function baseSentence(state: BaseState, host: string): string | null {
  const relation = String(state?.relation ?? '')
  if (!NOTEWORTHY.has(relation)) return null
  const machine = host || 'that machine'
  const name = String(state?.base ?? '')
  // Three forms of the same branch: opening a sentence, mid-sentence on its own, and after a determiner ("GitHub's …").
  const lead = name || 'The base branch', bare = name || 'the base branch', after = name || 'base branch'
  const at = short(state?.start)
  if (relation === 'fetch_failed') {
    return `Couldn't reach GitHub from ${machine}, so ${bare} may be out of date. This session starts from the last known ${after}${at}.`
  }
  if (relation === 'ahead' || relation === 'diverged') {
    return `${lead} on ${machine} has commits GitHub doesn't have. This session starts from the local ${after}${at}.`
  }
  return `${lead} on ${machine} is behind GitHub. This session starts from GitHub's ${after}${at}.`
}

/**
 * Whether to ask before starting: there is something to say, and the repository is not one reet cloned itself (nothing
 * local can be behind or ahead in a checkout reet made and keeps). `cloned` does not exist yet, so in this phase every
 * saved repository asks.
 */
export function needsConfirm(state: BaseState, project: { cloned?: boolean }): boolean {
  return NOTEWORTHY.has(String(state?.relation ?? '')) && project?.cloned !== true
}
