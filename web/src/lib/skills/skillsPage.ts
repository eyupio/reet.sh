import type { Skill, SkillHealth, SkillRow } from '../api'
import { installCommand, skillBlurb } from '../install'

export type Tone = 'ok' | 'work' | 'bad' | 'off'

const machines = (n: number) => `${n} machine${n === 1 ? '' : 's'}`

/**
 * One line saying where a skill stands on the person's machines. Every state has one. `status` is the row's: a machine
 * that is offline is waiting to install an Active skill, and to remove an Archived one.
 */
export function healthLine(h: SkillHealth, status?: SkillRow['status']): { text: string; tone: Tone; retry: boolean } {
  const hosts = h.hosts ?? []
  const where = hosts.length === 1 ? hosts[0] : machines(hosts.length)
  const line = (text: string, tone: Tone, retry = false) => ({ text, tone, retry })
  switch (h.state) {
    case 'ok':
      return line(h.total === 1 ? 'On 1 machine' : `On all ${h.total} machines`, 'ok')
    case 'installing':
      return line(`Installing on ${h.pending} of ${h.total}`, 'work')
    case 'removing':
      return line(hosts.length === 1 ? `Coming off ${hosts[0]}` : `Coming off ${machines(h.pending)}`, 'work')
    case 'failed':
      return line(`Failed on ${where}`, 'bad', true)
    case 'offline':
      if (status === 'active') return line(`Installing on ${where} when ${hosts.length > 1 ? "they're" : "it's"} back online`, 'work')
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
      return line(hosts.length === 0 ? 'Update reet on your machines to keep their skills in sync' : hosts.length === 1 ? `Update reet on ${hosts[0]} to keep its skills in sync` : `Update reet on ${hosts.length} machines to keep their skills in sync`, 'bad')
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
  const links = written.filter(r => r.published)
  const backup = 'each machine keeps a backup for 30 days'
  const parts: string[] = []
  if (rows.length === 1) {
    if (written.length) parts.push('You wrote this one.')
    if (noCopy.length) parts.push('reet has no copy of this skill.')
    const writing = links.length ? 'your writing and its public link' : 'your writing'
    if (written.length && noCopy.length) parts.push(`Deleting removes ${writing} and the skill from your machines; ${backup}.`)
    else if (written.length) parts.push(`Deleting it removes ${writing}, as well as the skill on your machines.`)
    else parts.push(`Deleting removes it from your machines; ${backup}.`)
  } else {
    if (written.length) parts.push(`You wrote ${nameList(written.map(r => r.name))}.`)
    if (noCopy.length) parts.push(`reet has no copy of ${nameList(noCopy.map(r => r.name))}.`)
    let end = 'Deleting removes them from your machines'
    if (written.length) end += links.length ? `, along with your writing and the public links of ${nameList(links.map(r => r.name))}` : ', along with your writing'
    if (noCopy.length) end += `; ${backup}`
    parts.push(end + '.')
  }
  return { title, body: parts.join(' '), writtenConfirmed: written.length > 0, noCopyConfirmed: noCopy.length > 0 }
}

/** Words to paste into a prompt to use a skill. It carries the name and description, never a link: a sentence of the description that holds one is left out. */
export function copyPrompt(row: Pick<SkillRow, 'name' | 'description'>): string {
  const link = /https?:\/\/|www\.|github\.com/i
  const kept = (row.description ?? '').split(/(?<=[.!?])\s+/).filter(sentence => !link.test(sentence)).join(' ')
  const blurb = skillBlurb(kept)
  return blurb ? `Use the ${row.name} skill: ${blurb}` : `Use the ${row.name} skill.`
}

/** The command to install a published skill from its public page; null for one that is not published. */
export function installCommandFor(row: SkillRow): string | null {
  if (!row.published || !row.publicUrl) return null
  return installCommand({ sourceType: 'hosted', url: row.publicUrl, name: row.name, slug: row.name } as unknown as Skill)
}

export type MachineWork = { failed: number; installing: number; removing: number; updating?: number }

/**
 * One sentence for the machines: those that need a look come first, then what the rest are doing. `installing` and
 * `removing` count machines; `updating` is how many machines do either, each once, which is what the sentence counts
 * when installs and removals mix (without it, the two are added). The server counts them (SkillsPageData.work).
 */
export function syncSentence(m: MachineWork): string {
  if (m.failed > 0) return `${machines(m.failed)} ${m.failed === 1 ? 'needs' : 'need'} a look.`
  if (m.installing > 0 && m.removing > 0) return `Updating ${machines(m.updating ?? m.installing + m.removing)}…`
  if (m.installing > 0) return `Installing on ${machines(m.installing)}…`
  if (m.removing > 0) return `Coming off ${machines(m.removing)}…`
  return 'Everything is in sync.'
}


/** A search split as the server splits it (store.go splitQuery): tag:x and tag:"x y" apart from the other words. */
export function splitSearch(q: string): { term: string; tags: string[] } {
  const words: string[] = [], tags: string[] = []
  let i = 0
  while (i < q.length) {
    while (i < q.length && q[i] === ' ') i++
    if (i >= q.length) break
    if (q.startsWith('tag:"', i)) {
      const end = q.indexOf('"', i + 5)
      if (end >= 0) {
        const name = q.slice(i + 5, end).trim()
        if (name) tags.push(name.toLowerCase())
        i = end + 1
        continue
      }
    }
    let j = i
    while (j < q.length && q[j] !== ' ') j++
    const word = q.slice(i, j)
    if (word.startsWith('tag:') && word.length > 4) tags.push(word.slice(4).toLowerCase())
    else words.push(word)
    i = j
  }
  return { term: words.join(' '), tags }
}

/**
 * Whether a row matches a search as Markdown and CSV export match it (store ListFor), so an export holds the rows the
 * page shows: the words, as one phrase, in the name, the shelf id, the description or the notes; and every tag:x.
 */
export function matchesSearch(row: Pick<SkillRow, 'name' | 'shelfId' | 'description' | 'notes' | 'tags'>, q: string): boolean {
  const { term, tags } = splitSearch(q)
  const own = new Set(row.tags.map(t => t.name.toLowerCase()))
  if (!tags.every(t => own.has(t))) return false
  if (!term) return true
  const needle = term.toLowerCase()
  return [row.name, row.shelfId, row.description, row.notes].some(field => (field ?? '').toLowerCase().includes(needle))
}
