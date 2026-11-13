import type { TerminalSession } from './api'

// A machine card's Open terminal is one action: the Sessions workspace with ?open=1, which reuses a usable session
// or starts one plain shell there. The intent is a query parameter so the address still works as a deep link.
export const OPEN_TERMINAL_PARAM = 'open'

export function openTerminalHref(agentId: string): string {
  return `/sessions?agent=${encodeURIComponent(agentId)}&${OPEN_TERMINAL_PARAM}=1`
}

export function wantsOpenTerminal(search: string): boolean {
  return new URLSearchParams(search).get(OPEN_TERMINAL_PARAM) === '1'
}

/** The session Open terminal joins instead of starting another: a running shell, or a creation already in flight. */
export function reusableSessionFor(sessions: readonly TerminalSession[], agentId: string): TerminalSession | undefined {
  const own = sessions.filter(item => item.agentId === agentId)
  return own.find(item => item.state === 'running') ?? own.find(item => item.state === 'creating')
}

const REQUEST_KEY_PREFIX = 'reet.open-terminal.request.'

type RequestStore = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>

function browserStore(): RequestStore | null {
  try {
    return sessionStorage
  } catch {
    return null
  }
}

/** The create request id a repeated Open terminal click reuses, so the controller collapses it into one session. */
export function openTerminalRequestId(agentId: string, store: RequestStore | null = browserStore(), make = () => crypto.randomUUID()): string {
  const key = REQUEST_KEY_PREFIX + agentId
  try {
    const existing = store?.getItem(key)
    if (existing) return existing
    const created = make()
    store?.setItem(key, created)
    return created
  } catch {
    return make()
  }
}

export function clearOpenTerminalRequestId(agentId: string, store: RequestStore | null = browserStore()): void {
  try {
    store?.removeItem(REQUEST_KEY_PREFIX + agentId)
  } catch {
    /* the next Open terminal starts a fresh request */
  }
}
