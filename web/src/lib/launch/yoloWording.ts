// What YOLO means in words, per agent. Most agents get "yes" to every permission request. Codex asks only in "Ask before leaving the
// sandbox"; in "Let Codex decide" its own reviewer decides and a card it still raises is left to the person (the connector does
// not answer it), so the same promise would be false there. No imports: the test transpiles this file alone.

/** The chat sentence in the YOLO confirmation. */
export function yoloChatSentence(machine: string, tool: string): string {
  if (tool === 'codex') {
    return `The agent will not wait for you where it asks. In “Ask before leaving the sandbox”, reet says yes, this once, to each request it makes on ${machine}, including running commands and changing files. In “Let Codex decide”, Codex’s own reviewer decides and a request it still raises is left to you.`
  }
  return `The agent will not wait for you. reet says yes, this once, to every permission request it makes on ${machine}, including running commands and changing files.`
}

/** The tooltip of the YOLO button in a live chat's session controls. */
export function yoloSwitchTitle(tool: string): string {
  return tool === 'codex' ? 'Answer approvals when Codex asks before leaving its sandbox (not in “Let Codex decide”)' : 'Answer every approval for this session'
}

/** The mode line with YOLO on, or null when the ordinary wording is right. `name` is the agent's display name. */
export function yoloModeMeaning(mode: string, name: string): string | null {
  if (name !== 'Codex') return null
  if (mode === 'read-only') return `${name} runs commands inside its sandbox. YOLO is on for this chat, so when it asks to leave the sandbox you are not asked.`
  if (mode === 'agent') return `${name} decides for itself what needs your approval. YOLO is on, but it does not answer there: a request ${name} still raises is left to you.`
  return null
}
