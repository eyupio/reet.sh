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
  lists?: ListRef[]
}
export type ListRef = { id: number; name: string }
export type SharedList = {
  id: number
  ownerId: number
  ownerName?: string
  name: string
  description: string
  visibility: 'private' | 'members' | 'link'
  slug?: string
  role?: string
  skillCount: number
  memberCount: number
  createdAt: number
  updatedAt: number
}
export type ListSkill = Skill & {
  note?: string
  addedBy?: number
  addedByName?: string
  useCount: number
  memberCount: number
}
export type ListMember = { id: number; name: string; email: string; role: string }
export type ListDetail = SharedList & {
  skills: ListSkill[]
  members?: ListMember[]
  linkEnabled: boolean
  canEdit: boolean
  canManage: boolean
  canTransfer: boolean
}
export type RollupMember = { id: number; name: string; email: string; role: string; used: number; installed: number }
export type RollupMemberUse = { id: number; name: string; used: boolean; environments?: string[] }
export type RollupSkill = {
  id: string
  name: string
  source: string
  tags?: string[]
  lists?: ListRef[]
  usedBy: number
  onShelf: number
  installedBy: number
  environmentLinks: number
  members?: RollupMemberUse[]
}
export type RollupUnused = { listId: number; listName: string; skillId: string; name: string; source: string }
export type Rollup = {
  generatedAt: number
  individual: boolean
  teamSize: number
  nothingInstalled: number
  members: RollupMember[]
  lists: ListRef[]
  skills: RollupSkill[]
  drift: { unlisted: string[]; unused: RollupUnused[] }
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
export type APIToken = { id: number; name: string; createdAt: number; lastUsedAt?: number }
export type UserMeta = User & { createdAt: number; disabledAt?: number; lastActive?: number; tokens: number }
export type Invite = { email: string; role: string; expiresAt: number }
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
export async function listTokens(): Promise<{ tokens: APIToken[] }> {
  return (await req('/api/v2/tokens')).json()
}
export async function createToken(name: string): Promise<{ id: number; name: string; token: string; createdAt: number }> {
  return (await req('/api/v2/tokens', { method: 'POST', body: json({ name }) })).json()
}
export async function revokeToken(id: number): Promise<void> {
  await req(`/api/v2/tokens?id=${id}`, { method: 'DELETE' })
}
export async function listAdminUsers(): Promise<{ users: UserMeta[] }> {
  return (await req('/api/v2/admin/users')).json()
}
export async function updateAdminUser(data: { id: number; role?: string; disabled?: boolean }): Promise<{ users: UserMeta[] }> {
  return (await req('/api/v2/admin/users', { method: 'PATCH', body: json(data) })).json()
}
export async function deleteAdminUser(id: number): Promise<void> {
  await req(`/api/v2/admin/users?id=${id}`, { method: 'DELETE' })
}
export async function resetUserPassword(id: number): Promise<{ password: string }> {
  return (await req('/api/v2/admin/users/reset', { method: 'POST', body: json({ id }) })).json()
}
export async function revokeUserTokens(id: number): Promise<void> {
  await req('/api/v2/admin/users/revoke', { method: 'POST', body: json({ id }) })
}
export async function transferOwner(id: number): Promise<{ users: UserMeta[] }> {
  return (await req('/api/v2/admin/users/transfer', { method: 'POST', body: json({ id }) })).json()
}
export async function createInvite(email: string, role: string): Promise<{ url: string; token: string; email: string; role: string; expiresAt: number }> {
  return (await req('/api/v2/admin/invites', { method: 'POST', body: json({ email, role }) })).json()
}
export async function getInvite(token: string): Promise<Invite> {
  return (await req(`/api/v2/auth/invite?token=${encodeURIComponent(token)}`)).json()
}
export async function joinAccount(data: { token: string; name: string; password: string }): Promise<User> {
  return (await req('/api/v2/auth/join', { method: 'POST', body: json(data) })).json()
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
  settings: { refreshEnabled: boolean; topicsEnabled: boolean; linkListsEnabled: boolean; rollupIndividual: boolean; instanceName: string }
}
export async function getAdminStatus(): Promise<AdminStatus> {
  return (await req('/api/admin/status')).json()
}
export async function saveAdminSettings(settings: { refreshEnabled?: boolean; topicsEnabled?: boolean; linkListsEnabled?: boolean; rollupIndividual?: boolean; instanceName?: string }): Promise<void> {
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
export async function listLists(): Promise<{ lists: SharedList[]; linkEnabled: boolean }> {
  return (await req('/api/v2/lists')).json()
}
export async function getList(id: number): Promise<ListDetail> {
  return (await req(`/api/v2/lists/${id}`)).json()
}
export async function getPublicList(slug: string): Promise<ListDetail> {
  return (await req(`/api/v2/public/lists/${encodeURIComponent(slug)}`)).json()
}
export async function createList(data: { name: string; description: string; visibility: string }): Promise<ListDetail> {
  return (await req('/api/v2/lists', { method: 'POST', body: json(data) })).json()
}
export async function updateList(id: number, data: { name?: string; description?: string; visibility?: string }): Promise<ListDetail> {
  return (await req(`/api/v2/lists/${id}`, { method: 'PATCH', body: json(data) })).json()
}
export async function deleteList(id: number): Promise<void> {
  await req(`/api/v2/lists/${id}`, { method: 'DELETE' })
}
export async function addListSkill(id: number, data: { id?: string; input?: string; note?: string }): Promise<ListDetail> {
  return (await req(`/api/v2/lists/${id}/skills`, { method: 'POST', body: json(data) })).json()
}
export async function updateListSkill(id: number, skillId: string, note: string): Promise<ListDetail> {
  return (await req(`/api/v2/lists/${id}/skills`, { method: 'PATCH', body: json({ id: skillId, note }) })).json()
}
export async function removeListSkill(id: number, skillId: string): Promise<ListDetail> {
  return (await req(`/api/v2/lists/${id}/skills?id=${encodeURIComponent(skillId)}`, { method: 'DELETE' })).json()
}
export async function addListMember(id: number, email: string, role: string): Promise<ListDetail> {
  return (await req(`/api/v2/lists/${id}/members`, { method: 'POST', body: json({ email, role }) })).json()
}
export async function updateListMember(id: number, userId: number, role: string): Promise<ListDetail> {
  return (await req(`/api/v2/lists/${id}/members`, { method: 'PATCH', body: json({ userId, role }) })).json()
}
export async function removeListMember(id: number, userId: number): Promise<ListDetail> {
  return (await req(`/api/v2/lists/${id}/members?userId=${userId}`, { method: 'DELETE' })).json()
}
export async function transferList(id: number, userId: number): Promise<ListDetail> {
  return (await req(`/api/v2/lists/${id}/transfer`, { method: 'POST', body: json({ userId }) })).json()
}
export async function addListToShelf(id: number): Promise<{ added: number }> {
  return (await req(`/api/v2/lists/${id}/add-to-shelf`, { method: 'POST', body: json({}) })).json()
}
export async function getRollup(): Promise<Rollup> {
  return (await req('/api/admin/rollup')).json()
}

export type SyncEnvironment = Environment & { project?: string; scannedAt: number; complete: boolean; os: string; arch: string; version: string }
export type SyncSkill = Pick<Skill, 'id' | 'name' | 'slug' | 'source' | 'sourceType'>
export type SyncTarget = { target: Environment & { project?: string }; sourceId: string; sourceName: string; desired: SyncSkill[]; missing: SyncSkill[]; extra: SyncSkill[]; revision: string; updatedAt: number }
export type SyncState = { environments: SyncEnvironment[]; targets: SyncTarget[] }
export async function getSyncState(): Promise<SyncState> { return (await req('/api/v2/sync')).json() }
export async function saveSyncTarget(sourceId: string, targetId: string): Promise<SyncState> { return (await req('/api/v2/sync', { method: 'PUT', body: json({ sourceId, targetId }) })).json() }
export async function deleteSyncTarget(targetId: string): Promise<SyncState> { return (await req(`/api/v2/sync?target=${encodeURIComponent(targetId)}`, { method: 'DELETE' })).json() }
export async function forgetEnvironment(id: string): Promise<SyncState> { return (await req(`/api/v2/environments?id=${encodeURIComponent(id)}`, { method: 'DELETE' })).json() }
