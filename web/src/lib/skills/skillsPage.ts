import type { Skill, SkillHealth, SkillRow } from '../api'
import { installCommand, skillBlurb } from '../install'

export type Tone = 'ok' | 'work' | 'bad' | 'off'

const machines = (n: number) => `${n} machine${n === 1 ? '' : 's'}`

/** One line saying where a skill stands on the person's machines. Every state has one. */
export function healthLine(h: SkillHealth): { text: string; tone: Tone; retry: boolean } {
  const hosts = h.hosts ?? []
  const where = hosts.length === 1 ? hosts[0] : machines(hosts.length)
  const line = (text: string, tone: Tone, retry = false) => ({ text, tone, retry })
  switch (h.state) {
    case 'ok':
      return line(`On all ${h.total} machines`, 'ok')
    case 'installing':
      return line(`Installing on ${h.pending} of ${h.total}`, 'work')
    case 'removing':
      return line(hosts.length === 1 ? `Coming off ${hosts[0]}` : `Coming off ${machines(h.pending)}`, 'work')
    case 'failed':
      return line(`Failed on ${where}`, 'bad', true)
    case 'offline':
      return line(`Removing from ${where} when it's back online`, 'work')
    case 'blocked':
      return line(`Can't write skills on ${where}`, 'bad')
    case 'copying':
      return line(`Copying into reet from ${where}`, 'work')
    case 'copy-failed':
      return line(`Couldn't copy into reet: ${h.reason ?? 'unknown reason'}. It stays on ${where} until you delete it.`, 'bad')
    case 'getting':
      return line(hosts.length === 0 ? 'Getting the files' : `Getting a copy from ${where}`, 'work')
    case 'fetch-failed':
      return line("Couldn't fetch the files", 'bad', true)
    case 'needs-update':
      return line(hosts.length === 1 ? `Update reet on ${hosts[0]} to keep its skills in sync` : `Update reet on ${hosts.length} machines to keep their skills in sync`, 'bad')
    case 'off':
      return line('Off every machine', 'off')
    default:
      return line('Checking', 'work')
  }
}

const skillsLabel = (names: string[]) => (names.length === 1 ? names[0] : `${names.length} skills`)

export function toastText(action: 'archived' | 'deleted' | 'restored' | 'kept', names: string[], machineCount: number): string {
  const what = skillsLabel(names)
  switch (action) {
    case 'archived':
      return `Archived ${what} · coming off ${machines(machineCount)}`
    case 'deleted':
      return `Deleted ${what} · coming off ${machines(machineCount)}`
    case 'restored':
      return `Restored ${what} · going back on ${machines(machineCount)}`
    case 'kept':
      return `Kept ${what} · going on every machine`
  }
}

/** The skill to focus once `removed` leave the list: the next one, else the one before, else nothing. */
export function focusAfterRemoval(order: string[], removed: string[]): string | null {
  const gone = new Set(removed)
  const first = order.findIndex(id => gone.has(id))
  if (first < 0) return null
  for (let i = first + 1; i < order.length; i++) if (!gone.has(order[i])) return order[i]
  for (let i = first - 1; i >= 0; i--) if (!gone.has(order[i])) return order[i]
  return null
}

function nameList(names: string[]): string {
  if (names.length <= 3) return names.length > 1 ? `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}` : names[0]
  return `${names.slice(0, 3).join(', ')} and ${names.length - 3} more`
}

/**
 * The one dialog before a delete, or null when none is needed. writtenConfirmed and noCopyConfirmed are the flags
 * the page sends with the delete: the server refuses without them (409 'written' and 'no-copy').
 */
export function deleteConfirm(rows: SkillRow[]): { title: string; body: string; writtenConfirmed: boolean; noCopyConfirmed: boolean } | null {
  const written = rows.filter(r => r.yours)
  const noCopy = rows.filter(r => !r.hasCopy)
  if (!written.length && !noCopy.length) return null
  const title = rows.length === 1 ? `Delete ${rows[0].name}?` : `Delete ${rows.length} skills?`
  const parts: string[] = []
  if (rows.length === 1) {
    if (written.length) parts.push('You wrote this one.')
    if (noCopy.length) parts.push('reet has no copy of this skill.')
    parts.push(written.length && noCopy.length ? 'Deleting it removes it from your machines and nothing can bring it back.' : written.length ? 'Deleting it removes it from your machines, and reet has the only copy.' : 'Deleting it removes it from your machines and nothing can bring it back.')
  } else {
    if (written.length) parts.push(`You wrote ${nameList(written.map(r => r.name))}.`)
    if (noCopy.length) parts.push(`reet has no copy of ${nameList(noCopy.map(r => r.name))}.`)
    parts.push('Deleting removes them from your machines.')
  }
  return { title, body: parts.join(' '), writtenConfirmed: written.length > 0, noCopyConfirmed: noCopy.length > 0 }
}

/** Words to paste into a prompt to use a skill. It carries the name and description, never a link. */
export function copyPrompt(row: Pick<SkillRow, 'name' | 'description'>): string {
  const blurb = skillBlurb(row.description.replace(/(?:https?:\/\/|www\.)\S+|\bgithub\.com\S*/gi, ''))
  return blurb ? `Use the ${row.name} skill: ${blurb}` : `Use the ${row.name} skill.`
}

/** The command to install a published skill from its public page; null for one that is not published. */
export function installCommandFor(row: SkillRow): string | null {
  if (!row.published || !row.publicUrl) return null
  return installCommand({ sourceType: 'hosted', url: row.publicUrl, name: row.name, slug: row.name } as unknown as Skill)
}

export function syncSentence(m: { failed: number; working: number }): string {
  if (m.failed > 0) return `${machines(m.failed)} ${m.failed === 1 ? 'needs' : 'need'} a look.`
  if (m.working > 0) return `Installing on ${machines(m.working)}…`
  return 'Everything is in sync.'
}
