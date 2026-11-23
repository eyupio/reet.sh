import type { TerminalAnswerResult } from './api'

/** Labels that grant a standing permission rather than answer this one question. Such an option never gets the stronger style. */
const STANDING = ["don't ask again", 'don’t ask again', 'always', 'for this session', 'all edits', 'auto-accept', 'auto accept']
export function standingPermission(label: string): boolean {
  const text = label.toLowerCase()
  return STANDING.some(phrase => text.includes(phrase))
}

/** One option as its button reads: the dialog's own number, then its words. */
export function optionText(option: { index: number; label: string }): string { return `${option.index}. ${option.label}` }

export type AnswerOutcome = {
  /** The sentence to show by the buttons (or in place of them when answered). */
  say: string
  /** done leads with a tick, error with the "!" mark, note with nothing: a plain fact the person acts on. */
  tone: 'done' | 'note' | 'error'
  /** The answer went through: the card reads "Answered" until the next session read removes the question. */
  done: boolean
  /** The buttons stay disabled: answering is not possible from here, or it is already done. */
  locked: boolean
  /** Read the sessions again now, to show whatever the terminal asks next. */
  reread: boolean
}

/** A server sentence is shown only when it reads as one (a capital, a full stop): wire messages such as "terminal machine is reconnecting" are not. */
export const fullSentence = (text: string | undefined): boolean => !!text && /^\p{Lu}.*[.!?]$/u.test(text.trim())

/** What the card does with an answer's result, in its own plain words. Nothing here answers anything: a changed question only asks to be read again. */
export function outcomeOf(result: TerminalAnswerResult, label: string): AnswerOutcome {
  const said = fullSentence(result.sentence) ? result.sentence!.trim() : ''
  switch (result.kind) {
    case 'answered':
    case 'alreadyAnswered': return { say: `Answered: ${label}`, tone: 'done', done: true, locked: true, reread: false }
    case 'changed': return { say: said || 'The question on the terminal changed. Look at it again before answering.', tone: 'note', done: false, locked: false, reread: true }
    case 'updateRequired': return { say: said || 'Update reet on this machine to answer from here.', tone: 'note', done: false, locked: true, reread: false }
    case 'readOnly': return { say: 'Your account can look but not answer.', tone: 'note', done: false, locked: true, reread: false }
    case 'busy': return { say: 'Another answer is on its way. Wait a moment.', tone: 'note', done: false, locked: false, reread: false }
    case 'offline': return { say: 'The machine is not connected. Try again when it is back.', tone: 'error', done: false, locked: false, reread: false }
    case 'noReply': return { say: 'No reply yet. Check the terminal before answering again.', tone: 'error', done: false, locked: false, reread: false }
    default: return { say: said ? `The answer could not be sent. ${said}` : 'The answer could not be sent.', tone: 'error', done: false, locked: false, reread: false }
  }
}
