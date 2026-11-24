// Words for the enrolment code in "Add a machine". Pure: no DOM, no fetch, no clock of its own (callers pass `now`, in ms).

export type Countdown = { expired: boolean; text: string }

/** `expiresAt` is unix seconds, as the server sends it. Rounds up, so a code that is still valid never reads 0:00. */
export function enrolCountdown(expiresAt: unknown, now: number): Countdown {
  const left = typeof expiresAt === 'number' ? Math.ceil(expiresAt - now / 1000) : NaN
  if (!(left > 0)) return { expired: true, text: 'This code has run out.' }
  return { expired: false, text: `Expires in ${Math.floor(left / 60)}:${String(left % 60).padStart(2, '0')}` }
}

/** The sentence shown by the button when making a code fails: the card's own words for each refusal, whatever the server said. */
export function enrolCodeProblem(e: unknown): string {
  const { status, message, name } = (e ?? {}) as { status?: unknown; message?: unknown; name?: unknown }
  const said = typeof message === 'string' ? message : ''
  if (status === 401 || name === 'UnauthorizedError') return 'Sign in again to make a code.'
  if (status === 403) return 'Your account can look but not add machines.'
  if (status === 429) return /unused/i.test(said) ? 'You have 20 unused codes. Wait for one to run out (each lasts 15 minutes), or use one.' : 'You have made too many codes. Try again later.'
  return 'The code could not be made. Try again.'
}

/** A machine enrolled again is done when it has been seen on a personal token and no longer is: being online already proves nothing. */
export function reenrolDone(agent: { legacyToken?: boolean } | undefined, sawLegacy: boolean): boolean {
  return !!agent && sawLegacy && !agent.legacyToken
}
