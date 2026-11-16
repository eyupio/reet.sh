// Putting a session that is already running on a desk: from a session's menu (Add to a desk…) and from a desk's + menu
// (Add a running session…). A session is in one pane across the account's desks, so one already on a desk is moved:
// taken off its old desk first (its old pane is closed, unless it is the only pane in its tab, which stays, empty), then added, in two saves. If the add then fails the session is
// on neither desk and the person is told so; nothing is guessed.
import { stripHiddenChars, type TerminalSession } from '../terminal/api'
import { getDesk, patchDesk, StaleDesk, type Desk, type DeskSummary } from './api'
import { apply } from './autosave'
import { leaves, MAX_PANES, MAX_TABS, newId, type Pane, type Tab } from './tree'

/** A pane name from a session's title: no hidden characters, at most 80 characters, never empty. */
export function paneName(title: string): string {
  return [...stripHiddenChars(title).trim()].slice(0, 80).join('') || 'Shell'
}

/** The pane that holds a running session: its machine, its live folder and its name. */
export function paneFor(s: Pick<TerminalSession, 'id' | 'agentId' | 'title' | 'tool' | 'dir' | 'liveDir'>, id = newId()): Pane {
  return { id, name: paneName(s.title), colour: '', host: s.agentId, session: s.id, kind: s.tool ? 'agent' : 'shell', dir: s.liveDir || s.dir || '', dirAt: Date.now(), start: '' }
}

/** A new tab holding one pane. */
export function tabFor(pane: Pane): Tab {
  return { id: newId(), name: pane.name, colour: '', root: { pane: pane.id }, zoomed: '', active: pane.id, panes: { [pane.id]: pane } }
}

/** A tab that can take one more pane. */
export const roomIn = (t: Tab) => leaves(t.root).length > 0 && Object.keys(t.panes).length < MAX_PANES

/** The desk with the session in a new pane beside the tab's focused pane (or in a new tab when `tab` is ''), made the
 *  focused pane; null when there is no room or the tab is gone. */
export function withSession(desk: Desk, s: Parameters<typeof paneFor>[0], tab: string): Desk | null {
  const pane = paneFor(s)
  if (!tab) {
    if (desk.doc.tabs.length >= MAX_TABS) return null
    const t = tabFor(pane)
    const added = apply(desk, { kind: 'addTab', tab: t })
    return added && apply(added, { kind: 'active', tab: t.id, pane: pane.id })
  }
  const t = desk.doc.tabs.find(x => x.id === tab)
  if (!t || !roomIn(t)) return null
  const at = t.panes[t.active] ? t.active : leaves(t.root)[0]
  const added = apply(desk, { kind: 'addPane', tab: t.id, at, dir: 'right', pane })
  return added && apply(added, { kind: 'active', tab: t.id, pane: pane.id })
}

/** The desk without the pane that holds the session, so a move leaves no hole: the pane is closed and its neighbours
 *  take the space. A pane that is the only one in its tab stays, empty (a tab is never closed by a move). The desk itself
 *  when no pane holds the session. */
export function withoutSession(desk: Desk, session: string): Desk {
  let out = desk
  for (const t of desk.doc.tabs) for (const p of Object.values(t.panes)) if (p.session === session) {
    const alone = Object.keys(t.panes).length <= 1
    out = apply(out, alone ? { kind: 'setSession', tab: t.id, pane: p.id, session: '' } : { kind: 'closePane', tab: t.id, pane: p.id }) ?? out
  }
  return out
}

/** The desk (among the listed ones) whose panes hold the session. */
export const deskHolding = (desks: Pick<DeskSummary, 'id' | 'name' | 'sessions' | 'template'>[], session: string) => desks.find(d => !d.template && d.sessions.includes(session))

/** Reads the desk, makes the edit and saves it on that version; a save that lost to a newer copy is made once more on it. */
async function write(id: string, edit: (d: Desk) => Desk | null): Promise<Desk> {
  let desk = await getDesk(id)
  for (let left = 1; ; left--) {
    const next = edit(desk)
    if (!next) throw new Error('There is no room for it there now. Choose another tab.')
    if (next === desk) return desk
    try {
      return await patchDesk(id, { expect: desk.version, doc: next.doc })
    } catch (e) {
      if (!(e instanceof StaleDesk) || left <= 0) throw e
      desk = e.desk
    }
  }
}

/** Takes the session off `from` (when it is on a desk) and puts it on `to`, in `tab` ('' for a new tab). */
export async function putOnDesk(s: Parameters<typeof paneFor>[0], to: { id: string; name: string }, tab: string, from?: { id: string; name: string }): Promise<void> {
  if (from) await write(from.id, d => withoutSession(d, s.id))
  try {
    await write(to.id, d => withSession(d, s, tab))
  } catch (e) {
    if (!from) throw e
    const why = e instanceof Error && e.message ? ` ${e.message}` : ''
    throw new Error(`It was taken off “${from.name}” but could not be added to “${to.name}”, so it is on neither desk. It is still running in Sessions.${why}`, { cause: e })
  }
}
