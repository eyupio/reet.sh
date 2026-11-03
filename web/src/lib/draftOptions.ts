/** What an idea needs to be saved: a title or a note. The machine and folder are checked by the composer as for any item. */
export function ideaProblem(i: { idea: boolean; title: string; prompt: string }): string {
  return i.idea && !i.title.trim() && !i.prompt.trim() ? 'Give the idea a title or a note.' : ''
}

/**
 * Why a backlog item can't start right now, or null when it can. The board resolves these facts from the machine and
 * project lists; the card and an idea's start dialog share the same rules and wording. An idea is not blocked here:
 * Start opens a dialog where it is refined, saved as a normal item, and only then checked and started.
 */
export type StartItem = { projectId: string; imageCount: number; effort: string }
/** Where an item would run, plus what the rules look at; a saved item and a half-filled dialog both fit. */
export type StartPlace = StartItem & { agentId: string; tool: string; dir: string }
export type StartTarget = {
  /** Why the machine can't take a launch ('' when it can). */
  readiness: string
  /** The chosen agent on that machine; undefined when the machine doesn't offer it. */
  agent?: { available: boolean; reason?: string }
  /** The machine can take reference images. */
  uploads: boolean
  /** The machine lets the session choose an effort. */
  effort: boolean
  /** The chosen project still points at this machine and folder. */
  projectCurrent: boolean
}
export function startProblem(i: StartItem, t: StartTarget): string | null {
  if (t.readiness) return t.readiness
  if (!t.agent?.available) return t.agent?.reason ?? 'This agent is unavailable on that machine.'
  if (i.imageCount && !t.uploads) return 'Image uploads are unavailable on this machine.'
  if (i.effort && !t.effort) return 'Update reet on this machine to choose effort.'
  if (i.projectId && !t.projectCurrent) return 'The project changed or was removed. Edit this backlog item first.'
  return null
}

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
