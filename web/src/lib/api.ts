export type Skill = {
  id: string
  source: string
  slug: string
  sourceType: 'github' | 'well-known' | 'local'
  url: string
  name: string
  description: string
  notes: string
  installs: number
  installsWeek: number
  trend: number[]
  liked: boolean
  bookmarked: boolean
  used: boolean
  createdAt: number
  updatedAt: number
  tags?: Tag[]
  environments?: Environment[]
}
export type Counts = { all: number; liked: number; bookmarked: number; used: number }
export type Tab = 'all' | 'liked' | 'bookmarked' | 'used'
export type Sort = 'recent' | 'name' | 'installs'
export type Flags = { liked?: boolean; bookmarked?: boolean; used?: boolean; notes?: string }
export type AddResult = Skill & { created: boolean }

export type Tag = { id: number; name: string; source?: 'topic' | 'user'; hidden?: boolean; count?: number }
export type Environment = {
  id: string
  name: string
  kind: string
  agent: string
  host: string
  path?: string
  firstSeen?: number
  lastSeen: number
  sourcePath?: string
  skillCount?: number
}
export type Event = {
  id: number
  at: number
  actor?: string
  source: string
  action: string
  skillId?: string
  skillName?: string
  detail?: string
}
export type Ignored = { id: string; name: string; reason?: string; at: number }
export type SkillQuery = { tab: Tab; q: string; sort: Sort; tags?: number[]; env?: string; only?: string }
export type Backup = { version?: number; exportedAt?: number; skills: Skill[]; ignored?: Ignored[]; events?: Event[] }

const KEY = 'reet-token'

/** The instance refused the request; the UI shows the sign-in card instead of an error. */
export class UnauthorizedError extends Error {
  constructor() {
    super('missing or wrong token')
    this.name = 'UnauthorizedError'
  }
}

export function getToken(): string {
  try {
    return localStorage.getItem(KEY) ?? ''
  } catch {
    return ''
  }
}

export function setToken(t: string) {
  try {
    localStorage.setItem(KEY, t)
  } catch {
    /* storage unavailable: token lasts until reload */
  }
}

export function clearToken() {
  try {
    localStorage.removeItem(KEY)
  } catch {
    /* storage unavailable: nothing was stored */
  }
}

async function req(path: string, init: RequestInit = {}): Promise<Response> {
  const headers = new Headers(init.headers)
  if (init.body && !headers.has('Content-Type')) headers.set('Content-Type', 'application/json')
  const token = getToken()
  if (token) headers.set('X-Reet-Token', token)
  const res = await fetch(path, { ...init, headers })
  if (res.status === 401) throw new UnauthorizedError()
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`
    try {
      msg = (await res.json()).error ?? msg
    } catch {
      /* keep status text */
    }
    throw new Error(msg)
  }
  return res
}

const json = (v: unknown) => JSON.stringify(v)

export async function listSkills({ tab, q, sort, tags, env, only }: SkillQuery): Promise<{ skills: Skill[]; counts: Counts }> {
  const p = new URLSearchParams({ tab, q, sort })
  if (tags?.length) p.set('tag', tags.join(','))
  if (env) p.set('env', env)
  if (only) p.set('only', only)
  return (await req(`/api/skills?${p}`)).json()
}
export async function addSkill(input: string, flags: Flags = {}): Promise<AddResult> {
  return (await req('/api/skills', { method: 'POST', body: json({ input, ...flags }) })).json()
}
export async function patchSkill(id: string, flags: Flags): Promise<Skill> {
  return (await req('/api/skill', { method: 'PATCH', body: json({ id, ...flags }) })).json()
}
export async function removeSkill(id: string, ignore = false): Promise<void> {
  await req(`/api/skill?id=${encodeURIComponent(id)}${ignore ? '&ignore=1' : ''}`, { method: 'DELETE' })
}
export async function refreshSkill(id: string): Promise<Skill> {
  return (await req('/api/skill/refresh', { method: 'POST', body: json({ id }) })).json()
}
export async function resolveSkill(id: string, input: string): Promise<Skill> {
  return (await req('/api/skill/resolve', { method: 'POST', body: json({ id, input }) })).json()
}
export async function importText(text: string): Promise<{ imported: number; ignored: number; errors: string[] }> {
  return (await req('/api/import', { method: 'POST', body: json({ text }) })).json()
}
export async function exportAll(includeEvents = true): Promise<Blob> {
  return (await req(`/api/export${includeEvents ? '' : '?events=0'}`)).blob()
}
export async function restoreAll(data: unknown): Promise<{ restored: number }> {
  return (await req('/api/restore', { method: 'POST', body: json(data) })).json()
}
export async function listEnvironments(): Promise<{ environments: Environment[] }> {
  return (await req('/api/environments')).json()
}
export async function listTags(): Promise<{ tags: Tag[] }> {
  return (await req('/api/tags')).json()
}
export async function addSkillTags(id: string, add: string[], remove: number[]): Promise<Skill> {
  return (await req('/api/skill/tags', { method: 'POST', body: json({ id, add, remove }) })).json()
}
export async function renameTag(id: number, name: string): Promise<void> {
  await req('/api/tag', { method: 'PATCH', body: json({ id, name }) })
}
export async function mergeTags(from: number, into: number): Promise<void> {
  await req('/api/tags/merge', { method: 'POST', body: json({ from, into }) })
}
export async function listEvents(limit = 20, skill?: string): Promise<{ events: Event[] }> {
  const p = new URLSearchParams({ limit: String(limit) })
  if (skill) p.set('skill', skill)
  return (await req(`/api/events?${p}`)).json()
}
export async function recordInstallCopy(id: string): Promise<void> {
  await req('/api/skill/install-copy', { method: 'POST', body: json({ id }) })
}
export async function listIgnored(): Promise<{ ignored: Ignored[] }> {
  return (await req('/api/ignored')).json()
}
export async function unignore(id: string): Promise<void> {
  await req(`/api/skill/ignore?id=${encodeURIComponent(id)}`, { method: 'DELETE' })
}
export async function getBuildInfo(): Promise<{ version: string; commit: string }> {
  return (await req('/api/version')).json()
}
