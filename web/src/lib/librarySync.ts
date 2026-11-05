import type { ImportRequest, SyncDuplicate, SyncSkill } from './api'

/**
 * What the Sync card says about the library besides the machines: what is being
 * copied, what needs a person, and what was left out. Pressing Sync all machines
 * brings across everything that can be brought, so a skill that cannot be is a
 * short note and never a stack of errors.
 */

/** Skills that were left out for the same reason, said once. */
export type LeftOutGroup = {
  key: string
  ids: string[]
  names: string[]
  /** The rest of the sentence after the names, starting with its own separator. */
  reason: string
}

export type LibraryNotice = {
  /** Private copies still travelling from a machine into the library. */
  copying: ImportRequest[]
  /** Copies that stopped so a person can look at some files first. */
  review: ImportRequest[]
  /** Skills that cannot sync right now, grouped by why. */
  leftOut: LeftOutGroup[]
  /** How many skills that is, however they are grouped. */
  leftOutCount: number
}

const IN_FLIGHT = new Set(['queued', 'previewing', 'approved', 'uploading'])

/**
 * A reason usually arrives as a sentence that starts with the skill's own name
 * ("reet-api-design cannot be synced privately: …"). Skills that fail for the same
 * reason are listed together, so the sentence is said once: "a, b, c cannot be
 * synced privately: …". A reason that does not start with the name is attached
 * with a colon instead: "seo: This skill folder could not be copied…".
 */
export function groupLeftOut(skills: Pick<SyncSkill, 'id' | 'name' | 'problem'>[]): LeftOutGroup[] {
  const groups = new Map<string, LeftOutGroup>()
  for (const skill of skills) {
    const problem = (skill.problem ?? '').trim()
    if (!problem) continue
    const reason = problem.startsWith(`${skill.name} `) ? problem.slice(skill.name.length) : `: ${problem}`
    const group = groups.get(reason) ?? { key: reason, ids: [], names: [], reason }
    group.ids.push(skill.id)
    group.names.push(skill.name)
    groups.set(reason, group)
  }
  return [...groups.values()]
}

export function libraryNotice(skills: SyncSkill[], imports: ImportRequest[], runs: { id: string; status: string }[] = []): LibraryNotice {
  // Only the newest copy request for a skill matters: an earlier failed attempt is
  // history once it was retried, and showing both is how one skill appeared twice.
  const newest = new Map<string, ImportRequest>()
  for (const item of imports) {
    if (!item.syncSkillId || !item.destinationId) continue
    const prior = newest.get(item.syncSkillId)
    if (!prior || item.createdAt > prior.createdAt) newest.set(item.syncSkillId, item)
  }
  const copying: ImportRequest[] = []
  const review: ImportRequest[] = []
  for (const item of newest.values()) {
    const installing = !item.syncRunId || runs.some(run => run.id === item.syncRunId && run.status !== 'completed')
    if (!installing) continue
    if (item.state === 'reviewing') review.push(item)
    // A failed copy is not repeated here. If the skill still matters, the library
    // already carries its reason as the skill's own problem, in plain words.
    else if (IN_FLIGHT.has(item.state) || (item.state === 'completed' && !item.syncRunId)) copying.push(item)
  }
  const left = skills.filter(skill => skill.problem)
  return { copying, review, leftOut: groupLeftOut(left), leftOutCount: left.length }
}

function names(list: string[], limit = 4): string {
  if (list.length <= limit) return list.join(', ')
  return `${list.slice(0, limit).join(', ')} and ${list.length - limit} more`
}

/** "3 skills are left out for now. Everything else is syncing." */
export function leftOutHeadline(count: number): string {
  return `${count} ${count === 1 ? 'skill is' : 'skills are'} left out for now. Everything else is syncing.`
}

/** "Copying seo and 2 more privately from your machines." */
export function copyingLine(items: Pick<ImportRequest, 'name'>[]): string {
  const unique = [...new Set(items.map(item => item.name))]
  return `Copying ${names(unique)} privately from your machines. This carries on when the page is closed.`
}

/**
 * Two sources can claim one name, but only one copy fits in a skills folder. Reet
 * picks one itself; this tells the person which names that happened to, so a skill
 * that did not arrive is not a mystery. It is information, not a problem.
 */
export function duplicatesNote(groups: SyncDuplicate[]): string {
  if (!groups.length) return ''
  const lead = groups.length === 1 ? 'One name is' : `${groups.length} names are`
  return `${lead} on your shelf more than once, so Reet syncs one copy of each (${names(groups.map(group => group.slug))}). The other copies stay on your shelf, untouched.`
}
