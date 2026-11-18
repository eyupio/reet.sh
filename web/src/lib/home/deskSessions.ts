// Sessions on a desk, for Home and the bell. A desk session leaves Home's session lists (Continue working, the Planning
// board, the head's count of open sessions), but when it needs the person it stays in Waiting for you and the bell,
// marked with its desk and opening on it. A template desk holds no sessions of its own. Pure apart from the pane reader,
// which is handed its read.
import type { DeskSummary } from '../desks/api'

/** What Home and the bell need of a desk from the desk list. */
export type DeskRef = Pick<DeskSummary, 'id' | 'name' | 'template' | 'sessions' | 'version'>
/** The desk a waiting session is on, as the list shows it. */
export type DeskMark = { id: string; name: string; href: string }
/** The desks as last read, and the pane each waiting desk session is in where that has been read. */
export type DeskLookup = { desks?: readonly DeskRef[]; panes?: ReadonlyMap<string, string> }

/** The desk at the pane that holds the session (`/desks/<id>#<pane>`), else the desk itself. */
export const deskHref = (deskId: string, pane = '') => `/desks/${encodeURIComponent(deskId)}${pane ? `#${encodeURIComponent(pane)}` : ''}`

/** The (non-template) desk holding the session, with the link to it. */
export function deskOf(desks: readonly DeskRef[] | undefined, sessionId: string, panes?: ReadonlyMap<string, string>): DeskMark | undefined {
  const desk = (desks ?? []).find(d => !d.template && d.sessions.includes(sessionId))
  return desk && { id: desk.id, name: desk.name, href: deskHref(desk.id, panes?.get(sessionId)) }
}

/** The sessions that are on no desk. `keep` are sessions shown as something else (a Factory pipeline's), which stay.
 *  With no desks read (undefined), no session counts as a desk session. */
export function offDesks<T extends { id: string }>(sessions: readonly T[], desks: readonly DeskRef[] | undefined, keep: ReadonlySet<string> = new Set()): T[] {
  const held = new Set((desks ?? []).flatMap(d => d.template ? [] : d.sessions))
  return held.size ? sessions.filter(s => keep.has(s.id) || !held.has(s.id)) : [...sessions]
}

type DeskDoc = { id: string; version: number; doc: { tabs: { panes: Record<string, { id: string; session: string }> }[] } }

/** Finds the pane each wanted session is in, reading only the desks that hold one, once per desk version. A desk that
 *  could not be read gives no panes (its link opens the desk) and is read again next time. */
export function deskPaneReader(read: (id: string) => Promise<DeskDoc>) {
  const kept = new Map<string, { version: number; panes: Map<string, string> }>()
  return async (desks: readonly DeskRef[], wanted: ReadonlySet<string>): Promise<Map<string, string>> => {
    const holding = desks.filter(d => !d.template && d.sessions.some(id => wanted.has(id)))
    await Promise.all(holding.filter(d => kept.get(d.id)?.version !== d.version).map(async d => {
      try {
        const desk = await read(d.id)
        const panes = new Map<string, string>()
        for (const tab of desk.doc.tabs) for (const pane of Object.values(tab.panes)) if (pane.session && !panes.has(pane.session)) panes.set(pane.session, pane.id)
        kept.set(d.id, { version: d.version, panes })
      } catch { kept.delete(d.id) }
    }))
    const out = new Map<string, string>()
    for (const d of holding) for (const [session, pane] of kept.get(d.id)?.panes ?? []) if (wanted.has(session) && d.sessions.includes(session)) out.set(session, pane)
    return out
  }
}
