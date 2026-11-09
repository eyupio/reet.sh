// What the Overview's stages offer to follow up on. Every item is a goal a person can hand to Gaffer for this session: the
// wording is the whole of it, so nothing here acts. A chat session opens it, already written, in the follow-up editor (which
// shows the limits and permissions and starts nothing until the person says so); a terminal session, which has no follow-up,
// copies it for the agent. No imports: the test transpiles this file alone.

export type StageKey = 'code' | 'pr' | 'checks' | 'review' | 'merge'
export type FollowItem = { id: string; label: string; text: string }

export const STAGE_KEYS: StageKey[] = ['code', 'pr', 'checks', 'review', 'merge']

/** The follow-up editor's goal field holds 2,000 characters; a longer goal would be cut silently. */
export const GOAL_LIMIT = 2000

const HOLD = 'Bring decisions back to me.'
const NO_MERGE = 'Do not merge.'
// Merging is the one step that cannot be taken back, so an item that allows it says exactly when, and still hands the decision back.
const WHEN_GREEN = 'every check passes, no review thread is open and there are no conflicts'

const ITEMS: Record<StageKey, FollowItem[]> = {
  code: [
    { id: 'code-raise-pr', label: 'Get it to a pull request', text: `Ensure a pull request is raised for this work. ${HOLD} ${NO_MERGE}` },
  ],
  pr: [
    { id: 'pr-babysit', label: 'Babysit the pull request', text: `Look after the pull request: ensure failing checks are fixed, review comments are addressed and merge conflicts are resolved, and keep it that way until I say otherwise. ${HOLD} ${NO_MERGE}` },
    { id: 'pr-babysit-merge', label: 'Babysit it, then merge when green', text: `Look after the pull request: ensure failing checks are fixed, review comments are addressed and merge conflicts are resolved. When ${WHEN_GREEN}, have it merged, then tell me. ${HOLD}` },
  ],
  checks: [
    { id: 'checks-fix', label: 'Fix failing checks', text: `Ensure every failing check on the pull request is fixed and stays passing. ${HOLD} ${NO_MERGE}` },
  ],
  review: [
    { id: 'review-by-gaffer', label: 'Have Gaffer review it', text: `Review the pull request carefully and send the agent your feedback as clear instructions, then review it again once it has addressed them. ${HOLD} ${NO_MERGE}` },
    { id: 'review-address', label: 'Address the review comments', text: `Ensure every open review comment on the pull request is addressed and its thread resolved. ${HOLD} ${NO_MERGE}` },
  ],
  merge: [
    { id: 'merge-when-green', label: 'Merge when green', text: `Ensure the pull request is merged as soon as ${WHEN_GREEN}, then tell me. ${HOLD}` },
    { id: 'merge-conflicts', label: 'Keep conflicts resolved', text: `Ensure the pull request has no merge conflicts with its base branch, and keep it that way. ${HOLD} ${NO_MERGE}` },
  ],
}

/** The items for a stage; an unknown stage has none. A fresh list each call, so a caller cannot change the table. */
export function stageItems(key: string): FollowItem[] {
  return (ITEMS[key as StageKey] ?? []).map(item => ({ ...item }))
}
