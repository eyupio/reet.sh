// Words for the enrolment code in "Add a machine". Pure: no DOM, no fetch, no clock of its own (callers pass `now`, in ms).

export type Countdown = { expired: boolean; text: string }

/** `expiresAt` is unix seconds, as the server sends it. Rounds up, so a code that is still valid never reads 0:00. */
export function enrolCountdown(expiresAt: unknown, now: number): Countdown {
  const left = typeof expiresAt === 'number' ? Math.ceil(expiresAt - now / 1000) : NaN
  if (!(left > 0)) return { expired: true, text: 'This code has run out.' }
  return { expired: false, text: `Expires in ${Math.floor(left / 60)}:${String(left % 60).padStart(2, '0')}` }
}

/** The sentence shown by the button when making a code fails. The server's own words for its refusals; plain words otherwise. */
export function enrolCodeProblem(e: unknown): string {
  const { status, message } = (e ?? {}) as { status?: unknown; message?: unknown }
  const said = typeof message === 'string' ? message : ''
  if (status === 429 || status === 403) return said || 'Could not make an enrolment code. Try again.'
  if (status === 404) return 'This controller does not use accounts, so it has no enrolment codes. Use the token set for this instance instead.'
  if (typeof status === 'number') return 'Could not make an enrolment code. Try again.'
  return 'Could not reach reet.sh. Check the connection and try again.'
}
