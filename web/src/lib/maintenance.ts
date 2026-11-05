import type { SweepItem } from './terminal/api'

/** The words for why a worktree is not simply removable. */
export function sweepReason(item: Pick<SweepItem, 'status' | 'changed' | 'reason'>): string {
  if (item.status === 'safe') return 'Nothing unsaved.'
  if (item.status === 'review') return item.reason === 'changed_since_checked'
    ? 'It changed while being checked.'
    : `${item.changed} ${item.changed === 1 ? 'file has' : 'files have'} uncommitted changes.`
  if (item.status === 'unreachable') return item.reason === 'offline' ? "That machine isn't connected." : "That machine couldn't be asked just now."
  if (item.status === 'gone') return 'Already removed.'
  const reasons: Record<string, string> = {
    unreferenced_commits: 'Some commits are on no branch.', hidden_changes: 'Git is not tracking some edits.',
    nested_repository: 'It holds another git repository.', check_incomplete: "It couldn't be fully checked.",
    not_registered: "Git doesn't list it as a worktree.", invalid: 'Not a valid worktree.',
  }
  return reasons[item.reason ?? ''] ?? 'It needs a look first.'
}

/** Only worktrees with nothing unsaved are ever cleaned in bulk. */
export const cleanable = (items: SweepItem[]): SweepItem[] => items.filter(i => i.status === 'safe' && !i.removed)
export const needsReview = (items: SweepItem[]): SweepItem[] => items.filter(i => i.status === 'review' || i.status === 'blocked')
export const itemKey = (i: Pick<SweepItem, 'agentId' | 'run'>): string => `${i.agentId}/${i.run}`
