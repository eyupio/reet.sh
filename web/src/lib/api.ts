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
}
export type Counts = { all: number; liked: number; bookmarked: number; used: number }
export type Tab = 'all' | 'liked' | 'bookmarked' | 'used'
export type Sort = 'recent' | 'name' | 'installs'
export type Flags = { liked?: boolean; bookmarked?: boolean; used?: boolean; notes?: string }
export type AddResult = Skill & { created: boolean }

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

export async function listSkills(tab: Tab, q: string, sort: Sort): Promise<{ skills: Skill[]; counts: Counts }> {
  const p = new URLSearchParams({ tab, q, sort })
  return (await req(`/api/skills?${p}`)).json()
}
export async function addSkill(input: string, flags: Flags = {}): Promise<AddResult> {
  return (await req('/api/skills', { method: 'POST', body: json({ input, ...flags }) })).json()
}
export async function patchSkill(id: string, flags: Flags): Promise<Skill> {
  return (await req('/api/skill', { method: 'PATCH', body: json({ id, ...flags }) })).json()
}
export async function removeSkill(id: string): Promise<void> {
  await req(`/api/skill?id=${encodeURIComponent(id)}`, { method: 'DELETE' })
}
export async function refreshSkill(id: string): Promise<Skill> {
  return (await req('/api/skill/refresh', { method: 'POST', body: json({ id }) })).json()
}
export async function resolveSkill(id: string, input: string): Promise<Skill> {
  return (await req('/api/skill/resolve', { method: 'POST', body: json({ id, input }) })).json()
}
export async function importText(text: string): Promise<{ imported: number; errors: string[] }> {
  return (await req('/api/import', { method: 'POST', body: json({ text }) })).json()
}
export async function exportAll(): Promise<Blob> {
  return (await req('/api/export')).blob()
}
export async function restoreAll(data: unknown): Promise<{ restored: number }> {
  return (await req('/api/restore', { method: 'POST', body: json(data) })).json()
}
export async function getBuildInfo(): Promise<{ version: string; commit: string }> {
  return (await req('/api/version')).json()
}
