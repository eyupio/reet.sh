// Pure logic for the per-session skill picker: grouping, the slash-query trigger, inserting an invocation and the freshness line.
// Names and descriptions are untrusted text from a machine; nothing here renders them.
// A machine skill is one reet knows on the session's machine (an Active skill, or one in a project folder); a new one is
// New on this machine and waits for the person to Keep or Archive it. Every Active skill is on every machine, so the
// person's own skills need no group of their own, and Archived ones are not offered.
export type SessionSkill = { id: string; name: string; description: string; source: 'machine' | 'new'; agent: string; native: boolean; onMachine: boolean; invocation: string }
export type SessionSkills = { skills: SessionSkill[]; machineSeen: number; host: string; agentId?: string }
export type SkillGroup = { key: 'native' | 'other' | 'new'; label: string; skills: SessionSkill[] }

export function groupSkills(skills: SessionSkill[], query: string, tool: string): SkillGroup[] {
  const q = query.trim().toLowerCase()
  const shown = q ? skills.filter(s => s.name.toLowerCase().includes(q) || s.description.toLowerCase().includes(q)) : skills
  const groups: SkillGroup[] = [
    { key: 'native', label: `On this machine for ${tool}`, skills: shown.filter(s => s.source === 'machine' && s.native) },
    { key: 'other', label: 'On this machine for other agents', skills: shown.filter(s => s.source === 'machine' && !s.native) },
    { key: 'new', label: 'New on this machine', skills: shown.filter(s => s.source === 'new') },
  ]
  return groups.filter(g => g.skills.length > 0)
}

/** Whether a row offers Keep: a skill New on this machine. */
export function keepable(skill: Pick<SessionSkill, 'source'>): boolean {
  return skill.source === 'new'
}

export function slashQuery(draft: string): string | null {
  if (!draft.startsWith('/')) return null
  const rest = draft.slice(1)
  return /\s/.test(rest) ? null : rest
}

export function applyInvocation(draft: string, invocation: string, limit: number): { draft: string } | { error: string } {
  if (!invocation) return { error: "This skill can't be inserted from here." }
  const next = slashQuery(draft) !== null ? invocation : draft && !/\s$/.test(draft) ? `${draft} ${invocation}` : draft + invocation
  if (next.length > limit || new TextEncoder().encode(next).length > limit) return { error: 'That would make the message longer than 2,000 bytes. Nothing was inserted.' }
  return { draft: next }
}

export function seenLine(machineSeen: number, host: string, now: number): string {
  if (!machineSeen) return `Skills on ${host} have not been read yet.`
  const minutes = Math.max(1, Math.floor((now - machineSeen) / 60_000))
  const [n, unit] = minutes >= 1440 ? [Math.floor(minutes / 1440), 'day'] : minutes >= 60 ? [Math.floor(minutes / 60), 'hour'] : [minutes, 'minute']
  return `Skills on ${host} were read ${n} ${unit}${n === 1 ? '' : 's'} ago.`
}
