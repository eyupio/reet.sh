import type { TerminalSession } from './terminal/api'

export type Stage = 'backlog' | 'active' | 'finished'
export type BoardView = 'board' | Stage
export type DraftImage = { name: string; data: string }
export type SessionDraft = {
  id: string; title: string; prompt: string; agentId: string; projectId: string; dir: string
  tool: string; model: string; effort: string; images?: DraftImage[]; imageCount: number
  idea?: boolean; yolo?: boolean; worktree?: boolean; base?: string; chat?: boolean; trust?: boolean
  starting: boolean; preparedPrompt?: string; sessionId?: string; version: number; createdAt: number; updatedAt: number
}
export type BoardItem = {
  key: string; title: string; stage: Stage; agentId: string; projectId: string; dir: string
  tool: string; model: string; effort: string; createdAt: number; updatedAt: number
  draft?: SessionDraft; session?: TerminalSession
}
export type BoardFilters = { query: string; machines: string[]; projects: string[]; agents: string[]; models: string[]; attention: boolean; images: boolean }
export const emptyFilters = (): BoardFilters => ({ query: '', machines: [], projects: [], agents: [], models: [], attention: false, images: false })
export const STAGES: { id: Stage; label: string; hint: string }[] = [
  { id: 'backlog', label: 'Backlog', hint: 'Good ideas, ready when you are.' },
  { id: 'active', label: 'Active', hint: 'Work under way across your machines.' },
  { id: 'finished', label: 'Finished', hint: 'Wrapped up and ready to review.' },
]

// A quiet terminal is still active. A completed chat turn does not end its
// persistent session. Only the lifecycle state places a session in Finished.
export function sessionStage(session: Pick<TerminalSession, 'state'>): Stage {
  return ['creating', 'running', 'end_requested'].includes(session.state) ? 'active' : 'finished'
}

export function boardItems(drafts: SessionDraft[], sessions: TerminalSession[]): BoardItem[] {
  return [
    ...drafts.filter(d => !d.sessionId).map(d => ({ ...d, key: `draft:${d.id}`, stage: 'backlog' as const, draft: d })),
    ...sessions.map(s => ({ key: `session:${s.id}`, title: s.title, stage: sessionStage(s), agentId: s.agentId, projectId: s.projectId ?? '', dir: s.dir ?? '', tool: s.tool ?? '', model: s.model ?? '', effort: s.effort ?? '', createdAt: s.createdAt, updatedAt: s.updatedAt, session: s })),
  ]
}

export function filterBoard(items: BoardItem[], filters: BoardFilters, names: { machines: Map<string, string>; projects: Map<string, string> }): BoardItem[] {
  const terms = filters.query.toLocaleLowerCase().trim().split(/\s+/).filter(Boolean)
  return items.filter(item => {
    const hay = [item.title, item.draft?.prompt, item.dir, item.tool, item.model, item.effort, names.machines.get(item.agentId), names.projects.get(item.projectId)].filter(Boolean).join(' ').toLocaleLowerCase()
    return terms.every(term => hay.includes(term))
      && (!filters.machines.length || filters.machines.includes(item.agentId))
      && (!filters.projects.length || filters.projects.includes(item.projectId || 'none'))
      && (!filters.agents.length || filters.agents.includes(item.tool || 'shell'))
      && (!filters.models.length || filters.models.includes(item.model || 'default'))
      && (!filters.attention || item.session?.attention === 'waiting' || item.session?.attention === 'failed' || item.session?.state === 'lost' || (item.session?.exitCode ?? 0) > 0)
      && (!filters.images || (item.draft?.imageCount ?? 0) > 0)
  })
}

export function sortBoard(items: BoardItem[], sort: string): BoardItem[] {
  return [...items].sort((a, b) => (sort === 'title' ? a.title.localeCompare(b.title) : sort === 'oldest' ? a.createdAt - b.createdAt : b.updatedAt - a.updatedAt) || a.key.localeCompare(b.key))
}

export function filterCount(filters: BoardFilters): number {
  return filters.machines.length + filters.projects.length + filters.agents.length + filters.models.length + Number(filters.attention) + Number(filters.images)
}
