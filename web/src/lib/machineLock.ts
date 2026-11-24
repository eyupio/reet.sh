// A machine its owner locked (reet agent lock): the words the web adds to the server's lock sentence.

/** What a session that is already open can still do while its machine is locked. */
export const SESSION_LOCKED = 'Locked on the machine. The agent keeps running; from here you can only stop its current turn or end the session.'

/** Sessions that are live on a machine: a lock does not stop them, so the card says how many. */
export function runningOnMachine(sessions: readonly { agentId: string; state: string }[] | undefined, agentId: string): number {
  return (sessions ?? []).filter(s => s.agentId === agentId && (s.state === 'running' || s.state === 'creating')).length
}

export function runningLine(n: number): string {
  if (n < 1) return ''
  const them = n === 1 ? 'it' : 'them'
  return `${n} ${n === 1 ? 'session is' : 'sessions are'} still running on this machine and can run commands there, including \`reet agent unlock\`. End ${them} from reet, or with \`reet agent lock --end-sessions\` on the machine.`
}

/** The card's status word: `lockedAt` is unix seconds, present only when the machine reported a time. */
export function lockedStatus(lockedAt: number | undefined, ago: (atMs: number) => string): string {
  return lockedAt ? `Locked since ${ago(lockedAt * 1000)}` : 'Locked'
}
