// What a desk pane is showing, and what it offers. Pure: the pane, its session as last listed, its machine and the
// attachment's own state go in; a state and its actions come out. A pane is never a blank terminal: every state but
// live is drawn in the pane with a sentence and one clear action (spec: Restore). Only type imports, so the unit test can
// load this file on its own.
import type { TerminalSession } from '../terminal/api'
import type { Pane } from './tree'

export type PaneState = 'live' | 'reconnecting' | 'elsewhere' | 'ended' | 'lost' | 'folderGone' | 'limit' | 'machineGone' | 'empty' | 'update'
export type PaneActionKind = 'reopen' | 'continue' | 'start' | 'takeover' | 'retry' | 'chooseFolder' | 'chooseMachine' | 'none'
export type PaneAction = { label: string; kind: PaneActionKind }
/** Why the last reopen of this pane was refused, kept by the screen until the next try. */
export type PaneFailure = 'folderGone' | 'limit'

const RUNNING = new Set(['creating', 'running', 'end_requested'])

/** The pane's state. `machine` is undefined when the pane's machine is no longer enrolled. A machine that does not say
 *  whether it has desk shells (`deskShell` undefined, before desk-shell-v1 is reported) is taken to have them; only an
 *  explicit `false` asks for an update. A pane with a running session is live, reconnecting or open elsewhere whatever
 *  the machine's version. */
export function paneState(pane: Pane, session: TerminalSession | undefined, machine: { online: boolean; deskShell?: boolean } | undefined,
  attach: { conflict: boolean; down: boolean }, failure?: PaneFailure): PaneState {
  if (!machine) return 'machineGone'
  if (pane.session && session && RUNNING.has(session.state)) {
    if (attach.conflict) return 'elsewhere'
    if (attach.down || !machine.online) return 'reconnecting'
    return 'live'
  }
  if (failure) return failure
  if (machine.deskShell === false && pane.kind === 'shell') return 'update'
  if (!pane.session) return 'empty'
  if (session?.state === 'lost') return 'lost'
  return 'ended'
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

const isAgent = (pane: Pane, session?: TerminalSession) => pane.kind === 'agent' || !!session?.tool

/** The actions a state offers, the main one first. */
export function paneAction(state: PaneState, pane: Pane, session?: TerminalSession): PaneAction[] {
  const reopen: PaneAction = { label: `Reopen in ${dirLabel(pane.dir)}`, kind: 'reopen' }
  switch (state) {
    case 'live': return []
    case 'reconnecting': return [{ label: 'Retry now', kind: 'retry' }]
    case 'elsewhere': return [{ label: 'Use it here', kind: 'takeover' }]
    case 'ended':
    case 'lost':
      if (isAgent(pane, session)) {
        const shell: PaneAction = { label: 'Start a shell here', kind: 'reopen' }
        return session?.tool ? [{ label: 'Continue', kind: 'continue' }, shell] : [shell]
      }
      return [reopen]
    case 'folderGone': return [{ label: 'Choose a folder', kind: 'chooseFolder' }, { label: 'Reopen in home', kind: 'reopen' }]
    case 'limit': return [{ label: 'Try again', kind: 'reopen' }]
    case 'machineGone': return [{ label: 'Choose a machine', kind: 'chooseMachine' }]
    case 'empty': return [{ label: 'Start', kind: 'start' }]
    case 'update': return pane.session ? [reopen] : [{ label: 'Start', kind: 'start' }]
  }
}

/** The pane's words for a state: a title and, for some, a line under it. `host` is the machine's name. */
export function paneCopy(state: PaneState, pane: Pane, session: TerminalSession | undefined, host: string, perMachine = 24): { title: string; body: string } {
  const where = host || 'That machine'
  const dir = dirLabel(pane.dir)
  switch (state) {
    case 'live': return { title: '', body: '' }
    case 'reconnecting': return { title: `${where} is not reporting.`, body: 'The shell may still be running there. reet keeps trying; nothing you typed is resent.' }
    case 'elsewhere': return { title: 'This terminal is open in another window.', body: 'A terminal has one keyboard at a time.' }
    case 'ended':
      if (isAgent(pane, session)) return { title: `${session?.tool ? agentName(session.tool) : 'The agent'} finished in this pane.`, body: `Continue the same conversation, or start a plain shell in ${dir}.` }
      return { title: 'This shell ended.', body: `It was in ${dir}. A new shell opens there with this pane's command history.` }
    case 'lost': return { title: `${where} restarted, so this shell was lost.`, body: `It was in ${dir}. A new shell opens there with this pane's command history.` }
    case 'folderGone': return { title: `${dir} no longer exists on ${where}.`, body: "reet won't quietly open somewhere else." }
    case 'limit': return { title: `${where} already has ${perMachine} sessions running.`, body: '' }
    case 'machineGone': return { title: 'This machine was removed from reet.', body: '' }
    case 'empty': return { title: `${where} · ${dir}`, body: 'Nothing is running in this pane yet.' }
    case 'update': return { title: "Update reet on that machine to keep this pane's folder and history.", body: '' }
  }
}

const AGENTS: Record<string, string> = { claude: 'Claude Code', codex: 'Codex', cline: 'Cline', opencode: 'OpenCode', antigravity: 'Google Antigravity (AGY)' }
const agentName = (tool: string) => AGENTS[tool] ?? tool
