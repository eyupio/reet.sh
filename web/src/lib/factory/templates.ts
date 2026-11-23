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

/** What a refusal of a template write means for the person; where the server's own sentence says it best, that is kept.
 *  `latest` is the version read again after a template_changed refusal, so the words can say which one was saved elsewhere. */
export function conflictWords(code: string, said: string, latest = 0, deleting = false): string {
  if (code === 'template_changed') {
    if (!latest) return 'Another tab or session saved a newer version of this template. Nothing was changed. Try again.'
    return deleting ? `Version ${latest} was saved elsewhere, so nothing was deleted. Delete again if you still want to.` : `Version ${latest} was saved elsewhere. This now saves as version ${latest + 1}.`
  }
  if (code === 'template_request_reused') return 'That request was already used for something else. Try again.'
  return said
}

/** What a template's pipeline does at the end: the setting it was saved with. */
export const mergeWords = (autoMerge: boolean): string => (autoMerge ? 'Merges by itself once checks pass.' : 'Stops at Ready to merge.')
/** The review limits every session of a run from the template gets. */
export const limitsWords = (c: { maxReviews: number; hours: number }): string => `Up to ${c.maxReviews} ${c.maxReviews === 1 ? 'review' : 'reviews'} and ${c.hours} ${c.hours === 1 ? 'hour' : 'hours'} for each session.`
/** The merge part of a start's access paragraph: which of the template's pipelines merge by themselves. */
export function mergeAccessWords(auto: string[], total: number, yoloHolds: boolean): string {
  if (!auto.length) return 'Gaffer cannot merge, run commands or answer approvals. Every pipeline stops at Ready to merge and waits for you.'
  const who = auto.length === total ? `${total === 1 ? 'This pipeline merges' : 'Every pipeline merges'} by itself once checks pass.` : `${auto.map(quoted).join(', ')} ${auto.length === 1 ? 'merges' : 'merge'} by itself once checks pass. The others stop at Ready to merge and wait for you.`
  return `Gaffer cannot run commands or answer approvals. ${who}${yoloHolds ? ' On YOLO they still stop at Ready to merge until you confirm on the Factory page that they may merge by itself.' : ''}`
}

/** Whether the server would save this outcome as a template: it has started, and every pipeline finished, or had its delivery accepted and did not then fail. */
export function canSaveAsTemplate(o: { state: string; sessions: { state: string; stages?: unknown[]; deliveryAccepted?: boolean }[] }): boolean {
  return o.state !== 'starting' && o.sessions.length > 0 && o.sessions.every(p => {
    const ended = p.state === 'failed' || p.state === 'unfollowed' || p.state === 'cancelled'
    return (p.stages?.length ?? 0) > 0 && (p.state === 'done' || (!!p.deliveryAccepted && !ended))
  })
}

export const usedWords = (lastUsed: number | undefined, at: number | undefined, when: (at: number) => string): string =>
  !lastUsed ? 'Not used yet' : at ? `Last used ${when(at)} (version ${lastUsed})` : `Last used at version ${lastUsed}`

/** The heading of the changes shown before a start: since the version last used, or how an older one differs from it. A note when the version last used is no longer kept. */
export function changesWords(diff: Pick<TemplateDiff, 'from' | 'to' | 'olderNotShown'>, lastUsed: number | undefined): { title: string; note: string } {
  if (diff.to < diff.from) return { title: `Differs from version ${diff.from}, the one you last used`, note: '' }
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
