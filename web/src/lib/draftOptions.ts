/** Which launch options a backlog item can carry, from what the composer already knows. Pure, so the rules are testable. */
export type DraftOptionInput = { tool: string; isRepo: boolean; canChat: boolean; chat: boolean }
export type DraftOptionRules = { showChat: boolean; chat: boolean; showWorktree: boolean; showTrust: boolean }

export function draftOptionRules(i: DraftOptionInput): DraftOptionRules {
  const showChat = i.tool === 'opencode' && i.canChat
  const chat = showChat && i.chat
  // A chat has no worktree or branch of its own; a worktree needs a repository project.
  return { showChat, chat, showWorktree: i.isRepo && !chat, showTrust: chat }
}

/** The fields to save: anything the rules hide is cleared so a stale choice never reaches Start. */
export function cleanDraftOptions(i: DraftOptionInput & { worktree: boolean; base: string; trust: boolean; yolo: boolean }) {
  const r = draftOptionRules(i)
  return { yolo: i.yolo, chat: r.chat, trust: r.showTrust && i.trust, worktree: r.showWorktree && i.worktree, base: r.showWorktree && i.worktree ? i.base.trim() : '' }
}
