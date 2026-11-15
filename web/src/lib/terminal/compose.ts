// The terminal's compose box: text written (or dictated) in an ordinary field, then sent to the shell as one paste.
// No browser APIs here, so the rules are unit tested.

/** The longest message the box takes: a prompt, not a file. */
export const COMPOSE_MAX = 8000

/** The draft as it is sent: one newline style, no trailing blank space. Empty when there is nothing to send. */
export function composeText(draft: string): string {
  return draft.replace(/\r\n?/g, '\n').replace(/[ \t]+$/gm, '').replace(/\n+$/, '').slice(0, COMPOSE_MAX)
}

/**
 * A dictated phrase joined to what is already written: a space between them unless the draft is empty or ends in
 * white space, and nothing added for an empty phrase. The phrase's own capitals and punctuation are the speech
 * service's and are kept.
 */
export function appendDictation(draft: string, spoken: string): string {
  const phrase = spoken.trim()
  if (!phrase) return draft
  const gap = draft === '' || /\s$/.test(draft) ? '' : ' '
  return (draft + gap + phrase).slice(0, COMPOSE_MAX)
}

/** The browser's speech recognition constructor, when it has one (Chrome, Edge and Safari name it differently). */
export function speechRecognition<T>(scope: { SpeechRecognition?: T; webkitSpeechRecognition?: T }): T | undefined {
  return scope.SpeechRecognition ?? scope.webkitSpeechRecognition
}

/** Why dictation stopped, in words a person can act on. Empty for an ordinary stop. */
export function dictationProblem(error: string): string {
  switch (error) {
    case 'not-allowed': case 'service-not-allowed': return 'The browser did not allow the microphone. Allow it for this site, then try again.'
    case 'audio-capture': return 'No microphone was found.'
    case 'network': return 'The browser could not reach its speech service.'
    case 'no-speech': case 'aborted': return ''
    default: return 'Dictation stopped. Try again, or type instead.'
  }
}
