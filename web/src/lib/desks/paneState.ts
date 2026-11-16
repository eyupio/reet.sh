// What a desk pane is showing, and what it offers. Pure: the pane, its session as last listed, its machine and the
// attachment's own state go in; a state and its card come out. A pane is never a blank terminal: every state but live is
// drawn in the pane as a card with a tag, a heading, a sentence and its actions, the main one first (spec: Restore).
// Only type imports, so the unit test can load this file on its own.
import type { TerminalSession } from '../terminal/api'
import type { Pane } from './tree'

export type PaneState = 'live' | 'reconnecting' | 'elsewhere' | 'ended' | 'lost' | 'agentEnded' | 'agentLost' | 'folderGone' | 'folderUnavailable' | 'limit' | 'machineGone' | 'empty'
export type PaneActionKind = 'reopen' | 'reopenHome' | 'continue' | 'start' | 'again' | 'takeover' | 'retry' | 'chooseFolder' | 'chooseMachine' | 'close'
export type PaneAction = { label: string; kind: PaneActionKind }
/** Why the last reopen of this pane was refused, kept by the screen until the next try. `offline`: the machine did not
 *  answer; `machineLimit` and `accountLimit`: the browser's own check of the session limits. */
export type PaneFailure = 'folderGone' | 'folderUnavailable' | 'machineLimit' | 'accountLimit' | 'offline' | 'machineGone'

const RUNNING = new Set(['creating', 'running', 'end_requested'])
/** A session that holds a slot of the session limits, and that a pane attaches to. */
export const isRunning = (s: Pick<TerminalSession, 'state'> | undefined) => !!s && RUNNING.has(s.state)

/** The pane's state. `machine` is undefined when the pane's machine is no longer enrolled. A pane with a running session
 *  is live, reconnecting or open elsewhere whatever the machine's version; an older machine only gets a note (paneNote). */
export function paneState(pane: Pane, session: TerminalSession | undefined, machine: { online: boolean; deskShell?: boolean } | undefined,
  attach: { conflict: boolean; down: boolean }, failure?: PaneFailure): PaneState {
  if (!machine) return 'machineGone'
  // A running session wins over a failure kept from an earlier try: the pane has a shell again.
  if (pane.session && isRunning(session)) {
    if (attach.conflict) return 'elsewhere'
    if (attach.down || !machine.online) return 'reconnecting'
    return 'live'
  }
  if (failure === 'machineGone') return 'machineGone'
  if (failure === 'offline') return 'reconnecting'
  if (failure === 'machineLimit' || failure === 'accountLimit') return 'limit'
  if (failure) return failure
  if (!pane.session) return 'empty'
  if (pane.kind === 'agent' || session?.tool) return session?.state === 'lost' ? 'agentLost' : 'agentEnded'
  if (session?.state === 'lost') return 'lost'
  return 'ended'
}

/** The one line under a live terminal's header when its machine keeps no folder or history for desk panes. */
export function paneNote(pane: Pane, state: PaneState, machine: { deskShell?: boolean } | undefined): string {
  return state === 'live' && pane.kind === 'shell' && machine?.deskShell === false ? "Update reet on that machine to keep this pane's folder and history." : ''
}

/** A folder as the person knows it: `~` for the home, `~/x` below a home, the full path otherwise. */
export function dirLabel(dir: string): string {
  if (!dir) return '~'
  if (dir === '~' || dir.startsWith('~/')) return dir
  const m = dir.match(/^\/(?:home\/[^/]+|Users\/[^/]+|root)(\/.*)?$/)
  if (!m) return dir
  const rest = (m[1] ?? '').replace(/\/+$/, '')
  return rest ? `~${rest}` : '~'
}

/** What a card's words are made from. `host` is the machine's name; `agent` the agent's name for an agent pane. */
export type CardContext = { host: string; dir: string; agent: string; perMachine: number; perAccount: number; accountLimit: boolean; canContinue: boolean }
export type PaneCard = { tag: string; heading: string; body: string; actions: PaneAction[] }

type CardRow = { tag: string; heading: (c: CardContext) => string; body: (c: CardContext) => string; actions: (c: CardContext) => PaneAction[] }
const where = (c: CardContext) => c.host || 'That machine'
const reopenHere = (c: CardContext): PaneAction[] => [{ label: `Reopen in ${c.dir}`, kind: 'reopen' }, { label: 'Close pane', kind: 'close' }]
const history = (c: CardContext) => `It was in ${c.dir}. A new shell opens there with this pane's command history.`
// An agent's pane offers the agent's own resume only where it is on offer (canContinue), and a plain shell always.
const agentBody = (c: CardContext) => (c.canContinue ? `Continue the same conversation, or start a plain shell in ${c.dir}.` : `Start a plain shell in ${c.dir}.`)
const agentActions = (c: CardContext): PaneAction[] => [...(c.canContinue ? [{ label: 'Continue', kind: 'continue' } as PaneAction] : []), { label: 'Start a shell here', kind: 'reopen' }]
const elsewhereFolder = (): PaneAction[] => [{ label: 'Choose a folder', kind: 'chooseFolder' }, { label: 'Reopen in home', kind: 'reopenHome' }]

/** Every card, by state: the component draws from it and the unit test reads it, so the two cannot drift apart. */
export const PANE_CARDS: Record<Exclude<PaneState, 'live'>, CardRow> = {
  lost: { tag: 'Lost', heading: c => `${where(c)} restarted, so this shell was lost.`, body: history, actions: reopenHere },
  ended: { tag: 'Ended', heading: () => 'This shell ended.', body: history, actions: reopenHere },
  folderGone: { tag: 'Folder gone', heading: c => `${c.dir} no longer exists on ${where(c)}.`, body: () => "reet won't quietly open somewhere else.", actions: elsewhereFolder },
  folderUnavailable: { tag: 'Folder unavailable', heading: c => `reet couldn't open a shell in ${c.dir} on ${where(c)}.`, body: () => "reet won't quietly open somewhere else.", actions: elsewhereFolder },
  limit: {
    tag: 'At the limit',
    heading: c => (c.accountLimit ? `You already have ${c.perAccount} sessions running.` : `${where(c)} already has ${c.perMachine} sessions running.`),
    body: () => '',
    actions: () => [{ label: 'Try again', kind: 'again' }],
  },
  machineGone: { tag: 'Machine removed', heading: () => 'This machine was removed from reet.', body: () => '', actions: () => [{ label: 'Choose a machine', kind: 'chooseMachine' }, { label: 'Close pane', kind: 'close' }] },
  reconnecting: { tag: 'Machine offline', heading: c => `${where(c)} is not reporting.`, body: () => 'The shell may still be running there. reet keeps trying; nothing you typed is resent.', actions: () => [{ label: 'Retry now', kind: 'retry' }] },
  elsewhere: { tag: 'Open elsewhere', heading: () => 'This terminal is open in another window.', body: () => 'A terminal has one keyboard at a time.', actions: () => [{ label: 'Use it here', kind: 'takeover' }] },
  agentEnded: { tag: 'Agent ended', heading: c => `${c.agent} finished in this pane.`, body: agentBody, actions: agentActions },
  agentLost: { tag: 'Lost', heading: c => `${where(c)} restarted, so this shell was lost.`, body: agentBody, actions: agentActions },
  empty: { tag: 'Not started', heading: c => `${where(c)} · ${c.dir}`, body: () => '', actions: () => [{ label: 'Start', kind: 'start' }] },
}

const AGENTS: Record<string, string> = { claude: 'Claude Code', codex: 'Codex', cline: 'Cline', opencode: 'OpenCode', antigravity: 'Google Antigravity (AGY)' }
/** The agent's name as people know it; `The agent` when the session is no longer listed. */
export const agentName = (tool: string | undefined) => (tool ? AGENTS[tool] ?? tool : 'The agent')

/** The card for a state, or null for a live pane. */
export function paneCard(state: PaneState, ctx: CardContext): PaneCard | null {
  if (state === 'live') return null
  const row = PANE_CARDS[state]
  return { tag: row.tag, heading: row.heading(ctx), body: row.body(ctx), actions: row.actions(ctx) }
}

/** Panes that Reopen all brings back: shells whose session ended or was lost, on a machine that is still enrolled, that
 *  no earlier try found a reason not to reopen. */
export const reopenable = (state: PaneState) => state === 'lost' || state === 'ended'
