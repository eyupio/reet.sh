// The desks API client. Same rules as the terminal client: cookie only, same origin, errors through terminalRequest.
import type { ProjectColour } from '../projectColour'
import { TerminalError, type TerminalSession } from '../terminal/api'
import type { Doc } from './tree'

export type DeskSummary = { id: string; name: string; colour: ProjectColour; template: string; position: number; version: number; tabs: number; panes: number; sessions: string[]; hosts: string[] }
export type Desk = { id: string; name: string; colour: ProjectColour; template: string; position: number; version: number; schema: number; doc: Doc }

/** A write that lost to a newer version (409 desk_stale). `desk` is the current record to merge onto. */
export class StaleDesk extends Error {
  desk: Desk
  constructor(desk: Desk) { super('This desk changed somewhere else.'); this.desk = desk }
}

const base = '/api/v2/desks'
const json = (method: string, body: unknown): RequestInit => ({ method, body: JSON.stringify(body) })

/** Cookie-only like terminalRequest, but a 409 desk_stale becomes a StaleDesk carrying the current desk. Other
 *  refusals become TerminalError so callers handle them as they do elsewhere. */
async function deskRequest(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers)
  if (init.body) headers.set('Content-Type', 'application/json')
  const response = await fetch(path, { ...init, headers, credentials: 'same-origin', cache: 'no-store' })
  if (response.ok) return response
  let message = 'The desk connection is unavailable.', code: string | undefined, desk: Desk | undefined
  try {
    const body = await response.json()
    if (typeof body?.error === 'string' && body.error) { message = body.error; code = body.error }
    if (typeof body?.code === 'string' && body.code) code = body.code
    if (response.status === 409 && body?.error === 'desk_stale' && body.desk && typeof body.desk === 'object') desk = body.desk
  } catch { /* fixed fallback */ }
  if (desk) throw new StaleDesk(desk)
  throw new TerminalError(response.status, message, code)
}

export async function listDesks(): Promise<DeskSummary[]> {
  const body = await (await deskRequest(base)).json()
  return Array.isArray(body?.desks) ? body.desks : []
}

export async function getDesk(id: string): Promise<Desk> {
  return (await deskRequest(`${base}/${encodeURIComponent(id)}`)).json()
}

export async function createDesk(input: { name: string; colour?: ProjectColour; template?: string; from?: string }): Promise<Desk> {
  return (await deskRequest(base, json('POST', input))).json()
}

export async function patchDesk(id: string, input: { expect: number; name?: string; colour?: ProjectColour; doc?: Doc }): Promise<Desk> {
  return (await deskRequest(`${base}/${encodeURIComponent(id)}`, json('PATCH', input))).json()
}

/** Deletes a desk. Sessions in it keep running unless `endSessions`; `left` lists the ones left running. */
export async function deleteDesk(id: string, input: { expect: number; endSessions: boolean }): Promise<{ left?: string[] }> {
  return (await deskRequest(`${base}/${encodeURIComponent(id)}`, json('DELETE', input))).json()
}

export async function reorderDesks(ids: string[]): Promise<void> {
  await deskRequest(`${base}/reorder`, json('POST', { ids }))
}

/** Starts a fresh shell in a pane that has none (a press, never on open). */
export async function reopenPane(id: string, pane: string, input: { expect: number; home?: boolean }): Promise<{ desk: Desk; session: TerminalSession }> {
  return (await deskRequest(`${base}/${encodeURIComponent(id)}/panes/${encodeURIComponent(pane)}/reopen`, json('POST', input))).json()
}
