// What a template's pages say, kept as plain functions so the wording can be tested without a page.
import type { GafferCrewTier, TemplateDiff, TemplateSkillGap } from '../api'
import { stageLabel } from '../gafferOutcome'

const short = (revision: string) => revision.slice(0, 8)
const quoted = (text: string) => `“${text}”`
const list = (kinds: string[]) => kinds.map(stageLabel).join(', ') || 'none'

/** The version to show changes since: the one last used, unless that is the one about to be used, or nothing was used yet. */
export const changesSince = (lastUsed: number | undefined, version: number): number => (lastUsed && lastUsed !== version ? lastUsed : 0)
/** Whether the latest version is newer than the one last used. A template never used has nothing to compare with. */
export const newerThanLastUsed = (t: { latest: number; lastUsedVersion?: number }): boolean => !!t.lastUsedVersion && t.latest > t.lastUsedVersion
/** Exactly the gaps shown, in the form a launch sends. */
export const skippedKeys = (gaps: readonly Pick<TemplateSkillGap, 'key'>[]): string[] => gaps.map(gap => gap.key)

export function gapWords(gap: Pick<TemplateSkillGap, 'name' | 'status'>): string {
  return gap.status === 'changed'
    ? `${quoted(gap.name)} has a newer approved revision. This template keeps the older one, so the stage starts without it.`
    : `${quoted(gap.name)} is not available now (archived, removed or switched off), so the stage starts without it.`
}

/** What changed between two versions, as lines for a person: a few general ones, then each pipeline that moved. */
export function diffLines(diff: TemplateDiff): { general: string[]; pipelines: { title: string; lines: string[] }[] } {
  const general = [
    ...(diff.nameChanged ? ['The name changed.'] : []), ...(diff.descriptionChanged ? ['The description changed.'] : []),
    ...(diff.objectiveChanged ? ['The objective changed.'] : []), ...(diff.limitsChanged ? ['The review limits changed.'] : []),
    ...(diff.orderChanged ? ['The pipelines run in another order.'] : []),
  ]
  const pipelines = diff.pipelines.filter(p => p.change !== 'same').map(p => {
    if (p.change === 'added') return { title: p.title, lines: ['Added.'] }
    if (p.change === 'removed') return { title: p.title, lines: ['Removed.'] }
    const lines: string[] = []
    if (p.promptChanged) lines.push('The prompt changed.')
    if (p.autoMergeChanged) lines.push('The merge setting changed.')
    if (p.stagesChanged) lines.push(`Stages were ${list(p.stagesBefore)}; now ${list(p.stagesAfter)}.`)
    for (const text of p.criteria.added) lines.push(`Criterion added: ${quoted(text)}`)
    for (const text of p.criteria.removed) lines.push(`Criterion removed: ${quoted(text)}`)
    for (const one of p.criteria.reworded) lines.push(`Criterion reworded: ${quoted(one.from)} to ${quoted(one.to)}`)
    for (const s of p.skills) lines.push(!s.from ? `${quoted(s.name)} added to ${stageLabel(s.stage)} at revision ${short(s.to)}.` : !s.to ? `${quoted(s.name)} removed from ${stageLabel(s.stage)}.` : `${quoted(s.name)} in ${stageLabel(s.stage)} moved from revision ${short(s.from)} to ${short(s.to)}.`)
    return { title: p.title, lines }
  })
  return { general, pipelines }
}

/** What a refusal of a template write means for the person; where the server's own sentence says it best, that is kept. */
export function conflictWords(code: string, said: string): string {
  if (code === 'template_changed') return 'Someone, or another tab, saved a newer version of this template. It has been read again, so look at it and try again.'
  if (code === 'template_request_reused') return 'That request was already used for something else. Try again.'
  return said
}

export const usedWords = (lastUsed: number | undefined, at: number | undefined, when: (at: number) => string): string =>
  !lastUsed ? 'Not used yet' : at ? `Last used ${when(at)} (version ${lastUsed})` : `Last used at version ${lastUsed}`

/** The heading of the changes shown before a start, and a note when the version last used is no longer kept. */
export function changesWords(diff: Pick<TemplateDiff, 'from' | 'olderNotShown'>, lastUsed: number | undefined): { title: string; note: string } {
  if (!diff.olderNotShown) return { title: `Changed since version ${diff.from}, the one you last used`, note: '' }
  return { title: `Changed since version ${diff.from}, the oldest kept`, note: `Version ${lastUsed}, the one you last used, is no longer kept, so changes before version ${diff.from} are not shown.` }
}

/** The gaps a template_skills_changed refusal carries, or null when it carries none and the template must be read again. */
export function refusedGaps(body: { gaps?: unknown } | undefined): TemplateSkillGap[] | null {
  return Array.isArray(body?.gaps) ? (body.gaps as TemplateSkillGap[]) : null
}

/** One tier of the crew a launch sends: what a stage on that tier is sent with, without the stage's own fields. */
export function tierCrew(stage: GafferCrewTier & { kind: string }): GafferCrewTier {
  const { tool, model, effort, provider, backup, moreBackups } = stage
  return { tool, ...(model ? { model } : {}), ...(effort ? { effort } : {}), ...(provider ? { provider } : {}), ...(backup ? { backup } : {}), ...(moreBackups ? { moreBackups } : {}) }
}
