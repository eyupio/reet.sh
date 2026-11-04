import type { SessionState, TerminalSession } from './terminal/api'

// One status vocabulary for every session surface. `stateWord` says what to read; this says which colour family the
// surface paints, so Home, the sessions panel, the session tabs and the session screen cannot disagree.
// working green · waiting amber · starting orange · finished blue · failed red · ended/quiet slate.

export type SessionStatus = 'working' | 'waiting' | 'starting' | 'ending' | 'finished' | 'failed' | 'ended' | 'quiet'

type SessionSignals = Pick<TerminalSession, 'state' | 'exitCode' | 'attention' | 'attentionSource'>

/** The colour family for a session: what needs the person first, then what the agent last did, then the lifecycle state. */
export function sessionStatus(session: SessionSignals): SessionStatus {
  switch (session.attention) {
    case 'waiting': return 'waiting'
    case 'working': return 'working'
    case 'quiet': return session.attentionSource === 'chat' ? 'finished' : 'quiet'
    case 'starting': return 'starting'
    case 'finished': return 'finished'
    case 'failed': return 'failed'
  }
  switch (session.state) {
    case 'creating': return 'starting'
    case 'running': return 'working'
    case 'end_requested': return 'ending'
    case 'ended': return 'ended'
    case 'exited':
      if (typeof session.exitCode === 'number') return session.exitCode === 0 ? 'finished' : 'failed'
      return 'ended'
    case 'lost': return 'failed'
    default: return 'ended'
  }
}

/** The status for a lifecycle state alone, when no session record is at hand (the panel's compact marks do this). */
export function stateStatus(state: SessionState): SessionStatus {
  return sessionStatus({ state, exitCode: null, attention: undefined, attentionSource: undefined })
}

/** The colour family for a Needs you card: a question or approval is waiting, a broken session has failed, and a
 *  finished turn (with or without a worktree to review) is finished. */
export function needStatus(kind: 'waiting' | 'failed' | 'review' | 'finished'): SessionStatus {
  if (kind === 'waiting') return 'waiting'
  if (kind === 'failed') return 'failed'
  return 'finished'
}

/** A compact name for the colour family, where a row has no room for stateWord's full sentence ("Waiting for you"). */
export function statusLabel(status: SessionStatus): string {
  return status === 'working' ? 'Working'
    : status === 'waiting' ? 'Waiting'
    : status === 'starting' ? 'Starting'
    : status === 'ending' ? 'Ending'
    : status === 'finished' ? 'Finished'
    : status === 'failed' ? 'Failed'
    : status === 'quiet' ? 'Quiet'
    : 'Ended'
}
