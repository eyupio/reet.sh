// How a fix request reaches the agent, and what the person is told. No imports: the test transpiles this file alone.

/** A terminal has no composer to fill, so its button says what it does. A chat keeps the tracker's wording. */
export function fixButtonText(structured: boolean, chatWording: string): string {
  return structured ? chatWording : 'Copy message for the agent'
}

/** Whether the message goes into the composer or onto the clipboard. A chat is tried first; nothing is ever sent. */
export function fixRoute(structured: boolean): 'composer' | 'clipboard' {
  return structured ? 'composer' : 'clipboard'
}

/** The line shown after a copy. `copied` is false when the clipboard itself refused. */
export function copyNote(structured: boolean, copied: boolean): string {
  if (!copied) return 'Could not copy the message. Try again.'
  return structured ? 'Copied. The composer is nearly full.' : 'Copied'
}

/** A draft that already ends with the message has it: asking again must not stack a second copy. */
export function alreadyThere(draft: string, message: string): boolean {
  return message !== '' && draft.trimEnd().endsWith(message)
}
