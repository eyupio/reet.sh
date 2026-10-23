export type Skill = {
  id: string
  source: string
  slug: string
  sourceType: 'github' | 'well-known' | 'hosted' | 'local'
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
  sharedWithMe?: boolean
  name: string
  description: string
  visibility: 'personal' | 'private' | 'team' | 'public' | 'members' | 'link'
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
  audience?: Audience
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
  signupEnabled?: boolean
  contactEmail?: string
  passwordReset?: boolean
  user: User | null
}
export type APIToken = { id: number; name: string; createdAt: number; lastUsedAt?: number }
export type MCPConnection = { id: number; name: string; scope: 'read' | 'write'; enabled: boolean; kind: 'bearer' | 'oauth'; createdAt: number; expiresAt: number; lastUsedAt: number }
export type MCPAccess = { enabled: boolean; browserEnabled: boolean; browserScope: 'read' | 'write'; canWrite: boolean; endpoint: string; connections: MCPConnection[] }
export type MCPTool = { name: string; title?: string; description: string; inputSchema: Record<string, unknown>; annotations: { readOnlyHint: boolean; destructiveHint?: boolean } }
export type MCPConsent = { clientName: string; callbackHost: string; scope: 'read' | 'write'; enabled: boolean; canWrite: boolean; account: string }
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
export type SkillQuery = { tab: Tab; q: string; sort: Sort; tags?: number[]; env?: string; machine?: string; only?: string; stage?: string }
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

export async function getMachineHealth(signal?:AbortSignal):Promise<import('./terminal/api').HealthResponse> { return (await req('/api/v2/machines/health',{signal,cache:'no-store'})).json() }
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
export async function signUpAccount(data: { name: string; email: string; password: string }): Promise<User> {
  return (await req('/api/v2/auth/signup', { method: 'POST', body: json(data) })).json()
}
export async function signInAccount(email: string, password: string): Promise<User> {
  return (await req('/api/v2/auth/sign-in', { method: 'POST', body: json({ email, password }) })).json()
}
export async function signOutAccount(): Promise<void> {
  await req('/api/v2/auth/sign-out', { method: 'POST', body: json({}) })
}
export async function listSkills({ tab, q, sort, tags, env, machine, only }: SkillQuery): Promise<{ skills: Skill[]; counts: Counts }> {
  const p = new URLSearchParams({ tab, q, sort })
  if (tags?.length) p.set('tag', tags.join(','))
  if (env) p.set('env', env)
  if (machine) p.set('machine', machine)
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
      if (filters.machine) p.set('machine', filters.machine)
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
export async function getMCPAccess(): Promise<MCPAccess> {
  return (await req('/api/v2/mcp')).json()
}
export async function saveMCPAccess(settings: Partial<Pick<MCPAccess, 'enabled' | 'browserEnabled' | 'browserScope'>>): Promise<MCPAccess> {
  return (await req('/api/v2/mcp', { method: 'PATCH', body: json(settings) })).json()
}
export async function createMCPConnection(name: string, scope: string, expiresInDays: number): Promise<{ connection: MCPConnection; token: string }> {
  return (await req('/api/v2/mcp/connections', { method: 'POST', body: json({ name, scope, expiresInDays }) })).json()
}
export async function updateMCPConnection(id: number, data: { name?: string; scope?: string; enabled?: boolean; expiresInDays?: number }): Promise<MCPAccess> {
  return (await req(`/api/v2/mcp/connections/${id}`, { method: 'PATCH', body: json(data) })).json()
}
export async function rotateMCPConnection(id: number): Promise<{ connection: MCPConnection; token: string }> {
  return (await req(`/api/v2/mcp/connections/${id}/rotate`, { method: 'POST', body: '{}' })).json()
}
export async function revokeMCPConnection(id?: number): Promise<void> {
  await req(`/api/v2/mcp/connections${id === undefined ? '' : `/${id}`}`, { method: 'DELETE' })
}
export async function getMCPConsent(query: string): Promise<MCPConsent> {
  return (await req(`/api/v2/mcp/authorization?${query}`)).json()
}
export async function answerMCPConsent(query: string, approve: boolean, scope: string): Promise<{ redirect: string }> {
  return (await req('/api/v2/mcp/authorization', { method: 'POST', body: json({ query, approve, scope }) })).json()
}
export async function getBrowserMCPTools(): Promise<{ tools: MCPTool[] }> {
  return (await req('/api/v2/mcp/browser/tools')).json()
}
export async function callBrowserMCP(name: string, args: Record<string, unknown>): Promise<Record<string, unknown>> {
  return (await req('/api/v2/mcp/browser/call', { method: 'POST', body: json({ name, arguments: args }) })).json()
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
export async function resetUserPassword(id: number): Promise<{ password?: string; emailed?: boolean; email?: string }> {
  return (await req('/api/v2/admin/users/reset', { method: 'POST', body: json({ id }) })).json()
}
export async function revokeUserTokens(id: number): Promise<void> {
  await req('/api/v2/admin/users/revoke', { method: 'POST', body: json({ id }) })
}
export async function transferOwner(id: number): Promise<{ users: UserMeta[] }> {
  return (await req('/api/v2/admin/users/transfer', { method: 'POST', body: json({ id }) })).json()
}
export async function createInvite(email: string, role: string): Promise<{ url: string; token: string; email: string; role: string; expiresAt: number; emailed?: boolean }> {
  return (await req('/api/v2/admin/invites', { method: 'POST', body: json({ email, role }) })).json()
}
export async function getInvite(token: string): Promise<Invite> {
  return (await req(`/api/v2/auth/invite?token=${encodeURIComponent(token)}`)).json()
}
export async function joinAccount(data: { token: string; name: string; password: string }): Promise<User> {
  return (await req('/api/v2/auth/join', { method: 'POST', body: json(data) })).json()
}
export async function listEnvironments(): Promise<{ environments: Environment[]; machines: Machine[] }> {
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
  settings: { refreshEnabled: boolean; topicsEnabled: boolean; linkListsEnabled: boolean; rollupIndividual: boolean; pruneServerEnvironmentsDays: number; instanceName: string }
}
export async function getAdminStatus(): Promise<AdminStatus> {
  return (await req('/api/admin/status')).json()
}
export async function saveAdminSettings(settings: { refreshEnabled?: boolean; topicsEnabled?: boolean; linkListsEnabled?: boolean; rollupIndividual?: boolean; pruneServerEnvironmentsDays?: number; instanceName?: string }): Promise<void> {
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
export async function listLists(): Promise<{ lists: SharedList[]; linkEnabled: boolean; userId?: number }> {
  return (await req('/api/v2/lists')).json()
}
export async function getList(id: number): Promise<ListDetail> {
  return (await req(`/api/v2/lists/${id}`)).json()
}
export async function getPublicList(slug: string): Promise<ListDetail> {
  return (await req(`/api/v2/public/lists/${encodeURIComponent(slug)}`)).json()
}
export async function createList(data: { name: string; description: string; visibility: string; audience?: Audience }): Promise<ListDetail> {
  return (await req('/api/v2/lists', { method: 'POST', body: json(data) })).json()
}
export async function updateList(id: number, data: { name?: string; description?: string; visibility?: string; audience?: Audience }): Promise<ListDetail> {
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

export type SyncEnvironment = Environment & { error?: string; project?: string; scannedAt: number; complete: boolean; os: string; arch: string; version: string; skills: SyncSkill[] }
export type SyncSkill = Pick<Skill, 'id' | 'name' | 'slug' | 'source' | 'sourceType'>
export type SyncTarget = { mode?: string; baseline?: SyncSkill[]; appliedRevision?: string; hostAliases?: string[]; targetAliases?: string[]; target: Environment & { project?: string }; sourceId: string; sourceName: string; desired: SyncSkill[]; missing: SyncSkill[]; extra: SyncSkill[]; revision: string; updatedAt: number }
export type SetupProfileSummary = { name: string; revision: number; snapshotId: string; snapshotDigest: string; updatedAt: number }
export type SetupProfileComponent = {
  kind: string; source: string; scope: string; state?: string
  format?: string; observed?: string; selection?: string
  declarations?: number; selected_declarations?: number; declaration_names?: string[]
  included_fields?: string[]; encrypted_fields?: string[]; reference_fields?: string[]
  excluded_fields?: string[]; unknown_fields?: number; digest?: string
}
export type SetupProfilePreview = {
  os?: string
  architecture?: string
  schema_version?: number
  agents?: { id: string; name: string; state?: string; components?: SetupProfileComponent[]; omitted?: { kind: string; reason: string; count?: number }[] }[]
  skills?: { name: string; agent_id: string; scope: string; source: string; origin?: string; origin_type?: string; files: number; bytes: number; digest: string }[]
  memories?: { name: string; agent_id: string; scope: string; source: string; bytes: number; digest: string }[]
  classes?: { kind: string; selection: string; handling: string; reason: string }[]
  credentials_included?: boolean
  limitations?: string[]
}
export type SetupProfileOverrides = {
  exclude_kinds?: string[]; exclude_sources?: string[]
  machines?: Record<string, { exclude_kinds?: string[]; exclude_sources?: string[] }>
  projects?: Record<string, { exclude_kinds?: string[]; exclude_sources?: string[] }>
}
export type SetupProfileRevision = {
  revision: number
  created_at: string
  snapshot_id: string
  snapshot_digest: string
  snapshot_taken_at: string
  preview?: SetupProfilePreview
  software?: { agent_id: string; name: string; application_version?: string; installation_package?: string }[]
  overrides?: SetupProfileOverrides
}
export type SetupProfileRecord = { schema_version: number; name: string; revisions: SetupProfileRevision[] }
export type Machine = { aliases?: string[]; host: string; groups: SyncEnvironment[]; skillCount: number }
export type SyncState = { agents?: SyncAgent[]; jobs?: SyncJob[]; machines?: Machine[]; environments: SyncEnvironment[]; targets: SyncTarget[] }
export async function getSyncState(): Promise<SyncState> { return (await req('/api/v2/sync')).json() }
export async function saveSyncTarget(sourceId: string, targetId: string): Promise<SyncState> { return (await req('/api/v2/sync', { method: 'PUT', body: json({ sourceId, targetId }) })).json() }
export async function deleteSyncTarget(targetId: string): Promise<SyncState> { return (await req(`/api/v2/sync?target=${encodeURIComponent(targetId)}`, { method: 'DELETE' })).json() }
export async function mergeMachines(from: string, to: string): Promise<SyncState> { return (await req('/api/v2/machines/merge', { method: 'POST', body: json({ from, to }) })).json() }
export async function forgetMachine(host: string): Promise<SyncState> { return (await req(`/api/v2/machines?host=${encodeURIComponent(host)}`, { method: 'DELETE' })).json() }
export async function forgetEnvironment(id: string): Promise<SyncState> { return (await req(`/api/v2/environments?id=${encodeURIComponent(id)}`, { method: 'DELETE' })).json() }

export type Audience = { groupId?: number; groupRole?: string; people: { email: string; role: string }[]; excluded: string[] }
export type GroupMember = { id: number; name: string; email: string; role: string }
export type Group = { id: number; name: string; kind: 'team' | 'organisation'; parentId?: number; ownerId: number; role: string; members?: GroupMember[] }
export async function getGroups(): Promise<{ groups: Group[] }> { return (await req('/api/v2/groups')).json() }
export async function createGroup(data: { name: string; kind: string; parentId?: number }): Promise<{ groups: Group[] }> { return (await req('/api/v2/groups', { method: 'POST', body: json(data) })).json() }
export async function setGroupMember(id: number, email: string, role: string, remove = false): Promise<{ groups: Group[] }> { return (await req(`/api/v2/groups/${id}/members`, { method: 'POST', body: json({ email, role, remove }) })).json() }
export async function getCommunity(): Promise<{ lists: SharedList[]; skills: ListSkill[] }> { return (await req('/api/community')).json() }
export function audienceLabel(visibility: string): string { return ({ personal: 'Personal', private: 'Selected people', team: 'Team / organisation', public: 'Public community', members: 'Legacy instance audience', link: 'Unlisted link' } as Record<string, string>)[visibility] ?? visibility }

export async function updateGroup(id: number, name: string, remove = false): Promise<{ groups: Group[] }> { return (await req(`/api/v2/groups/${id}`, { method: 'PATCH', body: json({ name, remove }) })).json() }

// --- backups ---------------------------------------------------------------

export type BackupRemote = {
  name: string
  source: 'environment' | 'database'
  endpoint: string
  region: string
  bucket: string
  prefix: string
  accessKey: string
  pathStyle: boolean | null
  keep: number
  disabled: boolean
  hasSecret: boolean
  where: string
  problem?: string
  uploading: boolean
  lastUploadAt?: number
  lastError?: string
  lastErrorAt?: number
}
export type BackupItem = {
  id: string
  kind: 'instance' | 'user'
  target: string
  takenAt: number
  bytes: number
  version?: string
  local: boolean
  remotes: string[]
}
export type BackupRun = {
  id: number
  scope: string
  trigger: string
  state: 'running' | 'succeeded' | 'failed'
  startedAt: number
  finishedAt: number
  bytes: number
  object: string
  offsite: string
  error: string
}
export type BackupFinding = { severity: 'error' | 'warning' | 'info'; code: string; message: string }
export type Notification = {
  id: number
  kind: string
  teamId?: number
  title: string
  body?: string
  link?: string
  tone?: 'warning' | 'danger' | ''
  readAt?: number
  emailedAt?: number
  createdAt: number
  updatedAt: number
}
export type NotificationPreference = {
  key: string
  audience: 'staff' | 'user'
  group: string
  label: string
  description: string
  locked?: string
  security?: boolean
  ownerDefault?: boolean
  adminDefault?: boolean
  tone?: string
  enabled: boolean
  mode: 'immediate' | 'none'
  source: string
}
export type NotificationDefault = {
  key: string
  audience: 'staff' | 'user'
  group: string
  label: string
  description: string
  locked?: string
  security?: boolean
  ownerDefault?: boolean
  adminDefault?: boolean
  tone?: string
  default: { enabled: boolean; mode: 'immediate' | 'none'; owner: boolean; admin: boolean; updatedAt?: number; updatedBy?: string }
}
export type TicketStatus = 'open' | 'waiting' | 'closed'
export type Ticket = {
  id: number
  userId?: number
  opener: string
  groupId?: number
  groupName?: string
  subject: string
  status: TicketStatus
  assigneeId?: number
  assignee?: string
  messages: number
  createdAt: number
  updatedAt: number
}
export type TicketMessage = {
  id: number
  ticketId: number
  authorId?: number
  authorKind: 'user' | 'team' | 'admin'
  author: string
  body: string
  internal?: boolean
  createdAt: number
}
export type TicketQueue = { tickets: Ticket[]; counts: Record<TicketStatus, number> }
export type TicketDetail = { ticket: Ticket; messages: TicketMessage[] }
export type MailSettingsView = {
  source: 'database' | 'none'
  mode: 'smtp' | 'outbox' | 'held' | 'off'
  summary: string
  problem?: string
  host: string
  port: number
  tls: 'starttls' | 'implicit' | 'none'
  user: string
  passwordSet: boolean
  from: string
  fromName: string
  replyTo: string
  baseUrl: string
  baseUrlInUse: string
  dkimSelector: string
  updatedAt?: number
  updatedBy?: string
}
export type MailMessage = {
  id: string
  template: string
  to: string
  subject: string
  state: string
  attempts: number
  lastError?: string
  via?: string
  createdAt: number
  nextAttemptAt?: number
  deliveredAt?: number
  secret: boolean
  resendable: boolean
}
export type MailOutbox = {
  mode: string
  summary: string
  warning?: string
  messages: MailMessage[]
  counts: Record<string, number>
}
export type MailCheck = { ok: boolean; message: string; transcript: string[] }
export type MailDNSRecord = { state: 'found' | 'missing' | 'unknown'; record?: string; advice: string }
export type MailDNS = { domain: string; spf: MailDNSRecord; dkim: MailDNSRecord; dmarc: MailDNSRecord; bimi: MailDNSRecord }
export type TeamNotify = {
  groupId: number
  enabled: boolean
  events: string[]
  recipients: 'managers' | 'members'
  extraEmails: string[]
  updatedAt?: number
  updatedBy?: number
}
export type TeamNotifyEvent = { key: string; label: string; description: string }
export type TeamNotifyView = { notify: TeamNotify; events: TeamNotifyEvent[]; mailReady: boolean; mailMode: string; groupName: string }
export type BackupStatus = {
  scope: 'instance' | 'user'
  enabled: boolean
  dir: string
  key: { fingerprint: string; source: string; path?: string }
  settings: { everySeconds: number; keep: number; destination: BackupDestination; compression: BackupCompression; hasPassword: boolean; nextAt: number; lastAt: number; lastState: string; lastError: string; updatedAt: number }
  defaults: { everySeconds: number; keep: number; userEverySeconds: number; userKeep: number; destination: BackupDestination; compression: BackupCompression }
  remotes: BackupRemote[]
  runs: BackupRun[]
  items: BackupItem[]
  unreachable?: string[]
  findings: BackupFinding[]
}
export type BackupRemoteInput = {
  name?: string
  originalName?: string
  endpoint?: string
  region?: string
  bucket?: string
  prefix?: string
  accessKey?: string
  secretKey?: string
  pathStyle?: boolean | null
  keep?: number
  enabled?: boolean
}
export type BackupCheck = { name: string; ok: boolean; error?: string; steps: { step: string; ok: boolean; detail: string }[] }
export type BackupDestination = 'local' | 'remote' | 'both'
export type BackupCompression = 'gzip' | 'none'

const bpath = (admin: boolean) => (admin ? '/api/v2/admin/backups' : '/api/v2/backups')

export async function listNotifications(unreadOnly = false, limit = 50): Promise<{ notifications: Notification[]; unread: number }> {
  const p = new URLSearchParams({ limit: String(limit) })
  if (unreadOnly) p.set('unread', '1')
  return (await req(`/api/v2/notifications?${p}`)).json()
}
export async function getNotificationCount(): Promise<{ unread: number }> {
  return (await req('/api/v2/notifications/count')).json()
}
export async function markNotificationsRead(ids: number[] = [], read = true): Promise<{ unread: number }> {
  return (await req('/api/v2/notifications/read', { method: 'POST', body: json({ ids, read }) })).json()
}
export async function getNotificationPreferences(): Promise<{ preferences: NotificationPreference[] }> {
  return (await req('/api/v2/notifications/preferences')).json()
}
export async function saveNotificationPreferences(preferences: { key: string; enabled: boolean; mode: string }[]): Promise<{ preferences: NotificationPreference[]; done: string }> {
  return (await req('/api/v2/notifications/preferences', { method: 'PATCH', body: json({ preferences }) })).json()
}
export async function getNotificationDefaults(): Promise<{ defaults: NotificationDefault[] }> {
  return (await req('/api/v2/admin/notifications/defaults')).json()
}
export async function saveNotificationDefaults(defaults: { kind: string; enabled: boolean; mode: string; owner: boolean; admin: boolean }[]): Promise<{ defaults: NotificationDefault[]; done: string }> {
  return (await req('/api/v2/admin/notifications/defaults', { method: 'PATCH', body: json({ defaults }) })).json()
}
export async function listTickets(status = ''): Promise<{ tickets: Ticket[] }> {
  return (await req(`/api/v2/tickets${status ? `?status=${encodeURIComponent(status)}` : ''}`)).json()
}
export async function openTicket(data: { subject: string; body: string; groupId?: number }): Promise<{ ticket: Ticket }> {
  return (await req('/api/v2/tickets', { method: 'POST', body: json(data) })).json()
}
export async function getTicket(id: number): Promise<TicketDetail> { return (await req(`/api/v2/tickets/${id}`)).json() }
export async function replyTicket(id: number, body: string): Promise<{ ticket: Ticket; message: TicketMessage }> {
  return (await req(`/api/v2/tickets/${id}/reply`, { method: 'POST', body: json({ body }) })).json()
}
export async function closeTicket(id: number): Promise<{ ticket: Ticket }> {
  return (await req(`/api/v2/tickets/${id}/close`, { method: 'POST', body: json({}) })).json()
}
export async function adminListTickets(status = '', assignee = 0): Promise<TicketQueue> {
  const p = new URLSearchParams()
  if (status) p.set('status', status)
  if (assignee) p.set('assignee', String(assignee))
  const qs = p.toString()
  return (await req(`/api/v2/admin/tickets${qs ? `?${qs}` : ''}`)).json()
}
export async function adminGetTicket(id: number): Promise<TicketDetail> { return (await req(`/api/v2/admin/tickets/${id}`)).json() }
export async function adminReplyTicket(id: number, body: string, status = 'waiting'): Promise<{ ticket: Ticket; message: TicketMessage }> {
  return (await req(`/api/v2/admin/tickets/${id}/reply`, { method: 'POST', body: json({ body, status }) })).json()
}
export async function adminNoteTicket(id: number, body: string): Promise<{ ticket: Ticket; message: TicketMessage }> {
  return (await req(`/api/v2/admin/tickets/${id}/note`, { method: 'POST', body: json({ body }) })).json()
}
export async function adminSetTicketStatus(id: number, status: TicketStatus): Promise<{ ticket: Ticket }> {
  return (await req(`/api/v2/admin/tickets/${id}/status`, { method: 'POST', body: json({ status }) })).json()
}
export async function adminAssignTicket(id: number, assigneeId: number): Promise<{ ticket: Ticket }> {
  return (await req(`/api/v2/admin/tickets/${id}/assign`, { method: 'POST', body: json({ assigneeId }) })).json()
}
export async function getMailSettings(): Promise<{ settings: MailSettingsView }> { return (await req('/api/v2/admin/mail')).json() }
export async function saveMailSettings(settings: {
  host: string; port: number; tls: string; user: string; password?: string; passwordAction: string;
  from: string; fromName: string; replyTo: string; baseUrl: string; dkimSelector: string
}): Promise<{ settings: MailSettingsView; done: string }> { return (await req('/api/v2/admin/mail', { method: 'PATCH', body: json(settings) })).json() }
export async function checkMailSettings(settings: {
  host: string; port: number; tls: string; user: string; password?: string; passwordAction: string;
  from: string; fromName: string; replyTo: string; baseUrl: string; dkimSelector: string
}): Promise<MailCheck> { return (await req('/api/v2/admin/mail/check', { method: 'POST', body: json(settings) })).json() }
export async function sendTestMail(): Promise<MailCheck> { return (await req('/api/v2/admin/mail/test', { method: 'POST', body: json({}) })).json() }
export async function getMailDNS(from: string, selector: string): Promise<MailDNS> {
  const p = new URLSearchParams({ from, selector })
  return (await req(`/api/v2/admin/mail/dns?${p}`)).json()
}
export async function listMailOutbox(state = ''): Promise<MailOutbox> {
  return (await req(`/api/v2/admin/mail/outbox${state ? `?state=${encodeURIComponent(state)}` : ''}`)).json()
}
export async function resendMail(id: string): Promise<{ message: MailMessage; done: string }> {
  return (await req('/api/v2/admin/mail/outbox/resend', { method: 'POST', body: json({ id }) })).json()
}
export async function forgotPassword(email: string): Promise<{ message: string }> {
  return (await req('/api/v2/auth/forgot', { method: 'POST', body: json({ email }) })).json()
}
export async function resetPassword(token: string, password: string): Promise<{ ok: boolean; message: string }> {
  return (await req('/api/v2/auth/reset', { method: 'POST', body: json({ token, password }) })).json()
}
export async function getTeamNotifications(id: number): Promise<TeamNotifyView> { return (await req(`/api/v2/groups/${id}/notifications`)).json() }
export async function saveTeamNotifications(id: number, notify: Partial<TeamNotify>): Promise<TeamNotifyView> {
  return (await req(`/api/v2/groups/${id}/notifications`, { method: 'PATCH', body: json(notify) })).json()
}
export async function getBackupStatus(admin = false): Promise<BackupStatus> {
  return (await req(bpath(admin))).json()
}
export async function saveBackupSettings(
  admin: boolean, everySeconds: number, keep: number, destination: BackupDestination,
  compression: BackupCompression, password?: string,
): Promise<void> {
  const body: Record<string, unknown> = { everySeconds, keep, destination, compression }
  if (password !== undefined) body.password = password
  await req(bpath(admin), { method: 'PATCH', body: json(body) })
}
export async function runBackupNow(admin = false): Promise<BackupItem> {
  return (await req(`${bpath(admin)}/run`, { method: 'POST', body: json({}) })).json()
}
export async function createBackupRemote(admin: boolean, input: BackupRemoteInput): Promise<BackupRemote> {
  return (await req(`${bpath(admin)}/remotes`, { method: 'POST', body: json(input) })).json()
}
export async function updateBackupRemote(admin: boolean, name: string, input: BackupRemoteInput): Promise<BackupRemote> {
  return (await req(`${bpath(admin)}/remotes/${encodeURIComponent(name)}`, { method: 'PATCH', body: json(input) })).json()
}
export async function deleteBackupRemote(admin: boolean, name: string): Promise<void> {
  await req(`${bpath(admin)}/remotes/${encodeURIComponent(name)}`, { method: 'DELETE' })
}
export async function testBackupRemoteDraft(admin: boolean, input: BackupRemoteInput): Promise<BackupCheck> {
  return (await req(`${bpath(admin)}/remotes/test`, { method: 'POST', body: json(input) })).json()
}
export async function testBackupRemote(admin: boolean, name: string): Promise<BackupCheck> {
  return (await req(`${bpath(admin)}/remotes/${encodeURIComponent(name)}/test`, { method: 'POST', body: json({}) })).json()
}
export async function listBackupRemoteCopies(admin: boolean, name: string): Promise<{ items: BackupItem[] }> {
  return (await req(`${bpath(admin)}/remotes/${encodeURIComponent(name)}/copies`)).json()
}
export type BackupShipment = { sent: { remote: string; id: string; bytes: number }[]; errors: Record<string, string> }
export type BackupPrune = { removed: string[]; remotes: { name: string; removed: string[]; error?: string }[] }
export async function shipBackupCopies(admin: boolean): Promise<BackupShipment> {
  return (await req(`${bpath(admin)}/offsite`, { method: 'POST', body: json({}) })).json()
}
export async function pruneBackupCopies(admin: boolean): Promise<BackupPrune> {
  return (await req(`${bpath(admin)}/prune`, { method: 'POST', body: json({}) })).json()
}
export async function deleteBackupRemoteCopy(admin: boolean, name: string, id: string): Promise<void> {
  await req(`${bpath(admin)}/remotes/${encodeURIComponent(name)}/copies/${encodeURIComponent(id)}`, { method: 'DELETE' })
}
export async function verifyBackup(admin: boolean, id: string, password?: string): Promise<{ ok: boolean; problems: string[] }> {
  const body = password ? { password } : {}
  return (await req(`${bpath(admin)}/items/${encodeURIComponent(id)}/verify`, { method: 'POST', body: json(body) })).json()
}
export async function deleteBackup(admin: boolean, id: string): Promise<void> {
  await req(`${bpath(admin)}/items/${encodeURIComponent(id)}`, { method: 'DELETE' })
}
export async function restoreUserBackup(id: string, password?: string): Promise<{ restored: number }> {
  const body = password ? { password } : {}
  return (await req(`/api/v2/backups/items/${encodeURIComponent(id)}/restore`, { method: 'POST', body: json(body) })).json()
}
export function backupDownloadUrl(admin: boolean, id: string): string {
  return `${bpath(admin)}/items/${encodeURIComponent(id)}/download`
}

// --- profile ---------------------------------------------------------------

export type Profile = { theme: 'system' | 'light' | 'dark'; timezone: string; dateFormat: string; shelfView: 'list' | 'board'; cardsExpanded: boolean; updatedAt: number }
export type ProfileInfo = { user: User; profile: Profile; sessionCount: number; accounts: boolean }
export type ProfileSession = { id: string; device: string; createdAt: number; lastSeen: number; expiresAt: number; current: boolean }

export function applyTheme(theme: string) {
  const root = document.documentElement
  if (theme === 'light' || theme === 'dark') root.dataset.theme = theme
  else delete root.dataset.theme
}
export async function getProfile(): Promise<ProfileInfo> {
  return (await req('/api/v2/profile')).json()
}
export async function saveProfile(input: { name?: string; theme?: string; timezone?: string; dateFormat?: string; shelfView?: 'list' | 'board'; cardsExpanded?: boolean }): Promise<ProfileInfo> {
  return (await req('/api/v2/profile', { method: 'PATCH', body: json(input) })).json()
}
export async function changePassword(current: string, password: string): Promise<void> {
  await req('/api/v2/profile/password', { method: 'POST', body: json({ current, password }) })
}
export async function listProfileSessions(): Promise<{ sessions: ProfileSession[] }> {
  return (await req('/api/v2/profile/sessions')).json()
}
export async function endProfileSession(id: string): Promise<void> {
  await req(`/api/v2/profile/sessions/${encodeURIComponent(id)}`, { method: 'DELETE' })
}
export async function endOtherProfileSessions(): Promise<{ ended: number }> {
  return (await req('/api/v2/profile/sessions/end-others', { method: 'POST', body: json({}) })).json()
}

export type HostedSkill = { slug: string; description: string; content: string; files: Record<string, string>; published: boolean; createdAt: number; updatedAt: number }
export type Hosting = { username: string; skills: HostedSkill[] }
export async function getHosting(): Promise<Hosting> { return (await req('/api/v2/hosting')).json() }
export async function claimPublishingUsername(username: string): Promise<Hosting> {
  return (await req('/api/v2/hosting/username', { method: 'POST', body: json({ username }) })).json()
}
export async function saveHostedSkill(skill: HostedSkill): Promise<HostedSkill> {
  return (await req(`/api/v2/hosting/skills/${encodeURIComponent(skill.slug)}`, { method: 'PUT', body: json(skill) })).json()
}
export async function deleteHostedSkill(slug: string): Promise<void> {
  await req(`/api/v2/hosting/skills/${encodeURIComponent(slug)}`, { method: 'DELETE' })
}

export type SyncAgent = { id: string; host: string; version: string; lastSeen: number; paused: boolean }
export type SyncPlan = { backups?: string[]; target: string; directory: string; revision: string; actions: { add: boolean; skill: SyncSkill }[] }
export type SyncJob = { id: string; agentId: string; targetId: string; kind: string; status: string; revision: string; hash: string; plan: Partial<SyncPlan>; message: string; createdAt: number; updatedAt: number }
export async function syncCandidates(listId = 0, collection = 'shelf'): Promise<{ skills: SyncSkill[] }> { return (await req(`/api/v2/sync/candidates?list=${listId}&collection=${encodeURIComponent(collection)}`)).json() }
export async function saveDesired(data: { targetId: string; sourceId: string; listId: number; skillIds: string[]; mode: string; revision: string }): Promise<SyncState> { return (await req('/api/v2/sync/desired', { method: 'PUT', body: json(data) })).json() }
export async function reconcileDesired(targetId: string): Promise<{ skills: SyncSkill[]; conflicts: string[]; revision: string }> { return (await req('/api/v2/sync/reconcile', { method: 'POST', body: json({ targetId }) })).json() }
export async function adoptSyncSkills(targetId: string, skillIds: string[], listId = 0): Promise<void> { await req('/api/v2/sync/adopt', { method: 'POST', body: json({ targetId, skillIds, listId }) }) }
export async function queueSyncJob(agentId: string, targetId: string, kind: string): Promise<SyncState> { return (await req('/api/v2/sync/jobs', { method: 'POST', body: json({ agentId, targetId, kind }) })).json() }
export async function controlSyncJob(id: string, action: string, hash = ''): Promise<SyncState> { return (await req('/api/v2/sync/jobs', { method: 'PATCH', body: json({ id, action, hash }) })).json() }
export async function controlSyncAgent(id: string, action: string): Promise<SyncState> { return (await req('/api/v2/sync/agents', { method: 'PATCH', body: json({ id, action }) })).json() }

export async function listSetupProfiles(): Promise<{ profiles: SetupProfileSummary[] }> {
  return (await req('/api/v2/setup/profiles')).json()
}
export async function getSetupProfile(name: string): Promise<SetupProfileRecord> {
  return (await req(`/api/v2/setup/profiles/${encodeURIComponent(name)}`)).json()
}
export async function renameSetupProfile(name: string, newName: string, baseRevision?: number): Promise<SetupProfileRecord> {
  return (await req(`/api/v2/setup/profiles/${encodeURIComponent(name)}`, { method: 'PATCH', body: json({ name: newName, base_revision: baseRevision }) })).json()
}
export async function updateSetupProfileOverrides(name: string, overrides: SetupProfileOverrides, baseRevision?: number): Promise<SetupProfileRecord> {
  return (await req(`/api/v2/setup/profiles/${encodeURIComponent(name)}`, { method: 'PATCH', body: json({ overrides, base_revision: baseRevision }) })).json()
}
export async function deleteSetupProfile(name: string): Promise<void> {
  await req(`/api/v2/setup/profiles/${encodeURIComponent(name)}`, { method: 'DELETE' })
}
