import type { TerminalAnswerResult } from './api'

/** Wordings that grant a standing permission, or mean to, rather than answer this one question. Such an option needs a second tap. */
const STANDING = ['ask again', 'asking again', 'without asking', 'every time', 'every later', 'from now on', 'remember', 'trust', 'auto-accept', 'auto accept', 'always', 'for this session', 'all edits', "don't ask", 'don’t ask', 'do not ask']
export function standingPermission(label: string): boolean {
  const text = label.toLowerCase()
  return STANDING.some(phrase => text.includes(phrase))
}

type Opt = { index: number; label: string; truncated?: boolean }
/** An option is confirmed with a second tap when it grants standing permission, or when its label was cut and so cannot be read whole. */
export const needsConfirm = (option: Opt): boolean => !!option.truncated || standingPermission(option.label)

/** The tap rule: a one-off option sends at once; one that needs confirming starts it, and only a second tap on that same option sends. */
export function nextTap(pending: number | null, index: number, needs: boolean): { pending: number | null; send: boolean } {
  if (!needs || pending === index) return { pending: null, send: true }
  return { pending: index, send: false }
}

/** An option whose answer leaves the agent asking for typed words in the terminal ("No, and tell Claude what to do differently"), never a plain yes. */
export function asksForWords(label: string): boolean {
  return !/^\s*yes\b/i.test(label) && /\b(tell|type|explain|provide|feedback)\b|what to do differently/i.test(label)
}

type Dialog = { question: string; truncated?: boolean; options: Opt[] }
/** Home shows the whole card only when it is small and complete; otherwise the sentence and Open, and the session screen has the card. */
export const fitsHome = (q: Dialog): boolean => q.options.length <= 3 && !q.truncated && !q.options.some(o => o.truncated)
/** The question sentence itself: the dialog's last line. */
export const questionSentence = (q: Pick<Dialog, 'question'>): string => q.question.split('\n').at(-1) ?? ''
/** One option as its button reads: the dialog's own number, then its words. */
export function optionText(option: { index: number; label: string }): string { return `${option.index}. ${option.label}` }

export type AnswerOutcome = {
  /** The sentence to show by the buttons (or in place of them when answered). */
  say: string
  /** done leads with a tick, error with the "!" mark, note with nothing: a plain fact the person acts on. */
  tone: 'done' | 'note' | 'error'
  /** The chosen option leaves the agent waiting for typed words in the terminal. */
  waitingForWords: boolean
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
export function outcomeOf(result: TerminalAnswerResult, label: string, ctx: { agent?: string; machine?: string } = {}): AnswerOutcome {
  const said = fullSentence(result.sentence) ? result.sentence!.trim() : ''
  switch (result.kind) {
    case 'answered':
    case 'alreadyAnswered': {
      const words = asksForWords(label)
      return { say: words ? `Answered: ${label}. ${ctx.agent || 'The agent'} is waiting for your words in the terminal.` : `Answered: ${label}`, tone: 'done', waitingForWords: words, done: true, locked: true, reread: false }
    }
    case 'changed': return { say: said || 'The question on the terminal changed. Look at it again before answering.', tone: 'note', waitingForWords: false, done: false, locked: false, reread: true }
    case 'updateRequired': return { say: `Update reet on ${ctx.machine || 'this machine'} to answer from here.`, tone: 'note', waitingForWords: false, done: false, locked: true, reread: false }
    case 'readOnly': return { say: 'Your account can look but not answer.', tone: 'note', waitingForWords: false, done: false, locked: true, reread: false }
    case 'busy': return { say: 'Another answer is on its way. Wait a moment.', tone: 'note', waitingForWords: false, done: false, locked: false, reread: false }
    case 'offline': return { say: 'The machine is not connected. Try again when it is back.', tone: 'error', waitingForWords: false, done: false, locked: false, reread: false }
    case 'noReply': return { say: 'No reply yet. Check the terminal before answering again.', tone: 'error', waitingForWords: false, done: false, locked: false, reread: false }
    default: return { say: `The answer could not be sent.${said ? ` ${said}` : ''} Try again, or answer in the terminal.`, tone: 'error', waitingForWords: false, done: false, locked: false, reread: false }
  }
}
