import { terminalRequest, type TerminalSession } from './terminal/api'
import type { SessionDraft } from './sessionBoard'

const path = '/api/v2/session-backlog'
const at = (id: string) => `${path}/${encodeURIComponent(id)}`
export async function listDrafts(signal?: AbortSignal): Promise<SessionDraft[]> {
  return (await (await terminalRequest(path, { signal })).json()).drafts
}
export async function getDraft(id: string, signal?: AbortSignal): Promise<SessionDraft> {
  return (await terminalRequest(at(id), { signal })).json()
}
export type DraftInput = Pick<SessionDraft, 'title' | 'prompt' | 'agentId' | 'projectId' | 'dir' | 'tool' | 'model' | 'effort' | 'yolo' | 'worktree' | 'base' | 'chat' | 'trust' | 'images' | 'version'>
export async function saveDraft(input: DraftInput, id?: string, signal?: AbortSignal): Promise<SessionDraft> {
  return (await terminalRequest(id ? at(id) : path, { method: id ? 'PUT' : 'POST', body: JSON.stringify(input), signal })).json()
}
export async function deleteDraft(draft: SessionDraft, signal?: AbortSignal): Promise<void> {
  await terminalRequest(at(draft.id), { method: 'DELETE', body: JSON.stringify({ version: draft.version }), signal })
}
export async function startDraft(draft: SessionDraft, prompt: string, signal?: AbortSignal): Promise<TerminalSession> {
  return (await terminalRequest(`${at(draft.id)}/start`, { method: 'POST', body: JSON.stringify({ version: draft.version, prompt }), signal })).json()
}
