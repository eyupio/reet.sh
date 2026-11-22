// The first-session guide on Home: when it shows, the one read-only prompt it suggests, and what counts as a first reply.
// Pure. The sign-in and model words come from launchReadiness, agentAccounts and models; nothing here words them again.
import type { TerminalSession } from '../terminal/api'

/** Asks for a look and changes nothing: the agent only says where it is and lists what it sees. */
export const FIRST_PROMPT = 'Say which folder you are in and list the files and folders you can see there. Do not change, create or delete anything.'

/** What the person did with the guide in this browser: `dismissed` with Not now, `done` once a first reply arrived. */
export type FirstRunMemory = '' | 'dismissed' | 'done'

/** The guide shows to someone with a connected machine whose sessions, if any, are all ones the guide started, until they
 *  dismiss it or a first reply arrives. Sessions not yet read, or failed to read, never show it. */
export function guideShown(memory: FirstRunMemory, machine: boolean, sessions: readonly Pick<TerminalSession, 'id'>[] | undefined, started: readonly string[]): boolean {
  if (memory || !machine || !sessions) return false
  return sessions.every(session => started.includes(session.id))
}

export type FirstReply = 'none' | 'asking' | 'replied' | 'ended'

/** Where the guide's own session stands. A chat that has finished its turn, or waits for the person with nothing to
 *  approve, has replied; a session that ended or stopped first has not. */
export function replyOf(session: Pick<TerminalSession, 'state' | 'attention' | 'pendingPermission' | 'pendingQuestion'> | undefined): FirstReply {
  if (!session) return 'none'
  if (session.attention === 'finished') return 'replied'
  if (session.attention === 'waiting' && !session.pendingPermission && !session.pendingQuestion) return 'replied'
  if (session.attention === 'failed' || session.state === 'ended' || session.state === 'exited' || session.state === 'lost') return 'ended'
  return 'asking'
}
