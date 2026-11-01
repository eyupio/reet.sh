// Pure logic for the per-session skill picker: grouping, the slash-query trigger, inserting an invocation and the freshness line.
// Names and descriptions are untrusted text from a machine; nothing here renders them.
export type SessionSkill = { id: string; name: string; description: string; source: 'machine' | 'reet'; agent: string; native: boolean; onMachine: boolean; invocation: string }
export type SessionSkills = { skills: SessionSkill[]; machineSeen: number; host: string }
export type SkillGroup = { key: 'native' | 'other' | 'reet'; label: string; skills: SessionSkill[] }

export function groupSkills(skills: SessionSkill[], query: string, tool: string): SkillGroup[] {
  const q = query.trim().toLowerCase()
  const shown = q ? skills.filter(s => s.name.toLowerCase().includes(q) || s.description.toLowerCase().includes(q)) : skills
  const groups: SkillGroup[] = [
    { key: 'native', label: `On this machine for ${tool}`, skills: shown.filter(s => s.source === 'machine' && s.native) },
    { key: 'other', label: 'On this machine for other agents', skills: shown.filter(s => s.source === 'machine' && !s.native) },
    { key: 'reet', label: 'reet-hosted', skills: shown.filter(s => s.source === 'reet') },
  ]
  return groups.filter(g => g.skills.length > 0)
}

export function slashQuery(draft: string): string | null {
  if (!draft.startsWith('/')) return null
  const rest = draft.slice(1)
  return /\s/.test(rest) ? null : rest
}

export function applyInvocation(draft: string, invocation: string, limit: number): { draft: string } | { error: string } {
  if (!invocation) return { error: "This skill can't be inserted from here." }
  const next = slashQuery(draft) !== null ? invocation : draft && !/\s$/.test(draft) ? `${draft} ${invocation}` : draft + invocation
  if (next.length > limit || new TextEncoder().encode(next).length > limit) return { error: 'That would pass the 2,000-character limit. Nothing was inserted.' }
  return { draft: next }
}

export function seenLine(machineSeen: number, host: string, now: number): string {
  if (!machineSeen) return `Skills on ${host} have not been read yet.`
  const minutes = Math.max(1, Math.floor((now - machineSeen) / 60_000))
  const [n, unit] = minutes >= 1440 ? [Math.floor(minutes / 1440), 'day'] : minutes >= 60 ? [Math.floor(minutes / 60), 'hour'] : [minutes, 'minute']
  return `Skills on ${host} were read ${n} ${unit}${n === 1 ? '' : 's'} ago.`
}
