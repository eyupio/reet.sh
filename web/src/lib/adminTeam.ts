import type { RollupSkill } from './api'

/** A skill is found by name, source, list name or tag, in any word order. */
export function teamUsageMatchesSearch(skill: RollupSkill, query: string): boolean {
  const term = query.trim().toLowerCase()
  if (!term) return true
  const lists = (skill.lists ?? []).map((list) => list.name).join(' ')
  const haystack = `${skill.name} ${skill.source} ${lists} ${(skill.tags ?? []).join(' ')}`.toLowerCase()
  return term.split(/\s+/).every((word) => haystack.includes(word))
}

export function teamUsageCountLabel(shown: number, total: number): string {
  if (total === 0) return 'No skills'
  if (shown === total) return `${total} ${total === 1 ? 'skill' : 'skills'}`
  return `${shown} of ${total} skills`
}
