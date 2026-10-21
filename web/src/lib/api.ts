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
export type RefreshResult = Skill & { changed: boolean; refreshed: boolean }

export type User = { id: number; email: string; name: string; role: string }
export type AuthStatus = {
  mode: 'accounts' | 'token' | 'none'
  setupRequired: boolean
  bootstrapRequired: boolean
  user: User | null
}
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
export type SkillQuery = { tab: Tab; q: string; sort: Sort; tags?: number[]; env?: string; only?: string; stage?: string }
export type Backup = { version?: number; exportedAt?: number; skills: Skill[]; ignored?: Ignored[]; events?: Event[] }

const KEY = 'reet-token'

/** The instance refused the request; the UI shows the sign-in card instead of an error. */
export class UnauthorizedError extends Error {
  constructor(message = 'missing or wrong token') {
    super(message)
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
  if (res.status === 401) {
    let msg = 'missing or wrong token'
    try {
      msg = (await res.json()).error ?? msg
    } catch {
      /* keep the generic message */
    }
    throw new UnauthorizedError(msg)
  }
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

export async function getAuthStatus(): Promise<AuthStatus> {
  return (await req('/api/v2/auth/status')).json()
}
export async function setupAccount(data: { name: string; email: string; password: string }, bootstrapToken = ''): Promise<User> {
  const headers: Record<string, string> = {}
  if (bootstrapToken) headers['X-Reet-Token'] = bootstrapToken
  return (await req('/api/v2/auth/setup', { method: 'POST', body: json(data), headers })).json()
}
export async function signInAccount(email: string, password: string): Promise<User> {
  return (await req('/api/v2/auth/sign-in', { method: 'POST', body: json({ email, password }) })).json()
}
export async function signOutAccount(): Promise<void> {
  await req('/api/v2/auth/sign-out', { method: 'POST', body: json({}) })
}
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
export async function patchSkills(ids: string[], flags: Flags): Promise<{ updated: number }> {
  return (await req('/api/skills', { method: 'PATCH', body: json({ ids, ...flags }) })).json()
}
export async function tagSkills(ids: string[], add: string[], remove: number[]): Promise<{ tagged: number }> {
  return (await req('/api/skills/tags', { method: 'POST', body: json({ ids, add, remove }) })).json()
}
export async function removeSkills(ids: string[], ignore = false): Promise<{ removed: number }> {
  return (await req('/api/skills/remove', { method: 'POST', body: json({ ids, ignore }) })).json()
}
export async function refreshSkill(id: string): Promise<RefreshResult> {
  return (await req('/api/skill/refresh', { method: 'POST', body: json({ id }) })).json()
}
export async function resolveSkill(id: string, input: string): Promise<Skill> {
  return (await req('/api/skill/resolve', { method: 'POST', body: json({ id, input }) })).json()
}
export async function importText(text: string): Promise<{ imported: number; ignored: number; errors: string[] }> {
  return (await req('/api/import', { method: 'POST', body: json({ text }) })).json()
}
export async function exportAll(includeEvents = true, format: 'json' | 'md' | 'csv' = 'json', filters?: SkillQuery): Promise<Blob> {
  const p = new URLSearchParams()
  if (format === 'json') {
    if (!includeEvents) p.set('events', '0')
  } else {
    p.set('format', format)
    if (filters) {
      p.set('tab', filters.tab)
      p.set('q', filters.q)
      p.set('sort', filters.sort)
      if (filters.stage) p.set('stage', filters.stage)
      if (filters.tags?.length) p.set('tag', filters.tags.join(','))
      if (filters.env) p.set('env', filters.env)
      if (filters.only) p.set('only', filters.only)
    }
  }
  const qs = p.toString()
  return (await req(`/api/export${qs ? `?${qs}` : ''}`)).blob()
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
export type AdminStatus = {
  version: string
  commit: string
  db: { path: string; size: number }
  counts: Counts
  stages: { use: number; try: number; shelf: number }
  environments: Environment[]
  events: Event[]
  skillsSh: boolean
  metaLastOK: string
  metaLastError: string
  topics: { lastSynced: string; lastError: string }
  settings: { refreshEnabled: boolean; topicsEnabled: boolean; instanceName: string }
}
export async function getAdminStatus(): Promise<AdminStatus> {
  return (await req('/api/admin/status')).json()
}
export async function saveAdminSettings(settings: { refreshEnabled?: boolean; topicsEnabled?: boolean; instanceName?: string }): Promise<void> {
  await req('/api/admin/settings', { method: 'PATCH', body: json(settings) })
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
