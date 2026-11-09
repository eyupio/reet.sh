// Goal prompts for "Look after this session". reet ships a few written goals; a person can keep their own in this
// browser. The text is ordinary goal text: choosing one only fills the goal box, which stays editable.
export const MAX_GOAL_PROMPTS = 20
export const MAX_GOAL_PROMPT_BYTES = 2000

export type GoalPrompt = { id: string; text: string; createdAt: number }

export type BuiltInGoalPrompt = { id: string; title: string; text: string }

export const BUILT_IN_GOAL_PROMPTS: ReadonlyArray<BuiltInGoalPrompt> = [
  {
    id: 'ship-pr',
    title: 'Ship the pull request',
    text: 'Ensure a pull request is raised and its description is accurate. Fix failing checks and resolve straightforward conflicts. Bring scope changes and decisions back to me. Do not merge.',
  },
  {
    id: 'green-checks',
    title: 'Get the checks green',
    text: 'Watch the pull request’s checks. Diagnose and fix failures caused by this work, and report any that look flaky or unrelated. Bring decisions back to me. Do not merge.',
  },
  {
    id: 'resolve-conflicts',
    title: 'Resolve merge conflicts',
    text: 'Keep the branch up to date with its base and resolve straightforward merge conflicts. Ask before discarding either side’s work. Bring decisions back to me. Do not merge.',
  },
  {
    id: 'keep-moving',
    title: 'Keep it moving',
    text: 'Keep making progress on the agreed work. Unblock straightforward problems yourself, and bring decisions, risks or scope changes back to me. Do not merge.',
  },
  {
    id: 'review-only',
    title: 'Review, don’t change',
    text: 'Review the worker’s progress and report risks, gaps or questionable choices. Do not ask for code changes unless I agree. Do not merge.',
  },
  {
    id: 'hand-back',
    title: 'Hand back for review',
    text: 'Finish the agreed implementation and make sure the pull request description reflects it. Stop and hand back for my review. Do not merge.',
  },
]

const encoder = new TextEncoder()

export function goalPromptBytes(text: string): number {
  return encoder.encode(text).length
}

/** The first line with something to say, shortened for a picker row. */
export function goalPromptTitle(text: string, limit = 80): string {
  const line = text.split('\n').map(part => part.trim()).find(Boolean) ?? ''
  const clean = line.replace(/\s+/g, ' ')
  if (!clean) return 'Untitled prompt'
  return clean.length > limit ? `${clean.slice(0, Math.max(1, limit - 1)).trimEnd()}…` : clean
}

export function goalPromptsKey(userId: number): string {
  return `reet.gaffer.goalPrompts.${userId || 'anon'}`
}

export function parseGoalPrompts(raw: string | null): GoalPrompt[] {
  if (!raw) return []
  let stored: unknown
  try {
    stored = JSON.parse(raw)
  } catch {
    return []
  }
  const rows = Array.isArray(stored) ? stored : (stored as { prompts?: unknown })?.prompts
  if (!Array.isArray(rows)) return []
  const out: GoalPrompt[] = []
  const seen = new Set<string>()
  for (const row of rows) {
    if (out.length >= MAX_GOAL_PROMPTS) break
    if (!row || typeof row !== 'object') continue
    const { id, text, createdAt } = row as Partial<GoalPrompt>
    if (typeof id !== 'string' || !id || typeof text !== 'string') continue
    const trimmed = text.trim()
    if (!trimmed || goalPromptBytes(trimmed) > MAX_GOAL_PROMPT_BYTES || seen.has(trimmed)) continue
    seen.add(trimmed)
    out.push({ id, text: trimmed, createdAt: typeof createdAt === 'number' && Number.isFinite(createdAt) ? createdAt : 0 })
  }
  return out
}

export function serialiseGoalPrompts(prompts: GoalPrompt[]): string {
  return JSON.stringify({ version: 1, prompts })
}

/** Adds the newest first, refuses an empty or oversized text and the same text twice, and drops the oldest past the
 *  limit. Returns the list it was given when nothing changed, so a caller can tell a save from a duplicate. */
export function addGoalPrompt(prompts: GoalPrompt[], text: string, id: string, createdAt: number): GoalPrompt[] {
  const trimmed = text.trim()
  if (!trimmed || goalPromptBytes(trimmed) > MAX_GOAL_PROMPT_BYTES) return prompts
  if (prompts.some(prompt => prompt.text === trimmed)) return prompts
  return [{ id, text: trimmed, createdAt }, ...prompts].slice(0, MAX_GOAL_PROMPTS)
}

export function removeGoalPrompt(prompts: GoalPrompt[], id: string): GoalPrompt[] {
  return prompts.filter(prompt => prompt.id !== id)
}

function storage(fallback?: Storage): Storage | null {
  try {
    return fallback ?? globalThis.localStorage ?? null
  } catch {
    return null
  }
}

export function readGoalPrompts(userId: number, area?: Storage): GoalPrompt[] {
  const store = storage(area)
  if (!store) return []
  try {
    return parseGoalPrompts(store.getItem(goalPromptsKey(userId)))
  } catch {
    return []
  }
}

export function writeGoalPrompts(userId: number, prompts: GoalPrompt[], area?: Storage): void {
  const store = storage(area)
  if (!store) return
  try {
    store.setItem(goalPromptsKey(userId), serialiseGoalPrompts(prompts))
  } catch {
    /* full or blocked: the prompts still work for this visit */
  }
}
