// Pure logic for the per-session skill picker: grouping, the slash-query trigger, inserting an invocation and the freshness line.
// Names and descriptions are untrusted text from a machine; nothing here renders them.
export type SessionSkill = { id: string; name: string; description: string; source: 'machine' | 'reet'; agent: string; native: boolean; onMachine: boolean; invocation: string; published: boolean }
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
  if (next.length > limit || new TextEncoder().encode(next).length > limit) return { error: 'That would make the message longer than 2,000 bytes. Nothing was inserted.' }
  return { draft: next }
}

export function seenLine(machineSeen: number, host: string, now: number): string {
  if (!machineSeen) return `Skills on ${host} have not been read yet.`
  const minutes = Math.max(1, Math.floor((now - machineSeen) / 60_000))
  const [n, unit] = minutes >= 1440 ? [Math.floor(minutes / 1440), 'day'] : minutes >= 60 ? [Math.floor(minutes / 60), 'hour'] : [minutes, 'minute']
  return `Skills on ${host} were read ${n} ${unit}${n === 1 ? '' : 's'} ago.`
}

export type InstallStep = 'waiting' | 'review' | 'installing' | 'done' | 'failed' | 'cancelled'

export function installStep(job: { kind: string; status: string } | undefined): InstallStep {
  if (!job) return 'waiting'
  if (job.status === 'succeeded') return 'done'
  if (job.status === 'failed') return 'failed'
  if (job.status === 'cancelled') return 'cancelled'
  if (job.status === 'awaiting-approval') return 'review'
  if (job.kind === 'apply') return 'installing'
  return 'waiting'
}

export type PlanVerdict = { ok: true; lines: string[] } | { ok: false; reason: string; lines: string[] }

// Approves only a plan whose every action is a plain add of the requested slug; the slugs come from a machine and are only ever shown as text.
export function planVerdict(plan: { directory?: string; actions?: { add: boolean; replace?: boolean; skill: { slug: string } }[] } | undefined, slug: string): PlanVerdict {
  const actions = plan?.actions ?? []
  const lines = actions.map(a => `${a.add ? (a.replace ? 'Replace' : 'Add') : 'Remove'} ${a.skill.slug}`)
  if (actions.length === 0) return { ok: false, reason: 'The machine has not sent its plan yet.', lines }
  if (actions.every(a => a.add === true && !a.replace && a.skill.slug === slug)) return { ok: true, lines }
  return { ok: false, reason: 'This install would change other skills. Review it on the Sync page.', lines }
}

export function installStatusLine(step: InstallStep, host: string, slug: string, waitedSeconds: number): string {
  switch (step) {
    case 'waiting': return waitedSeconds >= 60 ? `Waiting for ${host}… Is reet running on ${host}?` : `Waiting for ${host}…`
    case 'review': return 'Review what will be added.'
    case 'installing': return `Installing ${slug} on ${host}…`
    case 'done': return `${slug} is installed on ${host}.`
    case 'failed': return 'The install did not finish.'
    case 'cancelled': return 'Install cancelled.'
  }
}
