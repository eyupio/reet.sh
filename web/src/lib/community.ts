import type { ListSkill, SharedList } from './api'

export type CommunityTab = 'lists' | 'skills'

/** Every word of a search must appear somewhere in the record, in any order. */
export function matchesCommunity(query: string, ...fields: (string | undefined)[]): boolean {
  const term = query.trim().toLowerCase()
  if (!term) return true
  const haystack = fields.filter(Boolean).join(' ').toLowerCase()
  return term.split(/\s+/).every((word) => haystack.includes(word))
}

export function filterCommunityLists(lists: SharedList[], query: string): SharedList[] {
  return lists.filter((list) => matchesCommunity(query, list.name, list.description, list.ownerName))
}

export function filterCommunitySkills(skills: ListSkill[], query: string): ListSkill[] {
  return skills.filter((skill) => matchesCommunity(query, skill.name, skill.source, skill.description))
}

/** "3 lists", or "1 of 3 lists" while the search narrows the set. */
export function communityCountLabel(shown: number, total: number, noun: CommunityTab): string {
  const one = noun === 'lists' ? 'list' : 'skill'
  const many = `${one}s`
  if (total === 0) return `No ${many}`
  if (shown === total) return `${total} ${total === 1 ? one : many}`
  return `${shown} of ${total} ${total === 1 ? one : many}`
}
