import type { SyncSkill } from './api'

export function linked(skill: SyncSkill): boolean {
  return !!skill.source && skill.sourceType !== 'local'
}

export function canonicalSource(source: string): string {
  return source
    .replace(/^https?:\/\//, '')
    .replace(/^github\.com\//, '')
    .replace(/\/+$/, '')
    .replace(/\.git$/, '')
}

/** Keep one skill per name, preferring the copy that has an install source. */
export function dedupeSkills(skills: SyncSkill[]): SyncSkill[] {
  const bySlug = new Map<string, SyncSkill>()
  for (const skill of skills) {
    const current = bySlug.get(skill.slug)
    if (!current || (!linked(current) && linked(skill))) bySlug.set(skill.slug, skill)
  }
  return [...bySlug.values()].sort((a, b) => a.name.localeCompare(b.name))
}

/** A real conflict is a different known source, not an unlinked local copy. */
export function sourceConflicts(desired: SyncSkill[], actual: SyncSkill[]): SyncSkill[] {
  return desired.filter(want =>
    linked(want) &&
    actual.some(have =>
      have.slug === want.slug &&
      linked(have) &&
      canonicalSource(have.source) !== canonicalSource(want.source)))
}

/** Local copies with the same name are replaced automatically and backed up. */
export function localDuplicates(desired: SyncSkill[], actual: SyncSkill[]): SyncSkill[] {
  return desired.filter(want =>
    linked(want) &&
    actual.some(have => have.slug === want.slug && !linked(have)))
}
