import { terminalRequest } from './terminal/api'

export type BuddySkill = { id: string; name: string; description: string; source: 'builtin' | 'personal' | 'community' | 'curated'; revision: string; reason: string; state?: string; instructions?: string; url?: string }
export type BuddyContext = { sessionId?: string; outcomeId?: string; pipeline?: string; stage?: number; task?: string }
export type BuddyAnswer = { skills: BuddySkill[]; stage: string; editable: boolean; scope: string }
export type BuddyRecord = { scope: string; sessionId: string; stage: string; skill: BuddySkill; at: number }
export async function recommendSkills(context: BuddyContext, signal?: AbortSignal): Promise<BuddyAnswer> {
  return (await terminalRequest('/api/v2/skill-buddy/recommendations', { method: 'POST', body: JSON.stringify(context), signal })).json()
}
export async function chooseSkill(context: BuddyContext, skillId: string, action: string, revision = ''): Promise<{ text: string; skill: BuddySkill }> {
  return (await terminalRequest('/api/v2/skill-buddy/choose', { method: 'POST', body: JSON.stringify({ ...context, skillId, action, revision }), signal: AbortSignal.timeout(15000) })).json()
}
export async function skillInsights(signal?: AbortSignal): Promise<{ records: BuddyRecord[]; limit: number; retentionDays: number; sessionUsage?: Record<string, {inputTokens: number; outputTokens: number; cachedTokens: number; reported: boolean}> }> {
  return (await terminalRequest('/api/v2/skill-buddy/insights', { signal })).json()
}
export const sourceName = (source: string) => source === 'builtin' ? 'Reet built-in' : source === 'personal' ? 'Your library' : source === 'curated' ? 'Curated source' : 'Community'
export const receiptName = (state?: string) => state === 'selected' ? 'Selected for stage' : state === 'drafted' ? 'Chosen for draft' : state === 'prepared' ? 'Opening prompt prepared' : state === 'dismissed' ? 'Dismissed' : 'Suggested'

export function buddyTotals(records: BuddyRecord[]) {
  return { suggested: records.length, chosen: records.filter(r => ['selected', 'drafted', 'prepared'].includes(r.skill.state ?? '')).length, prepared: records.filter(r => r.skill.state === 'prepared').length, sessions: new Set(records.map(r => r.sessionId).filter(Boolean)).size }
}
