// The messages the Overview prefills into the chat composer when a person asks
// the agent to fix a pull request. No imports: the test transpiles this file alone.

export type FixKind = 'conflicts' | 'checks' | 'threads'

// The composer's textarea has maxlength="2000"; a longer paste is silently cut.
const LIMIT = 2000
const NAME_LIMIT = 60
const NAMES_SHOWN = 3

// Check names and branch names come from GitHub, so anyone who can open a pull
// request writes them. They are quoted into a message the agent will act on,
// so they lose control characters (a newline could start a new instruction) and
// URLs (a link is somewhere for the agent to be sent) before they get there.
function clean(raw: string): string {
  const text = raw
    .replace(/[\u0000-\u001f\u007f-\u009f\u2028\u2029]/g, ' ')
    .replace(/https?:\/\/\S+/gi, ' ')
    .replace(/\s+/g, ' ')
    .trim()
  const chars = Array.from(text)
  return chars.length > NAME_LIMIT ? chars.slice(0, NAME_LIMIT - 1).join('') + '…' : text
}

export function fixMessage(kind: FixKind, facts: { base?: string; checks?: string[]; threads?: number }): string {
  let message: string
  if (kind === 'conflicts') {
    message = `Resolve the merge conflicts with ${clean(facts.base ?? '') || 'the base branch'} on this branch, then push.`
  } else if (kind === 'checks') {
    const names = (facts.checks ?? []).map(clean).filter(Boolean)
    if (names.length === 0) {
      message = 'Some checks are failing on the pull request. Please find out why and fix them.'
    } else {
      const shown = names.slice(0, NAMES_SHOWN).map((n) => `"${n}"`).join(', ')
      const more = names.length > NAMES_SHOWN ? ` (and ${names.length - NAMES_SHOWN} more)` : ''
      message = names.length === 1
        ? `This check is failing on the pull request: ${shown}. Please find out why and fix it.`
        : `These checks are failing on the pull request: ${shown}${more}. Please find out why and fix them.`
    }
  } else {
    const n = facts.threads ?? 0
    message = n === 1
      ? 'There is 1 unresolved review thread on the pull request. Please read it and address it.'
      : `There are ${n} unresolved review threads on the pull request. Please read each one and address it.`
  }
  return message.length > LIMIT ? message.slice(0, LIMIT) : message
}

// Null rather than a cut-down result: whatever the person has half typed must
// never be overwritten or truncated, so the caller copies the message instead.
export function prefillText(draft: string, message: string, limit = LIMIT): string | null {
  const text = draft.trim() === '' ? message : draft + '\n' + message
  return text.length > limit ? null : text
}
