/** The chats an account turns on for itself because they have not been checked with a signed-in account yet. */
export const GATED_CHATS: readonly { id: string; label: string }[] = [
  { id: 'claude', label: 'Claude Code in Gaffer' },
  { id: 'codex', label: 'Codex in Gaffer' },
]

export const CHAT_GATE_HELP = 'Not yet checked with a signed-in account. Turn on to try it on your machines that have its chat adapter installed.'

/** One checkbox per gated chat, from the server's effective state. A chat the server did not mention reads as off. */
export function chatToggles(enabled: Record<string, boolean> | undefined): { id: string; label: string; on: boolean }[] {
  return GATED_CHATS.map(chat => ({ ...chat, on: enabled?.[chat.id] === true }))
}

/** The change one toggle sends: only its own agent, so another agent's choice is never touched. */
export function chatTogglePatch(id: string, on: boolean): { chatAgents: Record<string, boolean> } {
  return { chatAgents: { [id]: on } }
}
