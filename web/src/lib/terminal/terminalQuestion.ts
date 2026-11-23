import type { TerminalAnswerResult } from './api'

/** Labels that grant a standing permission rather than answer this one question. Such an option never gets the stronger style. */
const STANDING = ["don't ask again", 'don’t ask again', 'always', 'for this session', 'all edits']
export function standingPermission(label: string): boolean {
  const text = label.toLowerCase()
  return STANDING.some(phrase => text.includes(phrase))
}

/** One option as its button reads: the dialog's own number, then its words. */
export function optionText(option: { index: number; label: string }): string { return `${option.index}. ${option.label}` }

export type AnswerOutcome = {
  /** The sentence to show by the buttons (or in place of them when answered). */
  say: string
  /** The answer went through: the card reads "Answered" until the next session read removes the question. */
  done: boolean
  /** The buttons stay disabled: answering is not possible from here, or it is already done. */
  locked: boolean
  /** Read the sessions again now, to show whatever the terminal asks next. */
  reread: boolean
}

/** What the card does with an answer's result. Nothing here answers anything: a changed question only asks to be read again. */
export function outcomeOf(result: TerminalAnswerResult, label: string): AnswerOutcome {
  const sentence = (fallback: string) => result.sentence || fallback
  switch (result.kind) {
    case 'answered':
    case 'alreadyAnswered': return { say: `Answered: ${label}`, done: true, locked: true, reread: false }
    case 'changed': return { say: sentence('The question on the terminal changed. Look at it again before answering.'), done: false, locked: false, reread: true }
    case 'updateRequired': return { say: sentence('Update reet on this machine to answer from here.'), done: false, locked: true, reread: false }
    case 'readOnly': return { say: 'Your account can look at sessions but not answer them.', done: false, locked: true, reread: false }
    case 'busy': return { say: sentence('An answer is already on its way. Try again in a moment.'), done: false, locked: false, reread: false }
    case 'offline': return { say: sentence('This machine is offline. Try again when it is back.'), done: false, locked: false, reread: false }
    default: return { say: sentence('The answer was not sent. Try again.'), done: false, locked: false, reread: false }
  }
}
